package core

import (
	"strings"
	"testing"
	"time"
)

func (e *env) at(day, hour, min int) {
	e.t.Helper()
	e.clk.mu.Lock()
	e.clk.t = time.Date(2026, 10, day, hour, min, 0, 0, time.Local)
	e.clk.mu.Unlock()
	e.unlock()
}

func (e *env) sale(table int) int64 {
	e.t.Helper()
	no := e.add(table, "Cağ")
	e.pay(no, "nakit")
	return no
}

func (e *env) closeDay(date string) {
	e.t.Helper()
	if _, _, err := e.svc.CloseDay(date); err != nil {
		e.t.Fatalf("gün kapatılamadı %s: %v", date, err)
	}
}

func (e *env) dayOf(no int64) string { return e.adisyon(no).BusinessDate }

// Gün öğlen yanlışlıkla kapatılır, sonraki günlerde de gün sabah kapatılınca
// her gün bir gün kayar. Düzeltme adisyonları açıldıkları güne taşır, geçmiş
// günleri yeniden hesaplayıp kapalı tutar, bugünü yeniden açar.
func TestFixShiftedDays(t *testing.T) {
	e := newEnv(t)
	e.setup()

	e.at(19, 9, 0)
	a1 := e.sale(1)
	e.at(19, 12, 0)
	e.closeDay("2026-10-19") // öğlen, yanlışlıkla
	e.at(19, 14, 0)
	a2 := e.sale(2) // 20 Ekim'e yazılır
	if d := e.dayOf(a2); d != "2026-10-20" {
		t.Fatalf("kapalı günden sonra adisyon %s'e yazıldı", d)
	}
	if st := e.state(); st.BusinessDate != "2026-10-20" || st.ClockDate != "2026-10-19" {
		t.Fatalf("durum: iş günü %s, saat günü %s", st.BusinessDate, st.ClockDate)
	}
	e.at(20, 0, 30)
	a3 := e.sale(3) // gece yarısını geçen servis: hâlâ 19 Ekim'in, 20'ye yazılır
	e.at(20, 10, 0)
	e.closeDay("2026-10-20") // sabah "dünü" kapattığını sanıyor
	e.at(20, 13, 0)
	b1 := e.sale(1) // 21'e
	e.at(21, 11, 0)
	e.closeDay("2026-10-21")
	e.at(21, 13, 0)
	c1 := e.sale(1) // 22'ye
	e.at(21, 23, 0)
	c2 := e.add(4, "Cağ") // masada açık, 22'ye
	if e.dayOf(c1) != "2026-10-22" || e.dayOf(c2) != "2026-10-22" {
		t.Fatalf("kayma beklenen gibi değil: %s %s", e.dayOf(c1), e.dayOf(c2))
	}

	ds, err := e.svc.DaySummary("")
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Shifted) != 3 {
		t.Fatalf("kayma grubu: %+v", ds.Shifted)
	}
	g := ds.Shifted[0]
	if g.From != "2026-10-20" || g.To != "2026-10-19" || g.Count != 2 || g.Paid != 2 || !g.ToClosed || !g.FromClosed {
		t.Fatalf("ilk grup: %+v", g)
	}
	if last := ds.Shifted[2]; last.From != "2026-10-22" || last.To != "2026-10-21" || last.Active != 1 || last.Count != 2 {
		t.Fatalf("son grup: %+v", last)
	}

	pairs := []ShiftPair{}
	for _, g := range ds.Shifted {
		pairs = append(pairs, ShiftPair{g.From, g.To})
	}
	if _, _, err := e.svc.FixShifted(pairs, "Gün öğlen kapatıldı", ""); code(err) != "pin_required" {
		t.Fatalf("şifresiz düzeltildi: %v", err)
	}
	if _, _, err := e.svc.FixShifted(pairs, "", "2468"); code(err) != "invalid" {
		t.Fatalf("sebepsiz düzeltildi: %v", err)
	}
	res, st, err := e.svc.FixShifted(pairs, "Gün öğlen kapatıldı", "2468")
	if err != nil {
		t.Fatal(err)
	}
	if res.Moved != 5 || strings.Join(res.Reclosed, ",") != "2026-10-19,2026-10-20" || strings.Join(res.Reopened, ",") != "2026-10-21" {
		t.Fatalf("sonuç: %+v", res)
	}
	for no, want := range map[int64]string{a1: "2026-10-19", a2: "2026-10-19", a3: "2026-10-19", b1: "2026-10-20", c1: "2026-10-21", c2: "2026-10-21"} {
		if d := e.dayOf(no); d != want {
			t.Fatalf("No %d: %s, beklenen %s", no, d, want)
		}
	}
	if st.BusinessDate != "2026-10-21" {
		t.Fatalf("düzeltmeden sonra iş günü %s", st.BusinessDate)
	}
	d19, err := e.svc.DaySummary("2026-10-19")
	if err != nil {
		t.Fatal(err)
	}
	if !d19.Closed || !d19.Corrected || d19.Paid != 3 || d19.Revenue != 3*34000 || !d19.NoMissing || len(d19.Shifted) != 0 {
		t.Fatalf("19 Ekim: kapalı=%v düzeltildi=%v ödenen=%d ciro=%d eksiksiz=%v", d19.Closed, d19.Corrected, d19.Paid, d19.Revenue, d19.NoMissing)
	}
	d20, _ := e.svc.DaySummary("2026-10-20")
	if !d20.Closed || d20.Paid != 1 || !d20.NoMissing {
		t.Fatalf("20 Ekim: %+v", d20)
	}
	d21, _ := e.svc.DaySummary("2026-10-21")
	if d21.Closed || d21.Paid != 1 || d21.Open != 1 {
		t.Fatalf("21 Ekim açık olmalı: kapalı=%v ödenen=%d açık=%d", d21.Closed, d21.Paid, d21.Open)
	}
	// Yeni adisyon bugüne yazılır; masadaki adisyon kapanabilir, gün kapatılabilir.
	if n := e.sale(5); e.dayOf(n) != "2026-10-21" {
		t.Fatalf("yeni adisyon %s'e yazıldı", e.dayOf(n))
	}
	e.pay(c2, "kart")
	e.closeDay("2026-10-21")
	// İşlem kaydı: düzeltme ve her adisyonun gün değişikliği.
	log, err := e.svc.AuditLog(AuditQuery{From: "2026-10-01", To: "2026-10-30"})
	if err != nil {
		t.Fatal(err)
	}
	var fixed, moved int
	for _, r := range log.Rows {
		switch r.Action {
		case "gun_duzeltildi":
			fixed++
			if !strings.Contains(r.Text, "Gün öğlen kapatıldı") || !strings.Contains(r.Text, "21 Ekim 2026") {
				t.Fatalf("kayıt metni: %s", r.Text)
			}
		case "is_gunu_degisti":
			moved++
		}
	}
	if fixed != 1 || moved != 5 {
		t.Fatalf("işlem kaydı: düzeltme %d, taşınan %d", fixed, moved)
	}
}

