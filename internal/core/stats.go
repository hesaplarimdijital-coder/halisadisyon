package core

import (
	"fmt"
	"sort"
	"time"
)

// İstatistikler yalnız ödenen adisyonlardan hesaplanır (iptal ve birleştirilen
// adisyonlar ciroya girmez). Gün, iş günüdür (05:00 kuralı).

// StatsQuery istatistik dönemidir.
type StatsQuery struct {
	Mode string `json:"mode"` // "day" | "month" | "year" | "range"
	From string `json:"from"` // YYYY-AA-GG
	To   string `json:"to"`   // yalnız "range"
}

// StatCard dönem kartıdır: bu dönem, önceki dönem ve değişim yüzdesi.
type StatCard struct {
	Value  int64    `json:"value"`
	Prev   int64    `json:"prev"`
	Change *float64 `json:"change"` // önceki dönem 0 ise boş
}

func card(v, prev int64) StatCard {
	c := StatCard{Value: v, Prev: prev}
	if prev != 0 {
		ch := float64(v-prev) / float64(prev) * 100
		c.Change = &ch
	}
	return c
}

// DayPoint bir iş gününün toplamlarıdır.
type DayPoint struct {
	Date      string `json:"date"`
	Revenue   int64  `json:"revenue"`
	Count     int    `json:"count"`
	Featured  int    `json:"featured"`
	Cash      int64  `json:"cash"`
	Card      int64  `json:"card"`
	Transfer  int64  `json:"transfer"`
	Unknown   int64  `json:"unknown"` // ödeme şekli kaydı olmayan (eski sürüm)
	Cancelled int    `json:"cancelled"`
	VAT       int64  `json:"vat"`
}

// MonthPoint bir ayın toplamlarıdır (yıllık görünüm).
type MonthPoint struct {
	Month    string `json:"month"` // YYYY-AA
	Revenue  int64  `json:"revenue"`
	Count    int    `json:"count"`
	Featured int    `json:"featured"`
}

// HourPoint açılış saatine göre adisyon sayısıdır (yoğunluk).
type HourPoint struct {
	Hour    int   `json:"hour"`
	Count   int   `json:"count"`
	Revenue int64 `json:"revenue"`
}

// Stats istatistik ekranının verisidir.
type Stats struct {
	Mode      string `json:"mode"`
	From      string `json:"from"`
	To        string `json:"to"` // bugünü geçmez
	PrevFrom  string `json:"prevFrom"`
	PrevTo    string `json:"prevTo"`
	Label     string `json:"label"`
	PrevLabel string `json:"prevLabel"`
	Partial   bool   `json:"partial"` // dönem henüz bitmedi (bugüne kadar)

	Revenue  StatCard `json:"revenue"`
	Count    StatCard `json:"count"`
	Avg      StatCard `json:"avg"`
	Featured StatCard `json:"featured"`

	FeaturedID   int64  `json:"featuredId"`
	FeaturedName string `json:"featuredName"`
	FeaturedUnit string `json:"featuredUnit"`

	VAT            int64         `json:"vat"`
	Cancelled      int           `json:"cancelled"`
	CancelledTotal int64         `json:"cancelledTotal"`
	Days           []DayPoint    `json:"days"`
	Months         []MonthPoint  `json:"months"`
	Products       []ProductQty  `json:"products"`
	Matrix         [][]int       `json:"matrix"` // Days × Products adet
	Hours          []HourPoint   `json:"hours"`
	ByPayment      []PayTotal    `json:"byPayment"`
	ByWaiter       []WaiterTotal `json:"byWaiter"`
}

func monthLabel(t time.Time) string {
	return fmt.Sprintf("%s %d", aylarTR[t.Month()-1], t.Year())
}

var aylarTR = []string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}

