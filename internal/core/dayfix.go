package core

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Gün düzeltme: gün sonu yanlışlıkla gün bitmeden alınırsa (ör. öğlen) o günün
// sonraki adisyonları bir sonraki güne yazılır ve her gün bir gün kayar.
// Bu dosyada iki araç var:
//   - ReopenDay: kapatılmış günü yönetici şifresi ve sebeple yeniden açar.
//   - FixShifted: açıldığı iş gününden sonraki bir güne yazılmış adisyonları
//     ("kaymış") açıldıkları güne taşır; geçmiş günlerin özetini yeniden
//     hesaplayıp kapalı tutar, bugün ve sonrası yanlışlıkla kapatıldıysa açar.
// Adisyon numaraları, saatleri ve tutarları değişmez; her adım işlem kaydına yazılır.

// shiftWindowDays kaymış adisyonların arandığı geçmiş gün sayısıdır.
const shiftWindowDays = 60

const shiftIgnoreKey = "kayma_yoksay"

// ShiftGroup aynı günde açılıp aynı başka güne yazılmış adisyonlardır.
type ShiftGroup struct {
	From       string  `json:"from"` // şu an yazılı olduğu iş günü
	To         string  `json:"to"`   // açıldığı (doğru) iş günü
	Count      int     `json:"count"`
	Paid       int     `json:"paid"`
	Total      int64   `json:"total"` // ödenenlerin tutarı
	Active     int     `json:"active"`
	FirstAt    string  `json:"firstAt"`
	LastAt     string  `json:"lastAt"`
	FromClosed bool    `json:"fromClosed"`
	ToClosed   bool    `json:"toClosed"`
	ToClosedAt string  `json:"toClosedAt,omitempty"` // doğru günün (erken) kapanış saati
	Nos        []int64 `json:"nos"`
	// Veresiye tahsilatları: gün erken kapatıldıktan sonra alınan tahsilat da
	// ertesi güne yazılır; adisyonlarla birlikte doğru güne taşınır.
	Collections    int     `json:"collections"`
	CollectedTotal int64   `json:"collectedTotal"`
	CollectionIDs  []int64 `json:"collectionIds"`
}

// ShiftPair düzeltilecek bir gruptur.
type ShiftPair struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (p ShiftPair) key() string { return p.From + ">" + p.To }

// FixResult düzeltmenin özetidir.
type FixResult struct {
	Moved int `json:"moved"`
	// MovedCollections taşınan veresiye tahsilatı sayısıdır.
	MovedCollections int      `json:"movedCollections"`
	Reopened         []string `json:"reopened"` // yeniden açılan günler (bugün ve sonrası)
	Reclosed         []string `json:"reclosed"` // özeti yeniden hesaplanıp kapalı kalan geçmiş günler
}

// naturalDate adisyonun açıldığı saate göre iş günüdür.
func naturalDate(openedAt string) (string, bool) {
	t, err := time.Parse(time.RFC3339, openedAt)
	if err != nil {
		return "", false
	}
	return businessDate(t.In(time.Local)), true
}

func loadShiftIgnore(q querier) map[string]bool {
	out := map[string]bool{}
	var v string
	if err := q.QueryRow(`SELECT value FROM settings WHERE key=?`, shiftIgnoreKey).Scan(&v); err == nil {
		var list []string
		_ = json.Unmarshal([]byte(v), &list)
		for _, k := range list {
			out[k] = true
		}
	}
	return out
}

