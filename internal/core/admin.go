package core

import (
	"bytes"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"halisadisyon/internal/printing"
)

// ---- yönetici şifresi ----

func (s *Service) lockDur() time.Duration {
	m := s.set.LockMinutes
	if m < 1 {
		m = 5
	}
	return time.Duration(m) * time.Minute
}

// requireUnlocked yönetici girişi açıksa süreyi uzatır; değilse errLocked döner.
// Kurulum tamamlanmadan önce şifre istenmez.
func (s *Service) requireUnlocked() error {
	if !s.set.SetupDone {
		return nil
	}
	now := s.now()
	if now.Before(s.unlockedUntil) {
		s.unlockedUntil = now.Add(s.lockDur())
		return nil
	}
	return errLocked
}

func hashPin(pin, salt string) string {
	k, err := pbkdf2.Key(sha256.New, pin, []byte(salt), 60000, 32)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(k)
}

func newSalt() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func validPin(p string) bool {
	if len(p) != 4 {
		return false
	}
	for _, c := range p {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (s *Service) checkPin(pin string) bool {
	if s.set.PinHash == "" {
		return false
	}
	got := hashPin(pin, s.set.PinSalt)
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.set.PinHash)) == 1
}

// Yanlış şifre koruması: 5 yanlış denemeden sonra giderek uzayan bekleme.
// Sayaçlar veritabanında tutulur; programı kapatıp açmak sıfırlamaz.
var pinBlockSteps = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour}

type pinGuard struct {
	Fails  int    `json:"fails"`
	Blocks int    `json:"blocks"`
	Until  string `json:"until,omitempty"`
}

func (s *Service) loadPinGuard() {
	var raw string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key='pin_guard'`).Scan(&raw); err != nil {
		return
	}
	var g pinGuard
	if json.Unmarshal([]byte(raw), &g) != nil {
		return
	}
	s.pinFails, s.pinBlocks = g.Fails, g.Blocks
	if t, err := time.Parse(time.RFC3339, g.Until); err == nil {
		// Saat geri alınmışsa bekleme en fazla 1 saat sürsün.
		if max := s.now().Add(time.Hour + time.Minute); t.After(max) {
			t = max
		}
		s.pinBlockedUntil = t
	}
}

func (s *Service) savePinGuard() {
	g := pinGuard{Fails: s.pinFails, Blocks: s.pinBlocks}
	if !s.pinBlockedUntil.IsZero() {
		g.Until = s.pinBlockedUntil.Format(time.RFC3339)
	}
	b, _ := json.Marshal(g)
	_, _ = s.db.Exec(`INSERT INTO settings(key,value) VALUES('pin_guard',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(b))
}

func waitText(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d saniye", int(d.Seconds())+1)
	}
	return fmt.Sprintf("%d dakika", int((d+time.Minute-1)/time.Minute))
}

// pinAttempt şifreyi yanlış deneme korumasıyla kontrol eder. s.mu tutulurken çağrılır.
func (s *Service) pinAttempt(pin string) error {
	now := s.now()
	if now.Before(s.pinBlockedUntil) {
		return uerr("pin_blocked", "Çok fazla yanlış deneme. %s sonra tekrar deneyin.", waitText(s.pinBlockedUntil.Sub(now)))
	}
	if s.checkPin(pin) {
		if s.pinFails != 0 || s.pinBlocks != 0 || !s.pinBlockedUntil.IsZero() {
			s.pinFails, s.pinBlocks, s.pinBlockedUntil = 0, 0, time.Time{}
			s.savePinGuard()
		}
		return nil
	}
	s.pinFails++
	if s.pinFails >= 5 {
		step := s.pinBlocks
		if step >= len(pinBlockSteps) {
			step = len(pinBlockSteps) - 1
		}
		d := pinBlockSteps[step]
		s.pinFails = 0
		s.pinBlocks++
		s.pinBlockedUntil = now.Add(d)
		s.savePinGuard()
		audit(s.db, now, "sifre_cok_hatali", 0, map[string]any{"bekleme_dk": int(d / time.Minute)})
		return uerr("pin_blocked", "Çok fazla yanlış deneme. %s sonra tekrar deneyin.", waitText(d))
	}
	s.savePinGuard()
	return uerr("pin_wrong", "Şifre yanlış, tekrar deneyin.")
}

