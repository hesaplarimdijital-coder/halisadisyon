// Package backup veritabanını kendiliğinden yedekler: açılışta, saatte bir,
// gün sonunda ve kapanışta. Yedekler sıkıştırılmış .zip dosyasıdır (içinde
// adisyon.db). Son 24 saatlik ve son 60 günlük yedek saklanır. İsteğe bağlı ek
// yerlere (USB bellek, Google Drive klasörü) günlük yedeğin kopyası yazılır.
//
// Yedek alınırken siparişler beklemez: veritabanı ayrı bir okuma bağlantısından
// kopyalanır, durum bilgisi ayrı bir kilitle korunur.
//
// Geri yükleme iki adımdır: program açıkken seçilen yedek doğrulanıp veri
// klasörüne hazırlanır (PrepareRestore), program yeniden açılırken veritabanı
// açılmadan önce yerine konur (ApplyPending). Eski veri silinmez, eski_veri
// klasörüne taşınır.
package backup

import (
	"archive/zip"
	"compress/flate"
	"encoding/json"
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
	keepHourly  = 24
	keepDays    = 60
	keepSpecial = 90 // gün sonu ve geri yükleme öncesi kopyaları
	secondName  = "Halis Adisyon Yedekleri"
	zipEntry    = "adisyon.db"
	tempDir     = ".gecici" // yedek hazırlanırken kullanılan geçici dosyalar
	// Yedek için diskte veritabanının iki katı + bu kadar boş yer aranır.
	reserveBytes = 300 << 20

	pendingDB   = "geri_yukle.db"   // açılışta yerine konacak yedek
	pendingInfo = "geri_yukle.json" // hangi yedek, en büyük adisyon no
	oldDataDir  = "eski_veri"       // geri yüklemede kenara alınan veri
	keepOldData = 10
)

// Target ana yedeğin kopyalandığı ek yerdir (USB bellek, Google Drive klasörü).
type Target struct {
	Key  string // "usb" | "drive"
	Dir  string
	Keep int // saklanacak günlük kopya sayısı (0: 60)
}

// ExtraStatus ek yedek yerinin durumudur.
type ExtraStatus struct {
	At    string // son başarılı kopya (gg.aa.yyyy ss:dd)
	Error string
}

// Status ayarlar ekranındaki yedek durumudur.
type Status struct {
	Dir       string
	LastAt    string
	LastError string
	Extra     map[string]ExtraStatus
}

// File geri yüklenebilecek bir yedektir.
type File struct {
	Path  string
	Name  string
	Where string // Saatlik, Günlük, Gün sonu, USB bellek...
	At    time.Time
	Size  int64
}

type extraState struct {
	busy atomic.Bool
	err  string    // m.mu ile korunur
	at   time.Time // m.mu ile korunur
}

// Manager yedekleri yönetir.
type Manager struct {
	st        *store.Store
	dir       string
	targets   func() []Target
	now       func() time.Time
	log       *log.Logger
	freeSpace func(dir string) (uint64, error)

	runMu sync.Mutex // aynı anda tek yedek

	mu      sync.Mutex // yalnızca durum alanları
	lastAt  time.Time
	lastErr string
	extra   map[string]*extraState

	stop chan struct{}
	done chan struct{}
}

