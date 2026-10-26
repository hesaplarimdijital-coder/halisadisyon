package core

import (
	"database/sql"

	"halisadisyon/internal/money"
)

const maxUndo = 30

func (s *Service) withTx(fn func(tx *sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) pushUndo(no int64, lines []Line) {
	cp := make([]Line, len(lines))
	copy(cp, lines)
	st := append(s.undo[no], cp)
	if len(st) > maxUndo {
		st = st[len(st)-maxUndo:]
	}
	s.undo[no] = st
}

func pickWaiter(q querier, id int64) (*Waiter, error) {
	ws, err := loadWaiters(q, true)
	if err != nil {
		return nil, err
	}
	if len(ws) == 0 {
		return nil, uerr("invalid", "Garson listesi boş. Yöneticiden Ayarlar › Garsonlar bölümüne garson eklemesini isteyin.")
	}
	for i := range ws {
		if ws[i].ID == id {
			return &ws[i], nil
		}
	}
	return &ws[0], nil
}

func loadActiveOrFail(q querier, no int64) (*Adisyon, error) {
	a, err := loadAdisyon(q, no)
	if err != nil {
		return nil, err
	}
	if !a.active() {
		return nil, uerr("closed", "Adisyon %d kapanmış; masalara dönün.", no)
	}
	return a, nil
}

// AddItem masaya bir ürün ekler. Masada açık adisyon yoksa yeni numara verilir
// (numara ilk ürün eklenince alınır, boş masa numara harcamaz).
func (s *Service) AddItem(table int, productID, waiterID int64) (*State, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if table < 1 || table > s.set.Tables {
		return nil, 0, uerr("invalid", "Masa %d yok. Masa sayısı Ayarlar'dan değiştirilebilir.", table)
	}
	var no int64
	var before []Line
	err := s.withTx(func(tx *sql.Tx) error {
		p, err := loadProduct(tx, productID)
		if err != nil {
			return err
		}
		if !p.Active {
			return uerr("invalid", "%s şu an satışta değil.", p.Name)
		}
		if p.Price <= 0 {
			return uerr("invalid", "%s için fiyat girilmemiş. Yöneticiden Ürünler ekranında fiyatı girmesini isteyin.", p.Name)
		}
		a, err := activeForTable(tx, table)
		if err != nil {
			return err
		}
		if a != nil && s.inPrint[a.No] {
			return errBusy(a.No)
		}
		now := s.now()
		if a == nil {
			w, err := pickWaiter(tx, waiterID)
			if err != nil {
				return err
			}
			if no, err = nextNo(tx); err != nil {
				return err
			}
			day, err := openBusinessDay(tx, now)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO adisyons(no,business_date,table_no,waiter_id,waiter_name,status,opened_at)
				VALUES(?,?,?,?,?,'open',?)`, no, day, table, w.ID, w.Name, stamp(now)); err != nil {
				return err
			}
			audit(tx, now, "adisyon_acildi", no, map[string]any{"masa": table, "garson": w.Name})
			a = &Adisyon{No: no, Status: StatusOpen, Lines: []Line{}}
		} else {
			no = a.No
		}
		before = a.Lines
		vat := s.set.DefaultVAT
		if p.VAT != nil {
			vat = *p.VAT
		}
		var lineID int64
		for _, l := range a.Lines {
			if l.ProductID == p.ID && l.Price == p.Price && l.VAT == vat && !l.Comp {
				lineID = l.ID
			}
		}
		if lineID > 0 {
			if _, err := tx.Exec(`UPDATE adisyon_lines SET qty=qty+1 WHERE id=?`, lineID); err != nil {
				return err
			}
		} else {
			var pos int
			if err := tx.QueryRow(`SELECT COALESCE(MAX(pos),0)+1 FROM adisyon_lines WHERE adisyon_no=?`, no).Scan(&pos); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO adisyon_lines(adisyon_no,pos,product_id,name,unit,price,vat,qty)
				VALUES(?,?,?,?,?,?,?,1)`, no, pos, p.ID, p.Name, p.Unit, p.Price, vat); err != nil {
				return err
			}
		}
		if a.Status == StatusPrinted {
			if _, err := tx.Exec(`UPDATE adisyons SET changed=1 WHERE no=?`, no); err != nil {
				return err
			}
		}
		audit(tx, now, "kalem", no, map[string]any{"urun": p.Name, "fark": 1})
		return recalc(tx, no)
	})
	if err != nil {
		return nil, 0, err
	}
	s.pushUndo(no, before)
	st, err := s.stateLocked()
	return st, no, err
}

