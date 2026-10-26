package core

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"halisadisyon/internal/remote"
	"halisadisyon/internal/xlsx"
)

func (e *env) unlock() {
	e.t.Helper()
	if _, err := e.svc.Unlock("2468"); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) pay(no int64, method string) {
	e.t.Helper()
	if _, _, err := e.svc.Pay(no, method); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) print(no int64) {
	e.t.Helper()
	if out, _, err := e.svc.Print(no); err != nil || !out.OK {
		e.t.Fatalf("baskı: %+v %v", out, err)
	}
}

// Ödeme şekli adisyona kaydedilir; gün sonu, ödeme ve garson toplamları,
// KDV ve uyarılar doğru hesaplanır.
func TestDaySummaryPaymentsAndWarnings(t *testing.T) {
	e := newEnv(t)
	e.setup()
	a := e.add(1, "Cağ") // Ahmet
	e.add(1, "Ayran")
	e.print(a)
	e.pay(a, PayCash) // 39.000
	b := e.add(2, "Şiş")
	e.add(2, "Şiş")
	e.print(b)
	e.add(2, "Kola") // basıldıktan sonra değişti
	e.pay(b, PayCard) // 75.000
	c := e.add(3, "Çorba")
	e.pay(c, PayTransfer) // basılmadan, 12.000
	d := e.add(4, "Büyük Su")
	e.print(d)
	if _, err := e.svc.Cancel(d, "Müşteri vazgeçti", "2468"); err != nil {
		t.Fatal(err)
	}
	open := e.add(5, "Ayran")

	if _, err := e.svc.DaySummary(""); code(err) != "locked" {
		t.Fatalf("gün sonu şifresiz açıldı: %v", err)
	}
	e.unlock()
	ds, err := e.svc.DaySummary("")
	if err != nil {
		t.Fatal(err)
	}
	if ds.Date != "2026-10-11" || ds.Paid != 3 || ds.Cancelled != 1 || ds.Open != 1 || ds.Revenue != 126000 {
		t.Fatalf("özet yanlış: paid=%d cancel=%d open=%d ciro=%d", ds.Paid, ds.Cancelled, ds.Open, ds.Revenue)
	}
	if ds.FirstNo != a || ds.LastNo != open || !ds.NoMissing {
		t.Fatalf("numara aralığı yanlış: %d–%d %v", ds.FirstNo, ds.LastNo, ds.NoMissing)
	}
	want := map[string]int64{PayCash: 39000, PayCard: 75000, PayTransfer: 12000}
	for _, p := range ds.ByPayment {
		if p.Total != want[p.Method] || p.Count != 1 || p.Label != PaymentLabel(p.Method) {
			t.Fatalf("ödeme toplamı yanlış: %+v", p)
		}
	}
	var vatSum int64
	for _, v := range ds.VATs {
		vatSum += v.VAT
	}
	if vatSum != ds.VAT || ds.VAT == 0 {
		t.Fatalf("KDV oranlara göre toplamı tutmuyor: %d / %d", vatSum, ds.VAT)
	}
	if ds.Featured == nil || ds.Featured.Name != "Cağ" || ds.Featured.Qty != 1 {
		t.Fatalf("öne çıkan ürün yanlış: %+v", ds.Featured)
	}
	kinds := map[string]int{}
	for _, w := range ds.Warnings {
		kinds[w.Kind]++
	}
	if kinds["unprinted"] != 1 || kinds["changed"] != 1 || kinds["printed_cancel"] != 1 || kinds["open"] != 1 {
		t.Fatalf("uyarılar yanlış: %+v", ds.Warnings)
	}
	rows := map[int64]DayRow{}
	for _, r := range ds.Rows {
		rows[r.No] = r
	}
	if rows[b].PaymentLabel != "Kredi Kartı" || rows[c].Payment != PayTransfer || rows[d].StatusText != "İptal" {
		t.Fatalf("liste satırları yanlış: %+v", ds.Rows)
	}
	// Gün sonu fişi çizilir; ödeme şekli adisyon fişinde değil, yalnız gün sonu raporunda.
	if img, err := e.svc.DayReportPreview(ds.Date); err != nil || !strings.HasPrefix(img, "data:image/png") {
		t.Fatalf("gün sonu fişi: %v", err)
	}
}

