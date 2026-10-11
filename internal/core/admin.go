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
	"regexp"
	"strconv"
	"strings"
	"time"

	"halisadisyon/internal/printing"
	"halisadisyon/internal/remote"
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
		if err := s.saveSettingsTx(tx, ns); err != nil {
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
		if err := s.saveSettingsTx(tx, ns); err != nil {
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

func checkWritableDir(dir, hint string) error {
	if !filepath.IsAbs(dir) {
		return uerr("invalid", "Klasör tam yol olmalı. Örnek: E:\\Yedek")
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return uerr("invalid", "Klasör bulunamadı: %s. %s", dir, hint)
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
	DriveBackupDir  *string          `json:"driveBackupDir"`
	Fullscreen      *bool            `json:"fullscreen"`
	// FeaturedProductID istatistikte ve gün sonu mesajında öne çıkan ürün (0: adı Cağ olan).
	FeaturedProductID *int64         `json:"featuredProductId"`
	WhatsApp          *WhatsAppPatch `json:"whatsapp"`
	Sheets            *SheetsPatch   `json:"sheets"`
}

// WhatsAppPatch WhatsApp ayarında değişenlerdir. Token boş gelirse eskisi kalır.
type WhatsAppPatch struct {
	Enabled       *bool     `json:"enabled"`
	PhoneNumberID *string   `json:"phoneNumberId"`
	Token         *string   `json:"token"`
	ClearToken    bool      `json:"clearToken"`
	Template      *string   `json:"template"`
	Lang          *string   `json:"lang"`
	APIVersion    *string   `json:"apiVersion"`
	Recipients    *[]string `json:"recipients"`
}

// SheetsPatch Google Sheets ayarında değişenlerdir.
type SheetsPatch struct {
	Enabled   *bool   `json:"enabled"`
	URL       *string `json:"url"`
	NewSecret bool    `json:"newSecret"`
}

var (
	reDigits   = regexp.MustCompile(`^\d{5,30}$`)
	reTemplate = regexp.MustCompile(`^[a-z0-9_]{1,512}$`)
	reLang     = regexp.MustCompile(`^[a-z]{2,3}(_[A-Z]{2})?$`)
	reAPIVer   = regexp.MustCompile(`^v\d{1,3}\.\d{1,2}$`)
)

func newSecret() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func applyWhatsApp(w WhatsAppSettings, p *WhatsAppPatch) (WhatsAppSettings, error) {
	if p.PhoneNumberID != nil {
		v := strings.TrimSpace(*p.PhoneNumberID)
		if v != "" && !reDigits.MatchString(v) {
			return w, uerr("invalid", "Telefon numarası kimliği (Phone number ID) yalnız rakamlardan oluşur.")
		}
		w.PhoneNumberID = v
	}
	if p.ClearToken {
		w.Token = ""
	}
	if p.Token != nil {
		if v := strings.TrimSpace(*p.Token); v != "" {
			if len(v) > 2000 || strings.ContainsAny(v, " \t\r\n") {
				return w, uerr("invalid", "Erişim anahtarı geçersiz görünüyor; boşluksuz, tek parça yapıştırın.")
			}
			w.Token = v
		}
	}
	if p.Template != nil {
		v := strings.TrimSpace(*p.Template)
		if v != "" && !reTemplate.MatchString(v) {
			return w, uerr("invalid", "Şablon adı küçük harf, rakam ve alt çizgiden oluşur (örnek: gun_sonu_ozeti).")
		}
		w.Template = v
	}
	if p.Lang != nil {
		v := strings.TrimSpace(*p.Lang)
		if v == "" {
			v = "tr"
		}
		if !reLang.MatchString(v) {
			return w, uerr("invalid", "Şablon dili kodu geçersiz (Türkçe için: tr).")
		}
		w.Lang = v
	}
	if p.APIVersion != nil {
		v := strings.TrimSpace(*p.APIVersion)
		if v != "" && !reAPIVer.MatchString(v) {
			return w, uerr("invalid", "API sürümü v25.0 biçiminde olmalı.")
		}
		w.APIVersion = v
	}
	if p.Recipients != nil {
		seen := map[string]bool{}
		var out []string
		for _, r := range *p.Recipients {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			n := remote.NormalizePhone(r)
			if len(n) < 10 || len(n) > 15 {
				return w, uerr("invalid", "Telefon numarası anlaşılamadı: %s. Örnek: 0532 123 45 67", r)
			}
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
		if len(out) > 10 {
			return w, uerr("invalid", "En fazla 10 alıcı girilebilir.")
		}
		w.Recipients = out
	}
	if p.Enabled != nil {
		w.Enabled = *p.Enabled
	}
	if w.Enabled && (w.PhoneNumberID == "" || w.Token == "" || w.Template == "" || len(w.Recipients) == 0) {
		return w, uerr("invalid", "WhatsApp gönderimini açmak için telefon numarası kimliği, erişim anahtarı, şablon adı ve en az bir alıcı girin.")
	}
	return w, nil
}

func applySheets(sh SheetsSettings, p *SheetsPatch) (SheetsSettings, error) {
	if p.URL != nil {
		v := strings.TrimSpace(*p.URL)
		if v != "" && (!strings.HasPrefix(v, "https://script.google.com/macros/s/") || !strings.HasSuffix(v, "/exec")) {
			return sh, uerr("invalid", "Web uygulaması adresi https://script.google.com/macros/s/…/exec biçiminde olmalı.")
		}
		sh.URL = v
	}
	if p.NewSecret || sh.Secret == "" {
		sh.Secret = newSecret()
	}
	if p.Enabled != nil {
		sh.Enabled = *p.Enabled
	}
	if sh.Enabled && sh.URL == "" {
		return sh, uerr("invalid", "Google Sheets gönderimini açmak için web uygulaması adresini girin.")
	}
	return sh, nil
}

// SaveSettings ayarları doğrular ve kaydeder. Her değişiklik eski ve yeni
// değeriyle işlem kaydına yazılır (erişim anahtarı gibi gizli bilgiler hariç).
func (s *Service) SaveSettings(p SettingsPatch) (*State, error) {
	// Klasör denetimi (USB, Google Drive) yavaş olabilir: kilit dışında yapılır,
	// böylece takılan bir sürücü siparişleri bekletmez.
	if p.SecondBackupDir != nil {
		if dir := strings.TrimSpace(*p.SecondBackupDir); dir != "" {
			if err := checkWritableDir(dir, "USB bellek takılı mı?"); err != nil {
				return nil, err
			}
		}
	}
	if p.DriveBackupDir != nil {
		if dir := strings.TrimSpace(*p.DriveBackupDir); dir != "" {
			if err := checkWritableDir(dir, "Google Drive programı açık mı?"); err != nil {
				return nil, err
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ns := s.set
	var changed, detail []string
	diff := func(label, old, nw string) {
		if old != nw {
			detail = append(detail, fmt.Sprintf("%s: %s → %s", label, shortQuote(old), shortQuote(nw)))
		}
	}
	onOff := func(b bool) string {
		if b {
			return "açık"
		}
		return "kapalı"
	}
	if p.Business != nil {
		b, err := cleanBusiness(*p.Business)
		if err != nil {
			return nil, err
		}
		o := ns.Business
		diff("İşletme adı", o.Name, b.Name)
		diff("Slogan", o.Slogan, b.Slogan)
		diff("Adres 1", o.Address1, b.Address1)
		diff("Adres 2", o.Address2, b.Address2)
		diff("Telefon", o.Phone, b.Phone)
		diff("Instagram", o.Instagram, b.Instagram)
		diff("Facebook", o.Facebook, b.Facebook)
		diff("TikTok", o.TikTok, b.TikTok)
		diff("Davet metni", o.Invite, b.Invite)
		diff("Google yorum bağlantısı", o.GoogleURL, b.GoogleURL)
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
		diff("Masa sayısı", strconv.Itoa(ns.Tables), strconv.Itoa(n))
		ns.Tables = n
		changed = append(changed, "masa sayısı")
	}
	if p.DefaultVAT != nil {
		if !validVAT(*p.DefaultVAT) {
			return nil, uerr("invalid", "KDV oranı %%1, %%10 ya da %%20 olmalı.")
		}
		diff("Varsayılan KDV", "%"+strconv.Itoa(ns.DefaultVAT), "%"+strconv.Itoa(*p.DefaultVAT))
		ns.DefaultVAT = *p.DefaultVAT
		changed = append(changed, "varsayılan KDV")
	}
	if p.LockMinutes != nil {
		if *p.LockMinutes < 1 || *p.LockMinutes > 60 {
			return nil, uerr("invalid", "Kilit süresi 1 ile 60 dakika arasında olmalı.")
		}
		diff("Kilit süresi", fmt.Sprintf("%d dk", ns.LockMinutes), fmt.Sprintf("%d dk", *p.LockMinutes))
		ns.LockMinutes = *p.LockMinutes
		changed = append(changed, "kilit süresi")
	}
	if p.Printer != nil {
		c := *p.Printer
		if err := c.Normalize(); err != nil {
			return nil, uerr("invalid", "%s", err.Error())
		}
		diff("Yazıcı", connectionText(ns.Printer), connectionText(c))
		diff("Kâğıt", fmt.Sprintf("%d nokta", ns.Printer.Width), fmt.Sprintf("%d nokta", c.Width))
		diff("Kesme", cutText(ns.Printer), cutText(c))
		ns.Printer = c
		changed = append(changed, "yazıcı")
	}
	if p.SecondBackupDir != nil {
		dir := strings.TrimSpace(*p.SecondBackupDir)
		diff("İkinci yedek yeri", ns.SecondBackupDir, dir)
		ns.SecondBackupDir = dir
		changed = append(changed, "ikinci yedek yeri")
	}
	if p.DriveBackupDir != nil {
		dir := strings.TrimSpace(*p.DriveBackupDir)
		diff("Google Drive klasörü", ns.DriveBackupDir, dir)
		ns.DriveBackupDir = dir
		changed = append(changed, "Google Drive yedek klasörü")
	}
	if p.FeaturedProductID != nil {
		id := *p.FeaturedProductID
		name := func(id int64) string {
			if id == 0 {
				return "otomatik"
			}
			if p, err := loadProduct(s.db, id); err == nil {
				return p.Name
			}
			return "?"
		}
		if id != 0 {
			if _, err := loadProduct(s.db, id); err != nil {
				return nil, err
			}
		}
		diff("Öne çıkan ürün", name(ns.FeaturedProductID), name(id))
		ns.FeaturedProductID = id
		changed = append(changed, "öne çıkan ürün")
	}
	if p.WhatsApp != nil {
		w, err := applyWhatsApp(ns.WhatsApp, p.WhatsApp)
		if err != nil {
			return nil, err
		}
		o := ns.WhatsApp
		diff("WhatsApp gönderimi", onOff(o.Enabled), onOff(w.Enabled))
		diff("WhatsApp numara kimliği", o.PhoneNumberID, w.PhoneNumberID)
		diff("WhatsApp şablonu", o.Template, w.Template)
		diff("WhatsApp dili", o.Lang, w.Lang)
		diff("WhatsApp API sürümü", o.APIVersion, w.APIVersion)
		diff("WhatsApp alıcıları", strings.Join(o.Recipients, ", "), strings.Join(w.Recipients, ", "))
		if o.Token != w.Token {
			detail = append(detail, "WhatsApp erişim anahtarı değişti")
		}
		ns.WhatsApp = w
		s.tokenLost = s.tokenLost && w.Token == ""
		changed = append(changed, "WhatsApp")
	}
	if p.Sheets != nil {
		sh, err := applySheets(ns.Sheets, p.Sheets)
		if err != nil {
			return nil, err
		}
		diff("Google Sheets gönderimi", onOff(ns.Sheets.Enabled), onOff(sh.Enabled))
		diff("Google Sheets adresi", ns.Sheets.URL, sh.URL)
		if ns.Sheets.Secret != "" && ns.Sheets.Secret != sh.Secret {
			detail = append(detail, "Google Sheets gizli anahtarı yenilendi")
		}
		ns.Sheets = sh
		changed = append(changed, "Google Sheets")
	}
	if p.Fullscreen != nil {
		diff("Açılışta tam ekran", onOff(ns.Fullscreen), onOff(*p.Fullscreen))
		ns.Fullscreen = *p.Fullscreen
		changed = append(changed, "tam ekran")
	}
	if err := s.withTx(func(tx *sql.Tx) error {
		if err := s.saveSettingsTx(tx, ns); err != nil {
			return err
		}
		if len(detail) > 0 {
			audit(tx, s.now(), "ayar_degisti", 0, map[string]any{"bolum": changed, "degisiklik": detail})
		}
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

// shortQuote işlem kaydı için değeri tırnak içinde, en fazla 60 karakter yazar.
func shortQuote(v string) string {
	if v == "" {
		return "(boş)"
	}
	r := []rune(strings.ReplaceAll(v, "\n", " "))
	if len(r) > 60 {
		r = append(r[:57], '…')
	}
	return "“" + string(r) + "”"
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
		msg := s.opt.Backup.Status().LastError
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
			dir = s.opt.Backup.Status().Dir
		}
	case "second":
		dir = second
	case "drive":
		s.mu.Lock()
		dir = s.set.DriveBackupDir
		s.mu.Unlock()
	case "reports":
		dir = s.opt.Desktop.ReportsDir()
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

// DriveDirs bilgisayardaki Google Drive klasörlerini bulur (yedek için).
// Bulunamayan sürücü ekranı bekletmesin diye en fazla 5 saniye aranır.
func (s *Service) DriveDirs() ([]string, error) {
	s.mu.Lock()
	err := s.requireUnlocked()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	ch := make(chan []string, 1)
	go func() { ch <- s.opt.Desktop.GoogleDriveDirs() }()
	select {
	case d := <-ch:
		if d == nil {
			d = []string{}
		}
		return d, nil
	case <-time.After(5 * time.Second):
		return []string{}, nil
	}
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
		if err := s.saveSettingsTx(tx, ns); err != nil {
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