// Unlock yönetici şifresini kontrol eder ve kilitli ekranları açar.
func (s *Service) Unlock(pin string) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.set.SetupDone {
		return s.stateLocked()
	}
	if err := s.pinAttempt(pin); err != nil {
		return nil, err
	}
	s.unlockedUntil = s.now().Add(s.lockDur())
	return s.stateLocked()
}

// confirmPin tek bir işlem için şifre ister (iptal gibi); yönetici girişini açmaz.
// Kurulum bitmeden şifre istenmez. s.mu tutulurken çağrılır.
func (s *Service) confirmPin(pin, why string) error {
	if !s.set.SetupDone {
		return nil
	}
	if pin == "" {
		return uerr("pin_required", "%s", why)
	}
	return s.pinAttempt(pin)
}

// Lock yönetici girişini kapatır.
func (s *Service) Lock() (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unlockedUntil = time.Time{}
	return s.stateLocked()
}

// Touch yönetici ekranında çalışılırken kilit süresini uzatır.
func (s *Service) Touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.now().Before(s.unlockedUntil) {
		s.unlockedUntil = s.now().Add(s.lockDur())
	}
}

// ChangePin yönetici şifresini değiştirir.
func (s *Service) ChangePin(old, pin string) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	if err := s.pinAttempt(old); err != nil {
		var ue *UserError
		if errors.As(err, &ue) && ue.Code == "pin_wrong" {
			return nil, uerr("pin_wrong", "Mevcut şifre yanlış.")
		}
		return nil, err
	}
	if !validPin(pin) {
		return nil, uerr("invalid", "Yeni şifre 4 rakam olmalı.")
	}
	ns := s.set
	ns.PinSalt = newSalt()
	ns.PinHash = hashPin(pin, ns.PinSalt)
	if err := s.withTx(func(tx *sql.Tx) error {
		if err := saveSettingsTx(tx, ns); err != nil {
			return err
		}
		audit(tx, s.now(), "sifre_degisti", 0, nil)
		return nil
	}); err != nil {
		return nil, err
	}
	s.set = ns
	return s.stateLocked()
}

// ResetPin şifreyi verilen değere döndürür (veri klasörüne “sifre_sifirla.txt” konunca).
func (s *Service) ResetPin(pin string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validPin(pin) {
		return uerr("invalid", "Şifre 4 rakam olmalı.")
	}
	ns := s.set
	ns.PinSalt = newSalt()
	ns.PinHash = hashPin(pin, ns.PinSalt)
	if err := s.withTx(func(tx *sql.Tx) error {
		if err := saveSettingsTx(tx, ns); err != nil {
			return err
		}
		audit(tx, s.now(), "sifre_sifirlandi", 0, nil)
		return nil
	}); err != nil {
		return err
	}
	s.set = ns
	// Yanlış deneme beklemesi de kalkar.
	s.pinFails, s.pinBlocks, s.pinBlockedUntil = 0, 0, time.Time{}
	s.savePinGuard()
	return nil
}

// ---- ayarlar ----

func cleanBusiness(b Business) (Business, error) {
	var err error
	out := Business{}
	if out.Name, err = required(singleLine(b.Name), 60, "İşletme adı"); err != nil {
		return out, err
	}
	fields := []struct {
		dst   *string
		src   string
		max   int
		label string
	}{
		{&out.Slogan, b.Slogan, 80, "Slogan"},
		{&out.Address1, b.Address1, 80, "Adres satırı 1"},
		{&out.Address2, b.Address2, 80, "Adres satırı 2"},
		{&out.Phone, b.Phone, 30, "Telefon"},
		{&out.Instagram, b.Instagram, 50, "Instagram"},
		{&out.Facebook, b.Facebook, 50, "Facebook"},
		{&out.TikTok, b.TikTok, 50, "TikTok"},
	}
	for _, f := range fields {
		if *f.dst, err = clean(singleLine(f.src), f.max, f.label); err != nil {
			return out, err
		}
	}
	if out.Invite, err = clean(b.Invite, 300, "Davet metni"); err != nil {
		return out, err
	}
	if out.GoogleURL, err = clean(singleLine(b.GoogleURL), 500, "Google yorum bağlantısı"); err != nil {
		return out, err
	}
	if out.GoogleURL != "" && !strings.HasPrefix(out.GoogleURL, "https://") && !strings.HasPrefix(out.GoogleURL, "http://") {
		return out, uerr("invalid", "Google yorum bağlantısı https:// ile başlamalı.")
	}
	return out, nil
}

