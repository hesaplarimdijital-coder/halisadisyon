package core

import (
	"database/sql"
	"math"
)

// BulkPriceInput toplu zam formudur.
type BulkPriceInput struct {
	IDs     []int64 `json:"ids"`     // boş: tüm ürünler
	Percent float64 `json:"percent"` // ör. 10 (%10 zam), -5 (%5 indirim)
	Round   int64   `json:"round"`   // yuvarlama adımı, kuruş: 0 (yok), 100, 500, 1000
	Apply   bool    `json:"apply"`   // false: yalnız önizleme
}

// PriceChange toplu zamda bir ürünün eski ve yeni fiyatıdır.
type PriceChange struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Old  int64  `json:"old"`
	New  int64  `json:"new"`
}

// smallerStep yuvarlama adımını bir kademe küçültür: 10 ₺ → 5 ₺ → 1 ₺.
func smallerStep(step int64) int64 {
	switch {
	case step > 500:
		return 500
	case step > 100:
		return 100
	}
	return step
}

// newPrice yeni fiyatı hesaplar ve en yakın adıma yuvarlar. Adım ürünün
// fiyatına göre fazla kabaysa küçültülür (15 ₺'lik ürün 10 ₺'ye yuvarlanmasın:
// adımın 10 katından ucuz ürünlerde bir kademe küçük adım kullanılır). Yuvarlama
// zammı indirime (ya da indirimi zamma) çevirecekse adım yine küçültülür.
func newPrice(old int64, percent float64, step int64) int64 {
	exact := float64(old) * (1 + percent/100)
	if step <= 0 {
		return int64(math.Floor(exact + 0.5))
	}
	for step > 100 && old < 10*step {
		step = smallerStep(step)
	}
	for {
		n := int64(math.Floor(exact/float64(step)+0.5)) * step
		wrongWay := (percent > 0 && n < old) || (percent < 0 && n > old)
		if !wrongWay || step <= 100 {
			if wrongWay {
				n = old
			}
			if n < step && old > 0 {
				n = step
			}
			return n
		}
		step = smallerStep(step)
	}
}

// BulkResult toplu zam önizlemesi ya da sonucudur. Unchanged: yuvarlama
// nedeniyle fiyatı değişmeyen ürünler.
type BulkResult struct {
	Changes   []PriceChange `json:"changes"`
	Unchanged []string      `json:"unchanged"`
}

// BulkPrice seçili (ya da tüm) ürünlere yüzde ile zam yapar. Apply false iken
// yalnız yeni fiyatları hesaplar; true iken uygular ve her değişikliği kayda yazar.
// Geçmiş adisyonların fiyatı değişmez.
func (s *Service) BulkPrice(in BulkPriceInput) (*BulkResult, *State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, nil, err
	}
	if math.IsNaN(in.Percent) || in.Percent < -90 || in.Percent > 500 || in.Percent == 0 {
		return nil, nil, uerr("invalid", "Yüzdeyi -90 ile 500 arasında girin (ör. 10).")
	}
	switch in.Round {
	case 0, 100, 500, 1000:
	default:
		return nil, nil, uerr("invalid", "Yuvarlama 1 ₺, 5 ₺ ya da 10 ₺ olabilir.")
	}
	prods, err := loadProducts(s.db)
	if err != nil {
		return nil, nil, err
	}
	want := map[int64]bool{}
	for _, id := range in.IDs {
		want[id] = true
	}
	res := &BulkResult{Changes: []PriceChange{}, Unchanged: []string{}}
	selected := 0
	for _, p := range prods {
		if len(want) > 0 && !want[p.ID] {
			continue
		}
		if p.Price <= 0 {
			continue
		}
		selected++
		np := newPrice(p.Price, in.Percent, in.Round)
		if np > 10_000_000 {
			return nil, nil, uerr("invalid", "%s için yeni fiyat 100.000 ₺'yi geçiyor.", p.Name)
		}
		if np == p.Price {
			res.Unchanged = append(res.Unchanged, p.Name)
			continue
		}
		res.Changes = append(res.Changes, PriceChange{ID: p.ID, Name: p.Name, Old: p.Price, New: np})
	}
	if selected == 0 {
		return nil, nil, uerr("invalid", "Fiyatı girilmiş ürün seçin.")
	}
	if len(res.Changes) == 0 {
		return nil, nil, uerr("invalid", "Bu oran ve yuvarlamayla hiçbir ürünün fiyatı değişmiyor. Oranı büyütün ya da yuvarlamayı küçültün.")
	}
	changes := res.Changes
	if !in.Apply {
		return res, nil, nil
	}
	err = s.withTx(func(tx *sql.Tx) error {
		now := s.now()
		for _, c := range changes {
			if _, err := tx.Exec(`UPDATE products SET price=?, updated_at=? WHERE id=?`, c.New, stamp(now), c.ID); err != nil {
				return err
			}
			audit(tx, now, "toplu_fiyat", 0, map[string]any{"ad": c.Name, "eski_fiyat": c.Old, "yeni_fiyat": c.New, "yuzde": in.Percent})
		}
		audit(tx, now, "toplu_zam", 0, map[string]any{"yuzde": in.Percent, "urun": len(changes), "yuvarlama": in.Round})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	st, err := s.stateLocked()
	return res, st, err
}
