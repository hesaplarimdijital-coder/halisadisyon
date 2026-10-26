package core

import (
	"fmt"
	"image"
	"strings"
	"time"

	"halisadisyon/internal/receipt"
)

// dayReport gün özetini gün sonu fişine çevirir.
func (s *Service) dayReport(ds *DaySummary) receipt.DayReport {
	r := receipt.DayReport{
		Printed: s.now(), Closed: ds.Closed, Corrected: ds.Corrected, FirstNo: ds.FirstNo, LastNo: ds.LastNo,
		Paid: ds.Paid, Cancelled: ds.Cancelled, CancelledTotal: ds.CancelledTotal, Merged: ds.Merged, Open: ds.Open,
		Discounts: ds.Discounts, DiscountTotal: ds.DiscountTotal, Comps: ds.Comps, CompValue: ds.CompValue,
		Revenue: ds.Revenue, Avg: ds.Avg,
	}
	if t, err := parseDate(ds.Date); err == nil {
		r.Date = t
	}
	if t, err := time.Parse(time.RFC3339, ds.ClosedAt); err == nil {
		r.ClosedAt = t.Local()
	}
	for _, v := range ds.VATs {
		r.VATs = append(r.VATs, receipt.VATGroup{Rate: v.Rate, Gross: v.Gross, VAT: v.VAT})
	}
	for _, p := range ds.ByPayment {
		if p.Count > 0 || p.Method != "" {
			r.Payments = append(r.Payments, receipt.NameAmount{Name: p.Label, Count: p.Count, Total: p.Total})
		}
	}
	r.Split = ds.Split
	for _, v := range ds.CreditGiven {
		r.CreditGiven = append(r.CreditGiven, receipt.NameAmount{Name: v.Name, Count: int(v.No), Total: v.Amount})
	}
	for _, v := range ds.Collections {
		r.Collections = append(r.Collections, receipt.NameAmount{Name: v.Name + " · " + v.Label, Total: v.Amount})
	}
	for _, p := range ds.Drawer {
		r.Drawer = append(r.Drawer, receipt.NameAmount{Name: p.Label, Total: p.Total})
	}
	r.DrawerTotal = ds.DrawerTotal
	for _, w := range ds.ByWaiter {
		r.Waiters = append(r.Waiters, receipt.NameAmount{Name: w.Name, Count: w.Count, Total: w.Total})
	}
	for _, p := range ds.Products {
		r.Products = append(r.Products, receipt.ProductLine{Name: p.Name, Unit: p.Unit, Qty: p.Qty, Total: p.Total, Comp: p.Comp})
	}
	// Aynı türden uyarılar tek satırda: "Basılmadan kapatılan 7 adisyon: No 12, 15, …"
	groups := map[string][]DayWarning{}
	for _, w := range ds.Warnings {
		if w.Kind == "unprinted" || w.Kind == "changed" {
			groups[w.Kind] = append(groups[w.Kind], w)
			continue
		}
		r.Warnings = append(r.Warnings, w.Text)
	}
	for _, k := range []string{"unprinted", "changed"} {
		ws := groups[k]
		if len(ws) == 1 {
			r.Warnings = append(r.Warnings, ws[0].Text)
		} else if len(ws) > 1 {
			nos := make([]string, len(ws))
			for i, w := range ws {
				nos[i] = fmt.Sprint(w.No)
			}
			title := "Basılmadan kapatılan %d adisyon: No %s"
			if k == "changed" {
				title = "Son hali basılmadan kapatılan %d adisyon: No %s"
			}
			r.Warnings = append(r.Warnings, fmt.Sprintf(title, len(ws), strings.Join(nos, ", ")))
		}
	}
	return r
}

func (s *Service) dayImage(date string) (*image.Gray, Settings, error) {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return nil, Settings{}, err
	}
	if !validDate(date) {
		s.mu.Unlock()
		return nil, Settings{}, uerr("invalid", "Tarih anlaşılamadı.")
	}
	ds, err := s.daySummary(s.db, date)
	set, logo := s.set, s.logoImg
	s.mu.Unlock()
	if err != nil {
		return nil, set, err
	}
	img, err := receipt.RenderDaySummary(receiptBusiness(set, logo), s.dayReport(ds), set.Printer.Width)
	return img, set, err
}