// ChangeQty kalemin adedini artırır ya da azaltır; adet sıfırlanınca kalem kalkar.
// Basılmış adetin altına inmek (müşteriye verilen fişten ürün düşmek) yönetici
// şifresi ister.
func (s *Service) ChangeQty(no, lineID int64, delta int, pin string) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if delta == 0 || delta > 50 || delta < -50 {
		return nil, uerr("invalid", "Geçersiz adet değişikliği.")
	}
	if s.inPrint[no] {
		return nil, errBusy(no)
	}
	cur, err := loadActiveOrFail(s.db, no)
	if err != nil {
		return nil, err
	}
	for _, l := range cur.Lines {
		if l.ID == lineID && delta < 0 && l.Qty+delta < l.PrintedQty {
			if err := s.confirmPin(pin, "Basılmış adisyondan ürün düşmek yönetici şifresi ister."); err != nil {
				return nil, err
			}
		}
	}
	var before []Line
	err = s.withTx(func(tx *sql.Tx) error {
		a, err := loadActiveOrFail(tx, no)
		if err != nil {
			return err
		}
		var line *Line
		for i := range a.Lines {
			if a.Lines[i].ID == lineID {
				line = &a.Lines[i]
			}
		}
		if line == nil {
			return uerr("not_found", "Kalem bulunamadı; ekran yenilendi.")
		}
		before = a.Lines
		nq := line.Qty + delta
		if nq <= 0 {
			_, err = tx.Exec(`DELETE FROM adisyon_lines WHERE id=?`, lineID)
		} else {
			_, err = tx.Exec(`UPDATE adisyon_lines SET qty=? WHERE id=?`, nq, lineID)
		}
		if err != nil {
			return err
		}
		if a.Status == StatusPrinted {
			if _, err := tx.Exec(`UPDATE adisyons SET changed=1 WHERE no=?`, no); err != nil {
				return err
			}
		}
		if nq < 0 {
			nq = 0
		}
		detail := map[string]any{"urun": line.Name, "fark": delta, "adet": nq}
		if nq < line.PrintedQty {
			detail["basilan_adet"] = line.PrintedQty
		}
		audit(tx, s.now(), "kalem", no, detail)
		return recalc(tx, no)
	})
	if err != nil {
		return nil, err
	}
	s.pushUndo(no, before)
	return s.stateLocked()
}

