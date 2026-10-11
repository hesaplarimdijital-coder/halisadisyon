package core

import (
	"database/sql"
	"errors"
)

func validVAT(v int) bool { return v == 1 || v == 10 || v == 20 }

func validColor(c string) bool {
	for _, x := range Colors {
		if x.ID == c {
			return true
		}
	}
	return false
}

// ProductInput ürün ekleme/düzenleme formudur.
type ProductInput struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CategoryID int64  `json:"categoryId"`
	Price      int64  `json:"price"`
	VAT        *int   `json:"vat"`
	Unit       string `json:"unit"`
	Active     bool   `json:"active"`
}

// SaveProduct ürün ekler ya da değiştirir. Fiyat değişikliği yalnız yeni kalemlere uygulanır;
// geçmiş adisyonların fiyatı değişmez.
func (s *Service) SaveProduct(in ProductInput) (*State, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, 0, err
	}
	name, err := required(singleLine(in.Name), 40, "Ürün adı")
	if err != nil {
		return nil, 0, err
	}
	unit, err := required(singleLine(in.Unit), 15, "Birim")
	if err != nil {
		return nil, 0, err
	}
	if in.Price < 0 || in.Price > 10_000_000 {
		return nil, 0, uerr("invalid", "Fiyat 0 ile 100.000 ₺ arasında olmalı.")
	}
	if in.VAT != nil && !validVAT(*in.VAT) {
		return nil, 0, uerr("invalid", "KDV oranı %%1, %%10 ya da %%20 olmalı.")
	}
	id := in.ID
	err = s.withTx(func(tx *sql.Tx) error {
		var catOK int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM categories WHERE id=?`, in.CategoryID).Scan(&catOK); err != nil {
			return err
		}
		if catOK == 0 {
			return uerr("invalid", "Kategori seçin.")
		}
		var vat sql.NullInt64
		if in.VAT != nil {
			vat = sql.NullInt64{Int64: int64(*in.VAT), Valid: true}
		}
		now := s.now()
		if id == 0 {
			var sortN int
			if err := tx.QueryRow(`SELECT COALESCE(MAX(sort),-1)+1 FROM products`).Scan(&sortN); err != nil {
				return err
			}
			res, err := tx.Exec(`INSERT INTO products(name,category_id,price,vat,unit,active,sort,created_at,updated_at)
				VALUES(?,?,?,?,?,?,?,?,?)`, name, in.CategoryID, in.Price, vat, unit, in.Active, sortN, stamp(now), stamp(now))
			if err != nil {
				return err
			}
			id, _ = res.LastInsertId()
			audit(tx, now, "urun_eklendi", 0, map[string]any{"ad": name, "fiyat": in.Price, "kdv": in.VAT, "birim": unit})
			return nil
		}
		old, err := loadProduct(tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE products SET name=?,category_id=?,price=?,vat=?,unit=?,active=?,updated_at=? WHERE id=?`,
			name, in.CategoryID, in.Price, vat, unit, in.Active, stamp(now), id); err != nil {
			return err
		}
		audit(tx, now, "urun_degisti", 0, map[string]any{
			"ad": name, "eski_ad": old.Name, "eski_fiyat": old.Price, "yeni_fiyat": in.Price,
			"eski_kdv": old.VAT, "yeni_kdv": in.VAT, "satista": in.Active,
		})
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	st, err := s.stateLocked()
	return st, id, err
}

