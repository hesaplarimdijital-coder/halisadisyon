package core

import (
	"database/sql"
	"strings"

	"halisadisyon/internal/money"
)

// Ödeme şekilleri. Veritabanında kısa kod, ekranda ve raporda Türkçe ad kullanılır.
const (
	PayCash     = "nakit"
	PayCard     = "kart"
	PayTransfer = "havale"
	// PayCredit veresiye: ciroya yazılır, kasaya girmez; müşterinin borcuna eklenir.
	PayCredit = "veresiye"
	// PaySplit adisyons.payment_method'da parçalı ödemeyi gösterir (parçalar adisyon_payments'ta).
	PaySplit = "parcali"
)

// PaymentMethod ödeme şeklidir (arayüz için).
type PaymentMethod struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// PaymentMethods ekranda gösterilen sırayla ödeme şekilleridir.
var PaymentMethods = []PaymentMethod{
	{PayCash, "Nakit"},
	{PayCard, "Kredi Kartı"},
	{PayTransfer, "Havale"},
	{PayCredit, "Veresiye"},
}

// CollectMethods veresiye tahsilatında kullanılabilen (kasaya giren) ödeme şekilleridir.
var CollectMethods = []PaymentMethod{
	{PayCash, "Nakit"},
	{PayCard, "Kredi Kartı"},
	{PayTransfer, "Havale"},
}

func validPayment(m string) bool {
	for _, p := range PaymentMethods {
		if p.ID == m {
			return true
		}
	}
	return false
}

func validCollect(m string) bool { return m != PayCredit && validPayment(m) }

// PaymentLabel ödeme şeklinin Türkçe adını verir ("" → "").
func PaymentLabel(m string) string {
	for _, p := range PaymentMethods {
		if p.ID == m {
			return p.Label
		}
	}
	switch m {
	case "":
		return ""
	case PaySplit:
		return "Parçalı"
	}
	return "Bilinmiyor"
}

// PaymentPart ödenen adisyonun bir ödeme şekline düşen kısmıdır.
type PaymentPart struct {
	Method string `json:"method"`
	Label  string `json:"label,omitempty"`
	Amount int64  `json:"amount"`
}

// CreditRef veresiye müşterisidir: ID verilirse o müşteri, yoksa adıyla bulunur
// ya da yeni müşteri açılır.
type CreditRef struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

// PaymentText ödeme parçalarını okunur yazar: "Nakit" ya da "Nakit 500,00 ₺ + Kredi Kartı 300,00 ₺".
func PaymentText(parts []PaymentPart, fallback string) string {
	switch len(parts) {
	case 0:
		return PaymentLabel(fallback)
	case 1:
		return PaymentLabel(parts[0].Method)
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, PaymentLabel(p.Method)+" "+money.TL(p.Amount))
	}
	return strings.Join(out, " + ")
}

func creditPart(parts []PaymentPart) int64 {
	var v int64
	for _, p := range parts {
		if p.Method == PayCredit {
			v += p.Amount
		}
	}
	return v
}

// normalizeParts ödeme parçalarını denetler. Tek parça her zaman adisyonun
// tamamıdır; birden çok parçanın toplamı adisyon tutarına eşit olmalıdır.
// Tutarı sıfır olan parçalar atılır. Sıra PaymentMethods sırasıdır.
func normalizeParts(parts []PaymentPart, total int64) ([]PaymentPart, error) {
	if len(parts) == 0 {
		return nil, uerr("invalid", "Ödeme şeklini seçin.")
	}
	if len(parts) == 1 {
		if !validPayment(parts[0].Method) {
			return nil, uerr("invalid", "Ödeme şeklini seçin: Nakit, Kredi Kartı, Havale ya da Veresiye.")
		}
		if parts[0].Method == PayCredit && total == 0 {
			return nil, uerr("invalid", "Tutarı 0,00 ₺ olan adisyon veresiyeye yazılmaz.")
		}
		return []PaymentPart{{Method: parts[0].Method, Amount: total}}, nil
	}
	by := map[string]int64{}
	var sum int64
	for _, p := range parts {
		if !validPayment(p.Method) {
			return nil, uerr("invalid", "Bilinmeyen ödeme şekli.")
		}
		if p.Amount < 0 {
			return nil, uerr("invalid", "Tutar eksi olamaz.")
		}
		if _, dup := by[p.Method]; dup && p.Amount > 0 {
			return nil, uerr("invalid", "%s iki kez yazılmış.", PaymentLabel(p.Method))
		}
		by[p.Method] += p.Amount
		sum += p.Amount
	}
	if sum != total {
		if sum < total {
			return nil, uerr("invalid", "Parçaların toplamı %s; hesap %s. %s eksik.", money.TL(sum), money.TL(total), money.TL(total-sum))
		}
		return nil, uerr("invalid", "Parçaların toplamı %s; hesap %s. %s fazla.", money.TL(sum), money.TL(total), money.TL(sum-total))
	}
	out := []PaymentPart{}
	for _, m := range PaymentMethods {
		if v := by[m.ID]; v > 0 {
			out = append(out, PaymentPart{Method: m.ID, Amount: v})
		}
	}
	if len(out) == 0 { // 0,00 ₺'lik adisyon
		out = append(out, PaymentPart{Method: parts[0].Method, Amount: 0})
		if out[0].Method == PayCredit {
			out[0].Method = PayCash
		}
	}
	return out, nil
}

