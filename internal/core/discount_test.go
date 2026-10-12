package core

import (
	"strings"
	"testing"
)

// Yüzde indirim: yönetici şifresi ister, toplam ve KDV indirimli tutardan
// hesaplanır, fişe ve gün sonuna yansır, kaldırmak şifre istemez.
func TestDiscountNeedsPinAndRecalculates(t *testing.T) {
	e := newEnv(t)
	e.setup()
	no := e.add(1, "Cağ") // 340,00
	e.add(1, "Cağ")
	e.add(1, "Ayran") // 50,00 → ara toplam 730,00
	if _, err := e.svc.SetDiscount(no, 10, ""); code(err) != "pin_required" {
		t.Fatalf("indirim şifresiz uygulandı: %v", err)
	}
	if _, err := e.svc.SetDiscount(no, 101, "2468"); code(err) != "invalid" {
		t.Fatalf("yüzde sınırı: %v", err)
	}
	if _, err := e.svc.SetDiscount(no, 10, "2468"); err != nil {
		t.Fatal(err)
	}
	a := e.adisyon(no)
	if a.DiscountPercent != 10 || a.DiscountAmount != 7300 || a.Total != 65700 {
		t.Fatalf("indirim yanlış: %%%d %d toplam %d", a.DiscountPercent, a.DiscountAmount, a.Total)
	}
	groups, sub, disc, total, vat := Breakdown(a.Lines, a.DiscountPercent)
	if sub != 73000 || disc != 7300 || total != 65700 || vat != a.VATTotal || len(groups) != 1 || groups[0].Gross != 65700 {
		t.Fatalf("döküm yanlış: %d %d %d %d %+v", sub, disc, total, vat, groups)
	}
	// Ürün eklenince indirim yeni toplama göre yeniden hesaplanır.
	e.add(1, "Ayran") // ara toplam 780,00 → indirim 78,00
	if a = e.adisyon(no); a.DiscountAmount != 7800 || a.Total != 70200 {
		t.Fatalf("ürün sonrası indirim: %d / %d", a.DiscountAmount, a.Total)
	}
	// Fişte ara toplam ve indirim satırı var (önizleme çizilebiliyor).
	if p, err := e.svc.Preview(no); err != nil || !strings.HasPrefix(p, "data:image/png") {
		t.Fatalf("önizleme: %v", err)
	}
	// Basıldıktan sonra indirim değişirse “tekrar bas” uyarısı.
	e.print(no)
	if _, err := e.svc.SetDiscount(no, 20, "2468"); err != nil {
		t.Fatal(err)
	}
	if a = e.adisyon(no); !a.Changed || a.DiscountAmount != 15600 {
		t.Fatalf("basılmış adisyonda indirim: changed=%v tutar=%d", a.Changed, a.DiscountAmount)
	}
	// Kaldırmak şifre istemez.
	if _, err := e.svc.SetDiscount(no, 0, ""); err != nil {
		t.Fatal(err)
	}
	if a = e.adisyon(no); a.DiscountPercent != 0 || a.DiscountAmount != 0 || a.Total != 78000 {
		t.Fatalf("indirim kalkmadı: %+v", a)
	}
	// Ayar: şifresiz indirim.
	e.unlock()
	on := true
	if _, err := e.svc.SaveSettings(SettingsPatch{DiscountNoPIN: &on}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetDiscount(no, 100, ""); err != nil {
		t.Fatalf("şifresiz indirim ayarı: %v", err)
	}
	if a = e.adisyon(no); a.Total != 0 || a.DiscountAmount != 78000 || a.VATTotal != 0 {
		t.Fatalf("ikram (%%100): %+v", a)
	}
	if _, err := e.svc.SetDiscount(no, 10, ""); err != nil {
		t.Fatal(err)
	}
	e.pay(no, PayCash)
	// Gün sonu ve istatistik: ciro indirimli, indirim toplamı ayrı.
	ds, err := e.svc.DaySummary("")
	if err != nil {
		t.Fatal(err)
	}
	if ds.Revenue != 70200 || ds.Discounts != 1 || ds.DiscountTotal != 7800 || ds.Rows[0].DiscountPercent != 10 {
		t.Fatalf("gün sonu indirimi: ciro=%d %d/%d %+v", ds.Revenue, ds.Discounts, ds.DiscountTotal, ds.Rows[0])
	}
	var vatSum int64
	for _, v := range ds.VATs {
		vatSum += v.VAT
		if v.Discount != 7800 || v.Gross != 70200 {
			t.Fatalf("KDV grubu indirimi: %+v", v)
		}
	}
	if vatSum != ds.VAT {
		t.Fatalf("KDV toplamı: %d / %d", vatSum, ds.VAT)
	}
	st, err := e.svc.Stats(StatsQuery{Mode: "day"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Revenue.Value != 70200 || st.Discounts != 1 || st.DiscountTotal != 7800 || st.Days[0].Discount != 7800 {
		t.Fatalf("istatistik indirimi: %+v", st)
	}
	d, err := e.svc.Detail(no)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, h := range d.History {
		kinds = append(kinds, h.Action)
	}
	if !strings.Contains(strings.Join(kinds, ","), "indirim,") || !strings.Contains(strings.Join(kinds, ","), "indirim_kaldirildi") {
		t.Fatalf("işlem kaydı: %v", kinds)
	}
}

// Ödenmiş adisyon yönetici şifresiyle yeniden açılır (düzeltme) ya da iptal edilir;
// kapatılmış günün adisyonu açılamaz; dolu masaya açılamaz.
func TestReopenAndCancelPaid(t *testing.T) {
	e := newEnv(t)
	e.setup()
	no := e.add(5, "Cağ")
	for i := 0; i < 7; i++ {
		e.add(5, "Cağ") // yanlışlıkla 8 cağ
	}
	e.print(no)
	e.pay(no, PayCard)
	if _, _, err := e.svc.Reopen(no, 0, "Yanlış adet", ""); code(err) != "pin_required" {
		t.Fatalf("şifresiz açıldı: %v", err)
	}
	if _, _, err := e.svc.Reopen(no, 0, "", "2468"); code(err) != "invalid" {
		t.Fatalf("sebepsiz açıldı: %v", err)
	}
	// Masa 5 bu arada doldu: kendi masasına açılamaz, boş masaya açılır.
	other := e.add(5, "Ayran")
	if _, _, err := e.svc.Reopen(no, 0, "Yanlış adet", "2468"); code(err) != "invalid" || !strings.Contains(err.Error(), "dolu") {
		t.Fatalf("dolu masaya açıldı: %v", err)
	}
	st, table, err := e.svc.Reopen(no, 6, "Yanlış adet", "2468")
	if err != nil || table != 6 {
		t.Fatalf("yeniden açma: %v (masa %d)", err, table)
	}
	a := e.adisyon(no)
	if a.Status != StatusPrinted || a.Table != 6 || a.PaymentMethod != "" || a.ClosedAt != "" || a.PrintCount != 1 {
		t.Fatalf("yeniden açılan adisyon: %+v", a)
	}
	found := false
	for _, o := range st.Open {
		if o.No == no && o.Table == 6 {
			found = true
		}
	}
	if !found {
		t.Fatalf("açık masalarda yok: %+v", st.Open)
	}
	// Adet düşürmek basılmış adetin altına indiği için şifre ister; sonra tekrar ödenir.
	line := a.Lines[0].ID
	if _, err := e.svc.ChangeQty(no, line, -3, ""); code(err) != "pin_required" {
		t.Fatalf("basılmış adet şifresiz düştü: %v", err)
	}
	if _, err := e.svc.ChangeQty(no, line, -3, "2468"); err != nil {
		t.Fatal(err)
	}
	e.pay(no, PayCash)
	if a = e.adisyon(no); a.Status != StatusPaid || a.Lines[0].Qty != 5 || a.Total != 5*34000 || a.PaymentMethod != PayCash {
		t.Fatalf("düzeltilmiş adisyon: %+v", a)
	}
	e.unlock()
	ds, err := e.svc.DaySummary("")
	if err != nil {
		t.Fatal(err)
	}
	if ds.Paid != 1 || ds.Revenue != 5*34000 || ds.Open != 1 {
		t.Fatalf("özet: %+v", ds)
	}
	d, _ := e.svc.Detail(no)
	var acts []string
	for _, h := range d.History {
		acts = append(acts, h.Action)
	}
	joined := strings.Join(acts, ",")
	if !strings.Contains(joined, "odendi,yeniden_acildi,kalem,odendi") {
		t.Fatalf("işlem kaydı sırası: %s", joined)
	}
	// Ödenmiş adisyon iptal edilir; ödeme kaydı kalkar.
	if _, err := e.svc.Cancel(no, "Müşteri itiraz etti", "2468"); err != nil {
		t.Fatal(err)
	}
	if a = e.adisyon(no); a.Status != StatusCancelled || a.PaymentMethod != "" {
		t.Fatalf("ödenmiş iptal: %+v", a)
	}
	if _, _, err := e.svc.Reopen(no, 0, "x", "2468"); code(err) != "invalid" {
		t.Fatalf("iptal edilen yeniden açıldı: %v", err)
	}
	// Kapatılmış günün adisyonu açılamaz.
	e.pay(other, PayCash)
	if _, _, err := e.svc.CloseDay("2026-10-11"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Reopen(other, 0, "Yanlış", "2468"); code(err) != "closed" {
		t.Fatalf("kapalı günde açıldı: %v", err)
	}
}