// New yedek yöneticisi kurar. dir ana yedek klasörüdür; targets ek yerleri verir.
func New(st *store.Store, dir string, targets func() []Target, logger *log.Logger) *Manager {
	if targets == nil {
		targets = func() []Target { return nil }
	}
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &Manager{st: st, dir: dir, targets: targets, now: time.Now, log: logger, freeSpace: diskFree,
		extra: map[string]*extraState{}}
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

func (m *Manager) state(key string) *extraState {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.extra[key]
	if e == nil {
		e = &extraState{}
		m.extra[key] = e
	}
	return e
}

func (m *Manager) setExtra(key, msg string, okAt time.Time) {
	e := m.state(key)
	m.mu.Lock()
	e.err = msg
	if msg == "" && !okAt.IsZero() {
		e.at = okAt
	}
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

func targetLabel(key string) string {
	if key == "drive" {
		return "Google Drive klasörüne"
	}
	return "İkinci yedek yerine"
}

func targetHint(key string) string {
	if key == "drive" {
		return " Google Drive programı açık ve oturum açık mı?"
	}
	return " USB bellek takılı mı?"
}

// Run hemen yedek alır. Siparişleri bekletmez; aynı anda tek yedek çalışır.
func (m *Manager) Run(reason string) error {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	return m.run(reason)
}

func (m *Manager) run(reason string) error {
	targets := m.targets()
	now := m.now()
	day := now.Format("2006-01-02")

	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		m.setResult(now, friendly(err, m.dir))
		m.log.Printf("yedek alınamadı (%s): %v", reason, err)
		return err
	}
	// Disk dolmak üzereyse yedek almayıp uyar: eski yedekler korunur, sipariş kaydı sürer.
	if err := m.checkSpace(); err != nil {
		m.setResult(now, err.Error())
		m.log.Printf("yedek alınmadı (%s): %v", reason, err)
		return err
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

	active := map[string]bool{}
	for _, t := range targets {
		t.Dir = strings.TrimSpace(t.Dir)
		if t.Dir == "" || t.Key == "" || active[t.Key] {
			continue
		}
		active[t.Key] = true
		m.copyToTarget(t, daily, day, now)
	}
	// Kaldırılan ek yerin eski hatası ekranda kalmasın.
	m.mu.Lock()
	for k, e := range m.extra {
		if !active[k] {
			e.err = ""
		}
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) checkSpace() error {
	free, err := m.freeSpace(m.dir)
	if err != nil {
		return nil
	}
	need := uint64(reserveBytes)
	if fi, err := os.Stat(m.st.Path); err == nil {
		need += 2 * uint64(fi.Size())
	}
	if free < need {
		return fmt.Errorf("Diskte yer az kaldı (%d MB boş). Yedek alınmadı; gereksiz dosyaları silip yer açın.", free>>20)
	}
	return nil
}

// copyToTarget günlük yedeği ek yere arka planda kopyalar. Ek yer takılırsa
// (USB çıkarıldı, Drive yanıt vermiyor) ana yedeği ve programı bekletmez.
func (m *Manager) copyToTarget(t Target, daily, day string, now time.Time) {
	e := m.state(t.Key)
	if !e.busy.CompareAndSwap(false, true) {
		m.setExtra(t.Key, targetLabel(t.Key)+" ("+t.Dir+") önceki kopya hâlâ yazılıyor."+targetHint(t.Key), time.Time{})
		return
	}
	// Ek yere günlük dosyanın kendisi değil özel bir kopyası gönderilir: kopyalama
	// takılsa da (Windows'ta açık dosya değiştirilemez) sonraki yedekler yazılabilir.
	src := filepath.Join(m.dir, tempDir, fmt.Sprintf("%s_%d_%d.zip", t.Key, os.Getpid(), now.UnixNano()))
	if err := copyFile(daily, src); err != nil {
		e.busy.Store(false)
		m.setExtra(t.Key, targetLabel(t.Key)+" yazılamadı ("+t.Dir+").", time.Time{})
		m.log.Printf("%s yedeği hazırlanamadı: %v", t.Key, err)
		return
	}
	keep := t.Keep
	if keep <= 0 {
		keep = keepDays
	}
	go func() {
		defer e.busy.Store(false)
		defer os.Remove(src)
		target := filepath.Join(t.Dir, secondName, "adisyon_"+day+".zip")
		if err := copyFile(src, target); err != nil {
			m.setExtra(t.Key, targetLabel(t.Key)+" yazılamadı ("+t.Dir+")."+targetHint(t.Key), time.Time{})
			m.log.Printf("%s yedeği yazılamadı: %v", t.Key, err)
			return
		}
		m.setExtra(t.Key, "", m.now())
		prune(filepath.Join(t.Dir, secondName), keep, now)
	}()
}

// Busy ek yere kopyalama sürüyor mu (test için).
func (m *Manager) Busy(key string) bool { return m.state(key).busy.Load() }

// Status ayarlar ekranı için yedek durumunu verir. Yedek sürerken de beklemeden döner.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := Status{Dir: m.dir, LastError: m.lastErr, Extra: map[string]ExtraStatus{}}
	if !m.lastAt.IsZero() {
		s.LastAt = m.lastAt.Format("02.01.2006 15:04")
	}
	for k, e := range m.extra {
		x := ExtraStatus{Error: e.err}
		if !e.at.IsZero() {
			x.At = e.at.Format("02.01.2006 15:04")
		}
		s.Extra[k] = x
	}
	return s
}

// Start açılış yedeğini (delay kadar sonra, arka planda) alır ve saatlik yedeği başlatır.
func (m *Manager) Start(delay time.Duration) {
	stop := make(chan struct{})
	done := make(chan struct{})
	m.stop, m.done = stop, done
	go func() {
		defer close(done)
		select {
		case <-time.After(delay):
			_ = m.Run("açılış")
		case <-stop:
			return
		}
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				_ = m.Run("saatlik")
			case <-stop:
				return
			}
		}
	}()
}