// Kapatılmış gün yönetici şifresi ve sebeple yeniden açılır; bugün açılırsa
// yeni adisyonlar yine bugüne yazılır.
func TestReopenDay(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.at(19, 9, 0)
	a1 := e.sale(1)
	e.at(19, 12, 0)
	e.closeDay("2026-10-19")
	if _, _, err := e.svc.ReopenDay("2026-10-19", "yanlışlıkla kapatıldı", ""); code(err) != "pin_required" {
		t.Fatalf("şifresiz açıldı: %v", err)
	}
	if _, _, err := e.svc.ReopenDay("2026-10-19", " ", "2468"); code(err) != "invalid" {
		t.Fatalf("sebepsiz açıldı: %v", err)
	}
	if _, _, err := e.svc.ReopenDay("2026-10-18", "x", "2468"); code(err) != "invalid" {
		t.Fatalf("açık gün yeniden açıldı: %v", err)
	}
	ds, st, err := e.svc.ReopenDay("2026-10-19", "yanlışlıkla kapatıldı", "2468")
	if err != nil {
		t.Fatal(err)
	}
	if ds.Closed || ds.Paid != 1 || st.BusinessDate != "2026-10-19" {
		t.Fatalf("yeniden açılmadı: kapalı=%v iş günü=%s", ds.Closed, st.BusinessDate)
	}
	a2 := e.sale(2)
	if e.dayOf(a2) != "2026-10-19" {
		t.Fatalf("yeni adisyon %s'e yazıldı", e.dayOf(a2))
	}
	// Geçmiş gün açılınca yeni adisyonlar bugüne yazılmaya devam eder; ödenmiş
	// adisyon düzeltilebilir.
	e.at(19, 23, 30)
	e.closeDay("2026-10-19")
	e.at(20, 12, 0)
	if _, _, err := e.svc.Reopen(a1, 0, "adet yanlış", "2468"); code(err) != "closed" {
		t.Fatalf("kapalı günün adisyonu açıldı: %v", err)
	}
	if _, _, err := e.svc.ReopenDay("2026-10-19", "adet düzeltmesi", "2468"); err != nil {
		t.Fatal(err)
	}
	st = e.state()
	if st.BusinessDate != "2026-10-20" || len(st.UnclosedDays) != 1 || st.UnclosedDays[0] != "2026-10-19" {
		t.Fatalf("geçmiş gün açılınca: iş günü %s, kapatılmamış %v", st.BusinessDate, st.UnclosedDays)
	}
	if _, _, err := e.svc.Reopen(a1, 0, "adet yanlış", "2468"); err != nil {
		t.Fatalf("açılan günün adisyonu düzeltilemedi: %v", err)
	}
	e.pay(a1, "kart")
	e.closeDay("2026-10-19")
}

// Gece gün sonundan sonra bilerek ertesi güne yazılan adisyon "böyle kalsın"
// denince uyarıdan kalkar.
func TestIgnoreShift(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.at(19, 20, 0)
	e.sale(1)
	e.at(19, 23, 0)
	e.closeDay("2026-10-19")
	e.at(19, 23, 30)
	e.sale(2)
	ds, _ := e.svc.DaySummary("")
	if len(ds.Shifted) != 1 {
		t.Fatalf("kayma: %+v", ds.Shifted)
	}
	if err := e.svc.IgnoreShift(ShiftPair{From: "2026-10-20", To: "2026-10-19"}); err != nil {
		t.Fatal(err)
	}
	ds, _ = e.svc.DaySummary("")
	if len(ds.Shifted) != 0 {
		t.Fatalf("yoksayılan kayma hâlâ listede: %+v", ds.Shifted)
	}
}