func TestCloseDayRules(t *testing.T) {
	e := newEnv(t)
	e.setup()
	a := e.add(3, "Cağ")
	e.unlock()
	if _, _, err := e.svc.CloseDay("2026-10-11"); code(err) != "open_tables" || !strings.Contains(err.Error(), "Masa 3") {
		t.Fatalf("açık masa varken gün kapandı: %v", err)
	}
	if _, _, err := e.svc.CloseDay("2026-10-12"); code(err) != "invalid" {
		t.Fatalf("gelecek gün kapandı: %v", err)
	}
	e.print(a)
	e.pay(a, PayCash)
	ds, st, err := e.svc.CloseDay("2026-10-11")
	if err != nil || !ds.Closed || ds.Revenue != 34000 {
		t.Fatalf("gün kapanmadı: %+v %v", ds, err)
	}
	if st.BusinessDate != "2026-10-12" {
		t.Fatalf("kapanan günden sonra yeni adisyonlar ertesi güne yazılmalı: %s", st.BusinessDate)
	}
	if _, _, err := e.svc.CloseDay("2026-10-11"); code(err) != "closed" {
		t.Fatalf("gün iki kez kapandı: %v", err)
	}
	// Aynı saatte açılan yeni masa ertesi iş gününe yazılır.
	b := e.add(4, "Ayran")
	if e.adisyon(b).BusinessDate != "2026-10-12" {
		t.Fatalf("kapalı güne adisyon yazıldı: %s", e.adisyon(b).BusinessDate)
	}
	// Kapalı günün adisyonunun ödeme şekli değişmez.
	if _, err := e.svc.ChangePayment(a, PayCard, "2468"); code(err) != "closed" {
		t.Fatalf("kapalı gün değişti: %v", err)
	}
	if ds2, err := e.svc.DaySummary("2026-10-11"); err != nil || !ds2.Closed {
		t.Fatalf("kapalı gün özeti: %v", err)
	}
}

func TestUnclosedDaysAndCutoff(t *testing.T) {
	e := newEnv(t)
	e.setup()
	a := e.add(1, "Cağ")
	e.pay(a, PayCash)
	// Gece 01:30: hâlâ aynı iş günü.
	e.clk.add(13*time.Hour + 30*time.Minute)
	b := e.add(2, "Ayran")
	if e.adisyon(b).BusinessDate != "2026-10-11" {
		t.Fatalf("gece yarısından sonra açılan masa aynı güne yazılmalı: %s", e.adisyon(b).BusinessDate)
	}
	e.pay(b, PayCard)
	// Ertesi gün öğlen: önceki gün kapatılmadı uyarısı.
	e.clk.add(11 * time.Hour)
	st := e.state()
	if st.BusinessDate != "2026-10-12" || len(st.UnclosedDays) != 1 || st.UnclosedDays[0] != "2026-10-11" {
		t.Fatalf("gün sonu alınmamış gün uyarısı yok: %s %v", st.BusinessDate, st.UnclosedDays)
	}
	e.unlock()
	if _, _, err := e.svc.CloseDay("2026-10-11"); err != nil {
		t.Fatal(err)
	}
	if st := e.state(); len(st.UnclosedDays) != 0 {
		t.Fatalf("kapatılan gün hâlâ uyarıda: %v", st.UnclosedDays)
	}
}

func TestChangePaymentNeedsPin(t *testing.T) {
	e := newEnv(t)
	e.setup()
	a := e.add(1, "Cağ")
	e.pay(a, PayCash)
	if _, err := e.svc.ChangePayment(a, PayCard, ""); code(err) != "pin_required" {
		t.Fatalf("şifresiz değişti: %v", err)
	}
	if _, err := e.svc.ChangePayment(a, PayCard, "2468"); err != nil {
		t.Fatal(err)
	}
	if e.adisyon(a).PaymentMethod != PayCard {
		t.Fatal("ödeme şekli değişmedi")
	}
	e.unlock()
	d, err := e.svc.Detail(a)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range d.History {
		if h.Action == "odeme_degisti" && strings.Contains(h.Text, "Nakit → Kredi Kartı") {
			found = true
		}
	}
	if !found {
		t.Fatalf("değişiklik geçmişte yok: %+v", d.History)
	}
	// Açık adisyonun ödeme şekli değiştirilemez.
	b := e.add(2, "Ayran")
	if _, err := e.svc.ChangePayment(b, PayCard, "2468"); code(err) != "invalid" {
		t.Fatalf("açık adisyonun ödemesi değişti: %v", err)
	}
}