func rangeLabel(from, to time.Time) string {
	switch {
	case from.Equal(to):
		return fmt.Sprintf("%d %s %d", from.Day(), aylarTR[from.Month()-1], from.Year())
	case from.Year() == to.Year() && from.Month() == to.Month():
		return fmt.Sprintf("%d–%d %s %d", from.Day(), to.Day(), aylarTR[from.Month()-1], from.Year())
	case from.Year() == to.Year():
		return fmt.Sprintf("%d %s – %d %s %d", from.Day(), aylarTR[from.Month()-1], to.Day(), aylarTR[to.Month()-1], to.Year())
	}
	return fmt.Sprintf("%d %s %d – %d %s %d", from.Day(), aylarTR[from.Month()-1], from.Year(), to.Day(), aylarTR[to.Month()-1], to.Year())
}

func daysBetween(a, b time.Time) int {
	return int(time.Date(b.Year(), b.Month(), b.Day(), 12, 0, 0, 0, time.UTC).Sub(time.Date(a.Year(), a.Month(), a.Day(), 12, 0, 0, 0, time.UTC)).Hours() / 24)
}

// statsPeriod dönemi ve karşılaştırılacak önceki dönemi hesaplar. Dönem bugünü
// içeriyorsa bugüne kadar alınır; önceki dönemin de aynı sayıda günü alınır
// (ör. 1–11 Ekim ile 1–11 Eylül).
func statsPeriod(q StatsQuery, today string) (from, to, prevFrom, prevTo time.Time, label, prevLabel string, partial bool, err error) {
	t0, perr := parseDate(q.From)
	if perr != nil {
		t0, _ = parseDate(today)
	}
	td, _ := parseDate(today)
	var full time.Time // dönemin son günü (bitmemişse de)
	switch q.Mode {
	case "day":
		from, full = t0, t0
		prevFrom = from.AddDate(0, 0, -1)
	case "year":
		from = time.Date(t0.Year(), 1, 1, 0, 0, 0, 0, time.Local)
		full = time.Date(t0.Year(), 12, 31, 0, 0, 0, 0, time.Local)
		prevFrom = from.AddDate(-1, 0, 0)
	case "range":
		t1, perr := parseDate(q.To)
		if perr != nil {
			t1 = t0
		}
		if t1.Before(t0) {
			t0, t1 = t1, t0
		}
		if daysBetween(t0, t1) > 3700 {
			err = uerr("invalid", "Tarih aralığı en fazla 10 yıl olabilir.")
			return
		}
		from, full = t0, t1
		prevFrom = from.AddDate(0, 0, -(daysBetween(from, full) + 1))
	default: // month
		from = time.Date(t0.Year(), t0.Month(), 1, 0, 0, 0, 0, time.Local)
		full = from.AddDate(0, 1, -1)
		prevFrom = from.AddDate(0, -1, 0)
	}
	to = full
	if to.After(td) && !from.After(td) {
		to, partial = td, true
	}
	if !to.Before(td) && !from.After(td) {
		partial = true // dönem açık günü (bugünü) içeriyor
	}
	n := daysBetween(from, to)
	prevTo = prevFrom.AddDate(0, 0, n)
	// Önceki dönem kendi sınırını aşmasın (ör. 31 günlük aya karşı 30 günlük ay).
	var prevEnd time.Time
	switch q.Mode {
	case "year":
		prevEnd = time.Date(prevFrom.Year(), 12, 31, 0, 0, 0, 0, time.Local)
	case "month", "":
		prevEnd = prevFrom.AddDate(0, 1, -1)
	default:
		prevEnd = from.AddDate(0, 0, -1)
	}
	// Biten bir ay ya da yıl önceki ayın/yılın tamamıyla karşılaştırılır
	// (Şubat'a karşı Ocak'ın 31 günü); süren dönem aynı sayıda günle.
	if prevTo.After(prevEnd) || (!partial && (q.Mode == "month" || q.Mode == "year" || q.Mode == "")) {
		prevTo = prevEnd
	}
	switch q.Mode {
	case "day":
		label, prevLabel = rangeLabel(from, from)+" "+gunlerTR[from.Weekday()], rangeLabel(prevFrom, prevFrom)+" "+gunlerTR[prevFrom.Weekday()]
	case "year":
		label, prevLabel = fmt.Sprint(from.Year()), fmt.Sprint(prevFrom.Year())
		if partial {
			label, prevLabel = rangeLabel(from, to), rangeLabel(prevFrom, prevTo)
		}
	case "range":
		label, prevLabel = rangeLabel(from, to), rangeLabel(prevFrom, prevTo)
	default:
		label, prevLabel = monthLabel(from), monthLabel(prevFrom)
		if partial {
			label, prevLabel = rangeLabel(from, to), rangeLabel(prevFrom, prevTo)
		}
	}
	return
}

