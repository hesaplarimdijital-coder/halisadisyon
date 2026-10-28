package core

import "database/sql"

// DiscountPresets sipariş ekranındaki hazır indirim yüzdeleridir.
var DiscountPresets = []int{5, 10, 15, 20, 25, 50}

// SetDiscount adisyonun tamamına yüzde indirim uygular; 0 indirimi kaldırır.
// İndirim uygulamak yönetici şifresi ister (Ayarlar'dan kapatılabilir);
// kaldırmak istemez. İndirim fişe "İndirim (%10)" satırı olarak basılır ve
// basılmış adisyonda "tekrar bas" uyarısı çıkarır.
func (s *Service) SetDiscount(no int64, percent int, pin string) (*State, error) {
	if percent < 0 || percent > 100 {
		return nil, uerr("invalid", "İndirim yüzdesi 1 ile 100 arasında olmalı.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inPrint[no] {
		return nil, errBusy(no)
	}
	cur, err := loadActiveOrFail(s.db, no)
	if err != nil {
		return nil, err
	}
	if cur.DiscountPercent == percent {
		return s.stateLocked()
	}
	if percent > 0 && !s.set.DiscountNoPIN {
		if err := s.confirmPin(pin, "İndirim uygulamak yönetici şifresi ister."); err != nil {
			return nil, err
		}
	}
	err = s.withTx(func(tx *sql.Tx) error {
		a, err := loadActiveOrFail(tx, no)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE adisyons SET discount_percent=? WHERE no=?`, percent, no); err != nil {
			return err
		}
		if a.Status == StatusPrinted {
			if _, err := tx.Exec(`UPDATE adisyons SET changed=1 WHERE no=?`, no); err != nil {
				return err
			}
		}
		if err := recalc(tx, no); err != nil {
			return err
		}
		_, subtotal, discount, total, _ := Breakdown(a.Lines, percent)
		now := s.now()
		if percent == 0 {
			audit(tx, now, "indirim_kaldirildi", no, map[string]any{"eski_yuzde": a.DiscountPercent, "toplam": total, "masa": a.Table})
		} else {
			audit(tx, now, "indirim", no, map[string]any{"yuzde": percent, "eski_yuzde": a.DiscountPercent, "tutar": discount,
				"ara_toplam": subtotal, "toplam": total, "masa": a.Table})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.stateLocked()
}

// Reopen ödenmiş adisyonu düzeltmek için masaya geri açar: yanlış adet ya da
// ürünle kapatılmış hesap yönetici şifresi ve sebep yazılarak açılır, düzeltilir,
// gerekirse yeniden basılır ve yeniden ödenir. Ödeme kaydı kalkar (işlem
// kaydında durur); numara ve açılış saati aynı kalır. Kapatılmış günün adisyonu
// açılamaz. table 0 ise adisyon kendi masasına döner; masa doluysa boş bir masa
// seçilmelidir.
func (s *Service) Reopen(no int64, table int, reason, pin string) (*State, int, error) {
	s.mu.Lock()
	s.creditOK = false // veresiye listesi yenilensin
	defer s.mu.Unlock()
	reason, err := required(singleLine(reason), 200, "Sebep")
	if err != nil {
		return nil, 0, err
	}
	if table != 0 && (table < 1 || table > s.set.Tables) {
		return nil, 0, uerr("invalid", "Masa %d yok.", table)
	}
	if err := s.confirmPin(pin, "Ödenmiş adisyonu yeniden açmak yönetici şifresi ister."); err != nil {
		return nil, 0, err
	}
	err = s.withTx(func(tx *sql.Tx) error {
		a, err := loadAdisyon(tx, no)
		if err != nil {
			return err
		}
		if a.Status != StatusPaid {
			return uerr("invalid", "Yalnız ödenmiş adisyon yeniden açılır (No %d: %s).", no, statusText(a))
		}
		if closed, _, err := dayClosed(tx, a.BusinessDate); err != nil {
			return err
		} else if closed {
			return uerr("closed", "Adisyon %d'in günü kapatılmış; değiştirilemez.", no)
		}
		if table == 0 {
			table = a.Table
		}
		if table > s.set.Tables {
			return uerr("invalid", "Masa %d artık yok; başka masa seçin.", table)
		}
		if busy, err := activeForTable(tx, table); err != nil {
			return err
		} else if busy != nil {
			return uerr("invalid", "Masa %d dolu (No %d). Boş bir masa seçin.", table, busy.No)
		}
		status := StatusOpen
		if a.PrintCount > 0 {
			status = StatusPrinted
		}
		if err := checkCreditRemoval(tx, a, 0); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE adisyons SET status=?, closed_at=NULL, table_no=? WHERE no=? AND status='paid'`,
			status, table, no); err != nil {
			return err
		}
		if err := clearPayments(tx, no); err != nil {
			return err
		}
		d := map[string]any{"sebep": reason, "eski_odeme": a.PaymentMethod, "tutar": a.Total, "masa": table, "eski_masa": a.Table}
		if len(a.Payments) > 1 {
			d["eski_parcalar"] = PaymentText(a.Payments, "")
		}
		if a.CreditCustomer != "" {
			d["musteri"] = a.CreditCustomer
		}
		audit(tx, s.now(), "yeniden_acildi", no, d)
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	delete(s.undo, no)
	st, err := s.stateLocked()
	return st, table, err
}

// SetComp kalemin adedinin bir kısmını ya da tümünü ikram yapar (ücret alınmaz)
// ya da ikramı kaldırır. Kalem adisyonda ve fişte "İKRAM" olarak kalır, tutara
// ve KDV'ye girmez. İkram yapmak indirimle aynı kuralla yönetici şifresi ister;
// kaldırmak istemez. Kısmi ikramda kalem ikiye bölünür (ücretli ve ikram);
// ikram kaldırılınca aynı ürünün ücretli kalemine geri eklenir.
func (s *Service) SetComp(no, lineID int64, qty int, comp bool, pin string) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inPrint[no] {
		return nil, errBusy(no)
	}
	cur, err := loadActiveOrFail(s.db, no)
	if err != nil {
		return nil, err
	}
	var line *Line
	for i := range cur.Lines {
		if cur.Lines[i].ID == lineID {
			line = &cur.Lines[i]
		}
	}
	if line == nil {
		return nil, uerr("not_found", "Kalem bulunamadı; ekran yenilendi.")
	}
	if line.Comp == comp {
		return s.stateLocked()
	}
	if qty <= 0 || qty > line.Qty {
		qty = line.Qty
	}
	if comp && !s.set.DiscountNoPIN {
		if err := s.confirmPin(pin, "İkram yönetici şifresi ister."); err != nil {
			return nil, err
		}
	}
	err = s.withTx(func(tx *sql.Tx) error {
		a, err := loadActiveOrFail(tx, no)
		if err != nil {
			return err
		}
		var l *Line
		var other *Line // aynı ürünün karşı türdeki (ikram/ücretli) kalemi
		for i := range a.Lines {
			x := &a.Lines[i]
			if x.ID == lineID {
				l = x
			} else if x.ProductID == line.ProductID && x.Price == line.Price && x.VAT == line.VAT && x.Name == line.Name && x.Comp == comp {
				other = x
			}
		}
		if l == nil || l.Comp != line.Comp || l.Qty < qty {
			return uerr("changed", "Kalem bu arada değişti; ekran yenilendi.")
		}
		if qty == l.Qty {
			// Kalemin tümü: varsa karşı kaleme katılır, yoksa kalemin türü değişir.
			if other != nil {
				if _, err := tx.Exec(`UPDATE adisyon_lines SET qty=qty+?, printed_qty=printed_qty+? WHERE id=?`, l.Qty, l.PrintedQty, other.ID); err != nil {
					return err
				}
				if _, err := tx.Exec(`DELETE FROM adisyon_lines WHERE id=?`, l.ID); err != nil {
					return err
				}
			} else if _, err := tx.Exec(`UPDATE adisyon_lines SET comp=? WHERE id=?`, comp, l.ID); err != nil {
				return err
			}
		} else {
			// Bir kısmı: kalem küçülür, ayrılan adet karşı kaleme gider (yoksa yeni kalem).
			if _, err := tx.Exec(`UPDATE adisyon_lines SET qty=qty-? WHERE id=?`, qty, l.ID); err != nil {
				return err
			}
			if other != nil {
				if _, err := tx.Exec(`UPDATE adisyon_lines SET qty=qty+? WHERE id=?`, qty, other.ID); err != nil {
					return err
				}
			} else {
				var pos int
				if err := tx.QueryRow(`SELECT COALESCE(MAX(pos),0)+1 FROM adisyon_lines WHERE adisyon_no=?`, no).Scan(&pos); err != nil {
					return err
				}
				var pid sql.NullInt64
				if l.ProductID > 0 {
					pid = sql.NullInt64{Int64: l.ProductID, Valid: true}
				}
				if _, err := tx.Exec(`INSERT INTO adisyon_lines(adisyon_no,pos,product_id,name,unit,price,vat,qty,printed_qty,comp)
					VALUES(?,?,?,?,?,?,?,?,0,?)`, no, pos, pid, l.Name, l.Unit, l.Price, l.VAT, qty, comp); err != nil {
					return err
				}
			}
		}
		if a.Status == StatusPrinted {
			if _, err := tx.Exec(`UPDATE adisyons SET changed=1 WHERE no=?`, no); err != nil {
				return err
			}
		}
		if err := recalc(tx, no); err != nil {
			return err
		}
		action := "ikram"
		if !comp {
			action = "ikram_kaldirildi"
		}
		audit(tx, s.now(), action, no, map[string]any{"urun": l.Name, "adet": qty, "deger": l.Price * int64(qty), "masa": a.Table})
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.pushUndo(no, cur.Lines)
	return s.stateLocked()
}