func TestMoveAndMergeTables(t *testing.T) {
	e := newEnv(t)
	e.setup()
	a := e.add(1, "Cağ")
	e.add(1, "Cağ")
	e.add(1, "Ayran")
	if _, _, err := e.svc.MoveTable(a, 99); code(err) != "invalid" {
		t.Fatalf("olmayan masaya taşındı: %v", err)
	}
	res, _, err := e.svc.MoveTable(a, 6)
	if err != nil || res.Merged || res.TargetNo != a || e.adisyon(a).Table != 6 {
		t.Fatalf("taşınmadı: %+v %v", res, err)
	}
	b := e.add(7, "Cağ")
	e.add(7, "Kola")
	e.print(b)
	res, _, err = e.svc.MoveTable(a, 7)
	if err != nil || !res.Merged || res.TargetNo != b {
		t.Fatalf("birleşmedi: %+v %v", res, err)
	}
	t7 := e.adisyon(b)
	q := map[string]Line{}
	for _, l := range t7.Lines {
		q[l.Name] = l
	}
	if q["Cağ"].Qty != 3 || q["Cağ"].PrintedQty != 1 || q["Ayran"].Qty != 1 || q["Kola"].Qty != 1 {
		t.Fatalf("kalemler toplanmadı: %+v", t7.Lines)
	}
	if t7.Total != 3*34000+5000+15000 || !t7.Changed {
		t.Fatalf("birleşen tutar ya da değişti işareti yanlış: %d %v", t7.Total, t7.Changed)
	}
	src := e.adisyon(a)
	if src.Status != StatusCancelled || src.MergedInto != b {
		t.Fatalf("kaynak adisyon birleşti olarak kalmalı: %+v", src)
	}
	e.pay(b, PayCash)
	e.unlock()
	ds, _ := e.svc.DaySummary("")
	if ds.Merged != 1 || ds.Cancelled != 0 || ds.Revenue != t7.Total {
		t.Fatalf("birleşen masa iptal sayılmamalı: merged=%d cancel=%d ciro=%d", ds.Merged, ds.Cancelled, ds.Revenue)
	}
}

func TestBulkPrice(t *testing.T) {
	e := newEnv(t)
	e.setup()
	open := e.add(1, "Cağ")
	if _, _, err := e.svc.BulkPrice(BulkPriceInput{Percent: 10, Round: 500}); code(err) != "locked" {
		t.Fatalf("şifresiz zam: %v", err)
	}
	e.unlock()
	res, st, err := e.svc.BulkPrice(BulkPriceInput{Percent: 10, Round: 500})
	if err != nil || st != nil {
		t.Fatalf("önizleme: %v", err)
	}
	ch := res.Changes
	prices := map[string]int64{}
	for _, c := range ch {
		prices[c.Name] = c.New
	}
	// 340 → 374 → 375; 300 → 330; 50 → 55; fiyatsız ürünler listede yok.
	if prices["Cağ"] != 37500 || prices["Şiş"] != 33000 || prices["Ayran"] != 5500 || len(ch) != 6 {
		t.Fatalf("yeni fiyatlar yanlış: %+v", ch)
	}
	if e.productPrice("Cağ") != 34000 {
		t.Fatal("önizleme fiyatı değiştirdi")
	}
	if _, _, err := e.svc.BulkPrice(BulkPriceInput{IDs: []int64{e.productID("Cağ")}, Percent: -5, Round: 1000, Apply: true}); err != nil {
		t.Fatal(err)
	}
	if e.productPrice("Cağ") != 32000 || e.productPrice("Şiş") != 30000 {
		t.Fatalf("yalnız seçilen ürün değişmeli: %d %d", e.productPrice("Cağ"), e.productPrice("Şiş"))
	}
	if e.adisyon(open).Lines[0].Price != 34000 {
		t.Fatal("açık adisyonun fiyatı değişti")
	}
	if _, _, err := e.svc.BulkPrice(BulkPriceInput{Percent: 0}); code(err) != "invalid" {
		t.Fatal("sıfır yüzde kabul edildi")
	}
	// Yuvarlama zammı indirime çevirmez; ucuz üründe adım küçülür.
	cases := []struct {
		old  int64
		pct  float64
		step int64
		want int64
	}{
		{7200, 2, 1000, 7500},  // 73,44 → 10 ₺'ye 70 olurdu (indirim): 5 ₺ → 75
		{1500, -2, 1000, 1500}, // 14,70 → 10 ₺ değil: 1 ₺ adım → 15 (değişmez)
		{2500, 10, 500, 2800},  // 27,50 → 50 ₺ altı: 1 ₺ adım → 28
		{34000, -5, 1000, 32000},
		{5000, 10, 500, 5500},
		{1234, 3, 0, 1271},
	}
	for _, c := range cases {
		if got := newPrice(c.old, c.pct, c.step); got != c.want {
			t.Errorf("newPrice(%d, %v, %d) = %d, beklenen %d", c.old, c.pct, c.step, got, c.want)
		}
	}
}

func (e *env) productPrice(name string) int64 {
	for _, p := range e.state().Products {
		if p.Name == name {
			return p.Price
		}
	}
	return -1
}