// shiftedGroups son shiftWindowDays gündeki kaymış adisyonları gruplar (eskiden
// yeniye). Masada açık adisyon yalnız bugüne ya da sonrasına taşınabilir;
// geçmiş güne taşınamayacağı için listelenmez.
func shiftedGroups(q querier, now time.Time, withIgnored bool) ([]ShiftGroup, error) {
	today := businessDate(now)
	since := now.AddDate(0, 0, -shiftWindowDays).Format(dateLayout)
	rows, err := q.Query(`SELECT no, business_date, opened_at, status, total FROM adisyons WHERE business_date >= ? ORDER BY no`, since)
	if err != nil {
		return nil, err
	}
	type rec struct {
		no                  int64
		date, opened, state string
		total               int64
	}
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.no, &r.date, &r.opened, &r.state, &r.total); err != nil {
			rows.Close()
			return nil, err
		}
		recs = append(recs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ignore := map[string]bool{}
	if !withIgnored {
		ignore = loadShiftIgnore(q)
	}
	by := map[string]*ShiftGroup{}
	var order []string
	for _, r := range recs {
		nat, ok := naturalDate(r.opened)
		if !ok || nat >= r.date {
			continue
		}
		active := r.state == StatusOpen || r.state == StatusPrinted
		if active && nat < today {
			continue
		}
		k := r.date + ">" + nat
		if ignore[k] {
			continue
		}
		g := by[k]
		if g == nil {
			g = &ShiftGroup{From: r.date, To: nat, FirstAt: r.opened}
			by[k] = g
			order = append(order, k)
		}
		g.Count++
		g.Nos = append(g.Nos, r.no)
		if r.opened < g.FirstAt {
			g.FirstAt = r.opened
		}
		if r.opened > g.LastAt {
			g.LastAt = r.opened
		}
		switch {
		case r.state == StatusPaid:
			g.Paid++
			g.Total += r.total
		case active:
			g.Active++
		}
	}
	// Veresiye tahsilatları: alındığı saate göre iş günü.
	crows, err := q.Query(`SELECT id, business_date, at, amount FROM credit_payments WHERE business_date >= ? AND cancelled_at IS NULL ORDER BY id`, since)
	if err != nil {
		return nil, err
	}
	type col struct {
		id       int64
		date, at string
		amount   int64
	}
	var cols []col
	for crows.Next() {
		var c col
		if err := crows.Scan(&c.id, &c.date, &c.at, &c.amount); err != nil {
			crows.Close()
			return nil, err
		}
		cols = append(cols, c)
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return nil, err
	}
	for _, c := range cols {
		nat, ok := naturalDate(c.at)
		if !ok || nat >= c.date {
			continue
		}
		k := c.date + ">" + nat
		if ignore[k] {
			continue
		}
		g := by[k]
		if g == nil {
			g = &ShiftGroup{From: c.date, To: nat, FirstAt: c.at, Nos: []int64{}}
			by[k] = g
			order = append(order, k)
		}
		g.Collections++
		g.CollectedTotal += c.amount
		g.CollectionIDs = append(g.CollectionIDs, c.id)
		if c.at < g.FirstAt {
			g.FirstAt = c.at
		}
		if c.at > g.LastAt {
			g.LastAt = c.at
		}
	}
	out := make([]ShiftGroup, 0, len(order))
	for _, k := range order {
		g := by[k]
		var err error
		if g.FromClosed, _, err = dayClosed(q, g.From); err != nil {
			return nil, err
		}
		if g.ToClosed, g.ToClosedAt, err = dayClosed(q, g.To); err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].From < out[j].From
	})
	return out, nil
}

// ShiftedGroups gün sonu ekranındaki "kaymış adisyonlar" uyarısıdır.
func (s *Service) ShiftedGroups() ([]ShiftGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	return shiftedGroups(s.db, s.now(), false)
}

// IgnoreShift bir kayma grubunu "böyle kalsın" diye işaretler (ör. gün sonu
// gece alındıktan sonra gelen son müşteri bilerek ertesi güne yazıldıysa).
func (s *Service) IgnoreShift(p ShiftPair) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	if !validDate(p.From) || !validDate(p.To) {
		return uerr("invalid", "Tarih anlaşılamadı.")
	}
	list := loadShiftIgnore(s.db)
	list[p.key()] = true
	keys := make([]string, 0, len(list))
	for k := range list {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b, _ := json.Marshal(keys)
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, shiftIgnoreKey, string(b)); err != nil {
			return err
		}
		audit(tx, s.now(), "kayma_yoksayildi", 0, map[string]any{"eski": p.From, "dogru": p.To})
		return nil
	})
}

