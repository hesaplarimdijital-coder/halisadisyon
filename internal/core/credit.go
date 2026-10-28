package core

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"halisadisyon/internal/money"
)

// Veresiye: müşteri hesabın bir kısmını ya da tamamını sonra öder. Veresiye
// parçası adisyonun gününde ciroya yazılır, kasaya girmez. Borç, müşterinin
// veresiye parçalarının toplamından tahsilatların düşülmesiyle bulunur.
// Tahsilat (tamamı ya da bir kısmı) nakit, kart ya da havale ile alınır ve
// alındığı günün kasasına "veresiye tahsilatı" olarak girer; ciroya ikinci kez
// yazılmaz.

// CreditCustomer veresiye müşterisi ve borcudur.
type CreditCustomer struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	Note      string `json:"note"`
	Given     int64  `json:"given"`     // toplam veresiye
	Collected int64  `json:"collected"` // toplam tahsilat
	Balance   int64  `json:"balance"`   // kalan borç
	Count     int    `json:"count"`     // veresiyeli adisyon sayısı
	LastAt    string `json:"lastAt,omitempty"`
	CreatedAt string `json:"createdAt"`
}

// CreditBrief ödeme penceresindeki müşteri listesi içindir.
type CreditBrief struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Balance int64  `json:"balance"`
}