func TestStatsPeriodsAndBreakdowns(t *testing.T) {
	e := newEnv(t)
	e.setup()
	// Eylül'ün 1–3'ü ve Ekim'in 1–3'ü: her gün iki adisyon.
	e.clk.mu.Lock()
	e.clk.t = time.Date(2026, 9, 1, 12, 0, 0, 0, time.Local)
	e.clk.mu.Unlock()
	day := func() {
		a := e.add(1, "Cağ")
		e.add(1, "Ayran")
		e.pay(a, PayCash)
		e.clk.add(2 * time.Hour)
		b := e.add(2, "Cağ")
		e.add(2, "Cağ")
		e.pay(b, PayCard)
	}
	for i := 0; i < 3; i++ {
		day()
		e.clk.add(22 * time.Hour)
	}
	e.clk.mu.Lock()
	e.clk.t = time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local)
	e.clk.mu.Unlock()
	for i := 0; i < 3; i++ {
		day()
		if i == 2 {
			c := e.add(3, "Şiş")
			e.pay(c, PayTransfer)
		}
		e.clk.add(22 * time.Hour)
	}
	// Bugün 4 Ekim: Ekim'in 1–4'ü, Eylül'ün 1–4'üyle karşılaştırılır.
	e.unlock()
	st, err := e.svc.Stats(StatsQuery{Mode: "month", From: "2026-10-15"})
	if err != nil {
		t.Fatal(err)
	}
	if st.From != "2026-10-01" || st.To != "2026-10-04" || !st.Partial || st.PrevFrom != "2026-09-01" || st.PrevTo != "2026-09-04" {
		t.Fatalf("dönem yanlış: %s–%s önceki %s–%s", st.From, st.To, st.PrevFrom, st.PrevTo)
	}
	perDay := int64(34000 + 5000 + 2*34000)
	if st.Revenue.Value != 3*perDay+30000 || st.Revenue.Prev != 3*perDay || st.Count.Value != 7 || st.Count.Prev != 6 {
		t.Fatalf("kartlar yanlış: %+v %+v", st.Revenue, st.Count)
	}
	if st.Revenue.Change == nil || *st.Revenue.Change <= 0 {
		t.Fatalf("değişim yüzdesi yanlış: %v", st.Revenue.Change)
	}
	if st.Featured.Value != 9 || st.Featured.Prev != 9 || st.FeaturedName != "Cağ" {
		t.Fatalf("satılan cağ yanlış: %+v", st.Featured)
	}
	if len(st.Days) != 4 || st.Days[3].Revenue != 0 || st.Days[2].Transfer != 30000 || st.Days[0].Cash != 39000 {
		t.Fatalf("günlük veriler yanlış: %+v", st.Days)
	}
	var pay int64
	for _, p := range st.ByPayment {
		pay += p.Total
	}
	if pay != st.Revenue.Value {
		t.Fatalf("ödeme şekilleri ciroyu tutmuyor: %d / %d", pay, st.Revenue.Value)
	}
	// Gün × ürün tablosu ürün toplamlarını tutar.
	for j, p := range st.Products {
		sum := 0
		for i := range st.Days {
			sum += st.Matrix[i][j]
		}
		if sum != p.Qty {
			t.Fatalf("%s: tablo %d, toplam %d", p.Name, sum, p.Qty)
		}
	}
	hours := 0
	for _, h := range st.Hours {
		hours += h.Count
	}
	if hours != 7 || st.Hours[12].Count != 3 || st.Hours[14].Count != 4 {
		t.Fatalf("saat yoğunluğu yanlış: %+v", st.Hours)
	}
	// Yıl görünümü aylık toplar.
	y, err := e.svc.Stats(StatsQuery{Mode: "year", From: "2026-03-01"})
	if err != nil || len(y.Months) != 10 || y.Months[8].Revenue != 3*perDay || y.Months[9].Count != 7 {
		t.Fatalf("yıl görünümü: %v %+v", err, y.Months)
	}
	// Tarih aralığı: önceki dönem aynı uzunlukta hemen öncesidir.
	r, err := e.svc.Stats(StatsQuery{Mode: "range", From: "2026-10-01", To: "2026-10-03"})
	if err != nil || r.PrevFrom != "2026-09-28" || r.PrevTo != "2026-09-30" {
		t.Fatalf("aralık: %v %s–%s", err, r.PrevFrom, r.PrevTo)
	}
	// Excel raporu dört sayfa ve geçerli XML içerir.
	list, _ := e.svc.periodAdisyons(st.From, st.To)
	var buf bytes.Buffer
	if err := xlsx.Write(&buf, excelReport("Halis", st, list, nil, e.clk.now())); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var wbXML string
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		dec := xml.NewDecoder(bytes.NewReader(b))
		for {
			if _, err := dec.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("%s geçersiz XML: %v", f.Name, err)
			}
		}
		if f.Name == "xl/workbook.xml" {
			wbXML = string(b)
		}
		if f.Name == "xl/worksheets/sheet4.xml" && (!strings.Contains(string(b), "Havale") || !strings.Contains(string(b), "Kredi Kartı")) {
			t.Fatal("Adisyonlar sayfasında ödeme şekli yok")
		}
	}
	for _, n := range []string{"Özet", "Günlük", "Ürünler", "Adisyonlar"} {
		if !strings.Contains(wbXML, `name="`+n+`"`) {
			t.Fatalf("%s sayfası yok", n)
		}
	}
}