// stopTicker saatlik yedeği durdurur (Start ile aynı iş parçacığından çağrılır).
func (m *Manager) stopTicker() {
	if m.stop != nil {
		close(m.stop)
		m.stop = nil
	}
}

// Stop saatlik yedeği durdurur ve kapanış yedeği alır. Önceki bir yedek takılı
// kaldıysa (ör. bozuk USB) program kapanışı en fazla wait kadar bekler.
func (m *Manager) Stop(wait time.Duration) {
	m.stopTicker()
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

// Halt saatlik yedeği durdurur ve süren yedeğin bitmesini (en fazla wait) bekler;
// kapanış yedeği almaz (program yeniden başlatılırken).
func (m *Manager) Halt(wait time.Duration) {
	m.stopTicker()
	deadline := time.Now().Add(wait)
	for !m.runMu.TryLock() {
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	m.runMu.Unlock()
}

// SaveCopy şu anki verinin adlı, ayrı saklanan bir yedeğini alır (gün sonu,
// geri yükleme öncesi). Dosya "ozel" klasörüne yazılır ve yolu döner.
func (m *Manager) SaveCopy(label string) (string, error) {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	now := m.now()
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return "", errors.New(friendly(err, m.dir))
	}
	if err := m.checkSpace(); err != nil {
		return "", err
	}
	raw := filepath.Join(m.dir, tempDir, fmt.Sprintf("ozel_%d.db", os.Getpid()))
	defer os.Remove(raw)
	if err := m.st.BackupTo(raw); err != nil {
		m.log.Printf("özel yedek alınamadı (%s): %v", label, err)
		return "", errors.New("Veritabanının kopyası alınamadı. " + friendly(err, m.dir))
	}
	dst := filepath.Join(m.dir, "ozel", "adisyon_"+now.Format("2006-01-02_150405")+"_"+label+".zip")
	if err := zipFile(raw, dst, now); err != nil {
		m.log.Printf("özel yedek yazılamadı (%s): %v", label, err)
		return "", errors.New(friendly(err, m.dir))
	}
	prune(filepath.Join(m.dir, "ozel"), keepSpecial, now)
	m.log.Printf("özel yedek alındı: %s", dst)
	return dst, nil
}

// ---- geri yükleme ----

func specialWhere(name string) string {
	switch {
	case strings.Contains(name, "geri_yukleme_oncesi"):
		return "Geri yükleme öncesi"
	case strings.Contains(name, "gun_sonu"):
		return "Gün sonu"
	}
	return "Elle alınan"
}

// listDir klasördeki yedekleri okur. Ek yerler (USB, Drive) yanıt vermezse
// ekran beklemesin diye en fazla birkaç saniye beklenir.
func listDir(dir, where string, wait time.Duration) []File {
	ch := make(chan []File, 1)
	go func() {
		var out []File
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, e := range entries {
				n := e.Name()
				if e.IsDir() || !strings.HasPrefix(n, "adisyon_") || !(strings.HasSuffix(n, ".zip") || strings.HasSuffix(n, ".db")) {
					continue
				}
				fi, err := e.Info()
				if err != nil {
					continue
				}
				w := where
				if w == "" {
					w = specialWhere(n)
				}
				out = append(out, File{Path: filepath.Join(dir, n), Name: n, Where: w, At: fi.ModTime(), Size: fi.Size()})
			}
		}
		ch <- out
	}()
	select {
	case out := <-ch:
		return out
	case <-time.After(wait):
		return nil
	}
}