// CreditItem müşteri hesabındaki bir harekettir: veresiye adisyon ya da tahsilat.
type CreditItem struct {
	Kind      string `json:"kind"` // "veresiye" | "tahsilat"
	ID        int64  `json:"id"`   // tahsilat no (kind=tahsilat)
	No        int64  `json:"no,omitempty"`
	Date      string `json:"date"` // iş günü
	At        string `json:"at"`
	Amount    int64  `json:"amount"`
	Total     int64  `json:"total,omitempty"` // adisyonun toplamı (parçalıysa veresiye kısmından büyük)
	Method    string `json:"method,omitempty"`
	Label     string `json:"label,omitempty"`
	Note      string `json:"note,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
	CancelAt  string `json:"cancelAt,omitempty"`
	Reason    string `json:"reason,omitempty"`
	DayClosed bool   `json:"dayClosed"`
	Balance   int64  `json:"balance"` // bu hareketten sonraki borç
}

// CreditDetail müşteri hesabının dökümüdür (yeniden eskiye).
type CreditDetail struct {
	Customer CreditCustomer `json:"customer"`
	Items    []CreditItem   `json:"items"`
}

// CreditList Veresiye ekranının listesidir.
type CreditList struct {
	Customers []CreditCustomer `json:"customers"`
	Balance   int64            `json:"balance"` // toplam alacak
	Debtors   int              `json:"debtors"` // borcu olan müşteri
}

func trFold(s string) string {
	s = strings.NewReplacer("İ", "i", "I", "ı").Replace(s)
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

const creditGivenSQL = `SELECT COALESCE(SUM(p.amount),0) FROM adisyon_payments p JOIN adisyons a ON a.no=p.adisyon_no
	WHERE a.status='paid' AND p.method='veresiye' AND a.credit_customer_id=?`
const creditCollectedSQL = `SELECT COALESCE(SUM(amount),0) FROM credit_payments WHERE customer_id=? AND cancelled_at IS NULL`

func creditBalance(q querier, id int64) (int64, error) {
	var g, c int64
	if err := q.QueryRow(creditGivenSQL, id).Scan(&g); err != nil {
		return 0, err
	}
	if err := q.QueryRow(creditCollectedSQL, id).Scan(&c); err != nil {
		return 0, err
	}
	return g - c, nil
}

func loadCustomers(q querier) ([]CreditCustomer, error) {
	rows, err := q.Query(`SELECT c.id, c.name, c.phone, c.note, c.created_at,
		COALESCE((SELECT SUM(p.amount) FROM adisyon_payments p JOIN adisyons a ON a.no=p.adisyon_no
			WHERE a.status='paid' AND p.method='veresiye' AND a.credit_customer_id=c.id),0),
		COALESCE((SELECT COUNT(*) FROM adisyons a WHERE a.status='paid' AND a.credit_customer_id=c.id),0),
		COALESCE((SELECT SUM(amount) FROM credit_payments WHERE customer_id=c.id AND cancelled_at IS NULL),0),
		COALESCE((SELECT MAX(x) FROM (SELECT MAX(closed_at) x FROM adisyons WHERE status='paid' AND credit_customer_id=c.id
			UNION ALL SELECT MAX(at) FROM credit_payments WHERE customer_id=c.id)),'')
		FROM credit_customers c`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CreditCustomer{}
	for rows.Next() {
		var c CreditCustomer
		if err := rows.Scan(&c.ID, &c.Name, &c.Phone, &c.Note, &c.CreatedAt, &c.Given, &c.Count, &c.Collected, &c.LastAt); err != nil {
			return nil, err
		}
		c.Balance = c.Given - c.Collected
		out = append(out, c)
	}
	return out, rows.Err()
}

func loadCustomer(q querier, id int64) (*CreditCustomer, error) {
	list, err := loadCustomers(q)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, uerr("not_found", "Veresiye müşterisi bulunamadı.")
}

func cleanName(name string) (string, error) {
	return required(singleLine(name), 60, "Müşteri adı")
}

// resolveCustomer veresiye müşterisini bulur ya da (adı yeni ise) açar.
func resolveCustomer(tx *sql.Tx, ref *CreditRef, now time.Time) (int64, string, error) {
	if ref == nil {
		return 0, "", uerr("credit_name", "Veresiye için müşterinin adını yazın.")
	}
	if ref.ID > 0 {
		var name string
		err := tx.QueryRow(`SELECT name FROM credit_customers WHERE id=?`, ref.ID).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", uerr("not_found", "Veresiye müşterisi bulunamadı.")
		}
		return ref.ID, name, err
	}
	name, err := cleanName(ref.Name)
	if err != nil {
		return 0, "", uerr("credit_name", "Veresiye için müşterinin adını yazın.")
	}
	rows, err := tx.Query(`SELECT id, name FROM credit_customers`)
	if err != nil {
		return 0, "", err
	}
	want := trFold(name)
	var found int64
	var foundName string
	for rows.Next() {
		var id int64
		var n string
		if err := rows.Scan(&id, &n); err != nil {
			rows.Close()
			return 0, "", err
		}
		if trFold(n) == want {
			found, foundName = id, n
		}
	}
	rows.Close()
	if found > 0 {
		return found, foundName, nil
	}
	phone, err := clean(ref.Phone, 30, "Telefon")
	if err != nil {
		return 0, "", err
	}
	res, err := tx.Exec(`INSERT INTO credit_customers(name, phone, created_at) VALUES(?,?,?)`, name, phone, stamp(now))
	if err != nil {
		return 0, "", err
	}
	id, err := res.LastInsertId()
	if err == nil {
		audit(tx, now, "veresiye_musteri", 0, map[string]any{"ad": name, "telefon": phone})
	}
	return id, name, err
}

// checkCreditRemoval adisyondaki veresiye kaldırılırken (iptal, yeniden açma,
// ödeme değişikliği) müşterinin borcu eksiye düşmemelidir: o veresiyeden sonra
// tahsilat alınmışsa önce tahsilat iptal edilir.
func checkCreditRemoval(q querier, a *Adisyon, keep int64) error {
	v := creditPart(partsOf(a))
	if a.CreditCustomerID == 0 || v <= keep {
		return nil
	}
	bal, err := creditBalance(q, a.CreditCustomerID)
	if err != nil {
		return err
	}
	if bal-(v-keep) < 0 {
		return uerr("credit_paid", "%s adlı müşteriden bu veresiye için tahsilat alınmış; veresiye kaldırılırsa borç %s eksiye düşer. Önce Veresiye ekranından tahsilatı iptal edin.",
			a.CreditCustomer, money.TL(-(bal - (v - keep))))
	}
	return nil
}

// CreditBriefs ödeme penceresinde ad önerisi için müşterilerdir (borcu olanlar önce).
func creditBriefs(q querier) ([]CreditBrief, int64, error) {
	list, err := loadCustomers(q)
	if err != nil {
		return nil, 0, err
	}
	sortCustomers(list)
	out := make([]CreditBrief, 0, len(list))
	var total int64
	for _, c := range list {
		out = append(out, CreditBrief{ID: c.ID, Name: c.Name, Balance: c.Balance})
		total += c.Balance
	}
	return out, total, nil
}

func sortCustomers(list []CreditCustomer) {
	sort.SliceStable(list, func(i, j int) bool {
		bi, bj := list[i].Balance > 0, list[j].Balance > 0
		if bi != bj {
			return bi
		}
		if bi && list[i].LastAt != list[j].LastAt {
			return list[i].LastAt < list[j].LastAt // en eski borç önce
		}
		return trFold(list[i].Name) < trFold(list[j].Name)
	})
}

// Credits Veresiye ekranının listesidir. Yönetici girişi istemez (kasada
// tahsilat alınır); tahsilat iptali yönetici şifresi ister.
func (s *Service) Credits() (*CreditList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := loadCustomers(s.db)
	if err != nil {
		return nil, err
	}
	sortCustomers(list)
	out := &CreditList{Customers: list}
	for _, c := range list {
		if c.Balance > 0 {
			out.Debtors++
			out.Balance += c.Balance
		}
	}
	return out, nil
}

// CreditAccount müşterinin hesap dökümünü verir.
func (s *Service) CreditAccount(id int64) (*CreditDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creditAccount(s.db, id)
}

func (s *Service) creditAccount(q querier, id int64) (*CreditDetail, error) {
	c, err := loadCustomer(q, id)
	if err != nil {
		return nil, err
	}
	d := &CreditDetail{Customer: *c, Items: []CreditItem{}}
	rows, err := q.Query(`SELECT a.no, a.business_date, COALESCE(a.closed_at,''), a.total, p.amount FROM adisyon_payments p
		JOIN adisyons a ON a.no=p.adisyon_no WHERE a.status='paid' AND p.method='veresiye' AND a.credit_customer_id=?`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		it := CreditItem{Kind: "veresiye"}
		if err := rows.Scan(&it.No, &it.Date, &it.At, &it.Total, &it.Amount); err != nil {
			rows.Close()
			return nil, err
		}
		d.Items = append(d.Items, it)
	}
	rows.Close()
	rows, err = q.Query(`SELECT id, business_date, at, amount, method, note, COALESCE(cancelled_at,''), COALESCE(cancel_reason,'')
		FROM credit_payments WHERE customer_id=?`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		it := CreditItem{Kind: "tahsilat"}
		if err := rows.Scan(&it.ID, &it.Date, &it.At, &it.Amount, &it.Method, &it.Note, &it.CancelAt, &it.Reason); err != nil {
			rows.Close()
			return nil, err
		}
		it.Label = PaymentLabel(it.Method)
		it.Cancelled = it.CancelAt != ""
		d.Items = append(d.Items, it)
	}
	rows.Close()
	sort.SliceStable(d.Items, func(i, j int) bool { return d.Items[i].At < d.Items[j].At })
	var bal int64
	closed := map[string]bool{}
	for i := range d.Items {
		it := &d.Items[i]
		switch {
		case it.Kind == "veresiye":
			bal += it.Amount
		case !it.Cancelled:
			bal -= it.Amount
		}
		it.Balance = bal
		if v, ok := closed[it.Date]; ok {
			it.DayClosed = v
		} else {
			c, _, err := dayClosed(q, it.Date)
			if err != nil {
				return nil, err
			}
			closed[it.Date], it.DayClosed = c, c
		}
	}
	// Yeniden eskiye.
	for i, j := 0, len(d.Items)-1; i < j; i, j = i+1, j-1 {
		d.Items[i], d.Items[j] = d.Items[j], d.Items[i]
	}
	return d, nil
}

// CollectCredit müşterinin borcundan tahsilat alır (tamamı ya da bir kısmı).
// Tahsilat yeni adisyonların yazıldığı açık iş gününe yazılır.
func (s *Service) CollectCredit(id, amount int64, method, note string) (*CreditDetail, *State, error) {
	s.mu.Lock()
	s.creditOK = false // veresiye listesi yenilensin
	s.daysVer++        // tahsilatlı günler gün sonu hatırlatmasını etkiler
	defer s.mu.Unlock()
	if !validCollect(method) {
		return nil, nil, uerr("invalid", "Tahsilat şeklini seçin: Nakit, Kredi Kartı ya da Havale.")
	}
	note, err := clean(singleLine(note), 200, "Not")
	if err != nil {
		return nil, nil, err
	}
	if amount <= 0 {
		return nil, nil, uerr("invalid", "Tahsil edilen tutarı yazın.")
	}
	var d *CreditDetail
	err = s.withTx(func(tx *sql.Tx) error {
		c, err := loadCustomer(tx, id)
		if err != nil {
			return err
		}
		if c.Balance <= 0 {
			return uerr("invalid", "%s adlı müşterinin borcu yok.", c.Name)
		}
		if amount > c.Balance {
			return uerr("invalid", "Tutar borçtan fazla: borç %s.", money.TL(c.Balance))
		}
		now := s.now()
		day, err := openBusinessDay(tx, now)
		if err != nil {
			return err
		}
		res, err := tx.Exec(`INSERT INTO credit_payments(customer_id,amount,method,at,business_date,note) VALUES(?,?,?,?,?,?)`,
			id, amount, method, stamp(now), day, note)
		if err != nil {
			return err
		}
		pid, _ := res.LastInsertId()
		audit(tx, now, "veresiye_tahsil", 0, map[string]any{"musteri": c.Name, "tutar": amount, "odeme": method,
			"kalan": c.Balance - amount, "not": note, "tahsilat": pid})
		d, err = s.creditAccount(tx, id)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	st, err := s.stateLocked()
	return d, st, err
}

// CancelCollection yanlış girilen tahsilatı iptal eder (yönetici şifresi ve
// sebep); günü kapatılmış tahsilat iptal edilmez.
func (s *Service) CancelCollection(paymentID int64, reason, pin string) (*CreditDetail, *State, error) {
	s.mu.Lock()
	s.creditOK = false // veresiye listesi yenilensin
	s.daysVer++        // tahsilatlı günler gün sonu hatırlatmasını etkiler
	defer s.mu.Unlock()
	reason, err := required(singleLine(reason), 200, "Sebep")
	if err != nil {
		return nil, nil, err
	}
	var cid, amount int64
	var day, method, cancelled string
	err = s.db.QueryRow(`SELECT customer_id, amount, business_date, method, COALESCE(cancelled_at,'') FROM credit_payments WHERE id=?`, paymentID).
		Scan(&cid, &amount, &day, &method, &cancelled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, uerr("not_found", "Tahsilat bulunamadı.")
	}
	if err != nil {
		return nil, nil, err
	}
	if cancelled != "" {
		return nil, nil, uerr("invalid", "Tahsilat zaten iptal edilmiş.")
	}
	if closed, _, err := dayClosed(s.db, day); err != nil {
		return nil, nil, err
	} else if closed {
		return nil, nil, uerr("closed", "Tahsilatın günü (%s) kapatılmış; iptal edilemez.", dateTRLong(day))
	}
	if err := s.confirmPin(pin, "Tahsilat iptali yönetici şifresi ister."); err != nil {
		return nil, nil, err
	}
	var d *CreditDetail
	err = s.withTx(func(tx *sql.Tx) error {
		now := s.now()
		if _, err := tx.Exec(`UPDATE credit_payments SET cancelled_at=?, cancel_reason=? WHERE id=? AND cancelled_at IS NULL`, stamp(now), reason, paymentID); err != nil {
			return err
		}
		c, err := loadCustomer(tx, cid)
		if err != nil {
			return err
		}
		audit(tx, now, "veresiye_tahsil_iptal", 0, map[string]any{"musteri": c.Name, "tutar": amount, "odeme": method, "sebep": reason,
			"kalan": c.Balance})
		d, err = s.creditAccount(tx, cid)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	st, err := s.stateLocked()
	return d, st, err
}

// SaveCreditCustomer müşterinin adını, telefonunu ve notunu düzeltir.
func (s *Service) SaveCreditCustomer(id int64, name, phone, note string) (*CreditDetail, *State, error) {
	s.mu.Lock()
	s.creditOK = false // veresiye listesi yenilensin
	defer s.mu.Unlock()
	name, err := cleanName(name)
	if err != nil {
		return nil, nil, err
	}
	if phone, err = clean(singleLine(phone), 30, "Telefon"); err != nil {
		return nil, nil, err
	}
	if note, err = clean(singleLine(note), 200, "Not"); err != nil {
		return nil, nil, err
	}
	var d *CreditDetail
	err = s.withTx(func(tx *sql.Tx) error {
		c, err := loadCustomer(tx, id)
		if err != nil {
			return err
		}
		rows, err := tx.Query(`SELECT id, name FROM credit_customers WHERE id<>?`, id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var oid int64
			var n string
			if err := rows.Scan(&oid, &n); err != nil {
				rows.Close()
				return err
			}
			if trFold(n) == trFold(name) {
				rows.Close()
				return uerr("invalid", "“%s” adında başka bir müşteri var.", n)
			}
		}
		rows.Close()
		if _, err := tx.Exec(`UPDATE credit_customers SET name=?, phone=?, note=? WHERE id=?`, name, phone, note, id); err != nil {
			return err
		}
		if c.Name != name || c.Phone != phone || c.Note != note {
			audit(tx, s.now(), "veresiye_musteri_degisti", 0, map[string]any{"eski_ad": c.Name, "ad": name, "telefon": phone})
		}
		d, err = s.creditAccount(tx, id)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	st, err := s.stateLocked()
	return d, st, err
}