func TestRestoreKeepsNumbering(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.add(1, "Cağ")
	e.svc.RecordRestore("adisyon_2026-10-11_10.zip", 41)
	st := e.state()
	if st.NextNo != 42 || !strings.Contains(st.Notice, "adisyon_2026-10-11_10.zip") {
		t.Fatalf("geri yüklemeden sonra numara %d, bildirim %q", st.NextNo, st.Notice)
	}
	if no := e.add(2, "Ayran"); no != 42 {
		t.Fatalf("yeni adisyon no %d, beklenen 42", no)
	}
	// Daha küçük bir taban öncekini düşürmez.
	e.svc.RecordRestore("eski.zip", 10)
	if st := e.state(); st.NextNo != 43 {
		t.Fatalf("taban geriledi: %d", st.NextNo)
	}
	// Gün sonu numara atlamasını gösterir (geri yüklemede kaybolan numaralar).
	e.unlock()
	ds, _ := e.svc.DaySummary("")
	if ds.NoMissing || ds.Warnings[0].Kind != "gap" {
		t.Fatalf("numara atlaması uyarısı yok: %+v", ds.Warnings)
	}
}

func TestSettingsSecretsAndValidation(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.unlock()
	tok := "EAAGgizlianahtar1234"
	enable := true
	if _, err := e.svc.SaveSettings(SettingsPatch{WhatsApp: &WhatsAppPatch{Enabled: &enable}}); code(err) != "invalid" {
		t.Fatalf("eksik WhatsApp ayarı açıldı: %v", err)
	}
	bad := "Gün Sonu!"
	if _, err := e.svc.SaveSettings(SettingsPatch{WhatsApp: &WhatsAppPatch{Template: &bad}}); code(err) != "invalid" {
		t.Fatalf("geçersiz şablon adı kabul edildi: %v", err)
	}
	pn, tpl := "123456789012345", "gun_sonu_ozeti"
	rc := []string{"0532 123 45 67", "+90 532 123 45 67", "0216 555 44 33"}
	st, err := e.svc.SaveSettings(SettingsPatch{WhatsApp: &WhatsAppPatch{Enabled: &enable, PhoneNumberID: &pn, Token: &tok, Template: &tpl, Recipients: &rc}})
	if err != nil {
		t.Fatal(err)
	}
	if !st.WhatsApp.Enabled || !st.WhatsApp.HasToken || st.WhatsApp.TokenHint != "…1234" || len(st.WhatsApp.Recipients) != 2 || st.WhatsApp.Recipients[0] != "905321234567" {
		t.Fatalf("WhatsApp görünümü yanlış: %+v", st.WhatsApp)
	}
	b, _ := json.Marshal(st)
	if strings.Contains(string(b), tok) {
		t.Fatal("erişim anahtarı ekrana gönderildi")
	}
	// Boş anahtar eskisini silmez.
	empty := ""
	if _, err := e.svc.SaveSettings(SettingsPatch{WhatsApp: &WhatsAppPatch{Token: &empty}}); err != nil || e.svc.Settings().WhatsApp.Token != tok {
		t.Fatalf("boş anahtar eskisini sildi: %v", err)
	}
	u := "https://example.com/exec"
	if _, err := e.svc.SaveSettings(SettingsPatch{Sheets: &SheetsPatch{URL: &u}}); code(err) != "invalid" {
		t.Fatalf("geçersiz Sheets adresi kabul edildi: %v", err)
	}
	u = "https://script.google.com/macros/s/ABC/exec"
	st, err = e.svc.SaveSettings(SettingsPatch{Sheets: &SheetsPatch{URL: &u, Enabled: &enable}})
	if err != nil || len(st.Sheets.Secret) != 32 || !st.Sheets.Enabled {
		t.Fatalf("Sheets ayarı: %+v %v", st.Sheets, err)
	}
	if _, err := e.svc.Lock(); err != nil {
		t.Fatal(err)
	}
	if st := e.state(); st.Sheets.Secret != "" {
		t.Fatal("gizli anahtar kilitliyken gönderildi")
	}
}