// List geri yüklenebilecek yedekleri yeniden eskiye verir.
func (m *Manager) List() []File {
	var out []File
	out = append(out, listDir(filepath.Join(m.dir, "saatlik"), "Saatlik", 5*time.Second)...)
	out = append(out, listDir(filepath.Join(m.dir, "gunluk"), "Günlük", 5*time.Second)...)
	out = append(out, listDir(filepath.Join(m.dir, "ozel"), "", 5*time.Second)...)
	if entries, err := os.ReadDir(m.dir); err == nil {
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() || !strings.HasPrefix(n, "guncelleme_oncesi_") || !strings.HasSuffix(n, ".db") {
				continue
			}
			if fi, err := e.Info(); err == nil {
				out = append(out, File{Path: filepath.Join(m.dir, n), Name: n, Where: "Güncelleme öncesi", At: fi.ModTime(), Size: fi.Size()})
			}
		}
	}
	for _, t := range m.targets() {
		if d := strings.TrimSpace(t.Dir); d != "" {
			w := "USB bellek"
			if t.Key == "drive" {
				w = "Google Drive"
			}
			out = append(out, listDir(filepath.Join(d, secondName), w, 3*time.Second)...)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

type pending struct {
	File   string `json:"file"`
	Path   string `json:"path"`
	MaxNo  int64  `json:"maxNo"`
	At     string `json:"at"`
	OldDir string `json:"oldDir,omitempty"` // taşıma başladıysa eski verinin klasörü
}

// PrepareRestore seçilen yedeği doğrular ve veri klasörüne hazırlar; program
// yeniden açılınca ApplyPending onu yerine koyar. maxNo şu ana kadar verilmiş en
// büyük adisyon numarasıdır: geri yüklemeden sonra numaralar bunun üstünden
// devam eder (kâğıtta verilmiş bir numara tekrar kullanılmaz).
func (m *Manager) PrepareRestore(src string, maxNo int64) error {
	dataDir := filepath.Dir(m.st.Path)
	staged := filepath.Join(dataDir, pendingDB)
	tmp := staged + ".hazirlaniyor"
	os.Remove(tmp)
	var err error
	if strings.EqualFold(filepath.Ext(src), ".zip") {
		err = unzipDB(src, tmp)
	} else {
		err = copyFile(src, tmp)
	}
	if err != nil {
		os.Remove(tmp)
		m.log.Printf("geri yükleme: yedek okunamadı %s: %v", src, err)
		if strings.Contains(strings.ToLower(err.Error()), "space") || strings.Contains(strings.ToLower(err.Error()), "full") {
			return errors.New("Diskte yer yok; yedek açılamadı.")
		}
		return errors.New("Yedek dosyası okunamadı; dosya bozuk ya da yarım olabilir. Başka bir yedek seçin.")
	}
	if err := store.Validate(tmp); err != nil {
		os.Remove(tmp)
		m.log.Printf("geri yükleme: yedek doğrulanamadı %s: %v", src, err)
		return errors.New(store.ValidateMessage(err))
	}
	info, _ := json.Marshal(pending{File: filepath.Base(src), Path: src, MaxNo: maxNo, At: m.now().Format(time.RFC3339)})
	if err := os.WriteFile(filepath.Join(dataDir, pendingInfo), info, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, staged); err != nil {
		os.Remove(tmp)
		os.Remove(filepath.Join(dataDir, pendingInfo))
		return err
	}
	return nil
}

// Applied açılışta yerine konan yedektir.
type Applied struct {
	File   string // yedek dosyasının adı
	MaxNo  int64  // geri yüklemeden önce verilmiş en büyük adisyon no
	OldDir string // kenara alınan eski veri
}

// ApplyPending hazırlanmış bir geri yükleme varsa, veritabanı açılmadan önce
// yerine koyar. Şu anki veri silinmez: eski_veri\<tarih> klasörüne taşınır.
// Numaralar geriye düşmesin diye şu anki verideki en büyük adisyon numarası
// yerine konan yedeğe "no_floor" olarak yazılır. Geri yükleme yapılamazsa
// hazırlanan dosya kenara alınır (sonraki bir açılışta habersizce uygulanmaz).
// Bekleyen geri yükleme yoksa nil, nil döner.
func ApplyPending(dataDir string) (*Applied, error) {
	staged := filepath.Join(dataDir, pendingDB)
	infoPath := filepath.Join(dataDir, pendingInfo)
	if _, err := os.Stat(staged); err != nil {
		os.Remove(infoPath)
		return nil, nil
	}
	var info pending
	if b, err := os.ReadFile(infoPath); err == nil {
		_ = json.Unmarshal(b, &info)
	}
	// Taşıma yarıda kaldıysa (elektrik kesildi) aynı klasör kullanılır: veritabanı
	// ile -wal dosyası ayrı klasörlere düşmesin.
	if info.OldDir == "" || filepath.Dir(info.OldDir) != filepath.Join(dataDir, oldDataDir) {
		info.OldDir = filepath.Join(dataDir, oldDataDir, time.Now().Format("2006-01-02_150405"))
		if b, err := json.Marshal(info); err == nil {
			_ = os.WriteFile(infoPath, b, 0o644)
		}
	}
	oldDir := info.OldDir
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		return nil, abandon(staged, infoPath, oldDir, fmt.Errorf("eski veri klasörü oluşturulamadı: %w", err))
	}
	if err := store.Validate(staged); err != nil {
		return nil, abandon(staged, infoPath, oldDir, errors.New(store.ValidateMessage(err)))
	}
	db := filepath.Join(dataDir, "adisyon.db")
	// Şu anki (son) verideki en büyük numara: geri yükleme hazırlandıktan sonra
	// verilen numaralar da tekrar kullanılmasın.
	floor := info.MaxNo
	for _, p := range []string{db, filepath.Join(oldDir, "adisyon.db")} {
		if _, err := os.Stat(p); err == nil {
			if n, err := store.MaxNumber(p); err == nil && n > floor {
				floor = n
			}
			break
		}
	}
	if floor > 0 {
		if err := store.SetNumberFloor(staged, floor); err != nil {
			return nil, abandon(staged, infoPath, oldDir, fmt.Errorf("yedek hazırlanamadı: %w", err))
		}
	}
	var moved []string
	undo := func() {
		for _, suf := range moved {
			os.Rename(filepath.Join(oldDir, "adisyon.db"+suf), db+suf)
		}
	}
	for _, suf := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(db + suf); err != nil {
			continue
		}
		if err := renameRetry(db+suf, filepath.Join(oldDir, "adisyon.db"+suf)); err != nil {
			undo()
			return nil, abandon(staged, infoPath, oldDir, fmt.Errorf("şu anki veri kenara alınamadı: %w", err))
		}
		moved = append(moved, suf)
	}
	if err := renameRetry(staged, db); err != nil {
		undo()
		return nil, abandon(staged, infoPath, oldDir, fmt.Errorf("yedek yerine konamadı: %w", err))
	}
	os.Remove(infoPath)
	pruneDirs(filepath.Join(dataDir, oldDataDir), keepOldData)
	return &Applied{File: info.File, MaxNo: floor, OldDir: oldDir}, nil
}

