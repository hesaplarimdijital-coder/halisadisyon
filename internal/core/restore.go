package core

import (
	"database/sql"
	"path/filepath"
	"strings"
	"time"
)

// Backups geri yüklenebilecek yedekleri listeler (yönetici).
func (s *Service) Backups() ([]BackupFile, error) {
	s.mu.Lock()
	err := s.requireUnlocked()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if s.opt.Backup == nil {
		return []BackupFile{}, nil
	}
	list := s.opt.Backup.List()
	if list == nil {
		list = []BackupFile{}
	}
	return list, nil
}

// Restore seçilen yedeği geri yükler: önce şu anki verinin ayrı bir yedeği
// alınır, seçilen yedek doğrulanır ve program yeniden başlatılır. Asıl değişim
// açılışta, veritabanı açılmadan önce yapılır; şu anki veri silinmez.
func (s *Service) Restore(path, pin string) error {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	if err := s.confirmPin(pin, "Yedekten geri yükleme yönetici şifresi ister."); err != nil {
		s.mu.Unlock()
		return err
	}
	var open int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM adisyons WHERE status IN ('open','printed')`).Scan(&open)
	maxNo, err := nextNo(s.db)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	maxNo--
	if s.opt.Backup == nil {
		return uerr("invalid", "Yedekleme kullanılamıyor.")
	}
	// Yalnız listedeki yedekler geri yüklenebilir.
	clean := filepath.Clean(path)
	ok := false
	for _, b := range s.opt.Backup.List() {
		if filepath.Clean(b.Path) == clean {
			ok = true
			break
		}
	}
	if !ok {
		return uerr("not_found", "Yedek bulunamadı; listeyi yenileyin.")
	}
	if _, err := s.opt.Backup.SaveCopy("geri_yukleme_oncesi"); err != nil {
		return uerr("backup", "Şu anki verinin yedeği alınamadı; geri yükleme yapılmadı. %s", err.Error())
	}
	if err := s.opt.Backup.PrepareRestore(clean, maxNo); err != nil {
		return uerr("backup", "Bu yedek geri yüklenemez: %s", err.Error())
	}
	s.mu.Lock()
	audit(s.db, s.now(), "geri_yukleme_hazirlandi", 0, map[string]any{"dosya": filepath.Base(clean), "acik_masa": open, "en_buyuk_no": maxNo})
	s.mu.Unlock()
	s.opt.Log.Printf("yedekten geri yükleme hazırlandı: %s; program yeniden başlıyor", clean)
	if err := s.opt.Restart(); err != nil {
		return uerr("restart", "Yedek hazır. Programı kapatıp yeniden açınca geri yüklenecek.")
	}
	return nil
}

// RecordRestore açılışta yerine konan yedeği işlem kaydına yazar ve ekranda
// kısa süre bilgi gösterir. maxNo geri yüklemeden önce verilmiş en büyük
// adisyon numarasıdır; numaralar bunun üstünden devam eder.
func (s *Service) RecordRestore(file string, maxNo int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.withTx(func(tx *sql.Tx) error {
		if maxNo > 0 {
			if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES('no_floor',?)
				ON CONFLICT(key) DO UPDATE SET value=MAX(CAST(value AS INTEGER), CAST(excluded.value AS INTEGER))`, maxNo); err != nil {
				return err
			}
		}
		audit(tx, s.now(), "yedek_geri_yuklendi", 0, map[string]any{"dosya": file, "en_buyuk_no": maxNo})
		return nil
	})
	// Geri yüklenen yedekteki geri alma geçmişi bu veriye ait değil.
	s.undo = map[int64][][]Line{}
	s.daysVer++
	s.notice = "Yedek geri yüklendi: " + strings.TrimSpace(file)
	s.noticeUntil = s.now().Add(10 * time.Minute)
}