// Gün kapanınca özet WhatsApp (şablon mesajı) ve Google Sheets'e gönderilir;
// internet yoksa sırada bekler, gelince gider.
func TestOutboxSendsDayReports(t *testing.T) {
	var mu sync.Mutex
	var waBodies []map[string]any
	var waAuth string
	wa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		waBodies = append(waBodies, body)
		waAuth = r.Header.Get("Authorization")
		mu.Unlock()
		if !strings.HasSuffix(r.URL.Path, "/v25.0/123456789012345/messages") {
			w.WriteHeader(404)
			return
		}
		w.Write([]byte(`{"messaging_product":"whatsapp","messages":[{"id":"wamid.1"}]}`))
	}))
	defer wa.Close()
	old := remote.GraphBase
	remote.GraphBase = wa.URL
	defer func() { remote.GraphBase = old }()
	var sheetBody map[string]any
	sheetsUp := false
	sh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !sheetsUp {
			w.WriteHeader(503)
			return
		}
		json.NewDecoder(r.Body).Decode(&sheetBody)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer sh.Close()

	e := newEnv(t)
	e.setup()
	e.svc.mu.Lock()
	e.svc.set.WhatsApp = WhatsAppSettings{Enabled: true, PhoneNumberID: "123456789012345", Token: "EAAGtest", Template: "gun_sonu_ozeti", Lang: "tr", Recipients: []string{"905321234567"}}
	e.svc.set.Sheets = SheetsSettings{Enabled: true, URL: sh.URL + "/exec", Secret: "gizli"}
	e.svc.mu.Unlock()
	a := e.add(1, "Cağ")
	e.print(a)
	e.pay(a, PayCard)
	e.unlock()
	if _, _, err := e.svc.CloseDay("2026-10-11"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	e.svc.processOutbox(ctx)
	mu.Lock()
	if len(waBodies) != 1 || waAuth != "Bearer EAAGtest" {
		t.Fatalf("WhatsApp gönderilmedi: %d %q", len(waBodies), waAuth)
	}
	tpl := waBodies[0]["template"].(map[string]any)
	params := tpl["components"].([]any)[0].(map[string]any)["parameters"].([]any)
	if tpl["name"] != "gun_sonu_ozeti" || len(params) != 7 || waBodies[0]["to"] != "905321234567" {
		t.Fatalf("şablon yanlış: %+v", waBodies[0])
	}
	for _, p := range params {
		txt := p.(map[string]any)["text"].(string)
		if strings.ContainsAny(txt, "\n\t") || txt == "" {
			t.Fatalf("şablon değişkeni uygun değil: %q", txt)
		}
	}
	if !strings.Contains(params[5].(map[string]any)["text"].(string), "Kredi Kartı 340,00") {
		t.Fatalf("ödeme şekilleri mesajda yok: %v", params[5])
	}
	mu.Unlock()
	// Sheets yanıt vermedi: sırada bekler.
	st := e.state()
	if st.Sheets.Status.Pending != 1 || st.WhatsApp.Status.LastSent == "" {
		t.Fatalf("gönderim durumu yanlış: %+v %+v", st.Sheets.Status, st.WhatsApp.Status)
	}
	mu.Lock()
	sheetsUp = true
	mu.Unlock()
	e.clk.add(10 * time.Minute)
	e.svc.processOutbox(ctx)
	mu.Lock()
	defer mu.Unlock()
	if sheetBody == nil || sheetBody["secret"] != "gizli" || sheetBody["type"] != "gun_sonu" {
		t.Fatalf("Sheets verisi yanlış: %+v", sheetBody)
	}
	rows := sheetBody["adisyonlar"].([]any)
	if len(rows) != 1 || rows[0].([]any)[7] != "Kredi Kartı" {
		t.Fatalf("adisyon satırında ödeme şekli yok: %+v", rows)
	}
}

func TestSamplePreviewUsesUnsavedBusiness(t *testing.T) {
	e := newEnv(t)
	e.setup()
	if _, err := e.svc.SamplePreview(nil); code(err) != "locked" {
		t.Fatalf("şifresiz önizleme: %v", err)
	}
	e.unlock()
	a, err := e.svc.SamplePreview(&Business{Name: "Halis Erzurum Cağ Kebabı", Slogan: "Odun ateşinde"})
	if err != nil || !strings.HasPrefix(a, "data:image/png;base64,") {
		t.Fatalf("önizleme: %v", err)
	}
	b, _ := e.svc.SamplePreview(&Business{Name: "Halis Erzurum Cağ Kebabı", Slogan: "Başka bir slogan, daha uzun yazı"})
	if a == b {
		t.Fatal("önizleme yazılanla değişmedi")
	}
	if _, err := e.svc.SamplePreview(&Business{Name: strings.Repeat("x", 200)}); code(err) != "invalid" {
		t.Fatalf("uzun ad kabul edildi: %v", err)
	}
	if e.svc.Settings().Business.Slogan != "" {
		t.Fatal("önizleme ayarı kaydetti")
	}
}