func checkWritableDir(dir string) error {
	if !filepath.IsAbs(dir) {
		return uerr("invalid", "Klasör tam yol olmalı. Örnek: E:\\Yedek")
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return uerr("invalid", "Klasör bulunamadı: %s. USB bellek takılı mı?", dir)
	}
	f, err := os.CreateTemp(dir, ".halis_yazma_testi_*")
	if err != nil {
		return uerr("invalid", "Bu klasöre yazılamıyor: %s", dir)
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return nil
}

// SettingsPatch ayarlarda değişen bölümlerdir (boş alanlar değişmez).
type SettingsPatch struct {
	Business        *Business        `json:"business"`
	Tables          *int             `json:"tables"`
	DefaultVAT      *int             `json:"defaultVat"`
	LockMinutes     *int             `json:"lockMinutes"`
	Printer         *printing.Config `json:"printer"`
	SecondBackupDir *string          `json:"secondBackupDir"`
	Fullscreen      *bool            `json:"fullscreen"`
}

// SaveSettings ayarları doğrular ve kaydeder.
func (s *Service) SaveSettings(p SettingsPatch) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ns := s.set
	var changed []string
	if p.Business != nil {
		b, err := cleanBusiness(*p.Business)
		if err != nil {
			return nil, err
		}
		ns.Business = b
		changed = append(changed, "fiş bilgileri")
	}
	if p.Tables != nil {
		n := *p.Tables
		if n < 1 || n > 200 {
			return nil, uerr("invalid", "Masa sayısı 1 ile 200 arasında olmalı.")
		}
		var maxOpen int
		if err := s.db.QueryRow(`SELECT COALESCE(MAX(table_no),0) FROM adisyons WHERE status IN ('open','printed')`).Scan(&maxOpen); err != nil {
			return nil, err
		}
		if n < maxOpen {
			return nil, uerr("invalid", "Masa %d açık. Masa sayısını azaltmadan önce o masayı kapatın.", maxOpen)
		}
		ns.Tables = n
		changed = append(changed, "masa sayısı "+strconv.Itoa(n))
	}
	if p.DefaultVAT != nil {
		if !validVAT(*p.DefaultVAT) {
			return nil, uerr("invalid", "KDV oranı %%1, %%10 ya da %%20 olmalı.")
		}
		ns.DefaultVAT = *p.DefaultVAT
		changed = append(changed, "varsayılan KDV %"+strconv.Itoa(*p.DefaultVAT))
	}
	if p.LockMinutes != nil {
		if *p.LockMinutes < 1 || *p.LockMinutes > 60 {
			return nil, uerr("invalid", "Kilit süresi 1 ile 60 dakika arasında olmalı.")
		}
		ns.LockMinutes = *p.LockMinutes
		changed = append(changed, "kilit süresi")
	}
	if p.Printer != nil {
		c := *p.Printer
		if err := c.Normalize(); err != nil {
			return nil, uerr("invalid", "%s", err.Error())
		}
		ns.Printer = c
		changed = append(changed, "yazıcı")
	}
	if p.SecondBackupDir != nil {
		dir := strings.TrimSpace(*p.SecondBackupDir)
		if dir != "" {
			if err := checkWritableDir(dir); err != nil {
				return nil, err
			}
		}
		ns.SecondBackupDir = dir
		changed = append(changed, "ikinci yedek yeri")
	}
	if p.Fullscreen != nil {
		ns.Fullscreen = *p.Fullscreen
		changed = append(changed, "tam ekran")
	}
	if err := s.withTx(func(tx *sql.Tx) error {
		if err := saveSettingsTx(tx, ns); err != nil {
			return err
		}
		audit(tx, s.now(), "ayar_degisti", 0, map[string]any{"bolum": changed})
		return nil
	}); err != nil {
		return nil, err
	}
	s.set = ns
	if p.LockMinutes != nil && s.now().Before(s.unlockedUntil) {
		s.unlockedUntil = s.now().Add(s.lockDur())
	}
	return s.stateLocked()
}

