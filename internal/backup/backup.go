// Package backup veritabanını kendiliğinden yedekler: açılışta, saatte bir ve
// kapanışta. Yedekler sıkıştırılmış .zip dosyasıdır (içinde adisyon.db).
// Son 24 saatlik ve son 60 günlük yedek saklanır. İsteğe bağlı ikinci yere
// (USB bellek, ikinci disk) günlük yedeğin kopyası yazılır.
//
// Yedek alınırken siparişler beklemez: veritabanı ayrı bir okuma bağlantısından
// kopyalanır, durum bilgisi ayrı bir kilitle korunur.
package backup

import (
	"archive/zip"
	"compress/flate"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"halisadisyon/internal/store"
)

const (
	keepHourly = 24
	keepDays   = 60
	secondName = "Halis Adisyon Yedekleri"
	zipEntry   = "adisyon.db"
	tempDir    = ".gecici" // yedek hazırlanırken kullanılan geçici dosyalar
	// Yedek için diskte veritabanının iki katı + bu kadar boş yer aranır.
	reserveBytes = 300 << 20
)

// Manager yedekleri yönetir.
type Manager struct {
	st        *store.Store
	dir       string
	secondDir func() string
	now       func() time.Time
	log       *log.Logger
	freeSpace func(dir string) (uint64, error)

	runMu sync.Mutex // aynı anda tek yedek

	mu        sync.Mutex // yalnızca durum alanları
	lastAt    time.Time
	lastErr   string
	secondErr string

	secondBusy atomic.Bool // ikinci yere kopyalama sürüyor (takılan USB)

	stop chan struct{}
	done chan struct{}
}

// New yedek yöneticisi kurar. dir ana yedek klasörüdür.
func New(st *store.Store, dir string, secondDir func() string, logger *log.Logger) *Manager {
	if secondDir == nil {
		secondDir = func() string { return "" }
	}
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &Manager{st: st, dir: dir, secondDir: secondDir, now: time.Now, log: logger, freeSpace: diskFree}
}

// Dir ana yedek klasörüdür.
func (m *Manager) Dir() string { return m.dir }

// BeforeUpgrade veritabanı şeması güncellenmeden önce ayrı bir yedek alır.
func (m *Manager) BeforeUpgrade(from, to int) error {
	name := fmt.Sprintf("guncelleme_oncesi_v%d_%s.db", from, m.now().Format("2006-01-02_1504"))
	return m.st.BackupTo(filepath.Join(m.dir, name))
}

func (m *Manager) setResult(at time.Time, lastErr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if lastErr == "" {
		m.lastAt = at
	}
	m.lastErr = lastErr
}

func (m *Manager) setSecond(msg string) {
	m.mu.Lock()
	m.secondErr = msg
	m.mu.Unlock()
}

// friendly teknik hatayı kısa bir Türkçe açıklamaya çevirir (ayrıntı kayıt dosyasına yazılır).
func friendly(err error, dir string) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "disk is full") || strings.Contains(msg, "not enough space") || strings.Contains(msg, "no space left"):
		return "Diskte yer kalmadı."
	case errors.Is(err, fs.ErrPermission):
		return "Yedek klasörüne yazma izni yok (" + dir + ")."
	case errors.Is(err, fs.ErrNotExist):
		return "Yedek klasörü bulunamadı (" + dir + ")."
	}
	return "Yedek dosyası yazılamadı (" + dir + ")."
}

// Run hemen yedek alır. Siparişleri bekletmez; aynı anda tek yedek çalışır.
func (m *Manager) Run(reason string) error {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	return m.run(reason)
}