// Undo adisyondaki son kalem değişikliğini geri alır.
func (s *Service) Undo(no int64) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inPrint[no] {
		return nil, errBusy(no)
	}
	stack := s.undo[no]
	if len(stack) == 0 {
		return nil, uerr("invalid", "Geri alınacak işlem yok.")
	}
	snap := stack[len(stack)-1]
	err := s.withTx(func(tx *sql.Tx) error {
		a, err := loadActiveOrFail(tx, no)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM adisyon_lines WHERE adisyon_no=?`, no); err != nil {
			return err
		}
		for i, l := range snap {
			var pid sql.NullInt64
			if l.ProductID > 0 {
				pid = sql.NullInt64{Int64: l.ProductID, Valid: true}
			}
			if _, err := tx.Exec(`INSERT INTO adisyon_lines(adisyon_no,pos,product_id,name,unit,price,vat,qty,printed_qty,comp)
				VALUES(?,?,?,?,?,?,?,?,?,?)`, no, i+1, pid, l.Name, l.Unit, l.Price, l.VAT, l.Qty, l.PrintedQty, l.Comp); err != nil {
				return err
			}
		}
		if a.Status == StatusPrinted {
			if _, err := tx.Exec(`UPDATE adisyons SET changed=1 WHERE no=?`, no); err != nil {
				return err
			}
		}
		audit(tx, s.now(), "geri_al", no, nil)
		return recalc(tx, no)
	})
	if err != nil {
		return nil, err
	}
	s.undo[no] = stack[:len(stack)-1]
	return s.stateLocked()
}

// CanUndo adisyonda geri alınabilecek işlem sayısını verir.
func (s *Service) undoCounts() map[int64]int {
	out := map[int64]int{}
	for no, st := range s.undo {
		if len(st) > 0 {
			out[no] = len(st)
		}
	}
	return out
}

// CloseEmpty ürünleri silinmiş (boş) adisyonu kapatır; numara iptal olarak kalır.
// Adisyon daha önce basıldıysa bu bir iptal sayılır ve yönetici şifresi ister.
func (s *Service) CloseEmpty(no int64, pin string) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inPrint[no] {
		return nil, errBusy(no)
	}
	a, err := loadActiveOrFail(s.db, no)
	if err != nil {
		return nil, err
	}
	if len(a.Lines) > 0 {
		return nil, uerr("invalid", "Adisyonda ürün var. Ödendiyse “Hesap Ödendi”, yanlışsa yöneticiye iptal ettirin.")
	}
	reason := "Ürünler silindi, masa boşaltıldı"
	if a.PrintCount > 0 {
		if err := s.confirmPin(pin, "Basılmış adisyonu boşaltmak yönetici şifresi ister."); err != nil {
			return nil, err
		}
		reason = "Basıldıktan sonra ürünler silindi, masa boşaltıldı"
	}
	err = s.withTx(func(tx *sql.Tx) error {
		now := s.now()
		res, err := tx.Exec(`UPDATE adisyons SET status='cancelled', closed_at=?, cancel_reason=? WHERE no=? AND status IN ('open','printed')
			AND NOT EXISTS (SELECT 1 FROM adisyon_lines WHERE adisyon_no=?)`, stamp(now), reason, no, no)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return uerr("closed", "Adisyon %d bu arada değişti; ekran yenilendi.", no)
		}
		audit(tx, now, "bos_kapatildi", no, map[string]any{"masa": a.Table, "baski": a.PrintCount})
		return nil
	})
	if err != nil {
		return nil, err
	}
	delete(s.undo, no)
	return s.stateLocked()
}

// ChangeWaiter adisyonun garsonunu değiştirir.
func (s *Service) ChangeWaiter(no, waiterID int64) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.withTx(func(tx *sql.Tx) error {
		a, err := loadActiveOrFail(tx, no)
		if err != nil {
			return err
		}
		ws, err := loadWaiters(tx, true)
		if err != nil {
			return err
		}
		var w *Waiter
		for i := range ws {
			if ws[i].ID == waiterID {
				w = &ws[i]
			}
		}
		if w == nil {
			return uerr("not_found", "Garson bulunamadı.")
		}
		if _, err := tx.Exec(`UPDATE adisyons SET waiter_id=?, waiter_name=? WHERE no=?`, w.ID, w.Name, no); err != nil {
			return err
		}
		audit(tx, s.now(), "garson_degisti", no, map[string]any{"eski": a.WaiterName, "yeni": w.Name})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.stateLocked()
}

// Cancel adisyonu sebep yazarak iptal eder. Her iptal için yönetici şifresi
// ayrıca istenir (açık yönetici girişi yetmez). Silme yoktur. Ödenmiş adisyon
// da (günü kapatılmadıysa) iptal edilebilir; ödeme kaydı kalkar, işlem kaydında
// durur.
func (s *Service) Cancel(no int64, reason, pin string) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reason, err := required(singleLine(reason), 200, "İptal sebebi")
	if err != nil {
		return nil, err
	}
	if s.inPrint[no] {
		return nil, errBusy(no)
	}
	if err := s.confirmPin(pin, "Adisyon iptali yönetici şifresi ister."); err != nil {
		return nil, err
	}
	err = s.withTx(func(tx *sql.Tx) error {
		a, err := loadAdisyon(tx, no)
		if err != nil {
			return err
		}
		if a.Status == StatusCancelled {
			return uerr("invalid", "Adisyon %d zaten iptal edilmiş.", no)
		}
		if closed, _, err := dayClosed(tx, a.BusinessDate); err != nil {
			return err
		} else if closed {
			return uerr("closed", "Adisyon %d'in günü kapatılmış; değiştirilemez.", no)
		}
		now := s.now()
		if a.Status == StatusPaid {
			if err := checkCreditRemoval(tx, a, 0); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE adisyons SET status='cancelled', closed_at=?, cancel_reason=? WHERE no=?`,
			stamp(now), reason, no); err != nil {
			return err
		}
		if err := clearPayments(tx, no); err != nil {
			return err
		}
		d := map[string]any{"sebep": reason, "eski_durum": a.Status, "tutar": a.Total, "masa": a.Table}
		if a.Status == StatusPaid {
			d["eski_odeme"] = a.PaymentMethod
			if len(a.Payments) > 1 {
				d["eski_parcalar"] = PaymentText(a.Payments, "")
			}
			if a.CreditCustomer != "" {
				d["musteri"] = a.CreditCustomer
			}
		}
		audit(tx, now, "iptal", no, d)
		return nil
	})
	if err != nil {
		return nil, err
	}
	delete(s.undo, no)
	return s.stateLocked()
}

