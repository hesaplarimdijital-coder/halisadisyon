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
	for m.Busy("usb") || m.Busy("drive") {
		if time.Now().After(deadline) {
			t.Fatal("ikinci yere kopyalama bitmedi")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// unzipForTest yedek .zip dosyasındaki adisyon.db'yi çıkarıp yolunu verir.
func unzipForTest(t *testing.T, zipPath string) string {
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
	m := New(st, filepath.Join(dir, "yedek"), func() []Target { return []Target{{Key: "usb", Dir: second}} }, nil)
	cur := time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)
	m.now = func() time.Time { return cur }
	for i := 0; i < 24*100; i += 5 { // 100 gün boyunca 5 saatte bir
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
	b, err := store.Open(unzipForTest(t, filepath.Join(dir, "yedek", "gunluk", daily[len(daily)-1])))
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
	if s := m.Status(); s.LastError != "" || s.Extra["usb"].Error == "" {
		t.Fatalf("ikinci yer hatası raporlanmadı: %+v", s)
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
	if s := m.Status(); !strings.Contains(s.LastError, "Diskte yer az") {
		t.Fatalf("uyarı yok: %q", s.LastError)
	}
	m.freeSpace = diskFree
	if err := m.Run("test"); err != nil {
		t.Fatal(err)
	}
	if s := m.Status(); s.LastError != "" || s.LastAt == "" {
		t.Fatalf("yer açılınca uyarı kalkmadı: %+v", s)
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

// Ayarları olan (geri yüklenebilir) bir veritabanı kurar.
func newRestorableStore(t *testing.T, dir string) *store.Store {
	t.Helper()
	st := newStore(t, dir)
	if _, err := st.DB.Exec(`INSERT INTO settings(key,value) VALUES('settings','{}')`); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestDriveTargetKeepsFewerCopiesAndReportsTime(t *testing.T) {
	dir := t.TempDir()
	st := newStore(t, dir)
	usb, drive := filepath.Join(dir, "usb"), filepath.Join(dir, "Drive'ım")
	os.MkdirAll(usb, 0o755)
	os.MkdirAll(drive, 0o755)
	m := New(st, filepath.Join(dir, "yedek"), func() []Target {
		return []Target{{Key: "usb", Dir: usb}, {Key: "drive", Dir: drive, Keep: 30}}
	}, nil)
	cur := time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)
	m.now = func() time.Time { return cur }
	for i := 0; i < 40; i++ {
		if err := m.Run("test"); err != nil {
			t.Fatal(err)
		}
		waitSecond(t, m)
		cur = cur.Add(24 * time.Hour)
	}
	if n := len(backupFiles(filepath.Join(drive, secondName))); n != 30 {
		t.Fatalf("Drive'da %d kopya, beklenen 30", n)
	}
	if n := len(backupFiles(filepath.Join(usb, secondName))); n != 40 {
		t.Fatalf("USB'de %d kopya, beklenen 40", n)
	}
	s := m.Status()
	if s.Extra["drive"].At == "" || s.Extra["drive"].Error != "" || s.Extra["usb"].At == "" {
		t.Fatalf("ek yer durumu yanlış: %+v", s)
	}
	// Liste yeniden eskiye sıralı ve her yerden yedek içeriyor.
	list := m.List()
	where := map[string]int{}
	for i, f := range list {
		where[f.Where]++
		if i > 0 && f.At.After(list[i-1].At) {
			t.Fatalf("liste sıralı değil: %s > %s", f.At, list[i-1].At)
		}
	}
	if where["Saatlik"] == 0 || where["Günlük"] == 0 || where["USB bellek"] != 40 || where["Google Drive"] != 30 {
		t.Fatalf("liste eksik: %v", where)
	}
}

func TestSaveCopyAndRestoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st := newRestorableStore(t, dir)
	m := New(st, filepath.Join(dir, "yedek"), nil, nil)
	st.DB.Exec(`INSERT INTO settings(key,value) VALUES('durum','eski')`)
	path, err := m.SaveCopy("gun_sonu")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.Base(path), "gun_sonu") {
		t.Fatalf("dosya adı: %s", path)
	}
	if l := m.List(); len(l) != 1 || l[0].Where != "Gün sonu" {
		t.Fatalf("liste: %+v", l)
	}
	// Veri değişir, sonra eski kopya geri yüklenir. Hazırlıktan sonra da numara
	// verilir (No 151): geri yüklemeden sonra bu numara tekrar kullanılmamalı.
	st.DB.Exec(`UPDATE settings SET value='yeni' WHERE key='durum'`)
	if err := m.PrepareRestore(path, 150); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO adisyons(no,business_date,table_no,status,opened_at) VALUES(151,'2026-10-11',3,'open','2026-10-11T12:00:00+03:00')`); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Dir(st.Path)
	st.Close()
	ap, err := ApplyPending(dataDir)
	if err != nil || ap == nil {
		t.Fatalf("geri yükleme uygulanmadı: %v %v", ap, err)
	}
	if ap.MaxNo != 151 || ap.File != filepath.Base(path) {
		t.Fatalf("bilgi yanlış: %+v", ap)
	}
	// Eski veri silinmedi, kenara alındı.
	if _, err := os.Stat(filepath.Join(ap.OldDir, "adisyon.db")); err != nil {
		t.Fatalf("eski veri kenara alınmadı: %v", err)
	}
	st2, err := store.Open(filepath.Join(dataDir, "adisyon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	var v, floor string
	st2.DB.QueryRow(`SELECT value FROM settings WHERE key='durum'`).Scan(&v)
	if v != "eski" {
		t.Fatalf("geri yüklenen veri: %q", v)
	}
	st2.DB.QueryRow(`SELECT value FROM settings WHERE key='no_floor'`).Scan(&floor)
	if floor != "151" {
		t.Fatalf("numara tabanı yerine konan yedeğe yazılmadı: %q", floor)
	}
	// İkinci açılışta bekleyen bir şey kalmamalı.
	if ap, err := ApplyPending(dataDir); ap != nil || err != nil {
		t.Fatalf("geri yükleme iki kez uygulandı: %v %v", ap, err)
	}
}

func TestRestoreRejectsForeignAndBrokenFiles(t *testing.T) {
	dir := t.TempDir()
	st := newRestorableStore(t, dir)
	m := New(st, filepath.Join(dir, "yedek"), nil, nil)
	// Rastgele bir dosya.
	junk := filepath.Join(dir, "adisyon_bozuk.db")
	os.WriteFile(junk, []byte("bu bir veritabanı değil"), 0o644)
	if err := m.PrepareRestore(junk, 1); err == nil {
		t.Fatal("bozuk dosya kabul edildi")
	}
	// Başka bir SQLite veritabanı (Halis Adisyon değil).
	other, err := store.Open(filepath.Join(dir, "baska", "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	other.DB.Exec(`CREATE TABLE notlar(x TEXT); PRAGMA user_version=1`)
	other.Close()
	if err := m.PrepareRestore(filepath.Join(dir, "baska", "x.db"), 1); err == nil {
		t.Fatal("yabancı veritabanı kabul edildi")
	}
	dataDir := filepath.Dir(st.Path)
	if _, err := os.Stat(filepath.Join(dataDir, pendingDB)); err == nil {
		t.Fatal("reddedilen yedek hazırlandı")
	}
	// Açılışta hazırlanmış dosya bozulmuşsa şu anki veri yerinde kalır.
	os.WriteFile(filepath.Join(dataDir, pendingDB), []byte("bozuk"), 0o644)
	st.Close()
	if ap, err := ApplyPending(dataDir); err == nil || ap != nil {
		t.Fatalf("bozuk hazırlık uygulandı: %v %v", ap, err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "adisyon.db")); err != nil {
		t.Fatal("şu anki veri kayboldu")
	}
	// Yapılamayan geri yükleme sonraki açılışta habersizce uygulanmaz.
	if _, err := os.Stat(filepath.Join(dataDir, pendingDB)); err == nil {
		t.Fatal("bozuk hazırlık dosyası yerinde kaldı")
	}
	if ap, err := ApplyPending(dataDir); ap != nil || err != nil {
		t.Fatalf("ikinci açılışta yine denendi: %v %v", ap, err)
	}
}