func (m *Manager) run(reason string) error {
	sd := strings.TrimSpace(m.secondDir())
	now := m.now()
	day := now.Format("2006-01-02")

	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		m.setResult(now, friendly(err, m.dir))
		m.log.Printf("yedek alınamadı (%s): %v", reason, err)
		return err
	}
	// Disk dolmak üzereyse yedek almayıp uyar: eski yedekler korunur, sipariş kaydı sürer.
	if free, err := m.freeSpace(m.dir); err == nil {
		need := uint64(reserveBytes)
		if fi, err := os.Stat(m.st.Path); err == nil {
			need += 2 * uint64(fi.Size())
		}
		if free < need {
			msg := fmt.Sprintf("Diskte yer az kaldı (%d MB boş). Yedek alınmadı; gereksiz dosyaları silip yer açın.", free>>20)
			m.setResult(now, msg)
			m.log.Printf("yedek alınmadı (%s): boş yer %d MB", reason, free>>20)
			return errors.New(msg)
		}
	}

	cleanTemps(m.dir, now)
	raw := filepath.Join(m.dir, tempDir, fmt.Sprintf("hazirlaniyor_%d.db", os.Getpid()))
	defer os.Remove(raw)
	if err := m.st.BackupTo(raw); err != nil {
		m.setResult(now, "Veritabanının kopyası alınamadı. "+friendly(err, m.dir))
		m.log.Printf("yedek alınamadı (%s): %v", reason, err)
		return err
	}
	hourly := filepath.Join(m.dir, "saatlik", "adisyon_"+day+"_"+now.Format("15")+".zip")
	daily := filepath.Join(m.dir, "gunluk", "adisyon_"+day+".zip")
	if err := zipFile(raw, hourly, now); err != nil {
		m.setResult(now, friendly(err, m.dir))
		m.log.Printf("saatlik yedek yazılamadı (%s): %v", reason, err)
		return err
	}
	if err := copyFile(hourly, daily); err != nil {
		m.setResult(now, friendly(err, m.dir))
		m.log.Printf("günlük yedek yazılamadı (%s): %v", reason, err)
		return err
	}
	m.setResult(now, "")
	prune(filepath.Join(m.dir, "saatlik"), keepHourly, now)
	prune(filepath.Join(m.dir, "gunluk"), keepDays, now)
	m.log.Printf("yedek alındı (%s): %s", reason, daily)

	if sd == "" {
		m.setSecond("")
		return nil
	}
	// İkinci yer (USB) takılırsa ana yedeği ve programı bekletmesin.
	if !m.secondBusy.CompareAndSwap(false, true) {
		m.setSecond("İkinci yedek yerine (" + sd + ") önceki kopya hâlâ yazılıyor. USB bellek takılmış olabilir; çıkarıp yeniden takın.")
		return nil
	}
	// USB'ye günlük dosyanın kendisi değil özel bir kopyası gönderilir: kopyalama
	// takılsa da (Windows'ta açık dosya değiştirilemez) sonraki yedekler yazılabilir.
	src := filepath.Join(m.dir, tempDir, fmt.Sprintf("usb_%d_%d.zip", os.Getpid(), now.UnixNano()))
	if err := copyFile(daily, src); err != nil {
		m.secondBusy.Store(false)
		m.setSecond("İkinci yedek yerine yazılamadı (" + sd + ").")
		m.log.Printf("ikinci yedek hazırlanamadı: %v", err)
		return nil
	}
	go func() {
		defer m.secondBusy.Store(false)
		defer os.Remove(src)
		target := filepath.Join(sd, secondName, "adisyon_"+day+".zip")
		if err := copyFile(src, target); err != nil {
			m.setSecond("İkinci yedek yerine yazılamadı (" + sd + "). USB bellek takılı mı?")
			m.log.Printf("ikinci yedek yazılamadı: %v", err)
			return
		}
		m.setSecond("")
		prune(filepath.Join(sd, secondName), keepDays, now)
	}()
	return nil
}

// Status ayarlar ekranı için yedek durumunu verir. Yedek sürerken de beklemeden döner.
func (m *Manager) Status() (dir, lastAt, lastErr, secondErr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.lastAt.IsZero() {
		lastAt = m.lastAt.Format("02.01.2006 15:04")
	}
	return m.dir, lastAt, m.lastErr, m.secondErr
}