// Gün erken kapatılınca henüz başlamamış ertesi gün kapatılamaz (yoksa sonraki
// günlerin adisyonları bir gün kayardı).
func TestCannotCloseDayBeforeItStarts(t *testing.T) {
	e := newEnv(t)
	e.setup()
	a := e.add(1, "Cağ")
	e.pay(a, PayCash)
	e.clk.add(11 * time.Hour) // 23:00
	e.unlock()
	if _, _, err := e.svc.CloseDay("2026-10-11"); err != nil {
		t.Fatal(err)
	}
	if st := e.state(); st.BusinessDate != "2026-10-12" || st.ClockDate != "2026-10-11" {
		t.Fatalf("açık gün %s, saat günü %s", st.BusinessDate, st.ClockDate)
	}
	if _, _, err := e.svc.CloseDay("2026-10-12"); code(err) != "invalid" {
		t.Fatalf("başlamamış gün kapandı: %v", err)
	}
	// Ertesi gün öğlen açılan masa doğru güne yazılır ve o gün kapatılabilir.
	e.clk.add(13 * time.Hour)
	b := e.add(2, "Ayran")
	if d := e.adisyon(b).BusinessDate; d != "2026-10-12" {
		t.Fatalf("adisyon yanlış güne yazıldı: %s", d)
	}
	e.pay(b, PayCard)
	e.unlock()
	if _, _, err := e.svc.CloseDay("2026-10-12"); err != nil {
		t.Fatal(err)
	}
}