// ReopenDay kapatılmış günü yeniden açar (yönetici şifresi ve sebep ister).
// Açılan güne ait adisyonlar yeniden düzeltilebilir; gün daha sonra yeniden
// kapatılır ve özet yeniden gönderilir. Bugün açılırsa yeni adisyonlar yine
// bugüne yazılır; geçmiş bir gün açılırsa yeni adisyonlar bugüne yazılmaya
// devam eder.
func (s *Service) ReopenDay(date, reason, pin string) (*DaySummary, *State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, nil, err
	}
	if !validDate(date) {
		return nil, nil, uerr("invalid", "Tarih anlaşılamadı.")
	}
	reason, err := required(singleLine(reason), 200, "Sebep")
	if err != nil {
		return nil, nil, err
	}
	if err := s.confirmPin(pin, "Kapatılmış günü yeniden açmak yönetici şifresi ister."); err != nil {
		return nil, nil, err
	}
	var ds *DaySummary
	err = s.withTx(func(tx *sql.Tx) error {
		closed, closedAt, err := dayClosed(tx, date)
		if err != nil {
			return err
		}
		if !closed {
			return uerr("invalid", "%s günü zaten açık.", dateTRLong(date))
		}
		old, err := s.daySummary(tx, date)
		if err != nil {
			return err
		}
		if err := reopenDayTx(tx, date); err != nil {
			return err
		}
		audit(tx, s.now(), "gun_yeniden_acildi", 0, map[string]any{"tarih": date, "sebep": reason, "kapanis": closedAt,
			"ciro": old.Revenue, "adisyon": old.Paid})
		ds, err = s.daySummary(tx, date)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	s.daysVer++
	if cur, err := openBusinessDay(s.db, s.now()); err == nil {
		ds.Unclosed, _ = unclosedDays(s.db, cur)
	}
	if ds.Unclosed == nil {
		ds.Unclosed = []string{}
	}
	ds.Shifted, _ = shiftedGroups(s.db, s.now(), false)
	st, err := s.stateLocked()
	return ds, st, err
}

// reopenDayTx günün kapanışını kaldırır; gönderilmemiş eski raporlar silinir
// (gün yeniden kapatılınca güncel özet gönderilir).
func reopenDayTx(tx *sql.Tx, date string) error {
	if _, err := tx.Exec(`DELETE FROM business_days WHERE date=?`, date); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM outbox WHERE ref=? AND status='pending'`, date)
	return err
}

// FixShifted seçilen kayma gruplarındaki adisyonları açıldıkları güne taşır.
// Etkilenen günlerden bugün ve sonrası kapatılmışsa yeniden açılır (gün
// bitmeden kapatılmış demektir); geçmiş ve kapatılmış günlerin özeti yeniden
// hesaplanır, kapalı kalır ve raporları (WhatsApp, Sheets) yeniden gönderilir.
// Yönetici şifresi her zaman sorulur.
func (s *Service) FixShifted(pairs []ShiftPair, reason, pin string) (*FixResult, *State, error) {
	reason, err := required(singleLine(reason), 200, "Sebep")
	if err != nil {
		return nil, nil, err
	}
	if len(pairs) == 0 {
		return nil, nil, uerr("invalid", "Düzeltilecek gün seçilmedi.")
	}
	// Şifre doğruysa düzeltmeden önce ayrı bir yedek alınır (geri dönüş için).
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return nil, nil, err
	}
	if err := s.confirmPin(pin, "Adisyonları başka güne taşımak yönetici şifresi ister."); err != nil {
		s.mu.Unlock()
		return nil, nil, err
	}
	s.mu.Unlock()
	if s.opt.Backup != nil {
		if _, err := s.opt.Backup.SaveCopy("gun_duzeltme_oncesi"); err != nil {
			s.opt.Log.Printf("gün düzeltme öncesi yedek alınamadı: %v", err)
		}
	}

	s.mu.Lock()
	want := map[string]bool{}
	for _, p := range pairs {
		want[p.key()] = true
	}
	now := s.now()
	today := businessDate(now)
	res := &FixResult{Reopened: []string{}, Reclosed: []string{}}
	var reports []*DaySummary
	err = s.withTx(func(tx *sql.Tx) error {
		groups, err := shiftedGroups(tx, now, true)
		if err != nil {
			return err
		}
		days := map[string]bool{}
		wasClosed := map[string]bool{}
		var desc []string
		for _, g := range groups {
			if !want[ShiftPair{g.From, g.To}.key()] {
				continue
			}
			for _, no := range g.Nos {
				if s.inPrint[no] {
					return errBusy(no)
				}
			}
			for _, d := range []string{g.From, g.To} {
				if !days[d] {
					days[d] = true
					c, _, err := dayClosed(tx, d)
					if err != nil {
						return err
					}
					wasClosed[d] = c
				}
			}
			for _, no := range g.Nos {
				if _, err := tx.Exec(`UPDATE adisyons SET business_date=? WHERE no=? AND business_date=?`, g.To, no, g.From); err != nil {
					return err
				}
				audit(tx, now, "is_gunu_degisti", no, map[string]any{"eski": g.From, "yeni": g.To, "sebep": reason})
			}
			for _, id := range g.CollectionIDs {
				if _, err := tx.Exec(`UPDATE credit_payments SET business_date=? WHERE id=? AND business_date=?`, g.To, id, g.From); err != nil {
					return err
				}
			}
			res.Moved += len(g.Nos)
			res.MovedCollections += len(g.CollectionIDs)
			what := fmt.Sprintf("%d adisyon", len(g.Nos))
			if len(g.CollectionIDs) > 0 {
				what += fmt.Sprintf(" ve %d tahsilat", len(g.CollectionIDs))
				if len(g.Nos) == 0 {
					what = fmt.Sprintf("%d tahsilat", len(g.CollectionIDs))
				}
			}
			desc = append(desc, fmt.Sprintf("%s %s → %s", what, dateTRLong(g.From), dateTRLong(g.To)))
		}
		if res.Moved+res.MovedCollections == 0 {
			return uerr("changed", "Düzeltilecek adisyon kalmadı; ekran yenilendi.")
		}
		list := make([]string, 0, len(days))
		for d := range days {
			list = append(list, d)
		}
		sort.Strings(list)
		for _, d := range list {
			if !wasClosed[d] {
				continue
			}
			active := 0
			if err := tx.QueryRow(`SELECT COUNT(*) FROM adisyons WHERE business_date=? AND status IN ('open','printed')`, d).Scan(&active); err != nil {
				return err
			}
			if d >= today || active > 0 {
				if err := reopenDayTx(tx, d); err != nil {
					return err
				}
				res.Reopened = append(res.Reopened, d)
				continue
			}
			ds, err := s.daySummary(tx, d)
			if err != nil {
				return err
			}
			ds.Closed, ds.ClosedAt, ds.Corrected = true, stamp(now), true
			ds.Migrated = false
			b, _ := json.Marshal(ds)
			if _, err := tx.Exec(`UPDATE business_days SET closed_at=?, summary=? WHERE date=?`, stamp(now), string(b), d); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM outbox WHERE ref=? AND status='pending'`, d); err != nil {
				return err
			}
			res.Reclosed = append(res.Reclosed, d)
			reports = append(reports, ds)
		}
		audit(tx, now, "gun_duzeltildi", 0, map[string]any{"sebep": reason, "adisyon": res.Moved, "tahsilat": res.MovedCollections, "metin": strings.Join(desc, " · "),
			"acilan": res.Reopened, "yeniden_hesaplanan": res.Reclosed})
		return nil
	})
	if err != nil {
		s.mu.Unlock()
		return nil, nil, err
	}
	s.daysVer++
	for _, ds := range reports {
		s.enqueueDayReportsFix(ds)
	}
	st, err := s.stateLocked()
	s.mu.Unlock()
	if s.opt.Backup != nil {
		go func() { _ = s.opt.Backup.Run("gün düzeltme") }()
	}
	if len(reports) > 0 {
		s.kickOutbox()
	}
	return res, st, err
}

// enqueueDayReportsFix düzeltilen geçmiş günün raporlarını yeniden sıraya koyar;
// adisyonu kalmayan gün için WhatsApp gönderilmez (Sheets satırları temizlenir).
func (s *Service) enqueueDayReportsFix(ds *DaySummary) {
	s.enqueueReports(ds, len(ds.Rows) > 0)
}