var gunlerTR = []string{"Pazar", "Pazartesi", "Salı", "Çarşamba", "Perşembe", "Cuma", "Cumartesi"}

type periodTotals struct {
	revenue  int64
	count    int64
	featured int64
}

func (s *Service) periodTotals(from, to string, featuredID int64) (periodTotals, error) {
	var p periodTotals
	if err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(total),0) FROM adisyons
		WHERE status='paid' AND business_date BETWEEN ? AND ?`, from, to).Scan(&p.count, &p.revenue); err != nil {
		return p, err
	}
	if featuredID > 0 {
		if err := s.db.QueryRow(`SELECT COALESCE(SUM(l.qty),0) FROM adisyon_lines l JOIN adisyons a ON a.no=l.adisyon_no
			WHERE a.status='paid' AND a.business_date BETWEEN ? AND ? AND l.product_id=?`, from, to, featuredID).Scan(&p.featured); err != nil {
			return p, err
		}
	}
	return p, nil
}

// Stats seçilen dönemin istatistiklerini verir (yönetici).
func (s *Service) Stats(q StatsQuery) (*Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	return s.stats(q)
}

func avgOf(revenue, count int64) int64 {
	if count == 0 {
		return 0
	}
	return (revenue + count/2) / count
}

func (s *Service) stats(q StatsQuery) (*Stats, error) {
	// Dönemin "bugün"ü saate göre iş günüdür: gün erken kapatılınca ertesi gün
	// (henüz başlamamış) karşılaştırmaya girmesin.
	today := businessDate(s.now())
	switch q.Mode {
	case "day", "month", "year", "range":
	default:
		q.Mode = "month"
	}
	if q.From == "" {
		q.From = today
	}
	if !validDate(q.From) || (q.Mode == "range" && q.To != "" && !validDate(q.To)) {
		return nil, uerr("invalid", "Tarih anlaşılamadı.")
	}
	from, to, pf, pt, label, plabel, partial, err := statsPeriod(q, today)
	if err != nil {
		return nil, err
	}
	F, T := from.Format(dateLayout), to.Format(dateLayout)
	st := &Stats{Mode: q.Mode, From: F, To: T, PrevFrom: pf.Format(dateLayout), PrevTo: pt.Format(dateLayout),
		Label: label, PrevLabel: plabel, Partial: partial,
		Days: []DayPoint{}, Months: []MonthPoint{}, Products: []ProductQty{}, Matrix: [][]int{}, Hours: make([]HourPoint, 24),
		ByPayment: []PayTotal{}, ByWaiter: []WaiterTotal{}}
	for h := range st.Hours {
		st.Hours[h].Hour = h
	}
	fid, err := s.featuredProduct(s.db)
	if err != nil {
		return nil, err
	}
	st.FeaturedID = fid
	if fid > 0 {
		if p, err := loadProduct(s.db, fid); err == nil {
			st.FeaturedName, st.FeaturedUnit = p.Name, p.Unit
		}
	}

	// Günler (boş günler dahil)
	index := map[string]int{}
	if !to.Before(from) {
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			index[d.Format(dateLayout)] = len(st.Days)
			st.Days = append(st.Days, DayPoint{Date: d.Format(dateLayout)})
		}
	}
	rows, err := s.db.Query(`SELECT business_date, COUNT(*), COALESCE(SUM(total),0), COALESCE(SUM(vat_total),0),
		COALESCE(SUM(CASE WHEN payment_method='nakit' THEN total END),0),
		COALESCE(SUM(CASE WHEN payment_method='kart' THEN total END),0),
		COALESCE(SUM(CASE WHEN payment_method='havale' THEN total END),0),
		COALESCE(SUM(CASE WHEN COALESCE(payment_method,'') NOT IN ('nakit','kart','havale') THEN total END),0)
		FROM adisyons WHERE status='paid' AND business_date BETWEEN ? AND ? GROUP BY business_date`, F, T)
	if err != nil {
		return nil, err
	}
	var revenue, count int64
	for rows.Next() {
		var d DayPoint
		if err := rows.Scan(&d.Date, &d.Count, &d.Revenue, &d.VAT, &d.Cash, &d.Card, &d.Transfer, &d.Unknown); err != nil {
			rows.Close()
			return nil, err
		}
		if i, ok := index[d.Date]; ok {
			d.Featured = st.Days[i].Featured
			st.Days[i] = d
		}
		revenue += d.Revenue
		count += int64(d.Count)
		st.VAT += d.VAT
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// İptaller (birleştirilenler hariç)
	rows, err = s.db.Query(`SELECT business_date, COUNT(*), COALESCE(SUM(total),0) FROM adisyons
		WHERE status='cancelled' AND COALESCE(merged_into,0)=0 AND business_date BETWEEN ? AND ? GROUP BY business_date`, F, T)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d string
		var n int
		var tot int64
		if err := rows.Scan(&d, &n, &tot); err != nil {
			rows.Close()
			return nil, err
		}
		if i, ok := index[d]; ok {
			st.Days[i].Cancelled = n
		}
		st.Cancelled += n
		st.CancelledTotal += tot
	}
	rows.Close()

	// Ürünler: dönem toplamı ve gün × ürün tablosu
	if st.Products, err = productTotals(s.db, `a.business_date BETWEEN ? AND ?`, F, T); err != nil {
		return nil, err
	}
	col := map[string]int{}
	keyOf := func(pid int64, name string) string {
		if pid > 0 {
			return fmt.Sprint(pid)
		}
		return "ad:" + name
	}
	for i, p := range st.Products {
		col[keyOf(p.ProductID, p.Name)] = i
	}
	st.Matrix = make([][]int, len(st.Days))
	for i := range st.Matrix {
		st.Matrix[i] = make([]int, len(st.Products))
	}
	rows, err = s.db.Query(`SELECT a.business_date, COALESCE(l.product_id,0), MAX(l.name), SUM(l.qty)
		FROM adisyon_lines l JOIN adisyons a ON a.no=l.adisyon_no
		WHERE a.status='paid' AND a.business_date BETWEEN ? AND ?
		GROUP BY a.business_date, COALESCE(l.product_id,0), CASE WHEN l.product_id IS NULL THEN l.name ELSE '' END`, F, T)
	if err != nil {
		return nil, err
	}
	var featured int64
	for rows.Next() {
		var d, name string
		var pid int64
		var qty int
		if err := rows.Scan(&d, &pid, &name, &qty); err != nil {
			rows.Close()
			return nil, err
		}
		i, ok := index[d]
		if !ok {
			continue
		}
		if j, ok := col[keyOf(pid, name)]; ok {
			st.Matrix[i][j] += qty
		}
		if fid > 0 && pid == fid {
			st.Days[i].Featured += qty
			featured += int64(qty)
		}
	}
	rows.Close()

	// Saatlere göre yoğunluk (masanın açıldığı saat)
	rows, err = s.db.Query(`SELECT CAST(substr(opened_at,12,2) AS INTEGER), COUNT(*), COALESCE(SUM(total),0) FROM adisyons
		WHERE status='paid' AND business_date BETWEEN ? AND ? GROUP BY 1`, F, T)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var h, n int
		var tot int64
		if err := rows.Scan(&h, &n, &tot); err != nil {
			rows.Close()
			return nil, err
		}
		if h >= 0 && h < 24 {
			st.Hours[h].Count += n
			st.Hours[h].Revenue += tot
		}
	}
	rows.Close()

	// Ödeme şekilleri
	pay := map[string]*PayTotal{}
	rows, err = s.db.Query(`SELECT COALESCE(payment_method,''), COUNT(*), COALESCE(SUM(total),0) FROM adisyons
		WHERE status='paid' AND business_date BETWEEN ? AND ? GROUP BY 1`, F, T)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p PayTotal
		if err := rows.Scan(&p.Method, &p.Count, &p.Total); err != nil {
			rows.Close()
			return nil, err
		}
		if !validPayment(p.Method) {
			p.Method = ""
		}
		if old := pay[p.Method]; old != nil {
			old.Count += p.Count
			old.Total += p.Total
		} else {
			pp := p
			pay[p.Method] = &pp
		}
	}
	rows.Close()
	for _, m := range PaymentMethods {
		p := PayTotal{Method: m.ID, Label: m.Label}
		if x := pay[m.ID]; x != nil {
			p.Count, p.Total = x.Count, x.Total
		}
		st.ByPayment = append(st.ByPayment, p)
	}
	if x := pay[""]; x != nil {
		st.ByPayment = append(st.ByPayment, PayTotal{Method: "", Label: "Belirtilmemiş", Count: x.Count, Total: x.Total})
	}

	// Garsonlar
	rows, err = s.db.Query(`SELECT waiter_name, COUNT(*), COALESCE(SUM(total),0) FROM adisyons
		WHERE status='paid' AND business_date BETWEEN ? AND ? GROUP BY waiter_name ORDER BY SUM(total) DESC`, F, T)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var w WaiterTotal
		if err := rows.Scan(&w.Name, &w.Count, &w.Total); err != nil {
			rows.Close()
			return nil, err
		}
		st.ByWaiter = append(st.ByWaiter, w)
	}
	rows.Close()

	// Aylar (yıllık görünüm ve uzun aralıklar için)
	if len(st.Days) > 62 {
		mi := map[string]int{}
		for _, d := range st.Days {
			m := d.Date[:7]
			i, ok := mi[m]
			if !ok {
				i = len(st.Months)
				mi[m] = i
				st.Months = append(st.Months, MonthPoint{Month: m})
			}
			st.Months[i].Revenue += d.Revenue
			st.Months[i].Count += d.Count
			st.Months[i].Featured += d.Featured
		}
	}

	prev, err := s.periodTotals(st.PrevFrom, st.PrevTo, fid)
	if err != nil {
		return nil, err
	}
	st.Revenue = card(revenue, prev.revenue)
	st.Count = card(count, prev.count)
	st.Avg = card(avgOf(revenue, count), avgOf(prev.revenue, prev.count))
	st.Featured = card(featured, prev.featured)
	sort.SliceStable(st.ByWaiter, func(i, j int) bool { return st.ByWaiter[i].Total > st.ByWaiter[j].Total })
	return st, nil
}

// periodAdisyons dönemdeki tüm adisyonlardır (Excel "Adisyonlar" sayfası).
func (s *Service) periodAdisyons(from, to string) ([]Adisyon, error) {
	rows, err := s.db.Query(`SELECT `+adisyonCols+` FROM adisyons WHERE business_date BETWEEN ? AND ? ORDER BY no`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Adisyon
	for rows.Next() {
		a, err := scanAdisyon(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}
