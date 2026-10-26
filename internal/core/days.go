package core

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"halisadisyon/internal/money"
)

// İş günü kuralı: saat 05:00'ten önce açılan adisyon bir önceki güne yazılır
// (gece yarısını geçen servis aynı güne sayılır). Gün sonu alınıp kapatılan
// güne yeni adisyon yazılmaz; o durumda adisyon ilk kapatılmamış sonraki güne
// yazılır. Gün sonu unutulsa bile ertesi günün adisyonları ayrı güne düşer.

const dateLayout = "2006-01-02"

func parseDate(d string) (time.Time, error) { return time.ParseInLocation(dateLayout, d, time.Local) }

func nextDate(d string) string {
	t, err := parseDate(d)
	if err != nil {
		return d
	}
	return t.AddDate(0, 0, 1).Format(dateLayout)
}

func validDate(d string) bool {
	_, err := parseDate(d)
	return err == nil && len(d) == 10
}

// dayClosed iş gününün kapatılıp kapatılmadığını ve kapanış zamanını verir.
func dayClosed(q querier, date string) (bool, string, error) {
	var closedAt sql.NullString
	err := q.QueryRow(`SELECT closed_at FROM business_days WHERE date=?`, date).Scan(&closedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return closedAt.Valid && closedAt.String != "", closedAt.String, nil
}

// openBusinessDay yeni adisyonun yazılacağı iş günüdür.
func openBusinessDay(q querier, now time.Time) (string, error) {
	d := businessDate(now)
	for i := 0; i < 400; i++ {
		closed, _, err := dayClosed(q, d)
		if err != nil {
			return "", err
		}
		if !closed {
			return d, nil
		}
		d = nextDate(d)
	}
	return d, nil
}

// unclosedDays bugünden önceki, adisyonu olan ama gün sonu alınmamış günlerdir (yeniden eskiye).
func unclosedDays(q querier, before string) ([]string, error) {
	rows, err := q.Query(`SELECT DISTINCT business_date FROM adisyons
		WHERE business_date < ? AND business_date NOT IN (SELECT date FROM business_days WHERE closed_at IS NOT NULL)
		ORDER BY business_date DESC LIMIT 14`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// PayTotal bir ödeme şeklinin toplamıdır.
type PayTotal struct {
	Method string `json:"method"`
	Label  string `json:"label"`
	Count  int    `json:"count"`
	Total  int64  `json:"total"`
}

// WaiterTotal bir garsonun kapattığı adisyonlardır.
type WaiterTotal struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Total int64  `json:"total"`
}

// ProductQty bir ürünün satış adedi ve tutarıdır.
type ProductQty struct {
	ProductID int64  `json:"productId"`
	Name      string `json:"name"`
	Unit      string `json:"unit"`
	Qty       int    `json:"qty"`            // satılan adet (ikram hariç)
	Total     int64  `json:"total"`          // satılan tutar
	Comp      int    `json:"comp,omitempty"` // ikram adedi
}

// DayRow gün sonu listesindeki bir adisyondur.
type DayRow struct {
	No           int64  `json:"no"`
	Table        int    `json:"table"`
	Waiter       string `json:"waiter"`
	OpenedAt     string `json:"openedAt"`
	ClosedAt     string `json:"closedAt,omitempty"`
	Status       string `json:"status"`
	StatusText   string `json:"statusText"`
	Payment      string `json:"payment,omitempty"`
	PaymentLabel string `json:"paymentLabel,omitempty"`
	Total        int64  `json:"total"`
	PrintCount   int    `json:"printCount"`
	Changed      bool   `json:"changed"`
	MergedInto   int64  `json:"mergedInto,omitempty"`
	CancelReason string `json:"cancelReason,omitempty"`
	// DiscountPercent ve Discount: adisyona uygulanan yüzde indirim ve tutarı.
	DiscountPercent int   `json:"discountPercent,omitempty"`
	Discount        int64 `json:"discount,omitempty"`
	// Payments ödeme parçaları; Customer veresiye müşterisi.
	Payments []PaymentPart `json:"payments,omitempty"`
	Customer string        `json:"customer,omitempty"`
}

// CreditLine gün sonundaki bir veresiye adisyondur.
type CreditLine struct {
	No     int64  `json:"no"`
	Name   string `json:"name"`
	Amount int64  `json:"amount"` // veresiye kısmı
	Total  int64  `json:"total"`  // adisyonun tamamı
}

// CreditCollection gün sonundaki bir veresiye tahsilatıdır.
type CreditCollection struct {
	ID         int64  `json:"id"`
	CustomerID int64  `json:"customerId"`
	Name       string `json:"name"`
	Method     string `json:"method"`
	Label      string `json:"label"`
	Amount     int64  `json:"amount"`
	At         string `json:"at"`
	Note       string `json:"note,omitempty"`
}

func dayCollections(q querier, date string) ([]CreditCollection, error) {
	rows, err := q.Query(`SELECT cp.id, c.id, c.name, cp.method, cp.amount, cp.at, cp.note FROM credit_payments cp
		JOIN credit_customers c ON c.id=cp.customer_id WHERE cp.business_date=? AND cp.cancelled_at IS NULL ORDER BY cp.at, cp.id`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CreditCollection{}
	for rows.Next() {
		var c CreditCollection
		if err := rows.Scan(&c.ID, &c.CustomerID, &c.Name, &c.Method, &c.Amount, &c.At, &c.Note); err != nil {
			return nil, err
		}
		c.Label = PaymentLabel(c.Method)
		out = append(out, c)
	}
	return out, rows.Err()
}

// DayWarning gün sonu uyarısıdır. Level: "bad" (kırmızı), "warn" (sarı).
// Kind: "gap" numara atlaması, "unprinted" basılmadan kapatılan, "changed" son
// hali basılmadan kapatılan, "printed_cancel" basıldıktan sonra iptal, "open" açık masa.
type DayWarning struct {
	Level string `json:"level"`
	Kind  string `json:"kind"`
	Text  string `json:"text"`
	No    int64  `json:"no,omitempty"`
}

// DaySummary bir iş gününün özetidir (gün sonu ekranı, fişi ve gönderimler).
type DaySummary struct {
	Date           string        `json:"date"`
	Closed         bool          `json:"closed"`
	ClosedAt       string        `json:"closedAt,omitempty"`
	Migrated       bool          `json:"migrated,omitempty"` // 1.0.0 verisinden: kapatılmış sayıldı
	FirstNo        int64         `json:"firstNo"`
	LastNo         int64         `json:"lastNo"`
	Paid           int           `json:"paid"`
	Cancelled      int           `json:"cancelled"`
	Merged         int           `json:"merged"`
	Open           int           `json:"open"`
	Revenue        int64         `json:"revenue"`
	VAT            int64         `json:"vat"`
	VATs           []VATGroup    `json:"vats"` // KDV oranlarına göre
	Avg            int64         `json:"avg"`
	CancelledTotal int64         `json:"cancelledTotal"`
	Discounts      int           `json:"discounts"`     // indirimli ödenen adisyon
	DiscountTotal  int64         `json:"discountTotal"` // yapılan indirim toplamı
	Comps          int           `json:"comps"`         // ikram edilen ürün adedi
	CompValue      int64         `json:"compValue"`     // ikramların liste fiyatıyla değeri
	ByPayment      []PayTotal    `json:"byPayment"`
	ByWaiter       []WaiterTotal `json:"byWaiter"`
	Products       []ProductQty  `json:"products"`
	Featured       *ProductQty   `json:"featured,omitempty"` // öne çıkan ürün (Cağ)
	Rows           []DayRow      `json:"rows"`
	Warnings       []DayWarning  `json:"warnings"`
	NoMissing      bool          `json:"noMissing"` // numara atlaması yok
	Unclosed       []string      `json:"unclosed"`  // gün sonu alınmamış önceki günler
	// Split parçalı ödenen adisyon sayısı.
	Split int `json:"split"`
	// CreditGiven bugün veresiyeye yazılanlar; Collections bugün alınan veresiye
	// tahsilatları (ciroya girmez); Drawer kasaya giren para (satış + tahsilat;
	// veresiye hariç), şekle göre.
	CreditGiven      []CreditLine       `json:"creditGiven"`
	CreditGivenTotal int64              `json:"creditGivenTotal"`
	Collections      []CreditCollection `json:"collections"`
	CollectedTotal   int64              `json:"collectedTotal"`
	Drawer           []PayTotal         `json:"drawer"`
	DrawerTotal      int64              `json:"drawerTotal"`
	// Corrected: kapandıktan sonra kaymış adisyonlar taşınıp özet yeniden hesaplandı.
	Corrected bool `json:"corrected,omitempty"`
	// Shifted: açıldığı günden sonraki bir güne yazılmış adisyon grupları (gün erken kapatıldıysa).
	Shifted []ShiftGroup `json:"shifted"`
}

func statusText(a *Adisyon) string {
	switch a.Status {
	case StatusPaid:
		return "Ödendi"
	case StatusCancelled:
		if a.MergedInto > 0 {
			return fmt.Sprintf("Birleşti → No %d", a.MergedInto)
		}
		return "İptal"
	case StatusPrinted:
		return "Açık (basıldı)"
	default:
		return "Açık"
	}
}

func loadDayAdisyons(q querier, date string) ([]Adisyon, error) {
	rows, err := q.Query(`SELECT `+adisyonCols+` FROM adisyons WHERE business_date=? ORDER BY no`, date)
	if err != nil {
		return nil, err
	}
	var out []Adisyon
	for rows.Next() {
		a, err := scanAdisyon(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := attachPayments(q, out, `a.business_date=?`, date); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Payments = partsOf(&out[i])
	}
	return out, nil
}

// featuredProduct öne çıkan ürünü (istatistikte "satılan cağ") verir: ayarlarda
// seçilen ürün; seçilmemişse adı "Cağ" olan ürün.
func (s *Service) featuredProduct(q querier) (int64, error) {
	if s.set.FeaturedProductID > 0 {
		return s.set.FeaturedProductID, nil
	}
	var id int64
	err := q.QueryRow(`SELECT id FROM products WHERE lower(name) IN ('cağ','cag','cağ kebabı','cağ kebap') ORDER BY id LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// daySummary bir iş gününün özetini veritabanından hesaplar.
func (s *Service) daySummary(q querier, date string) (*DaySummary, error) {
	ds := &DaySummary{Date: date, ByPayment: []PayTotal{}, ByWaiter: []WaiterTotal{}, Products: []ProductQty{}, Rows: []DayRow{},
		Warnings: []DayWarning{}, VATs: []VATGroup{}, Shifted: []ShiftGroup{},
		CreditGiven: []CreditLine{}, Collections: []CreditCollection{}, Drawer: []PayTotal{}}
	closed, closedAt, err := dayClosed(q, date)
	if err != nil {
		return nil, err
	}
	ds.Closed, ds.ClosedAt = closed, closedAt
	if closed {
		var summary sql.NullString
		_ = q.QueryRow(`SELECT summary FROM business_days WHERE date=?`, date).Scan(&summary)
		ds.Migrated = summary.String == `{"gecis":true}`
		var meta struct {
			Corrected bool `json:"corrected"`
		}
		if json.Unmarshal([]byte(summary.String), &meta) == nil {
			ds.Corrected = meta.Corrected
		}
	}
	list, err := loadDayAdisyons(q, date)
	if err != nil {
		return nil, err
	}
	pay := map[string]*PayTotal{}
	waiters := map[string]*WaiterTotal{}
	var waiterOrder []string
	have := map[int64]bool{}
	for i := range list {
		a := &list[i]
		have[a.No] = true
		if ds.FirstNo == 0 || a.No < ds.FirstNo {
			ds.FirstNo = a.No
		}
		if a.No > ds.LastNo {
			ds.LastNo = a.No
		}
		row := DayRow{No: a.No, Table: a.Table, Waiter: a.WaiterName, OpenedAt: a.OpenedAt, ClosedAt: a.ClosedAt,
			Status: a.Status, StatusText: statusText(a), Payment: a.PaymentMethod, PaymentLabel: PaymentText(a.Payments, a.PaymentMethod),
			Payments: a.Payments, Customer: a.CreditCustomer,
			Total: a.Total, PrintCount: a.PrintCount, Changed: a.Changed, MergedInto: a.MergedInto, CancelReason: a.CancelReason,
			DiscountPercent: a.DiscountPercent, Discount: a.DiscountAmount}
		ds.Rows = append(ds.Rows, row)
		switch a.Status {
		case StatusPaid:
			ds.Paid++
			ds.Revenue += a.Total
			ds.VAT += a.VATTotal
			if a.DiscountAmount > 0 {
				ds.Discounts++
				ds.DiscountTotal += a.DiscountAmount
			}
			parts := a.Payments
			if len(parts) == 0 { // 1.0.0'dan kalan, ödeme şekli olmayan kayıt
				parts = []PaymentPart{{Method: "", Amount: a.Total}}
			}
			for _, pp := range parts {
				p := pay[pp.Method]
				if p == nil {
					p = &PayTotal{Method: pp.Method, Label: PaymentLabel(pp.Method)}
					if p.Label == "" {
						p.Label = "Belirtilmemiş"
					}
					pay[pp.Method] = p
				}
				p.Count++
				p.Total += pp.Amount
				if pp.Method == PayCredit {
					ds.CreditGiven = append(ds.CreditGiven, CreditLine{No: a.No, Name: a.CreditCustomer, Amount: pp.Amount, Total: a.Total})
					ds.CreditGivenTotal += pp.Amount
				}
			}
			if len(parts) > 1 {
				ds.Split++
			}
			w := waiters[a.WaiterName]
			if w == nil {
				w = &WaiterTotal{Name: a.WaiterName}
				waiters[a.WaiterName] = w
				waiterOrder = append(waiterOrder, a.WaiterName)
			}
			w.Count++
			w.Total += a.Total
			if a.PrintCount == 0 {
				ds.Warnings = append(ds.Warnings, DayWarning{Level: "warn", Kind: "unprinted", Text: fmt.Sprintf("No %d (Masa %d) basılmadan kapatıldı.", a.No, a.Table), No: a.No})
			} else if a.Changed {
				ds.Warnings = append(ds.Warnings, DayWarning{Level: "warn", Kind: "changed", Text: fmt.Sprintf("No %d (Masa %d) basıldıktan sonra değişti; son hali basılmadan kapatıldı.", a.No, a.Table), No: a.No})
			}
		case StatusCancelled:
			if a.MergedInto > 0 {
				ds.Merged++
				continue
			}
			ds.Cancelled++
			// İptal edilen adisyonun tutarı: kalemler duruyorsa onların toplamı.
			ds.CancelledTotal += a.Total
			if a.PrintCount > 0 {
				ds.Warnings = append(ds.Warnings, DayWarning{Level: "warn", Kind: "printed_cancel", Text: fmt.Sprintf("No %d (Masa %d) basıldıktan sonra iptal edildi: %s", a.No, a.Table, a.CancelReason), No: a.No})
			}
		default:
			ds.Open++
			ds.Warnings = append(ds.Warnings, DayWarning{Level: "warn", Kind: "open", Text: fmt.Sprintf("Masa %d (No %d) hâlâ açık.", a.Table, a.No), No: a.No})
		}
	}
	if ds.Paid > 0 {
		ds.Avg = (ds.Revenue + int64(ds.Paid)/2) / int64(ds.Paid)
	}
	for _, m := range PaymentMethods {
		if p := pay[m.ID]; p != nil {
			ds.ByPayment = append(ds.ByPayment, *p)
		} else if m.ID != PayCredit { // veresiye yalnız varsa gösterilir
			ds.ByPayment = append(ds.ByPayment, PayTotal{Method: m.ID, Label: m.Label})
		}
	}
	if p := pay[""]; p != nil {
		ds.ByPayment = append(ds.ByPayment, *p)
	}
	// Veresiye tahsilatları ve kasaya giren para (satış + tahsilat).
	if ds.Collections, err = dayCollections(q, date); err != nil {
		return nil, err
	}
	coll := map[string]int64{}
	for _, c := range ds.Collections {
		ds.CollectedTotal += c.Amount
		coll[c.Method] += c.Amount
	}
	for _, m := range CollectMethods {
		d := PayTotal{Method: m.ID, Label: m.Label, Total: coll[m.ID]}
		if p := pay[m.ID]; p != nil {
			d.Total += p.Total
		}
		ds.Drawer = append(ds.Drawer, d)
		ds.DrawerTotal += d.Total
	}
	for _, n := range waiterOrder {
		ds.ByWaiter = append(ds.ByWaiter, *waiters[n])
	}
	sort.SliceStable(ds.ByWaiter, func(i, j int) bool { return ds.ByWaiter[i].Total > ds.ByWaiter[j].Total })

	// KDV oranlarına göre: her adisyonun KDV'si kendi içinde hesaplandığı gibi toplanır.
	if ds.VATs, err = vatTotals(q, `a.business_date=?`, date); err != nil {
		return nil, err
	}
	if ds.Comps, ds.CompValue, err = compTotals(q, `a.business_date=?`, date); err != nil {
		return nil, err
	}

	// Ürün adetleri (yalnız ödenen adisyonlar)
	prods, err := productTotals(q, `a.business_date=?`, date)
	if err != nil {
		return nil, err
	}
	ds.Products = prods
	if fid, err := s.featuredProduct(q); err == nil && fid > 0 {
		for i := range prods {
			if prods[i].ProductID == fid {
				f := prods[i]
				ds.Featured = &f
			}
		}
		if ds.Featured == nil {
			if p, err := loadProduct(q, fid); err == nil {
				ds.Featured = &ProductQty{ProductID: p.ID, Name: p.Name, Unit: p.Unit}
			}
		}
	}

	// Numara atlaması: önceki günlerin son numarasından bu günün son numarasına
	// kadar her numara bu güne yazılmış olmalı.
	ds.NoMissing = true
	if ds.LastNo > 0 {
		var prevMax int64
		if err := q.QueryRow(`SELECT COALESCE(MAX(no),0) FROM adisyons WHERE business_date < ?`, date).Scan(&prevMax); err != nil {
			return nil, err
		}
		var missing []int64
		for n := prevMax + 1; n < ds.LastNo && len(missing) < 50; n++ {
			if have[n] {
				continue
			}
			var other string
			err := q.QueryRow(`SELECT business_date FROM adisyons WHERE no=?`, n).Scan(&other)
			if errors.Is(err, sql.ErrNoRows) {
				missing = append(missing, n)
			} else if err != nil {
				return nil, err
			}
		}
		if len(missing) > 0 {
			ds.NoMissing = false
			parts := make([]string, len(missing))
			for i, n := range missing {
				parts[i] = fmt.Sprint(n)
			}
			ds.Warnings = append([]DayWarning{{Level: "bad", Kind: "gap", Text: "Numara atlaması: No " + strings.Join(parts, ", ") + " kayıtlarda yok."}}, ds.Warnings...)
		}
	}
	return ds, nil
}

// vatTotals ödenen adisyonların KDV'sini orana göre toplar. Her adisyonun KDV'si
// fişte olduğu gibi (adisyon içinde, oran başına, indirim düşülerek) hesaplanır.
func vatTotals(q querier, where string, args ...any) ([]VATGroup, error) {
	rows, err := q.Query(`SELECT l.vat, a.discount_percent, SUM(l.qty*l.price) FROM adisyon_lines l JOIN adisyons a ON a.no=l.adisyon_no
		WHERE a.status='paid' AND l.comp=0 AND `+where+` GROUP BY l.adisyon_no, l.vat`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	by := map[int]*VATGroup{}
	for rows.Next() {
		var rate, pct int
		var gross int64
		if err := rows.Scan(&rate, &pct, &gross); err != nil {
			return nil, err
		}
		g := by[rate]
		if g == nil {
			g = &VATGroup{Rate: rate}
			by[rate] = g
		}
		d := discountOf(gross, pct)
		g.Gross += gross - d
		g.Discount += d
		g.VAT += money.VAT(gross-d, rate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []VATGroup{}
	for _, g := range by {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rate < out[j].Rate })
	return out, nil
}

// compTotals ödenen adisyonlardaki ikramların adedini ve liste fiyatıyla değerini verir.
func compTotals(q querier, where string, args ...any) (int, int64, error) {
	var n int
	var v int64
	err := q.QueryRow(`SELECT COALESCE(SUM(l.qty),0), COALESCE(SUM(l.qty*l.price),0) FROM adisyon_lines l JOIN adisyons a ON a.no=l.adisyon_no
		WHERE a.status='paid' AND l.comp=1 AND `+where, args...).Scan(&n, &v)
	return n, v, err
}

// productTotals ödenen adisyonlardaki ürün adetlerini verir (çok satandan aza).
// Satılan adet ve tutar ikram hariçtir; ikram adedi ayrıca verilir.
func productTotals(q querier, where string, args ...any) ([]ProductQty, error) {
	rows, err := q.Query(`SELECT COALESCE(l.product_id,0), MAX(l.name), MAX(l.unit),
		SUM(CASE WHEN l.comp=0 THEN l.qty ELSE 0 END), SUM(CASE WHEN l.comp=0 THEN l.qty*l.price ELSE 0 END), SUM(CASE WHEN l.comp=1 THEN l.qty ELSE 0 END)
		FROM adisyon_lines l JOIN adisyons a ON a.no=l.adisyon_no
		WHERE a.status='paid' AND `+where+`
		GROUP BY COALESCE(l.product_id,0), CASE WHEN l.product_id IS NULL THEN l.name ELSE '' END
		ORDER BY SUM(CASE WHEN l.comp=0 THEN l.qty ELSE 0 END) DESC, MAX(l.name)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProductQty{}
	for rows.Next() {
		var p ProductQty
		if err := rows.Scan(&p.ProductID, &p.Name, &p.Unit, &p.Qty, &p.Total, &p.Comp); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Ürünün güncel adı (yeniden adlandırıldıysa)
	for i := range out {
		if out[i].ProductID > 0 {
			var name, unit string
			if err := q.QueryRow(`SELECT name, unit FROM products WHERE id=?`, out[i].ProductID).Scan(&name, &unit); err == nil {
				out[i].Name, out[i].Unit = name, unit
			}
		}
	}
	return out, nil
}

// DaySummary bir iş gününün gün sonu özetini verir (yönetici ekranı).
func (s *Service) DaySummary(date string) (*DaySummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	if date == "" {
		d, err := openBusinessDay(s.db, s.now())
		if err != nil {
			return nil, err
		}
		date = d
	}
	if !validDate(date) {
		return nil, uerr("invalid", "Tarih anlaşılamadı.")
	}
	ds, err := s.daySummary(s.db, date)
	if err != nil {
		return nil, err
	}
	cur, err := openBusinessDay(s.db, s.now())
	if err != nil {
		return nil, err
	}
	if ds.Unclosed, err = unclosedDays(s.db, cur); err != nil {
		return nil, err
	}
	if ds.Shifted, err = shiftedGroups(s.db, s.now(), false); err != nil {
		return nil, err
	}
	return ds, nil
}

// CloseDay günü onaylayıp kapatır: açık masa varsa kapatmaz; kapatılan gün
// değiştirilemez; kapanışta yedek alınır ve (ayarlıysa) özet WhatsApp ile
// Google Sheets'e gönderilmek üzere sıraya konur.
func (s *Service) CloseDay(date string) (*DaySummary, *State, error) {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return nil, nil, err
	}
	if !validDate(date) {
		s.mu.Unlock()
		return nil, nil, uerr("invalid", "Tarih anlaşılamadı.")
	}
	// Saate göre bugünden sonraki bir gün kapatılamaz: önceden kapatılırsa sonraki
	// günlerin adisyonları bir gün kayardı.
	if date > businessDate(s.now()) {
		s.mu.Unlock()
		return nil, nil, uerr("invalid", "%s henüz başlamadı; gün sonu gün bitince alınır.", dateTRLong(date))
	}
	var ds *DaySummary
	err := s.withTx(func(tx *sql.Tx) error {
		closed, _, err := dayClosed(tx, date)
		if err != nil {
			return err
		}
		if closed {
			return uerr("closed", "%s günü zaten kapatılmış.", dateTRLong(date))
		}
		ds, err = s.daySummary(tx, date)
		if err != nil {
			return err
		}
		if ds.Open > 0 {
			var tables []string
			for _, r := range ds.Rows {
				if r.Status == StatusOpen || r.Status == StatusPrinted {
					tables = append(tables, fmt.Sprintf("Masa %d", r.Table))
				}
			}
			return uerr("open_tables", "Açık masa var (%s). Gün kapatılmadan önce hesapları kapatın ya da iptal edin.", strings.Join(tables, ", "))
		}
		now := s.now()
		ds.Closed, ds.ClosedAt = true, stamp(now)
		b, _ := json.Marshal(ds)
		if _, err := tx.Exec(`INSERT INTO business_days(date,closed_at,summary) VALUES(?,?,?)
			ON CONFLICT(date) DO UPDATE SET closed_at=excluded.closed_at, summary=excluded.summary`, date, stamp(now), string(b)); err != nil {
			return err
		}
		audit(tx, now, "gun_kapatildi", 0, map[string]any{"tarih": date, "ciro": ds.Revenue, "adisyon": ds.Paid, "iptal": ds.Cancelled})
		return nil
	})
	if err != nil {
		s.mu.Unlock()
		return nil, nil, err
	}
	s.daysVer++
	s.enqueueDayReports(ds)
	if cur, err := openBusinessDay(s.db, s.now()); err == nil {
		ds.Unclosed, _ = unclosedDays(s.db, cur)
	}
	if ds.Unclosed == nil {
		ds.Unclosed = []string{}
	}
	ds.Shifted, _ = shiftedGroups(s.db, s.now(), false)
	st, err := s.stateLocked()
	s.mu.Unlock()
	// Kapanış yedeği arka planda (siparişleri bekletmez). Günün kapanış hali
	// ayrıca "Gün sonu" yedeği olarak saklanır.
	if s.opt.Backup != nil {
		go func() {
			_ = s.opt.Backup.Run("gün sonu")
			if _, err := s.opt.Backup.SaveCopy("gun_sonu_" + date); err != nil {
				s.opt.Log.Printf("gün sonu yedeği alınamadı: %v", err)
			}
		}()
	}
	s.kickOutbox()
	return ds, st, err
}

func dateTRLong(d string) string {
	t, err := parseDate(d)
	if err != nil {
		return d
	}
	months := []string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()-1], t.Year())
}
