package core

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"halisadisyon/internal/escpos"
	"halisadisyon/internal/printing"
	"halisadisyon/internal/store"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type fakePrinter struct {
	mu    sync.Mutex
	jobs  [][]byte
	fail  *printing.Result
	block chan struct{} // nil değilse gönderim bu kanal kapanana kadar bekler
}

func (f *fakePrinter) send(cfg printing.Config, data []byte, doc string) printing.Result {
	f.mu.Lock()
	b := f.block
	f.mu.Unlock()
	if b != nil {
		<-b
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return *f.fail
	}
	f.jobs = append(f.jobs, append([]byte(nil), data...))
	return printing.Result{OK: true}
}

type env struct {
	t   *testing.T
	st  *store.Store
	svc *Service
	clk *clock
	prn *fakePrinter
	dir string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "adisyon.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(nil); err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, st: st, dir: dir, clk: &clock{t: time.Date(2026, 10, 11, 12, 0, 0, 0, time.Local)}, prn: &fakePrinter{}}
	e.svc, err = New(st.DB, Options{Version: "test", Now: e.clk.now, Send: e.prn.send})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return e
}

func (e *env) state() *State {
	st, err := e.svc.State()
	if err != nil {
		e.t.Fatal(err)
	}
	return st
}

func (e *env) productID(name string) int64 {
	for _, p := range e.state().Products {
		if p.Name == name {
			return p.ID
		}
	}
	e.t.Fatalf("ürün yok: %s", name)
	return 0
}

