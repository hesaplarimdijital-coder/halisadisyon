package receipt

import (
	"fmt"
	"image"
	"strings"
	"time"

	"halisadisyon/internal/money"
)

// NameAmount gün sonu fişindeki adlı bir toplamdır (ödeme şekli, garson).
type NameAmount struct {
	Name  string
	Count int
	Total int64
}

// ProductLine gün sonu fişindeki ürün satırıdır.
type ProductLine struct {
	Name, Unit string
	Qty        int
	Total      int64
}

// DayReport gün sonu fişidir. İşletme içi rapordur, müşteriye verilmez.
type DayReport struct {
	Date           time.Time // iş günü
	Printed        time.Time
	Closed         bool
	ClosedAt       time.Time
	FirstNo        int64
	LastNo         int64
	Paid           int
	Cancelled      int
	CancelledTotal int64
	Discounts      int   // indirimli ödenen adisyon sayısı
	DiscountTotal  int64 // yapılan indirim toplamı (kuruş)
	Merged         int
	Open           int
	Revenue        int64
	VATs           []VATGroup
	Avg            int64
	Payments       []NameAmount
	Waiters        []NameAmount
	Products       []ProductLine
	Warnings       []string
}

var gunler = []string{"Pazar", "Pazartesi", "Salı", "Çarşamba", "Perşembe", "Cuma", "Cumartesi"}
var aylar = []string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}

// DateLong "11 Ekim 2026 Pazar" biçimidir.
func DateLong(t time.Time) string {
	return fmt.Sprintf("%d %s %d %s", t.Day(), aylar[t.Month()-1], t.Year(), gunler[t.Weekday()])
}

// RenderDaySummary gün sonu özet fişini çizer.
func RenderDaySummary(b Business, d DayReport, width int) (*image.Gray, error) {
	renderMu.Lock()
	defer renderMu.Unlock()
	if err := loadFonts(); err != nil {
		return nil, err
	}
	sf := scale(width)
	c := newCanvas(width, int(12*sf))
	c.gap(int(8 * sf))
	if name := strings.TrimSpace(b.Name); name != "" {
		c.fitLine(trUpper(name), bold, 26*sf, center, int(2*sf), int(4*sf))
	}
	c.fitLine("GÜN SONU RAPORU", bold, 38*sf, center, int(3*sf), int(6*sf))
	c.fitLine(DateLong(d.Date), semibold, 27*sf, center, 0, int(6*sf))
	small := face(regular, 22*sf)
	if d.Closed {
		c.line("Gün kapatıldı: "+d.ClosedAt.Format("02.01.2006 15:04"), small, center, 0, int(2*sf))
	} else {
		c.gap(int(4 * sf))
		c.boxed("ARA RAPOR · GÜN KAPATILMADI", face(bold, 22*sf), int(4*sf), 3)
		c.gap(int(4 * sf))
	}
	c.line("Basım: "+d.Printed.Format("02.01.2006 15:04"), small, center, 0, 0)
	c.rule(true, 2, int(12*sf), int(10*sf))

	f := face(regular, 25*sf)
	fb := face(semibold, 25*sf)
	if d.FirstNo > 0 {
		c.row("Adisyon no", f, fmt.Sprintf("%d – %d", d.FirstNo, d.LastNo), fb, 0, int(3*sf))
	}
	c.row("Ödenen adisyon", f, fmt.Sprint(d.Paid), fb, 0, int(3*sf))
	cancel := fmt.Sprint(d.Cancelled)
	if d.Cancelled > 0 {
		cancel += " · " + money.Format(d.CancelledTotal)
	}
	c.row("İptal", f, cancel, fb, 0, int(3*sf))
	if d.Discounts > 0 {
		c.row("İndirimli adisyon", f, fmt.Sprintf("%d · −%s", d.Discounts, money.Format(d.DiscountTotal)), fb, 0, int(3*sf))
	}
	if d.Merged > 0 {
		c.row("Birleştirilen masa", f, fmt.Sprint(d.Merged), fb, 0, int(3*sf))
	}
	if d.Open > 0 {
		c.row("Açık masa", f, fmt.Sprint(d.Open), face(bold, 25*sf), 0, int(3*sf))
	}
	c.rule(true, 2, int(8*sf), int(12*sf))

	c.row("CİRO", face(bold, 32*sf), money.TL(d.Revenue), face(bold, 40*sf), 0, int(4*sf))
	c.line("KDV dahil, ödenen adisyonlar", small, left, 0, int(6*sf))
	var vatSum int64
	for _, v := range d.VATs {
		c.row(fmt.Sprintf("KDV %%%d (matrah %s)", v.Rate, money.Format(v.Gross-v.VAT)), small, money.Format(v.VAT), small, 0, int(2*sf))
		vatSum += v.VAT
	}
	c.row("KDV toplamı", small, money.Format(vatSum), small, 0, int(2*sf))
	c.row("Matrah", small, money.Format(d.Revenue-vatSum), small, 0, int(2*sf))
	if d.Paid > 0 {
		c.row("Ortalama adisyon", small, money.Format(d.Avg), small, 0, 0)
	}

	section := func(title string) {
		c.rule(true, 2, int(12*sf), int(10*sf))
		c.line(title, face(bold, 22*sf), left, int(3*sf), int(6*sf))
	}
	if len(d.Payments) > 0 {
		section("ÖDEMELER")
		for _, p := range d.Payments {
			c.row(fmt.Sprintf("%s (%d)", p.Name, p.Count), f, money.Format(p.Total), fb, 0, int(3*sf))
		}
	}
	if len(d.Waiters) > 0 {
		section("GARSONLAR")
		for _, w := range d.Waiters {
			c.row(fmt.Sprintf("%s · %d adisyon", w.Name, w.Count), f, money.Format(w.Total), fb, 0, int(3*sf))
		}
	}
	if len(d.Products) > 0 {
		section("SATILAN ÜRÜNLER")
		for _, p := range d.Products {
			c.row(fmt.Sprintf("%s · %d %s", p.Name, p.Qty, p.Unit), fb, money.Format(p.Total), f, 0, int(3*sf))
		}
	}
	if len(d.Warnings) > 0 {
		section("UYARILAR")
		for _, w := range d.Warnings {
			c.para("• "+w, face(regular, 22*sf), left, int(3*sf))
		}
	}
	c.rule(false, 3, int(14*sf), int(12*sf))
	c.fitLine("İŞLETME İÇİ RAPORDUR · MALİ DEĞERİ YOKTUR", bold, 20*sf, center, int(2*sf), 0)
	return c.finish(int(24 * sf)), nil
}