// SetLogo fiş logosunu kaydeder (boş verilirse kaldırır). Arayüz logoyu PNG'ye çevirip gönderir.
func (s *Service) SetLogo(b64 string) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	b64 = strings.TrimSpace(b64)
	if i := strings.Index(b64, ","); strings.HasPrefix(b64, "data:") && i > 0 {
		b64 = b64[i+1:]
	}
	var raw []byte
	var img image.Image
	if b64 != "" {
		var err error
		raw, err = base64.StdEncoding.DecodeString(b64)
		if err != nil || len(raw) == 0 {
			return nil, uerr("invalid", "Logo dosyası okunamadı.")
		}
		if len(raw) > 2<<20 {
			return nil, uerr("invalid", "Logo dosyası çok büyük (en fazla 2 MB).")
		}
		img, _, err = image.Decode(bytes.NewReader(raw))
		if err != nil {
			return nil, uerr("invalid", "Logo PNG ya da JPG olmalı.")
		}
		if b := img.Bounds(); b.Dx() < 8 || b.Dy() < 8 || b.Dx() > 4000 || b.Dy() > 4000 {
			return nil, uerr("invalid", "Logo boyutu uygun değil.")
		}
	}
	if err := s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES('logo',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
			base64.StdEncoding.EncodeToString(raw)); err != nil {
			return err
		}
		audit(tx, s.now(), "logo", 0, map[string]any{"var": len(raw) > 0})
		return nil
	}); err != nil {
		return nil, err
	}
	s.logo, s.logoImg = raw, img
	s.logoRev = s.now().UnixMilli()
	return s.stateLocked()
}

// SecondBackupDir yedek bileşeninin ikinci yedek yerini okumasını sağlar.
func (s *Service) SecondBackupDir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.set.SecondBackupDir
}

// BackupNow hemen yedek alır.
func (s *Service) BackupNow() (*State, error) {
	s.mu.Lock()
	err := s.requireUnlocked()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if s.opt.Backup == nil {
		return nil, uerr("invalid", "Yedekleme kullanılamıyor.")
	}
	if err := s.opt.Backup.Run("elle"); err != nil {
		_, _, msg, _ := s.opt.Backup.Status()
		if msg == "" {
			msg = "Yedek dosyası yazılamadı."
		}
		return nil, uerr("backup", "Yedek alınamadı. %s", msg)
	}
	return s.State()
}

// SetAutostart Windows açılınca programın kendiliğinden başlamasını açar/kapatır.
func (s *Service) SetAutostart(on bool) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	if err := s.opt.Desktop.SetAutostart(on); err != nil {
		return nil, uerr("invalid", "Ayar yapılamadı: %s", err.Error())
	}
	audit(s.db, s.now(), "otomatik_baslat", 0, map[string]any{"acik": on})
	return s.stateLocked()
}

// Window pencere işlemi yapar: "fullscreen", "minimize", "quit".
func (s *Service) Window(action string) error {
	switch action {
	case "fullscreen", "minimize", "quit":
	default:
		return uerr("invalid", "Geçersiz pencere işlemi.")
	}
	return s.opt.Desktop.WindowAction(action)
}