func (e *env) setup() {
	st := e.state()
	prices := map[string]int64{}
	defaults := map[string]int64{"Cağ": 34000, "Şiş": 30000, "Ayran": 5000, "Kola": 15000, "Büyük Su": 5000, "Çorba": 12000}
	for _, p := range st.Products {
		prices[strconv.FormatInt(p.ID, 10)] = defaults[p.Name]
	}
	_, err := e.svc.FinishSetup(SetupInput{
		Business: Business{Name: "Halis Erzurum Cağ Kebabı", Address1: "Merkez"},
		Tables:   24, Waiters: []string{"Ahmet", "Mehmet", "  ", "ahmet"},
		Printer: printing.Config{Mode: printing.ModeNetwork, Host: "127.0.0.1", Port: 9100, Width: 576, Cut: true},
		Prices:  prices, Pin: "2468",
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) add(table int, name string) int64 {
	_, no, err := e.svc.AddItem(table, e.productID(name), 0)
	if err != nil {
		e.t.Fatal(err)
	}
	return no
}

func (e *env) adisyon(no int64) *Adisyon {
	a, err := loadAdisyon(e.st.DB, no)
	if err != nil {
		e.t.Fatal(err)
	}
	return a
}

func code(err error) string {
	var ue *UserError
	if errors.As(err, &ue) {
		return ue.Code
	}
	return ""
}

func TestFirstRunDefaults(t *testing.T) {
	e := newEnv(t)
	st := e.state()
	if st.SetupDone || !st.Unlocked {
		t.Fatal("ilk açılışta kurulum bekleniyor ve kilit açık olmalı")
	}
	if len(st.Products) != 12 || len(st.Categories) != 3 || len(st.Waiters) != 1 {
		t.Fatalf("başlangıç verisi yanlış: %d ürün, %d kategori, %d garson", len(st.Products), len(st.Categories), len(st.Waiters))
	}
	want := []string{"Cağ", "Şiş", "Yoğurt", "Tatlı", "Salata", "Ayran", "Şalgam", "Kola", "Soda", "Büyük Su", "Küçük Su", "Çorba"}
	for i, p := range st.Products {
		if p.Name != want[i] || p.Price != 0 || p.VAT != nil || !p.Active {
			t.Fatalf("ürün %d yanlış: %+v", i, p)
		}
	}
	if st.NextNo != 1 || st.Tables != 24 || st.DefaultVAT != 10 {
		t.Fatalf("başlangıç ayarları yanlış: %+v", st)
	}
	// Fiyatı olmayan ürün satılamaz.
	if _, _, err := e.svc.AddItem(1, st.Products[0].ID, 0); code(err) != "invalid" {
		t.Fatalf("fiyatsız ürün eklenebildi: %v", err)
	}
}

func TestSetupNumberingMergeUndo(t *testing.T) {
	e := newEnv(t)
	e.setup()
	st := e.state()
	if !st.SetupDone || st.Unlocked {
		t.Fatal("kurulumdan sonra kilitli olmalı")
	}
	if len(st.Waiters) != 2 || st.Waiters[0].Name != "Ahmet" {
		t.Fatalf("garsonlar yanlış: %+v", st.Waiters)
	}
	no1 := e.add(5, "Cağ")
	if no1 != 1 {
		t.Fatalf("ilk adisyon no %d, beklenen 1", no1)
	}
	e.add(5, "Cağ")
	e.add(5, "Ayran")
	no2 := e.add(7, "Kola")
	if no2 != 2 {
		t.Fatalf("ikinci masa no %d, beklenen 2", no2)
	}
	a := e.adisyon(no1)
	if len(a.Lines) != 2 || a.Lines[0].Qty != 2 || a.Lines[0].Name != "Cağ" {
		t.Fatalf("aynı ürün birleşmedi: %+v", a.Lines)
	}
	if a.Total != 2*34000+5000 || a.VATTotal != 6636 {
		t.Fatalf("toplam/KDV yanlış: %d / %d", a.Total, a.VATTotal)
	}
	if a.WaiterName != "Ahmet" || a.BusinessDate != "2026-10-11" || a.Table != 5 {
		t.Fatalf("adisyon bilgisi yanlış: %+v", a)
	}
	// azalt, sıfırla, geri al
	if _, err := e.svc.ChangeQty(no1, a.Lines[1].ID, -1, ""); err != nil {
		t.Fatal(err)
	}
	if got := e.adisyon(no1); len(got.Lines) != 1 {
		t.Fatalf("adet 0 olunca kalem kalkmadı: %+v", got.Lines)
	}
	if _, err := e.svc.Undo(no1); err != nil {
		t.Fatal(err)
	}
	got := e.adisyon(no1)
	if len(got.Lines) != 2 || got.Lines[1].Name != "Ayran" || got.Total != 73000 {
		t.Fatalf("geri al çalışmadı: %+v", got)
	}
	if e.state().UndoCounts[no1] == 0 {
		t.Fatal("geri al sayısı görünmüyor")
	}
	// Tüm geri almalar adisyonu boşaltır; boş adisyon kapatılır ve numara iptal olarak kalır.
	for e.state().UndoCounts[no1] > 0 {
		if _, err := e.svc.Undo(no1); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.adisyon(no1); len(got.Lines) != 0 {
		t.Fatalf("tüm geri almalardan sonra adisyon boş olmalı: %+v", got.Lines)
	}
	if _, err := e.svc.CloseEmpty(no1, ""); err != nil {
		t.Fatal(err)
	}
	if got := e.adisyon(no1); got.Status != StatusCancelled || got.CancelReason == "" {
		t.Fatalf("boş masa kapatılmadı: %+v", got)
	}
	if no3 := e.add(5, "Şiş"); no3 != 3 {
		t.Fatalf("numara tekrar kullanıldı: %d", no3)
	}
}

func TestPrintCopyChangedAndFailure(t *testing.T) {
	e := newEnv(t)
	e.setup()
	no := e.add(3, "Cağ")
	out, _, err := e.svc.Print(no)
	if err != nil || !out.OK || out.Copy {
		t.Fatalf("ilk baskı: %+v %v", out, err)
	}
	a := e.adisyon(no)
	if a.Status != StatusPrinted || a.PrintCount != 1 || a.PrintedAt == "" {
		t.Fatalf("basıldı bilgisi yok: %+v", a)
	}
	if len(e.prn.jobs) != 1 {
		t.Fatal("yazıcıya iş gitmedi")
	}
	d, err := escpos.Decode(e.prn.jobs[0])
	if err != nil || d.Image.Bounds().Dx() != 576 || !d.Cut {
		t.Fatalf("ESC/POS çıktısı çözülemedi: %v", err)
	}
	out, _, _ = e.svc.Print(no)
	if !out.Copy {
		t.Fatal("ikinci baskı KOPYA olmalı")
	}
	e.add(3, "Ayran")
	if a := e.adisyon(no); !a.Changed {
		t.Fatal("basıldıktan sonra değişiklik işaretlenmedi")
	}
	out, _, _ = e.svc.Print(no)
	if out.Copy || e.adisyon(no).Changed || e.adisyon(no).PrintCount != 3 {
		t.Fatal("değişiklikten sonraki baskı kopya sayılmamalı ve işaret kalkmalı")
	}
	// Yazıcı hatası: adisyon kaybolmaz, basılamadı işaretlenir, masa açık kalır.
	e.prn.fail = &printing.Result{Code: "offline", Title: "Adisyon basılamadı", Message: "Yazıcıya ulaşılamıyor."}
	out, _, err = e.svc.Print(no)
	if err != nil || out.OK {
		t.Fatalf("baskı hatası bildirilmedi: %+v %v", out, err)
	}
	if a := e.adisyon(no); a.Status != StatusPrinted || !a.PrintFailed {
		t.Fatalf("basılamadı işareti yok: %+v", a)
	}
	e.prn.fail = nil
	// Ödeme şekli seçilmeden hesap kapanmaz.
	if _, _, err := e.svc.Pay(no, ""); code(err) != "invalid" {
		t.Fatalf("ödeme şekilsiz kapandı: %v", err)
	}
	if _, _, err := e.svc.Pay(no, "cek"); code(err) != "invalid" {
		t.Fatalf("bilinmeyen ödeme şekli kabul edildi: %v", err)
	}
	jobs := len(e.prn.jobs)
	pay, _, err := e.svc.Pay(no, PayCard)
	if err != nil || !pay.Paid || pay.Method != PayCard || pay.MethodLabel != "Kredi Kartı" {
		t.Fatalf("hesap kapanmadı: %+v %v", pay, err)
	}
	// Hesap kapatılırken fiş basılmaz (adisyon fişi ödemeden önce basılır).
	if len(e.prn.jobs) != jobs {
		t.Fatal("hesap kapatılırken baskı yapıldı")
	}
	if a := e.adisyon(no); a.Status != StatusPaid || a.ClosedAt == "" || a.PaymentMethod != PayCard {
		t.Fatalf("ödendi durumu yanlış: %+v", a)
	}
	if len(e.state().Open) != 0 {
		t.Fatal("masa boşalmadı")
	}
	if _, err := e.svc.ChangeQty(no, 1, 1, ""); code(err) != "closed" {
		t.Fatal("kapanmış adisyon değiştirilebildi")
	}
}

func TestPreviewModeReturnsImage(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.svc.mu.Lock()
	e.svc.set.Printer.Mode = printing.ModeNone
	e.svc.mu.Unlock()
	no := e.add(1, "Çorba")
	out, _, err := e.svc.Print(no)
	if err != nil || !out.OK || len(out.Preview) < 100 || len(e.prn.jobs) != 0 {
		t.Fatalf("önizleme modu çalışmadı: %v", err)
	}
	b64 := out.Preview[len("data:image/png;base64,"):]
	raw, _ := base64.StdEncoding.DecodeString(b64)
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil || img.Bounds().Dx() != 576 {
		t.Fatalf("önizleme resmi bozuk: %v", err)
	}
}

func TestCancelNeedsPinAndReason(t *testing.T) {
	e := newEnv(t)
	e.setup()
	no := e.add(9, "Kola")
	if _, err := e.svc.Cancel(no, "Yanlış masa", ""); code(err) != "pin_required" {
		t.Fatalf("şifresiz iptal edildi: %v", err)
	}
	if _, err := e.svc.Cancel(no, "Yanlış masa", "0000"); code(err) != "pin_wrong" {
		t.Fatalf("yanlış şifre kabul edildi: %v", err)
	}
	if _, err := e.svc.Cancel(no, "   ", "2468"); code(err) != "invalid" {
		t.Fatal("sebepsiz iptal edildi")
	}
	// Açık yönetici girişi iptal için yetmez; her iptal şifre ister.
	if _, err := e.svc.Unlock("2468"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Cancel(no, "Yanlış masa", ""); code(err) != "pin_required" {
		t.Fatalf("yönetici girişi açıkken şifresiz iptal edildi: %v", err)
	}
	e.svc.Lock()
	if _, err := e.svc.Cancel(no, "Yanlış masa", "2468"); err != nil {
		t.Fatal(err)
	}
	a := e.adisyon(no)
	if a.Status != StatusCancelled || a.CancelReason != "Yanlış masa" {
		t.Fatalf("iptal kaydı yanlış: %+v", a)
	}
	// İptal şifresi yönetici girişini açmaz.
	if _, err := e.svc.SaveWaiter(0, "Ali"); code(err) != "locked" {
		t.Fatal("iptal yönetici girişini açtı")
	}
	// Kilit süresi dolunca tekrar şifre ister.
	e.svc.Unlock("2468")
	e.clk.add(6 * time.Minute)
	if _, err := e.svc.SaveWaiter(0, "Ali"); code(err) != "locked" {
		t.Fatal("kilit kendiliğinden kapanmadı")
	}
}

// Basılmış adisyonu “−” ile boşaltıp kapatmak bir iptaldir; şifre ister.
func TestCloseEmptyAfterPrintNeedsPin(t *testing.T) {
	e := newEnv(t)
	e.setup()
	no := e.add(22, "Cağ")
	e.add(22, "Cağ")
	if out, _, err := e.svc.Print(no); err != nil || !out.OK {
		t.Fatalf("baskı: %v", err)
	}
	line := e.adisyon(no).Lines[0].ID
	// Basılan adetin altına inmek şifre ister; şifre girilince düşülür.
	if _, err := e.svc.ChangeQty(no, line, -1, ""); code(err) != "pin_required" {
		t.Fatalf("basılmış üründen şifresiz düşüldü: %v", err)
	}
	if a := e.adisyon(no); a.Lines[0].Qty != 2 {
		t.Fatal("şifre sorulurken adet değişti")
	}
	if _, err := e.svc.ChangeQty(no, line, -2, "2468"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CloseEmpty(no, ""); code(err) != "pin_required" {
		t.Fatalf("basılmış adisyon şifresiz kapatıldı: %v", err)
	}
	if _, err := e.svc.CloseEmpty(no, "1111"); code(err) != "pin_wrong" {
		t.Fatalf("yanlış şifreyle kapatıldı: %v", err)
	}
	if a := e.adisyon(no); a.Status != StatusPrinted {
		t.Fatalf("adisyon kapanmamalıydı: %s", a.Status)
	}
	if _, err := e.svc.CloseEmpty(no, "2468"); err != nil {
		t.Fatal(err)
	}
	a := e.adisyon(no)
	if a.Status != StatusCancelled || a.CancelReason == "" {
		t.Fatalf("iptal kaydı yanlış: %+v", a)
	}
	// Hiç basılmamış boş adisyon şifresiz kapanır.
	no2 := e.add(23, "Ayran")
	e.svc.ChangeQty(no2, e.adisyon(no2).Lines[0].ID, -1, "")
	if _, err := e.svc.CloseEmpty(no2, ""); err != nil {
		t.Fatalf("basılmamış boş adisyon kapanmadı: %v", err)
	}
}

// Aynı adisyon iki kez aynı anda basılamaz (ikisi de “asıl” sayılırdı).
func TestPrintTwiceAtOnceIsRefused(t *testing.T) {
	e := newEnv(t)
	e.setup()
	no := e.add(4, "Şiş")
	e.prn.mu.Lock()
	e.prn.block = make(chan struct{})
	e.prn.mu.Unlock()
	done := make(chan *PrintOutcome)
	go func() {
		out, _, _ := e.svc.Print(no)
		done <- out
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		e.svc.mu.Lock()
		busy := e.svc.inPrint[no]
		e.svc.mu.Unlock()
		if busy || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, _, err := e.svc.Print(no); code(err) != "busy" {
		t.Fatalf("ikinci baskı reddedilmedi: %v", err)
	}
	if _, _, err := e.svc.Pay(no, PayCash); code(err) != "busy" {
		t.Fatalf("baskı sürerken hesap kapandı: %v", err)
	}
	// Baskı sürerken başka masada sipariş alınabilmeli.
	e.add(5, "Ayran")
	close(e.prn.block)
	if out := <-done; !out.OK || out.Copy {
		t.Fatalf("ilk baskı: %+v", out)
	}
	e.prn.mu.Lock()
	e.prn.block = nil
	e.prn.mu.Unlock()
	if out, _, _ := e.svc.Print(no); !out.Copy {
		t.Fatal("ikinci baskı KOPYA olmalı")
	}
}

func TestPinLockoutAndChange(t *testing.T) {
	e := newEnv(t)
	e.setup()
	for i := 0; i < 4; i++ {
		e.svc.Unlock("1111")
	}
	if _, err := e.svc.Unlock("1111"); code(err) != "pin_blocked" {
		t.Fatalf("5 yanlıştan sonra bekletme yok: %v", err)
	}
	if _, err := e.svc.Unlock("2468"); code(err) != "pin_blocked" {
		t.Fatal("bekleme süresinde doğru şifre de beklemeli")
	}
	// Programı kapatıp açmak beklemeyi kaldırmaz.
	svc2, err := New(e.st.DB, Options{Version: "test", Now: e.clk.now, Send: e.prn.send})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc2.Unlock("2468"); code(err) != "pin_blocked" {
		t.Fatalf("yeniden açılınca bekleme kalktı: %v", err)
	}
	e.clk.add(61 * time.Second)
	// İkinci seri yanlış denemede bekleme uzar; şifre değiştirme denemeleri de sayılır.
	for i := 0; i < 4; i++ {
		svc2.Unlock("1111")
	}
	if _, err := svc2.Unlock("1111"); code(err) != "pin_blocked" {
		t.Fatal("ikinci seride bekletme yok")
	}
	e.clk.add(61 * time.Second)
	if _, err := svc2.Unlock("2468"); code(err) != "pin_blocked" {
		t.Fatal("ikinci bekleme 1 dakikadan uzun olmalı")
	}
	e.clk.add(5 * time.Minute)
	if _, err := e.svc.Unlock("2468"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if _, err := e.svc.ChangePin("9999", "1111"); code(err) != "pin_wrong" {
			t.Fatalf("eski şifre yanlışken: %v", err)
		}
	}
	if _, err := e.svc.ChangePin("9999", "1111"); code(err) != "pin_blocked" {
		t.Fatal("şifre değiştirme ekranından sınırsız deneme yapılabiliyor")
	}
	e.clk.add(2 * time.Minute)
	if _, err := e.svc.Unlock("2468"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ChangePin("2468", "12a4"); code(err) != "invalid" {
		t.Fatal("harfli şifre kabul edildi")
	}
	if _, err := e.svc.ChangePin("2468", "1357"); err != nil {
		t.Fatal(err)
	}
	e.svc.Lock()
	if _, err := e.svc.Unlock("1357"); err != nil {
		t.Fatal("yeni şifre çalışmadı")
	}
	if err := e.svc.ResetPin("1234"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Unlock("1234"); err != nil {
		t.Fatal("sıfırlanan şifre çalışmadı")
	}
}

func TestPriceAndVATChangesKeepHistory(t *testing.T) {
	e := newEnv(t)
	e.setup()
	no := e.add(2, "Cağ")
	e.svc.Unlock("2468")
	pid := e.productID("Cağ")
	twenty := 20
	if _, _, err := e.svc.SaveProduct(ProductInput{ID: pid, Name: "Cağ", CategoryID: e.state().Categories[0].ID, Price: 36000, VAT: &twenty, Unit: "şiş", Active: true}); err != nil {
		t.Fatal(err)
	}
	e.add(2, "Cağ")
	a := e.adisyon(no)
	if len(a.Lines) != 2 || a.Lines[0].Price != 34000 || a.Lines[0].VAT != 10 || a.Lines[1].Price != 36000 || a.Lines[1].VAT != 20 {
		t.Fatalf("eski kalem korunmadı: %+v", a.Lines)
	}
	groups, total, vat := VATBreakdown(a.Lines)
	if len(groups) != 2 || total != 70000 || vat != 3091+6000 {
		t.Fatalf("KDV dökümü yanlış: %+v %d %d", groups, total, vat)
	}
	// Varsayılan oranı izleyen ürün yeni orana geçer, eski kalem geçmez.
	if _, err := e.svc.SaveSettings(SettingsPatch{DefaultVAT: ptr(1)}); err != nil {
		t.Fatal(err)
	}
	no2 := e.add(4, "Ayran")
	if e.adisyon(no2).Lines[0].VAT != 1 {
		t.Fatal("varsayılan oran yeni kaleme uygulanmadı")
	}
	if e.adisyon(no).Lines[0].VAT != 10 {
		t.Fatal("eski kalemin oranı değişti")
	}
}

func ptr[T any](v T) *T { return &v }

func TestBusinessDateCutoff(t *testing.T) {
	if got := businessDate(time.Date(2026, 10, 12, 3, 30, 0, 0, time.Local)); got != "2026-10-11" {
		t.Fatalf("gece 03:30 bir önceki güne yazılmalı: %s", got)
	}
	if got := businessDate(time.Date(2026, 10, 12, 5, 0, 0, 0, time.Local)); got != "2026-10-12" {
		t.Fatalf("05:00 yeni gün olmalı: %s", got)
	}
}

func TestSettingsRules(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.svc.Unlock("2468")
	e.add(20, "Kola")
	if _, err := e.svc.SaveSettings(SettingsPatch{Tables: ptr(10)}); code(err) != "invalid" {
		t.Fatal("açık masanın altına inilebildi")
	}
	if _, err := e.svc.SaveSettings(SettingsPatch{Tables: ptr(22)}); err != nil {
		t.Fatal(err)
	}
	b := e.state().Business
	b.GoogleURL = "g.page/abc"
	if _, err := e.svc.SaveSettings(SettingsPatch{Business: &b}); code(err) != "invalid" {
		t.Fatal("https olmayan bağlantı kabul edildi")
	}
	b.GoogleURL = "https://g.page/r/abc/review"
	b.Name = "  "
	if _, err := e.svc.SaveSettings(SettingsPatch{Business: &b}); code(err) != "invalid" {
		t.Fatal("boş işletme adı kabul edildi")
	}
	if _, err := e.svc.SaveSettings(SettingsPatch{SecondBackupDir: ptr("/olmayan/klasor")}); code(err) != "invalid" {
		t.Fatal("olmayan klasör kabul edildi")
	}
	if _, err := e.svc.SaveSettings(SettingsPatch{SecondBackupDir: ptr(e.dir)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SaveSettings(SettingsPatch{Printer: &printing.Config{Mode: "usb"}}); code(err) != "invalid" {
		t.Fatal("geçersiz yazıcı türü kabul edildi")
	}
}

func TestWaitersAndCategories(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.svc.Unlock("2468")
	st := e.state()
	ahmet := st.Waiters[0].ID
	no := e.add(1, "Cağ")
	if _, err := e.svc.RemoveWaiter(ahmet); code(err) != "invalid" {
		t.Fatal("açık masası olan garson kaldırıldı")
	}
	if _, err := e.svc.SaveWaiter(ahmet, "Ahmet Usta"); err != nil {
		t.Fatal(err)
	}
	if e.adisyon(no).WaiterName != "Ahmet Usta" {
		t.Fatal("açık adisyondaki garson adı güncellenmedi")
	}
	if _, err := e.svc.ChangeWaiter(no, st.Waiters[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RemoveWaiter(ahmet); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RemoveWaiter(st.Waiters[1].ID); code(err) != "invalid" {
		t.Fatal("son garson / açık masası olan garson kaldırıldı")
	}
	// kategori: dolu kategori silinemez, boş olan silinir
	cat := st.Categories[2].ID // Tatlı ve Diğer
	if _, err := e.svc.DeleteCategory(cat); code(err) != "invalid" {
		t.Fatal("dolu kategori silindi")
	}
	_, newID, err := e.svc.SaveCategory(CategoryInput{Name: "Mezeler", Color: "yesil"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.MoveCategory(newID, -1); err != nil {
		t.Fatal(err)
	}
	if e.state().Categories[2].Name != "Mezeler" {
		t.Fatal("kategori sırası değişmedi")
	}
	if _, err := e.svc.DeleteCategory(newID); err != nil {
		t.Fatal(err)
	}
	// ürün sırası yalnız kendi kategorisinde değişir
	sis := e.productID("Şiş")
	if _, err := e.svc.MoveProduct(sis, -1); err != nil {
		t.Fatal(err)
	}
	ps := e.state().Products
	if ps[0].Name != "Şiş" || ps[1].Name != "Cağ" {
		t.Fatalf("ürün sırası yanlış: %s, %s", ps[0].Name, ps[1].Name)
	}
	ayran := e.productID("Ayran")
	if _, err := e.svc.MoveProduct(ayran, -1); err != nil {
		t.Fatal(err)
	}
	if e.state().Products[5].Name != "Ayran" {
		t.Fatal("ürün başka kategorinin üstüne geçti")
	}
}

func TestDataSurvivesReopen(t *testing.T) {
	e := newEnv(t)
	e.setup()
	no := e.add(12, "Cağ")
	e.add(12, "Kola")
	path := e.st.Path
	e.st.Close()
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(nil); err != nil {
		t.Fatal(err)
	}
	svc, err := New(st.DB, Options{Now: e.clk.now})
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := svc.State()
	if !s2.SetupDone || len(s2.Open) != 1 || s2.Open[0].No != no || len(s2.Open[0].Lines) != 2 {
		t.Fatalf("yeniden açınca masa gelmedi: %+v", s2.Open)
	}
}

func TestLogo(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.svc.Unlock("2468")
	img := image.NewGray(image.Rect(0, 0, 120, 60))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	for x := 10; x < 110; x++ {
		img.Set(x, 30, color.Black)
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	if _, err := e.svc.SetLogo("data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())); err != nil {
		t.Fatal(err)
	}
	if !e.state().HasLogo || len(e.svc.LogoPNG()) == 0 {
		t.Fatal("logo kaydedilmedi")
	}
	if _, err := e.svc.SetLogo("bozuk!!"); code(err) != "invalid" {
		t.Fatal("bozuk logo kabul edildi")
	}
	no := e.add(1, "Cağ")
	if _, err := e.svc.Preview(no); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetLogo(""); err != nil || e.state().HasLogo {
		t.Fatal("logo kaldırılamadı")
	}
}

func TestTestPrintBeforeAndAfterSetup(t *testing.T) {
	e := newEnv(t)
	cfg := printing.Config{Mode: printing.ModeNetwork, Host: "127.0.0.1", Port: 9100, Width: 576, Cut: true}
	out, err := e.svc.TestPrint(&cfg)
	if err != nil || !out.OK || out.Preview == "" || len(e.prn.jobs) != 1 {
		t.Fatalf("kurulumda test fişi basılmadı: %+v %v", out, err)
	}
	e.setup()
	if _, err := e.svc.TestPrint(nil); code(err) != "locked" {
		t.Fatal("kurulumdan sonra şifresiz test fişi basıldı")
	}
}

// Basıldıktan sonra eklenen ürün şifresiz azaltılabilir; basılan adetin altı şifre ister.
// Baskıdan sonra “Geri Al” baskı öncesine dönmez.
func TestPrintedQuantitiesNeedPinToReduce(t *testing.T) {
	e := newEnv(t)
	e.setup()
	no := e.add(6, "Cağ")
	e.add(6, "Cağ")
	if out, _, err := e.svc.Print(no); err != nil || !out.OK {
		t.Fatalf("baskı: %v", err)
	}
	if e.state().UndoCounts[no] != 0 {
		t.Fatal("baskıdan sonra geri al baskı öncesine dönebiliyor")
	}
	e.add(6, "Cağ") // basıldıktan sonra 3. şiş
	line := e.adisyon(no).Lines[0]
	if line.Qty != 3 || line.PrintedQty != 2 {
		t.Fatalf("basılan adet yanlış: %+v", line)
	}
	if _, err := e.svc.ChangeQty(no, line.ID, -1, ""); err != nil {
		t.Fatalf("basılmamış fazlalık şifresiz düşülemedi: %v", err)
	}
	if _, err := e.svc.ChangeQty(no, line.ID, -1, ""); code(err) != "pin_required" {
		t.Fatalf("basılan adetin altına şifresiz inildi: %v", err)
	}
	if _, err := e.svc.ChangeQty(no, line.ID, 1, ""); err != nil {
		t.Fatalf("artırmak şifre istememeli: %v", err)
	}
	// Geri al, basılan adetleri korur.
	if _, err := e.svc.Undo(no); err != nil {
		t.Fatal(err)
	}
	if l := e.adisyon(no).Lines[0]; l.PrintedQty != 2 {
		t.Fatalf("geri alınca basılan adet kayboldu: %+v", l)
	}
}

// Basılırken o adisyona ürün eklenemez ve hesap kapatılamaz (basılan ile kapanan
// tutar farklı olmasın); başka masalar çalışmaya devam eder.
func TestNoChangesWhilePrinting(t *testing.T) {
	e := newEnv(t)
	e.setup()
	no := e.add(8, "Şiş")
	e.prn.mu.Lock()
	e.prn.block = make(chan struct{})
	e.prn.mu.Unlock()
	done := make(chan *PrintOutcome)
	go func() {
		out, _, _ := e.svc.Print(no)
		done <- out
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		e.svc.mu.Lock()
		busy := e.svc.inPrint[no]
		e.svc.mu.Unlock()
		if busy || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, _, err := e.svc.AddItem(8, e.productID("Ayran"), 0); code(err) != "busy" {
		t.Fatalf("baskı sırasında ürün eklendi: %v", err)
	}
	line := e.adisyon(no).Lines[0].ID
	if _, err := e.svc.ChangeQty(no, line, 1, ""); code(err) != "busy" {
		t.Fatalf("baskı sırasında adet değişti: %v", err)
	}
	if _, _, err := e.svc.Pay(no, PayCash); code(err) != "busy" {
		t.Fatalf("baskı sırasında hesap kapandı: %v", err)
	}
	if _, _, err := e.svc.MoveTable(no, 12); code(err) != "busy" {
		t.Fatalf("baskı sırasında masa taşındı: %v", err)
	}
	e.add(9, "Ayran") // başka masa çalışır
	close(e.prn.block)
	if out := <-done; out == nil || !out.OK {
		t.Fatalf("baskı: %+v", out)
	}
	e.prn.mu.Lock()
	e.prn.block = nil
	e.prn.mu.Unlock()
	pay, _, err := e.svc.Pay(no, PayCash)
	if err != nil || !pay.Paid || pay.Total != 30000 {
		t.Fatalf("hesap yanlış kapandı: %+v %v", pay, err)
	}
	if a := e.adisyon(no); a.Total != 30000 || a.Status != StatusPaid || a.PaymentMethod != PayCash {
		t.Fatalf("kayıtlı tutar basılandan farklı: %+v", a)
	}
}

func TestResetPinClearsLockout(t *testing.T) {
	e := newEnv(t)
	e.setup()
	for i := 0; i < 5; i++ {
		e.svc.Unlock("1111")
	}
	if _, err := e.svc.Unlock("2468"); code(err) != "pin_blocked" {
		t.Fatal("bekleme yok")
	}
	if err := e.svc.ResetPin("1234"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Unlock("1234"); err != nil {
		t.Fatalf("sıfırlanan şifre bekleme yüzünden çalışmadı: %v", err)
	}
}