// Pay hesabı ödeme şekliyle (nakit, kart, havale) kapatır. Ödeme şekli yalnız
// programda adisyona kaydedilir; adisyon fişi ödemeden önce basıldığı için fişte
// yer almaz. Hesap kapatılırken baskı yapılmaz (önce “Adisyonu Bas”).
func (s *Service) Pay(no int64, method string) (*PayOutcome, *State, error) {
	return s.PayChecked(no, method, -1)
}

// PayChecked hesabı tek ödeme şekliyle kapatır; expected ≥ 0 ise ekranda
// gösterilen toplam kayıttakiyle aynı olmalıdır (bu arada ürün eklendiyse
// kapatılmaz). Veresiye için PayParts kullanılır (müşteri adı gerekir).
func (s *Service) PayChecked(no int64, method string, expected int64) (*PayOutcome, *State, error) {
	return s.PayParts(no, []PaymentPart{{Method: method}}, nil, expected)
}

// PayParts hesabı bir ya da birden çok ödeme şekliyle kapatır. Tek parça
// adisyonun tamamıdır; parçalı ödemede parçaların toplamı hesaba eşit olmalıdır.
// Veresiye parçası varsa credit müşteriyi verir (adıyla bulunur ya da açılır).
func (s *Service) PayParts(no int64, parts []PaymentPart, credit *CreditRef, expected int64) (*PayOutcome, *State, error) {
	for _, p := range parts {
		if !validPayment(p.Method) {
			return nil, nil, uerr("invalid", "Ödeme şeklini seçin: Nakit, Kredi Kartı, Havale ya da Veresiye.")
		}
	}
	if len(parts) == 0 {
		return nil, nil, uerr("invalid", "Ödeme şeklini seçin: Nakit, Kredi Kartı, Havale ya da Veresiye.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Fiş basılırken kapatılmaz: basılan ile kapanan tutar aynı olsun.
	if s.inPrint[no] {
		return nil, nil, errBusy(no)
	}
	out := &PayOutcome{Paid: true, No: no}
	err := s.withTx(func(tx *sql.Tx) error {
		a, err := loadActiveOrFail(tx, no)
		if err != nil {
			return err
		}
		if len(a.Lines) == 0 {
			return uerr("invalid", "Adisyonda ürün yok. Masayı boşaltmak için “Masayı boşalt”ı kullanın.")
		}
		if expected >= 0 && expected != a.Total {
			return uerr("changed", "Adisyon bu arada değişti; yeni toplam %s. Kontrol edip yeniden kapatın.", money.TL(a.Total))
		}
		ps, err := normalizeParts(parts, a.Total)
		if err != nil {
			return err
		}
		now := s.now()
		var cid int64
		if creditPart(ps) > 0 {
			if cid, out.Customer, err = resolveCustomer(tx, credit, now); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE adisyons SET status='paid', closed_at=? WHERE no=?`, stamp(now), no); err != nil {
			return err
		}
		if err := writePayments(tx, no, ps, cid); err != nil {
			return err
		}
		for i := range ps {
			ps[i].Label = PaymentLabel(ps[i].Method)
		}
		out.Total, out.Parts, out.Method = a.Total, ps, methodOf(ps)
		out.MethodLabel = PaymentText(ps, "")
		d := map[string]any{"tutar": a.Total, "basildi": a.PrintCount > 0, "degisti": a.PrintCount > 0 && a.Changed,
			"masa": a.Table, "odeme": out.Method}
		if len(ps) > 1 {
			d["parcalar"] = PaymentText(ps, "")
		}
		if out.Customer != "" {
			d["musteri"] = out.Customer
			d["veresiye"] = creditPart(ps)
		}
		audit(tx, now, "odendi", no, d)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	delete(s.undo, no)
	st, err := s.stateLocked()
	return out, st, err
}

func errBusy(no int64) error {
	return uerr("busy", "Adisyon %d şu an basılıyor ya da kapanıyor; bitmesini bekleyin.", no)
}