// DayReportPreview gün sonu fişinin önizlemesini verir.
func (s *Service) DayReportPreview(date string) (string, error) {
	img, _, err := s.dayImage(date)
	if err != nil {
		return "", err
	}
	return pngDataURL(img)
}

// PrintDayReport gün sonu fişini basar (gün kapatılmadıysa “ara rapor” yazar).
func (s *Service) PrintDayReport(date string) (*PrintOutcome, error) {
	img, set, err := s.dayImage(date)
	if err != nil {
		return nil, err
	}
	out := &PrintOutcome{}
	out.Result, out.Preview = s.send(set.Printer, img, "Gün sonu "+date)
	if !out.OK && out.Code != "" {
		out.Title = "Gün sonu fişi basılamadı"
	}
	s.mu.Lock()
	audit(s.db, s.now(), "gun_sonu_fisi", 0, map[string]any{"tarih": date, "ok": out.OK})
	s.mu.Unlock()
	return out, nil
}

// sampleAdisyon ayarlar ekranındaki canlı önizleme için örnek bir adisyondur:
// satıştaki ilk fiyatlı ürünlerden üç kalem.
func (s *Service) sampleAdisyon() (*Adisyon, error) {
	prods, err := loadProducts(s.db)
	if err != nil {
		return nil, err
	}
	a := &Adisyon{Table: 5, WaiterName: "Garson", PrintCount: 0}
	if ws, err := loadWaiters(s.db, true); err == nil && len(ws) > 0 {
		a.WaiterName = ws[0].Name
	}
	if n, err := nextNo(s.db); err == nil {
		a.No = n
	}
	qty := []int{2, 1, 2}
	for _, p := range prods {
		if !p.Active || p.Price <= 0 {
			continue
		}
		vat := s.set.DefaultVAT
		if p.VAT != nil {
			vat = *p.VAT
		}
		a.Lines = append(a.Lines, Line{Name: p.Name, Unit: p.Unit, Price: p.Price, VAT: vat, Qty: qty[len(a.Lines)]})
		if len(a.Lines) == 3 {
			break
		}
	}
	if len(a.Lines) == 0 {
		v := s.set.DefaultVAT
		a.Lines = []Line{{Name: "Cağ", Unit: "şiş", Price: 35000, VAT: v, Qty: 2}, {Name: "Ayran", Unit: "adet", Price: 4000, VAT: v, Qty: 2}}
	}
	return a, nil
}

// sampleImage örnek adisyonu verilen (henüz kaydedilmemiş olabilir) fiş bilgileriyle çizer.
func (s *Service) sampleImage(b *Business) (*image.Gray, Settings, error) {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return nil, Settings{}, err
	}
	set, logo := s.set, s.logoImg
	a, err := s.sampleAdisyon()
	s.mu.Unlock()
	if err != nil {
		return nil, set, err
	}
	if b != nil {
		cb, err := cleanBusiness(*b)
		if err != nil {
			return nil, set, err
		}
		set.Business = cb
	}
	img, err := receipt.RenderAdisyon(receiptBusiness(set, logo), s.receiptAdisyon(a, false), set.Printer.Width)
	return img, set, err
}

// SamplePreview ayarlar ekranındaki canlı fiş önizlemesidir; basılan fişle aynı çizilir.
func (s *Service) SamplePreview(b *Business) (string, error) {
	img, _, err := s.sampleImage(b)
	if err != nil {
		return "", err
	}
	return pngDataURL(img)
}

// SamplePrint örnek adisyonu yazıcıya basar (fiş düzenini kâğıtta görmek için).
func (s *Service) SamplePrint() (*PrintOutcome, error) {
	img, set, err := s.sampleImage(nil)
	if err != nil {
		return nil, err
	}
	out := &PrintOutcome{}
	out.Result, out.Preview = s.send(set.Printer, img, "Örnek adisyon")
	if out.Preview == "" {
		out.Preview, _ = pngDataURL(img)
	}
	if !out.OK && out.Code != "" {
		out.Title = "Örnek adisyon basılamadı"
	}
	s.mu.Lock()
	audit(s.db, s.now(), "test_fisi", 0, map[string]any{"ok": out.OK, "kod": out.Code, "baglanti": connectionText(set.Printer), "ornek": true})
	s.mu.Unlock()
	return out, nil
}