// Start açılış yedeğini (delay kadar sonra, arka planda) alır ve saatlik yedeği başlatır.
func (m *Manager) Start(delay time.Duration) {
	m.stop = make(chan struct{})
	m.done = make(chan struct{})
	go func() {
		defer close(m.done)
		select {
		case <-time.After(delay):
			_ = m.Run("açılış")
		case <-m.stop:
			return
		}
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				_ = m.Run("saatlik")
			case <-m.stop:
				return
			}
		}
	}()
}

// Stop saatlik yedeği durdurur ve kapanış yedeği alır. Önceki bir yedek takılı
// kaldıysa (ör. bozuk USB) program kapanışı en fazla wait kadar bekler.
func (m *Manager) Stop(wait time.Duration) {
	if m.stop != nil {
		close(m.stop)
		m.stop = nil
	}
	deadline := time.Now().Add(wait)
	for !m.runMu.TryLock() {
		if time.Now().After(deadline) {
			m.log.Printf("kapanış yedeği atlandı: önceki yedek bitmedi")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	defer m.runMu.Unlock()
	_ = m.run("kapanış")
}

func zipFile(src, dst string, mod time.Time) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := fmt.Sprintf("%s.%d.yaziliyor", dst, os.Getpid())
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		out.Close()
		os.Remove(tmp)
		return err
	}
	zw := zip.NewWriter(out)
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestSpeed)
	})
	w, err := zw.CreateHeader(&zip.FileHeader{Name: zipEntry, Method: zip.Deflate, Modified: mod})
	if err != nil {
		return fail(err)
	}
	if _, err := io.Copy(w, in); err != nil {
		return fail(err)
	}
	if err := zw.Close(); err != nil {
		return fail(err)
	}
	if err := out.Sync(); err != nil {
		return fail(err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := fmt.Sprintf("%s.%d.yaziliyor", dst, os.Getpid())
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// cleanTemps yarıda kalmış (program kapanınca ya da USB çıkınca) geçici
// dosyaları siler. Kullanımda olabilecek yeni dosyalara dokunmaz.
func cleanTemps(dir string, now time.Time) {
	old := func(path string) bool {
		fi, err := os.Stat(path)
		return err == nil && now.Sub(fi.ModTime()) > 2*time.Hour
	}
	if entries, err := os.ReadDir(filepath.Join(dir, tempDir)); err == nil {
		for _, e := range entries {
			if p := filepath.Join(dir, tempDir, e.Name()); old(p) {
				os.Remove(p)
			}
		}
	}
	for _, sub := range []string{"saatlik", "gunluk"} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if p := filepath.Join(dir, sub, e.Name()); strings.HasSuffix(e.Name(), ".yaziliyor") && old(p) {
				os.Remove(p)
			}
		}
	}
}

// backupFiles klasördeki yedekleri tarih sırasıyla (eskiden yeniye) verir.
func backupFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, "adisyon_") && (strings.HasSuffix(n, ".zip") || strings.HasSuffix(n, ".db")) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// fileDate yedek adındaki tarihi verir ("adisyon_2026-10-11_14.zip" → "2026-10-11").
func fileDate(name string) string {
	d := strings.TrimPrefix(name, "adisyon_")
	if len(d) < 10 {
		return ""
	}
	return d[:10]
}

// prune klasörde en yeni keep yedeği bırakır. Bilgisayarın saati yanlış
// ayarlandığında yedekler toptan silinmesin diye silme yaşa göre değil sayıya
// göre yapılır; tarihi ileride görünen dosyalar ne sayılır ne silinir.
func prune(dir string, keep int, now time.Time) {
	limit := now.Add(36 * time.Hour).Format("2006-01-02")
	var normal []string
	for _, n := range backupFiles(dir) {
		if d := fileDate(n); d != "" && d <= limit {
			normal = append(normal, n)
		}
	}
	for i := 0; i < len(normal)-keep; i++ {
		os.Remove(filepath.Join(dir, normal[i]))
	}
}
