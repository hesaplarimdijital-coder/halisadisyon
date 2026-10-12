package core

import (
	"database/sql"
	"fmt"
)

// MoveResult masa taşıma sonucudur.
type MoveResult struct {
	Merged   bool  `json:"merged"`
	TargetNo int64 `json:"targetNo"` // birleşmede kalan adisyon
	Table    int   `json:"table"`
}

// MoveTable adisyonu başka masaya taşır. Hedef masa doluysa iki adisyonu
// birleştirir: kalemler hedef adisyona eklenir (aynı ürünler toplanır), taşınan
// adisyon numarası silinmez, “birleşti” olarak kayıtta kalır.
func (s *Service) MoveTable(no int64, target int) (*MoveResult, *State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if target < 1 || target > s.set.Tables {
		return nil, nil, uerr("invalid", "Masa %d yok.", target)
	}
	if s.inPrint[no] {
		return nil, nil, errBusy(no)
	}
	res := &MoveResult{Table: target}
	err := s.withTx(func(tx *sql.Tx) error {
		src, err := loadActiveOrFail(tx, no)
		if err != nil {
			return err
		}
		if src.Table == target {
			return uerr("invalid", "Adisyon zaten Masa %d'de.", target)
		}
		dst, err := activeForTable(tx, target)
		if err != nil {
			return err
		}
		now := s.now()
		if dst == nil {
			if _, err := tx.Exec(`UPDATE adisyons SET table_no=? WHERE no=?`, target, no); err != nil {
				return err
			}
			audit(tx, now, "masa_tasindi", no, map[string]any{"eski_masa": src.Table, "yeni_masa": target})
			res.TargetNo = no
			return nil
		}
		if s.inPrint[dst.No] {
			return errBusy(dst.No)
		}
		// Birleştir: kaynak kalemleri hedefe ekle.
		var pos int
		if err := tx.QueryRow(`SELECT COALESCE(MAX(pos),0) FROM adisyon_lines WHERE adisyon_no=?`, dst.No).Scan(&pos); err != nil {
			return err
		}
		for _, l := range src.Lines {
			var lineID int64
			for _, d := range dst.Lines {
				if d.ProductID == l.ProductID && d.Price == l.Price && d.VAT == l.VAT && d.Name == l.Name && d.Comp == l.Comp {
					lineID = d.ID
				}
			}
			if lineID > 0 {
				if _, err := tx.Exec(`UPDATE adisyon_lines SET qty=qty+?, printed_qty=printed_qty+? WHERE id=?`, l.Qty, l.PrintedQty, lineID); err != nil {
					return err
				}
				continue
			}
			pos++
			var pid sql.NullInt64
			if l.ProductID > 0 {
				pid = sql.NullInt64{Int64: l.ProductID, Valid: true}
			}
			if _, err := tx.Exec(`INSERT INTO adisyon_lines(adisyon_no,pos,product_id,name,unit,price,vat,qty,printed_qty,comp)
				VALUES(?,?,?,?,?,?,?,?,?,?)`, dst.No, pos, pid, l.Name, l.Unit, l.Price, l.VAT, l.Qty, l.PrintedQty, l.Comp); err != nil {
				return err
			}
		}
		if len(src.Lines) > 0 && dst.Status == StatusPrinted {
			if _, err := tx.Exec(`UPDATE adisyons SET changed=1 WHERE no=?`, dst.No); err != nil {
				return err
			}
		}
		if err := recalc(tx, dst.No); err != nil {
			return err
		}
		reason := fmt.Sprintf("Masa %d ile birleştirildi (No %d)", target, dst.No)
		if _, err := tx.Exec(`UPDATE adisyons SET status='cancelled', closed_at=?, cancel_reason=?, merged_into=? WHERE no=?`,
			stamp(now), reason, dst.No, no); err != nil {
			return err
		}
		audit(tx, now, "masa_birlesti", dst.No, map[string]any{"kaynak_no": no, "kaynak_masa": src.Table, "hedef_masa": target, "tutar": src.Total})
		res.Merged, res.TargetNo = true, dst.No
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	delete(s.undo, no)
	if res.Merged {
		delete(s.undo, res.TargetNo)
	}
	st, err := s.stateLocked()
	return res, st, err
}