// abandon yapılamayan geri yüklemenin hazırlık dosyasını kenara alır.
func abandon(staged, infoPath, oldDir string, err error) error {
	_ = os.MkdirAll(oldDir, 0o755)
	if rerr := os.Rename(staged, filepath.Join(oldDir, "geri_yuklenemeyen.db")); rerr != nil {
		os.Remove(staged)
	}
	os.Remove(infoPath)
	return err
}

// renameRetry dosyayı taşır; Windows'ta önceki program kopyası dosyayı henüz
// bırakmadıysa birkaç saniye yeniden dener.
func renameRetry(from, to string) error {
	var err error
	for i := 0; i < 20; i++ {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return err
}

// pruneDirs klasördeki en yeni keep alt klasörü bırakır (adlar tarih sırasındadır).
func pruneDirs(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for i := 0; i < len(names)-keep; i++ {
		os.RemoveAll(filepath.Join(dir, names[i]))
	}
}

// ---- dosya yardımcıları ----

func unzipDB(src, dst string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	var f *zip.File
	for _, z := range zr.File {
		if z.Name == zipEntry {
			f = z
		}
	}
	if f == nil {
		return errors.New("zip içinde adisyon.db yok")
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
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
	for _, sub := range []string{"saatlik", "gunluk", "ozel"} {
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