func methodOf(parts []PaymentPart) string {
	if len(parts) == 1 {
		return parts[0].Method
	}
	return PaySplit
}

// writePayments ödenmiş adisyonun parçalarını ve veresiye müşterisini yazar.
func writePayments(tx *sql.Tx, no int64, parts []PaymentPart, customerID int64) error {
	if _, err := tx.Exec(`DELETE FROM adisyon_payments WHERE adisyon_no=?`, no); err != nil {
		return err
	}
	for _, p := range parts {
		if _, err := tx.Exec(`INSERT INTO adisyon_payments(adisyon_no,method,amount) VALUES(?,?,?)`, no, p.Method, p.Amount); err != nil {
			return err
		}
	}
	var cid sql.NullInt64
	if customerID > 0 && creditPart(parts) > 0 {
		cid = sql.NullInt64{Int64: customerID, Valid: true}
	}
	_, err := tx.Exec(`UPDATE adisyons SET payment_method=?, credit_customer_id=? WHERE no=?`, methodOf(parts), cid, no)
	return err
}

// clearPayments iptal ya da yeniden açmada ödeme kaydını kaldırır.
func clearPayments(tx *sql.Tx, no int64) error {
	if _, err := tx.Exec(`DELETE FROM adisyon_payments WHERE adisyon_no=?`, no); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE adisyons SET payment_method=NULL, credit_customer_id=NULL WHERE no=?`, no)
	return err
}

// loadPayments adisyonun ödeme parçalarını okur.
func loadPayments(q querier, no int64) ([]PaymentPart, error) {
	rows, err := q.Query(`SELECT method, amount FROM adisyon_payments WHERE adisyon_no=? ORDER BY id`, no)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PaymentPart{}
	for rows.Next() {
		var p PaymentPart
		if err := rows.Scan(&p.Method, &p.Amount); err != nil {
			return nil, err
		}
		p.Label = PaymentLabel(p.Method)
		out = append(out, p)
	}
	return out, rows.Err()
}

// attachPayments ödenmiş adisyonların parçalarını tek sorguda doldurur.
func attachPayments(q querier, list []Adisyon, where string, args ...any) error {
	idx := map[int64]int{}
	for i := range list {
		idx[list[i].No] = i
		list[i].Payments = []PaymentPart{}
	}
	rows, err := q.Query(`SELECT p.adisyon_no, p.method, p.amount FROM adisyon_payments p JOIN adisyons a ON a.no=p.adisyon_no WHERE `+where+` ORDER BY p.id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var no int64
		var p PaymentPart
		if err := rows.Scan(&no, &p.Method, &p.Amount); err != nil {
			return err
		}
		p.Label = PaymentLabel(p.Method)
		if i, ok := idx[no]; ok {
			list[i].Payments = append(list[i].Payments, p)
		}
	}
	return rows.Err()
}

// partsOf eski (parçasız) kayıtta tek parçayı tamamlar.
func partsOf(a *Adisyon) []PaymentPart {
	if len(a.Payments) > 0 || a.Status != StatusPaid || a.PaymentMethod == "" || a.PaymentMethod == PaySplit {
		return a.Payments
	}
	return []PaymentPart{{Method: a.PaymentMethod, Label: PaymentLabel(a.PaymentMethod), Amount: a.Total}}
}
