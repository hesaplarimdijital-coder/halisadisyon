package backup

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"halisadisyon/internal/store"
)

func newStore(t *testing.T, dir string) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(dir, "data", "adisyon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO settings(key,value) VALUES('x','y')`); err != nil {
		t.Fatal(err)
	}
	return st
}

func waitSecond(t *testing.T, m *Manager) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for m.secondBusy.Load() {
		if time.Now().After(deadline) {
			t.Fatal("ikinci yere kopyalama bitmedi")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// unzipDB yedek .zip dosyasındaki adisyon.db'yi çıkarıp yolunu verir.
func unzipDB(t *testing.T, zipPath string) string {
	t.Helper()
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 1 || zr.File[0].Name != zipEntry {
		t.Fatalf("zip içeriği beklenmedik: %d dosya", len(zr.File))
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	out := filepath.Join(t.TempDir(), "adisyon.db")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(f, rc); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return out
}

func TestBackupRotationAndSecondDir(t *testing.T) {
	dir := t.TempDir()
	st := newStore(t, dir)
	second := filepath.Join(dir, "usb")
	os.MkdirAll(second, 0o755)
	m := New(st, filepath.Join(dir, "yedek"), func() string { return second }, nil)
	cur := time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)
	m.now = func() time.Time { return cur }
	for i := 0; i < 24*70; i += 5 { // 70 gün boyunca 5 saatte bir
		if err := m.Run("test"); err != nil {
			t.Fatal(err)
		}
		waitSecond(t, m)
		cur = cur.Add(5 * time.Hour)
	}
	hourly := backupFiles(filepath.Join(dir, "yedek", "saatlik"))
	if len(hourly) != keepHourly {
		t.Fatalf("saatlik yedek sayısı %d, beklenen %d", len(hourly), keepHourly)
	}
	daily := backupFiles(filepath.Join(dir, "yedek", "gunluk"))
	if len(daily) != keepDays {
		t.Fatalf("günlük yedek sayısı %d, beklenen %d", len(daily), keepDays)
	}
	sec := backupFiles(filepath.Join(second, secondName))
	if len(sec) != len(daily) {
		t.Fatalf("ikinci konum %d, ana konum %d", len(sec), len(daily))
	}
	for _, d := range []string{filepath.Join(dir, "yedek"), filepath.Join(dir, "yedek", "saatlik"), filepath.Join(second, secondName)} {
		entries, _ := os.ReadDir(d)
		for _, e := range entries {
			if strings.Contains(e.Name(), "yaziliyor") || strings.Contains(e.Name(), "hazirlaniyor") {
				t.Fatalf("yarım dosya kaldı: %s", e.Name())
			}
		}
	}
	// Yedek, açılabilen bir veritabanı içermeli.
	b, err := store.Open(unzipDB(t, filepath.Join(dir, "yedek", "gunluk", daily[len(daily)-1])))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var v string
	if err := b.DB.QueryRow(`SELECT value FROM settings WHERE key='x'`).Scan(&v); err != nil || v != "y" {
		t.Fatalf("yedek içeriği yanlış: %v %q", err, v)
	}
	// İkinci yer yazılamıyorsa uyarı verir ama ana yedek alınır.
	os.RemoveAll(second)
	os.WriteFile(second, []byte("dosya"), 0o644) // klasör yerine dosya: yazılamaz
	if err := m.Run("test"); err != nil {
		t.Fatal(err)
	}
	waitSecond(t, m)
	if _, _, lastErr, secErr := m.Status(); lastErr != "" || secErr == "" {
		t.Fatalf("ikinci yer hatası raporlanmadı: %q %q", lastErr, secErr)
	}
}

// Bilgisayarın saati bir süre ileri kayarsa eski yedekler toptan silinmemeli;
// saat düzelince yeni yedekler de hemen silinmemeli.
func TestWrongClockDoesNotWipeBackups(t *testing.T) {
	dir := t.TempDir()
	st := newStore(t, dir)
	m := New(st, filepath.Join(dir, "yedek"), nil, nil)
	cur := time.Date(2026, 8, 1, 12, 0, 0, 0, time.Local)
	m.now = func() time.Time { return cur }
	for i := 0; i < 60; i++ {
		m.Run("test")
		cur = cur.Add(24 * time.Hour)
	}
	real := cur
	cur = cur.AddDate(1, 0, 0) // saat bir yıl ileri
	for i := 0; i < 30; i++ {
		m.Run("test")
		cur = cur.Add(time.Hour)
	}
	daily := backupFiles(filepath.Join(dir, "yedek", "gunluk"))
	if len(daily) < 58 {
		t.Fatalf("saat ileri kayınca günlük yedekler silindi: %d kaldı", len(daily))
	}
	cur = real // saat düzeltildi
	for i := 0; i < 5; i++ {
		m.Run("test")
		cur = cur.Add(time.Hour)
	}
	kept := 0
	for _, n := range backupFiles(filepath.Join(dir, "yedek", "saatlik")) {
		if strings.HasPrefix(n, "adisyon_"+real.Format("2006-01-02")) {
			kept++
		}
	}
	if kept != 5 {
		t.Fatalf("saat düzelince yeni saatlik yedekler silindi: %d/5 kaldı", kept)
	}
}

func TestLowDiskSpaceSkipsBackup(t *testing.T) {
	dir := t.TempDir()
	st := newStore(t, dir)
	m := New(st, filepath.Join(dir, "yedek"), nil, nil)
	m.freeSpace = func(string) (uint64, error) { return 50 << 20, nil }
	if err := m.Run("test"); err == nil {
		t.Fatal("disk doluyken yedek alındı")
	}
	if _, _, lastErr, _ := m.Status(); !strings.Contains(lastErr, "Diskte yer az") {
		t.Fatalf("uyarı yok: %q", lastErr)
	}
	m.freeSpace = diskFree
	if err := m.Run("test"); err != nil {
		t.Fatal(err)
	}
	if _, at, lastErr, _ := m.Status(); lastErr != "" || at == "" {
		t.Fatalf("yer açılınca uyarı kalkmadı: %q %q", at, lastErr)
	}
}

// Yedek sürerken durum bilgisi beklemeden okunabilmeli (ekran donmasın).
func TestStatusDoesNotWaitForRunningBackup(t *testing.T) {
	dir := t.TempDir()
	st := newStore(t, dir)
	m := New(st, filepath.Join(dir, "yedek"), nil, nil)
	m.runMu.Lock()
	done := make(chan struct{})
	go func() { m.Status(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("durum okuma yedeği bekledi")
	}
	m.runMu.Unlock()
}

// Kapanışta önceki yedek takılı kalmışsa program kapanışı sonsuza dek beklememeli.
func TestStopDoesNotHangOnStuckBackup(t *testing.T) {
	dir := t.TempDir()
	st := newStore(t, dir)
	m := New(st, filepath.Join(dir, "yedek"), nil, nil)
	m.Start(time.Hour)
	m.runMu.Lock()
	start := time.Now()
	m.Stop(300 * time.Millisecond)
	if time.Since(start) > 2*time.Second {
		t.Fatal("kapanış takıldı")
	}
	m.runMu.Unlock()
}