func TestStatsFullMonthComparesWholePreviousMonth(t *testing.T) {
	e := newEnv(t)
	e.setup()
	at := func(y int, m time.Month, d, h int) {
		e.clk.mu.Lock()
		e.clk.t = time.Date(y, m, d, h, 0, 0, 0, time.Local)
		e.clk.mu.Unlock()
	}
	at(2026, 1, 31, 12) // Ocak'ın son günü
	a := e.add(1, "Cağ")
	e.pay(a, PayCash)
	at(2026, 2, 10, 12)
	b := e.add(1, "Cağ")
	e.pay(b, PayCash)
	at(2026, 3, 15, 12) // bugün Mart: Şubat bitti
	e.unlock()
	st, err := e.svc.Stats(StatsQuery{Mode: "month", From: "2026-02-01"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Partial || st.To != "2026-02-28" || st.PrevFrom != "2026-01-01" || st.PrevTo != "2026-01-31" {
		t.Fatalf("dönem: %s–%s önceki %s–%s", st.From, st.To, st.PrevFrom, st.PrevTo)
	}
	if st.Revenue.Prev != 34000 {
		t.Fatalf("Ocak'ın son günü karşılaştırmaya girmedi: %d", st.Revenue.Prev)
	}
	y, _ := e.svc.Stats(StatsQuery{Mode: "year", From: "2025-06-01"})
	if y.PrevFrom != "2024-01-01" || y.PrevTo != "2024-12-31" {
		t.Fatalf("yıl karşılaştırması: %s–%s", y.PrevFrom, y.PrevTo)
	}
}

// 1.0.0'dan gelen veride geçmiş günler (açık masası olmayan) bir kez kapatılmış sayılır.
func TestUpgradeAdoptsPastDays(t *testing.T) {
	e := newEnv(t)
	e.setup()
	for d := 1; d <= 3; d++ {
		e.clk.mu.Lock()
		e.clk.t = time.Date(2026, 10, d, 12, 0, 0, 0, time.Local)
		e.clk.mu.Unlock()
		no := e.add(d, "Cağ")
		if d != 2 {
			e.pay(no, PayCash)
		}
	}
	// 1.0.0 verisi gibi: geçiş işareti yok.
	if _, err := e.st.DB.Exec(`DELETE FROM settings WHERE key='gun_sonu_gecisi'`); err != nil {
		t.Fatal(err)
	}
	e.clk.mu.Lock()
	e.clk.t = time.Date(2026, 10, 11, 12, 0, 0, 0, time.Local)
	e.clk.mu.Unlock()
	svc, err := New(e.st.DB, Options{Version: "test", Now: e.clk.now, Send: e.prn.send})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := svc.State()
	if len(st.UnclosedDays) != 1 || st.UnclosedDays[0] != "2026-10-02" {
		t.Fatalf("yalnız açık masalı gün hatırlatılmalı: %v", st.UnclosedDays)
	}
	if closed, _, _ := dayClosed(e.st.DB, "2026-10-01"); !closed {
		t.Fatal("geçmiş gün kapatılmış sayılmadı")
	}
	svc.Unlock("2468")
	ds, _ := svc.DaySummary("2026-10-03")
	if !ds.Closed || !ds.Migrated {
		t.Fatalf("geçiş günü işaretlenmedi: %+v", ds)
	}
	// İkinci açılışta tekrar yapılmaz (sonradan açılan günler etkilenmez).
	var n int
	e.st.DB.QueryRow(`SELECT COUNT(*) FROM business_days`).Scan(&n)
	if _, err := New(e.st.DB, Options{Version: "test", Now: e.clk.now, Send: e.prn.send}); err != nil {
		t.Fatal(err)
	}
	var n2 int
	e.st.DB.QueryRow(`SELECT COUNT(*) FROM business_days`).Scan(&n2)
	if n != n2 {
		t.Fatalf("geçiş iki kez yapıldı: %d → %d", n, n2)
	}
}

// WhatsApp erişim anahtarı ayarlarda şifreli saklanır; başka bilgisayarda açılamazsa yeniden istenir.
func TestTokenSealedAtRest(t *testing.T) {
	e := newEnv(t)
	seal := func(b []byte) ([]byte, error) {
		out := make([]byte, len(b))
		for i, c := range b {
			out[i] = c ^ 0x5a
		}
		return out, nil
	}
	svc, err := New(e.st.DB, Options{Version: "test", Now: e.clk.now, Send: e.prn.send, Seal: seal, Unseal: seal})
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	e.setup()
	e.unlock()
	tok := "EAAGcokgizlianahtar"
	if _, err := svc.SaveSettings(SettingsPatch{WhatsApp: &WhatsAppPatch{Token: &tok}}); err != nil {
		t.Fatal(err)
	}
	var raw string
	e.st.DB.QueryRow(`SELECT value FROM settings WHERE key='settings'`).Scan(&raw)
	if strings.Contains(raw, tok) || !strings.Contains(raw, sealedPrefix) {
		t.Fatal("erişim anahtarı düz metin saklandı")
	}
	again, _ := New(e.st.DB, Options{Version: "test", Now: e.clk.now, Send: e.prn.send, Seal: seal, Unseal: seal})
	if again.Settings().WhatsApp.Token != tok {
		t.Fatal("anahtar geri açılamadı")
	}
	other, _ := New(e.st.DB, Options{Version: "test", Now: e.clk.now, Send: e.prn.send,
		Unseal: func([]byte) ([]byte, error) { return nil, io.ErrUnexpectedEOF }})
	st, _ := other.State()
	if other.Settings().WhatsApp.Token != "" || !st.WhatsApp.TokenLost || st.WhatsApp.HasToken {
		t.Fatalf("başka bilgisayarda anahtar durumu yanlış: %+v", st.WhatsApp)
	}
}

type reportsDesktop struct {
	nullDesktop
	dir string
}

func (d reportsDesktop) ReportsDir() string { return d.dir }

// Excel raporu Belgeler'deki klasöre yazılır; oraya yazılamazsa programın veri
// klasöründeki Raporlar klasörüne.
func TestExportExcelFallsBackToDataDir(t *testing.T) {
	e := newEnv(t)
	e.setup()
	no := e.add(1, "Cağ")
	e.pay(no, "nakit")
	e.unlock()
	docs := filepath.Join(e.dir, "Belgeler", "Halis Adisyon Raporları")
	e.svc.opt.DataDir = filepath.Join(e.dir, "veri")
	e.svc.opt.Desktop = reportsDesktop{dir: docs}
	p, err := e.svc.ExportExcel(StatsQuery{Mode: "month"})
	if err != nil || filepath.Dir(p) != docs {
		t.Fatalf("rapor: %q %v", p, err)
	}
	// Belgeler'deki klasörün yolunda bir dosya var: klasör açılamaz.
	blocked := filepath.Join(e.dir, "engel")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.svc.opt.Desktop = reportsDesktop{dir: filepath.Join(blocked, "Raporlar")}
	p, err = e.svc.ExportExcel(StatsQuery{Mode: "month"})
	if err != nil || filepath.Dir(p) != filepath.Join(e.svc.opt.DataDir, "Raporlar") {
		t.Fatalf("yedek klasör: %q %v", p, err)
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		t.Fatal(err)
	}
	zr.Close()
	// Hiçbir klasör açılamıyorsa anlaşılır bir hata verilir.
	e.svc.opt.DataDir = blocked
	if _, err := e.svc.ExportExcel(StatsQuery{Mode: "month"}); code(err) != "invalid" || !strings.Contains(err.Error(), "Rapor klasörü") {
		t.Fatalf("hata beklenirdi: %v", err)
	}
}
