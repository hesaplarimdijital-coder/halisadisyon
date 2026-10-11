// Package store SQLite veritabanını açar, şemayı günceller ve yedek alır.
// Her işlem kendi işleminde (transaction) yazılır; synchronous=FULL ile
// elektrik kesilse bile son onaylanan işlem kaybolmaz.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// SchemaVersion programın beklediği şema sürümüdür.
const SchemaVersion = 1

// Store açık veritabanı bağlantısını tutar.
type Store struct {
	DB   *sql.DB
	Path string
}

// Open veritabanı dosyasını açar (yoksa oluşturur).
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("veri klasörü oluşturulamadı: %w", err)
	}
	db, err := driver.Open(path, func(c *sqlite3.Conn) error {
		return c.Exec(`PRAGMA busy_timeout=10000;
PRAGMA foreign_keys=ON;
PRAGMA journal_mode=WAL;
PRAGMA synchronous=FULL;`)
	})
	if err != nil {
		return nil, fmt.Errorf("veritabanı açılamadı: %w", err)
	}
	// Tek bağlantı: tüm yazmalar sırayla yapılır.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("veritabanı açılamadı: %w", err)
	}
	return &Store{DB: db, Path: path}, nil
}

// Close bağlantıyı kapatır.
func (s *Store) Close() error { return s.DB.Close() }

// Version şema sürümünü okur.
func (s *Store) Version() (int, error) {
	var v int
	err := s.DB.QueryRow(`PRAGMA user_version`).Scan(&v)
	return v, err
}

// Migrate şemayı son sürüme getirir. Var olan bir veritabanı güncellenecekse
// önce beforeUpgrade çağrılır (güncelleme öncesi yedek için).
func (s *Store) Migrate(beforeUpgrade func(from, to int) error) error {
	v, err := s.Version()
	if err != nil {
		return err
	}
	if v > SchemaVersion {
		return fmt.Errorf("veritabanı bu programdan daha yeni bir sürümle oluşturulmuş (şema %d). Programın güncel sürümünü kurun", v)
	}
	if v == SchemaVersion {
		return nil
	}
	if v > 0 && beforeUpgrade != nil {
		if err := beforeUpgrade(v, SchemaVersion); err != nil {
			return fmt.Errorf("güncelleme öncesi yedek alınamadı: %w", err)
		}
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for step := v + 1; step <= SchemaVersion; step++ {
		if _, err := tx.Exec(migrations[step]); err != nil {
			return fmt.Errorf("şema %d uygulanamadı: %w", step, err)
		}
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version=%d`, SchemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

// BackupTo veritabanının tutarlı bir kopyasını path dosyasına yazar.
// Program açıkken de güvenlidir (VACUUM INTO). Kopya ayrı, salt okunur bir
// bağlantıdan alınır; böylece büyük veritabanında bile siparişler beklemez
// (WAL kipinde okuyucu yazanı durdurmaz).
func (s *Store) BackupTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.yaziliyor", path, os.Getpid())
	_ = os.Remove(tmp)
	query := `VACUUM INTO '` + strings.ReplaceAll(tmp, "'", "''") + `'`
	err := func() error {
		c, err := sqlite3.OpenFlags(s.Path, sqlite3.OPEN_READONLY)
		if err != nil {
			return err
		}
		defer c.Close()
		_ = c.BusyTimeout(10 * time.Second)
		return c.Exec(query)
	}()
	if err != nil {
		// Ayrı bağlantı açılamadıysa ana bağlantıyla dene.
		_ = os.Remove(tmp)
		if _, err2 := s.DB.Exec(query); err2 != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("%v (ayrı bağlantı: %v)", err2, err)
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// migrations[n] şemayı n-1'den n'ye taşır.
var migrations = map[int]string{
	1: `
CREATE TABLE settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE categories (
	id    INTEGER PRIMARY KEY AUTOINCREMENT,
	name  TEXT NOT NULL,
	color TEXT NOT NULL DEFAULT 'bordo',
	sort  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE products (
	id          INTEGER PRIMARY KEY,
	name        TEXT NOT NULL,
	category_id INTEGER NOT NULL REFERENCES categories(id),
	price       INTEGER NOT NULL DEFAULT 0,   -- kuruş, KDV dahil
	vat         INTEGER,                      -- NULL: varsayılan oran
	unit        TEXT NOT NULL DEFAULT 'adet',
	active      INTEGER NOT NULL DEFAULT 1,
	sort        INTEGER NOT NULL DEFAULT 0,
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);

CREATE TABLE waiters (
	id     INTEGER PRIMARY KEY,
	name   TEXT NOT NULL,
	active INTEGER NOT NULL DEFAULT 1,
	sort   INTEGER NOT NULL DEFAULT 0
);

-- Adisyon numarası 1'den başlar, silinmez, tekrar kullanılmaz.
CREATE TABLE adisyons (
	no            INTEGER PRIMARY KEY,
	business_date TEXT NOT NULL,              -- iş günü (YYYY-AA-GG)
	table_no      INTEGER NOT NULL,
	waiter_id     INTEGER REFERENCES waiters(id),
	waiter_name   TEXT NOT NULL DEFAULT '',
	status        TEXT NOT NULL CHECK (status IN ('open','printed','paid','cancelled')),
	opened_at     TEXT NOT NULL,
	printed_at    TEXT,
	closed_at     TEXT,
	print_count   INTEGER NOT NULL DEFAULT 0,
	changed       INTEGER NOT NULL DEFAULT 0, -- basıldıktan sonra değişti
	print_failed  INTEGER NOT NULL DEFAULT 0, -- son baskı denemesi başarısız
	cancel_reason TEXT,
	total         INTEGER NOT NULL DEFAULT 0, -- kuruş, KDV dahil
	vat_total     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX adisyons_status ON adisyons(status);
CREATE INDEX adisyons_date ON adisyons(business_date);

-- Kalemde ürün adı, fiyat ve KDV o anki haliyle saklanır.
-- AUTOINCREMENT: silinen kalemin numarası yeni kaleme verilmez; ekranda kalmış
-- eski bir +/− tıklaması başka bir kalemi değiştiremez.
CREATE TABLE adisyon_lines (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	adisyon_no INTEGER NOT NULL REFERENCES adisyons(no),
	pos        INTEGER NOT NULL,
	product_id INTEGER,
	name       TEXT NOT NULL,
	unit       TEXT NOT NULL,
	price      INTEGER NOT NULL,
	vat        INTEGER NOT NULL,
	qty        INTEGER NOT NULL CHECK (qty > 0),
	printed_qty INTEGER NOT NULL DEFAULT 0      -- son basılan adet
);
CREATE INDEX adisyon_lines_no ON adisyon_lines(adisyon_no);

-- İşlem kaydı (denetim izi): baskı, iptal, fiyat değişikliği...
CREATE TABLE audit_log (
	id         INTEGER PRIMARY KEY,
	at         TEXT NOT NULL,
	action     TEXT NOT NULL,
	adisyon_no INTEGER,
	detail     TEXT
);
CREATE INDEX audit_log_at ON audit_log(at);
`,
}