// MoveProduct ürünü kendi kategorisi içinde bir yukarı (-1) ya da aşağı (+1) taşır.
func (s *Service) MoveProduct(id int64, dir int) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	err := s.withTx(func(tx *sql.Tx) error {
		ps, err := loadProducts(tx)
		if err != nil {
			return err
		}
		idx := -1
		for i, p := range ps {
			if p.ID == id {
				idx = i
			}
		}
		if idx < 0 {
			return uerr("not_found", "Ürün bulunamadı.")
		}
		// aynı kategorideki komşuyu bul
		j := idx
		for {
			j += dir
			if j < 0 || j >= len(ps) {
				return nil
			}
			if ps[j].CategoryID == ps[idx].CategoryID {
				break
			}
		}
		ps[idx], ps[j] = ps[j], ps[idx]
		for i, p := range ps {
			if _, err := tx.Exec(`UPDATE products SET sort=? WHERE id=?`, i, p.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.stateLocked()
}

// CategoryInput kategori formudur.
type CategoryInput struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// SaveCategory kategori ekler ya da değiştirir.
func (s *Service) SaveCategory(in CategoryInput) (*State, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, 0, err
	}
	name, err := required(singleLine(in.Name), 30, "Kategori adı")
	if err != nil {
		return nil, 0, err
	}
	if !validColor(in.Color) {
		in.Color = "bordo"
	}
	id := in.ID
	err = s.withTx(func(tx *sql.Tx) error {
		if id == 0 {
			var n int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM categories`).Scan(&n); err != nil {
				return err
			}
			if n >= 12 {
				return uerr("invalid", "En fazla 12 kategori olabilir.")
			}
			var sortN int
			if err := tx.QueryRow(`SELECT COALESCE(MAX(sort),-1)+1 FROM categories`).Scan(&sortN); err != nil {
				return err
			}
			res, err := tx.Exec(`INSERT INTO categories(name,color,sort) VALUES(?,?,?)`, name, in.Color, sortN)
			if err != nil {
				return err
			}
			id, _ = res.LastInsertId()
			audit(tx, s.now(), "kategori_eklendi", 0, map[string]any{"ad": name})
			return nil
		}
		res, err := tx.Exec(`UPDATE categories SET name=?, color=? WHERE id=?`, name, in.Color, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return uerr("not_found", "Kategori bulunamadı.")
		}
		audit(tx, s.now(), "kategori_degisti", 0, map[string]any{"ad": name, "renk": in.Color})
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	st, err := s.stateLocked()
	return st, id, err
}

// MoveCategory kategoriyi bir yukarı ya da aşağı taşır.
func (s *Service) MoveCategory(id int64, dir int) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	err := s.withTx(func(tx *sql.Tx) error {
		cs, err := loadCategories(tx)
		if err != nil {
			return err
		}
		idx := -1
		for i, c := range cs {
			if c.ID == id {
				idx = i
			}
		}
		j := idx + dir
		if idx < 0 || j < 0 || j >= len(cs) {
			return nil
		}
		cs[idx], cs[j] = cs[j], cs[idx]
		for i, c := range cs {
			if _, err := tx.Exec(`UPDATE categories SET sort=? WHERE id=?`, i, c.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.stateLocked()
}

// DeleteCategory boş kategoriyi siler.
func (s *Service) DeleteCategory(id int64) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	err := s.withTx(func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM products WHERE category_id=?`, id).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return uerr("invalid", "Bu kategoride %d ürün var. Önce ürünleri başka kategoriye taşıyın.", n)
		}
		var total int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM categories`).Scan(&total); err != nil {
			return err
		}
		if total <= 1 {
			return uerr("invalid", "En az bir kategori kalmalı.")
		}
		var name string
		if err := tx.QueryRow(`SELECT name FROM categories WHERE id=?`, id).Scan(&name); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return uerr("not_found", "Kategori bulunamadı.")
			}
			return err
		}
		if _, err := tx.Exec(`DELETE FROM categories WHERE id=?`, id); err != nil {
			return err
		}
		audit(tx, s.now(), "kategori_silindi", 0, map[string]any{"ad": name})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.stateLocked()
}

// SaveWaiter garson ekler ya da adını değiştirir.
func (s *Service) SaveWaiter(id int64, name string) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	name, err := required(singleLine(name), 30, "Garson adı")
	if err != nil {
		return nil, err
	}
	err = s.withTx(func(tx *sql.Tx) error {
		if id == 0 {
			var n int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM waiters WHERE active=1`).Scan(&n); err != nil {
				return err
			}
			if n >= 30 {
				return uerr("invalid", "En fazla 30 garson olabilir.")
			}
			var sortN int
			if err := tx.QueryRow(`SELECT COALESCE(MAX(sort),-1)+1 FROM waiters`).Scan(&sortN); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO waiters(name,active,sort) VALUES(?,1,?)`, name, sortN); err != nil {
				return err
			}
			audit(tx, s.now(), "garson_eklendi", 0, map[string]any{"ad": name})
			return nil
		}
		var old string
		if err := tx.QueryRow(`SELECT name FROM waiters WHERE id=? AND active=1`, id).Scan(&old); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return uerr("not_found", "Garson bulunamadı.")
			}
			return err
		}
		if _, err := tx.Exec(`UPDATE waiters SET name=? WHERE id=?`, name, id); err != nil {
			return err
		}
		// açık adisyonlarda görünen ad da güncellensin
		if _, err := tx.Exec(`UPDATE adisyons SET waiter_name=? WHERE waiter_id=? AND status IN ('open','printed')`, name, id); err != nil {
			return err
		}
		audit(tx, s.now(), "garson_degisti", 0, map[string]any{"eski": old, "yeni": name})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.stateLocked()
}

// RemoveWaiter garsonu listeden kaldırır; geçmiş kayıtlarda adı kalır.
func (s *Service) RemoveWaiter(id int64) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	err := s.withTx(func(tx *sql.Tx) error {
		var open int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM adisyons WHERE waiter_id=? AND status IN ('open','printed')`, id).Scan(&open); err != nil {
			return err
		}
		if open > 0 {
			return uerr("invalid", "Bu garsonun açık masası var. Önce masaları kapatın ya da garsonunu değiştirin.")
		}
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM waiters WHERE active=1`).Scan(&n); err != nil {
			return err
		}
		if n <= 1 {
			return uerr("invalid", "En az bir garson kalmalı.")
		}
		var name string
		if err := tx.QueryRow(`SELECT name FROM waiters WHERE id=? AND active=1`, id).Scan(&name); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return uerr("not_found", "Garson bulunamadı.")
			}
			return err
		}
		if _, err := tx.Exec(`UPDATE waiters SET active=0 WHERE id=?`, id); err != nil {
			return err
		}
		audit(tx, s.now(), "garson_kaldirildi", 0, map[string]any{"ad": name})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.stateLocked()
}
