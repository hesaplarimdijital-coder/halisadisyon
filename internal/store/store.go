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
// 1: ilk sürüm (1.0.0) · 2: ödeme şekli, gün sonu, masa birleştirme, dış gönderim kuyruğu (2.0.0)
// 3: yüzde indirim (2.1.0)
const SchemaVersion = 3

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

// requiredTables her Halis Adisyon veritabanında bulunan tablolardır.
var requiredTables = []string{"settings", "categories", "products", "waiters", "adisyons", "adisyon_lines", "audit_log"}

// Validate dosyanın bu programın açabileceği sağlam bir Halis Adisyon
// veritabanı olduğunu denetler (yedekten geri yüklemeden önce). Dosya
// programın kendi geçici kopyası olmalıdır; asıl yedeğe dokunulmaz.
func Validate(path string) error {
	c, err := sqlite3.OpenFlags(path, sqlite3.OPEN_READWRITE)
	if err != nil {
		return fmt.Errorf("yedek dosyası açılamadı: %w", err)
	}
	defer c.Close()
	_ = c.BusyTimeout(5 * time.Second)
	one := func(q string) (string, int64, error) {
		stmt, _, err := c.Prepare(q)
		if err != nil {
			return "", 0, err
		}
		defer stmt.Close()
		if !stmt.Step() {
			return "", 0, stmt.Err()
		}
		return stmt.ColumnText(0), stmt.ColumnInt64(0), nil
	}
	if res, _, err := one(`PRAGMA integrity_check`); err != nil || res != "ok" {
		if err == nil {
			err = fmt.Errorf("%s", res)
		}
		return fmt.Errorf("yedek dosyası bozuk: %w", err)
	}
	_, ver, err := one(`PRAGMA user_version`)
	if err != nil {
		return err
	}
	if ver < 1 {
		return fmt.Errorf("bu dosya bir Halis Adisyon yedeği değil")
	}
	if ver > SchemaVersion {
		return fmt.Errorf("yedek programın daha yeni bir sürümüyle alınmış (şema %d); önce programı güncelleyin", ver)
	}
	quoted := make([]string, len(requiredTables))
	for i, t := range requiredTables {
		quoted[i] = "'" + t + "'"
	}
	_, n, err := one(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN (` + strings.Join(quoted, ",") + `)`)
	if err != nil {
		return err
	}
	if int(n) != len(requiredTables) {
		return fmt.Errorf("bu dosya bir Halis Adisyon yedeği değil (tablolar eksik)")
	}
	if _, n, err := one(`SELECT COUNT(*) FROM settings WHERE key='settings'`); err != nil || n != 1 {
		return fmt.Errorf("yedekte program ayarları yok")
	}
	return nil
}

// ValidateMessage doğrulama hatasını kullanıcıya gösterilecek sade bir cümleye çevirir.
func ValidateMessage(err error) string {
	m := err.Error()
	switch {
	case strings.Contains(m, "daha yeni bir sürüm"):
		return "Bu yedek programın daha yeni bir sürümüyle alınmış; önce programı güncelleyin."
	case strings.Contains(m, "Halis Adisyon yedeği değil"), strings.Contains(m, "ayarları yok"):
		return "Bu dosya bir Halis Adisyon yedeği değil."
	}
	return "Yedek dosyası bozuk ya da yarım; başka bir yedek seçin."
}

// MaxNumber veritabanında verilmiş en büyük adisyon numarasını verir (no_floor dahil).
// Dosya programın kendi verisidir ve o an başka bir bağlantı açık değildir.
func MaxNumber(path string) (int64, error) {
	c, err := sqlite3.OpenFlags(path, sqlite3.OPEN_READWRITE)
	if err != nil {
		return 0, err
	}
	defer c.Close()
	_ = c.BusyTimeout(5 * time.Second)
	stmt, _, err := c.Prepare(`SELECT MAX(COALESCE((SELECT MAX(no) FROM adisyons),0),
		COALESCE((SELECT CAST(value AS INTEGER) FROM settings WHERE key='no_floor'),0))`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	if !stmt.Step() {
		return 0, stmt.Err()
	}
	return stmt.ColumnInt64(0), nil
}

// SetNumberFloor geri yüklenecek veritabanına numara tabanını yazar: sıradaki
// adisyon numarası bunun üstünden devam eder.
func SetNumberFloor(path string, floor int64) error {
	c, err := sqlite3.OpenFlags(path, sqlite3.OPEN_READWRITE)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.BusyTimeout(5 * time.Second)
	stmt, _, err := c.Prepare(`INSERT INTO settings(key,value) VALUES('no_floor',?)
		ON CONFLICT(key) DO UPDATE SET value=MAX(CAST(value AS INTEGER), CAST(excluded.value AS INTEGER))`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	if err := stmt.BindInt64(1, floor); err != nil {
		return err
	}
	stmt.Step()
	return stmt.Err()
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
	2: `
-- Ödeme şekli: 'nakit' | 'kart' | 'havale' (yalnız ödenen adisyonda)
ALTER TABLE adisyons ADD COLUMN payment_method TEXT;
-- Masa birleştirmede kapanan adisyonun eklendiği adisyon numarası
ALTER TABLE adisyons ADD COLUMN merged_into INTEGER;
CREATE INDEX adisyons_closed ON adisyons(closed_at);
CREATE INDEX audit_log_no ON audit_log(adisyon_no);
CREATE INDEX adisyon_lines_product ON adisyon_lines(product_id);

-- Gün sonu: kapatılan iş günü değiştirilemez; kapanıştaki özet saklanır.
CREATE TABLE business_days (
	date      TEXT PRIMARY KEY,               -- iş günü (YYYY-AA-GG)
	closed_at TEXT,                           -- NULL: açık
	summary   TEXT                            -- kapanıştaki özet (JSON)
);

-- İnternet gerektiren gönderimler (WhatsApp, Google Sheets): internet yoksa sırada bekler.
CREATE TABLE outbox (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	kind       TEXT NOT NULL,                 -- 'whatsapp' | 'sheets'
	target     TEXT NOT NULL DEFAULT '',      -- telefon numarası (WhatsApp)
	ref        TEXT NOT NULL DEFAULT '',      -- ilgili iş günü ya da 'test'
	payload    TEXT NOT NULL,                 -- JSON
	created_at TEXT NOT NULL,
	status     TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','sent','failed')),
	attempts   INTEGER NOT NULL DEFAULT 0,
	next_at    TEXT NOT NULL,
	sent_at    TEXT,
	last_error TEXT
);
CREATE INDEX outbox_pending ON outbox(status, next_at);
`,
	3: `
-- Yüzde indirim: adisyonun tamamına uygulanır; tutar kuruş olarak saklanır,
-- total ve vat_total indirim düşülmüş hallerdir.
ALTER TABLE adisyons ADD COLUMN discount_percent INTEGER NOT NULL DEFAULT 0;
ALTER TABLE adisyons ADD COLUMN discount_amount INTEGER NOT NULL DEFAULT 0;
`,
}