// OpenFolder yedek ya da veri klasörünü Windows Gezgini'nde açar.
func (s *Service) OpenFolder(which string) error {
	s.mu.Lock()
	err := s.requireUnlocked()
	second := s.set.SecondBackupDir
	s.mu.Unlock()
	if err != nil {
		return err
	}
	var dir string
	switch which {
	case "backup":
		if s.opt.Backup != nil {
			dir, _, _, _ = s.opt.Backup.Status()
		}
	case "second":
		dir = second
	case "data":
		dir = s.opt.DataDir
	}
	if dir == "" {
		return uerr("invalid", "Klasör ayarlanmamış.")
	}
	if err := s.opt.Desktop.OpenFolder(dir); err != nil {
		return uerr("invalid", "Klasör açılamadı: %s", dir)
	}
	return nil
}

// Drives takılı USB bellekleri listeler.
func (s *Service) Drives() ([]Drive, error) {
	s.mu.Lock()
	err := s.requireUnlocked()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	d := s.opt.Desktop.RemovableDrives()
	if d == nil {
		d = []Drive{}
	}
	return d, nil
}

// ---- ilk kurulum ----

// SetupInput ilk açılış sihirbazında girilen bilgilerdir.
type SetupInput struct {
	Business Business         `json:"business"`
	Tables   int              `json:"tables"`
	Waiters  []string         `json:"waiters"`
	Printer  printing.Config  `json:"printer"`
	Prices   map[string]int64 `json:"prices"` // ürün no → kuruş
	Pin      string           `json:"pin"`
}

// FinishSetup ilk açılış sihirbazını tamamlar.
func (s *Service) FinishSetup(in SetupInput) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.set.SetupDone {
		return nil, uerr("invalid", "Kurulum zaten tamamlanmış.")
	}
	b, err := cleanBusiness(in.Business)
	if err != nil {
		return nil, err
	}
	if in.Tables < 1 || in.Tables > 200 {
		return nil, uerr("invalid", "Masa sayısı 1 ile 200 arasında olmalı.")
	}
	var names []string
	seen := map[string]bool{}
	for _, w := range in.Waiters {
		n, err := clean(singleLine(w), 30, "Garson adı")
		if err != nil {
			return nil, err
		}
		if n == "" || seen[strings.ToLower(n)] {
			continue
		}
		seen[strings.ToLower(n)] = true
		names = append(names, n)
	}
	if len(names) == 0 {
		return nil, uerr("invalid", "En az bir garson adı girin.")
	}
	if len(names) > 30 {
		return nil, uerr("invalid", "En fazla 30 garson girilebilir.")
	}
	pc := in.Printer
	if err := pc.Normalize(); err != nil {
		return nil, uerr("invalid", "%s", err.Error())
	}
	if !validPin(in.Pin) {
		return nil, uerr("invalid", "Yönetici şifresi 4 rakam olmalı.")
	}
	prices := map[int64]int64{}
	for k, v := range in.Prices {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			continue
		}
		if v < 0 || v > 10_000_000 {
			return nil, uerr("invalid", "Fiyatlar 0 ile 100.000 ₺ arasında olmalı.")
		}
		prices[id] = v
	}
	ns := s.set
	ns.Business = b
	ns.Tables = in.Tables
	ns.Printer = pc
	ns.PinSalt = newSalt()
	ns.PinHash = hashPin(in.Pin, ns.PinSalt)
	ns.SetupDone = true
	now := s.now()
	if err := s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE waiters SET active=0`); err != nil {
			return err
		}
		for i, n := range names {
			if _, err := tx.Exec(`INSERT INTO waiters(name,active,sort) VALUES(?,1,?)`, n, i); err != nil {
				return err
			}
		}
		for id, price := range prices {
			if _, err := tx.Exec(`UPDATE products SET price=?, updated_at=? WHERE id=?`, price, stamp(now), id); err != nil {
				return err
			}
		}
		if err := saveSettingsTx(tx, ns); err != nil {
			return err
		}
		audit(tx, now, "kurulum_tamamlandi", 0, map[string]any{"masa": in.Tables, "garson": len(names), "yazici": connectionText(pc)})
		return nil
	}); err != nil {
		return nil, err
	}
	s.set = ns
	return s.stateLocked()
}
