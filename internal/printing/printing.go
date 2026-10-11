// Package printing hazırlanmış ESC/POS baytlarını yazıcıya gönderir.
//
// Üç bağlantı türü vardır:
//   - "windows": Windows yazıcı kuyruğu (Koddata sürücüsü kurulu, USB kablo)
//   - "network": Ethernet ile IP adresi üzerinden (TCP 9100)
//   - "none":    yazıcı yok; fiş yalnızca ekranda önizlenir
package printing

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Bağlantı türleri.
const (
	ModeWindows = "windows"
	ModeNetwork = "network"
	ModeNone    = "none"
)

// Config yazıcı ayarlarıdır.
type Config struct {
	Mode    string `json:"mode"`
	Name    string `json:"name"`    // Windows yazıcı adı
	Host    string `json:"host"`    // ağ yazıcısı IP adresi
	Port    int    `json:"port"`    // ağ yazıcısı portu (genelde 9100)
	Width   int    `json:"width"`   // baskı genişliği (nokta): 576 = 80 mm
	Cut     bool   `json:"cut"`     // her fişten sonra kâğıdı kes
	CutMode string `json:"cutMode"` // "partial" (kısmi) | "full" (tam)
}

// DefaultConfig Koddata KPS80 için başlangıç ayarlarıdır.
func DefaultConfig() Config {
	return Config{Mode: ModeWindows, Port: 9100, Width: 576, Cut: true, CutMode: "partial"}
}

// Widths desteklenen baskı genişlikleridir (nokta).
var Widths = []int{576, 512, 384}

// Normalize eksik alanları doldurur ve ayarı doğrular.
func (c *Config) Normalize() error {
	c.Mode = strings.TrimSpace(c.Mode)
	c.Name = strings.TrimSpace(c.Name)
	c.Host = strings.TrimSpace(c.Host)
	switch c.Mode {
	case ModeWindows, ModeNetwork, ModeNone:
	default:
		return errors.New("Yazıcı bağlantı türü geçersiz.")
	}
	if c.Port == 0 {
		c.Port = 9100
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("Port 1 ile 65535 arasında olmalı (genelde 9100).")
	}
	ok := false
	for _, w := range Widths {
		if c.Width == w {
			ok = true
		}
	}
	if !ok {
		c.Width = 576
	}
	if c.CutMode != "full" {
		c.CutMode = "partial"
	}
	if c.Mode == ModeNetwork && c.Host != "" && net.ParseIP(c.Host) == nil {
		if strings.ContainsAny(c.Host, " /\\") {
			return errors.New("IP adresi geçersiz. Örnek: 192.168.1.87")
		}
	}
	if len(c.Name) > 200 || len(c.Host) > 200 {
		return errors.New("Yazıcı adı çok uzun.")
	}
	return nil
}

// PrinterInfo Windows'ta kurulu bir yazıcıdır.
type PrinterInfo struct {
	Name    string `json:"name"`
	Port    string `json:"port"`
	Driver  string `json:"driver"`
	Default bool   `json:"default"`
}

// Result bir baskı denemesinin sonucudur. Mesajlar garsonun anlayacağı dilde yazılır.
type Result struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code,omitempty"`
	Title   string `json:"title,omitempty"`
	Message string `json:"message,omitempty"`
	Hint    string `json:"hint,omitempty"`
	Detail  string `json:"detail,omitempty"` // teknik ayrıntı (kayıt için)
}

func fail(code, title, msg, hint, detail string) Result {
	return Result{Code: code, Title: title, Message: msg, Hint: hint, Detail: detail}
}

// Ortak hata metinleri.
const (
	titleFailed = "Adisyon basılamadı"
	msgUnreach  = "Yazıcıya ulaşılamıyor. Kablo takılı ve yazıcı açık mı?"
	hintUnreach = "Yazıcının ışığını ve kâğıt kapağını kontrol edin, sonra Tekrar Dene'ye basın."
	msgPaper    = "Yazıcıda kâğıt bitmiş ya da kapak açık olabilir."
	hintPaper   = "Yeni rulo takıp kapağı kapatın, sonra Tekrar Dene'ye basın."
	msgNotSet   = "Yazıcı seçilmemiş."
	hintNotSet  = "Yöneticiden Ayarlar › Yazıcı bölümünden yazıcıyı seçmesini isteyin."
)

// jobWait bir işin yazıcıda bitmesi için beklenecek süredir. Uzun fişler daha
// uzun sürer: yazıcı veriyi bastığı hızda alır (KPS80 için ~140 KB/sn; burada
// temkinli olarak 50 KB/sn sayılır).
func jobWait(n int) time.Duration {
	d := 6*time.Second + time.Duration(n)*time.Second/(50*1024)
	if d > 20*time.Second {
		d = 20 * time.Second
	}
	return d
}

// jobHardLimit Windows kuyruğunda yazıcıya veri akarken (hata yokken) bir işin
// bitmesi için beklenecek en uzun süredir.
const jobHardLimit = 25 * time.Second

// sendSlot aynı anda yalnızca bir işin yazıcıya gitmesini sağlar.
var sendSlot = make(chan struct{}, 1)

// Logf geç gelen baskı sonuçlarını kayıt dosyasına yazmak için ayarlanır.
var Logf = func(format string, args ...any) {}

// Send baytları ayardaki yazıcıya gönderir. Yazıcı ya da Windows yazıcı kuyruğu
// takılsa bile en geç birkaç saniye içinde bir sonuç döner; takılan gönderim
// bitene kadar yeni baskı başlatılmaz.
func Send(cfg Config, data []byte, docName string) Result {
	switch cfg.Mode {
	case ModeNone:
		return Result{OK: true}
	case ModeNetwork:
		if strings.TrimSpace(cfg.Host) == "" {
			return fail("not_configured", titleFailed, "Ağ yazıcısının IP adresi girilmemiş.", hintNotSet, "network host empty")
		}
	case ModeWindows:
		if strings.TrimSpace(cfg.Name) == "" {
			return fail("not_configured", titleFailed, msgNotSet, hintNotSet, "windows printer name empty")
		}
	default:
		return fail("not_configured", titleFailed, msgNotSet, hintNotSet, "unknown mode "+cfg.Mode)
	}
	if len(data) == 0 {
		return fail("error", titleFailed, msgUnreach, hintUnreach, "empty data")
	}
	wait := jobWait(len(data))
	select {
	case sendSlot <- struct{}{}:
	case <-time.After(8 * time.Second):
		return fail("busy", titleFailed, "Yazıcı önceki fişi henüz bitirmedi.",
			"Birkaç saniye sonra Tekrar Dene'ye basın. Sorun sürerse yazıcıyı ve programı kapatıp açın.",
			"previous send still running")
	}
	done := make(chan Result, 1)
	go func() {
		defer func() { <-sendSlot }()
		defer func() {
			if r := recover(); r != nil {
				done <- fail("error", titleFailed, msgUnreach, hintUnreach, fmt.Sprint("panic: ", r))
			}
		}()
		if cfg.Mode == ModeNetwork {
			done <- sendNetwork(cfg, data)
		} else {
			done <- sendWindows(cfg, data, docName, wait)
		}
	}()
	limit := wait
	if limit < jobHardLimit {
		limit = jobHardLimit
	}
	select {
	case r := <-done:
		return r
	case <-time.After(limit + 10*time.Second):
		go func() {
			r := <-done
			Logf("baskı sonucu geç geldi (%s): ok=%v kod=%s ayrıntı=%s", docName, r.OK, r.Code, r.Detail)
		}()
		return fail("timeout", titleFailed, "Yazıcı ya da Windows yazıcı kuyruğu yanıt vermiyor.",
			"Fiş çıktıysa tekrar basmayın. Çıkmadıysa yazıcıyı kapatıp açın, sonra Tekrar Dene'ye basın.",
			"send did not return in time")
	}
}

func dialPrinter(cfg Config) (net.Conn, *Result) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		r := fail("network", titleFailed,
			fmt.Sprintf("Yazıcıya ağdan ulaşılamıyor (IP %s). Yazıcı açık mı, ağ kablosu takılı mı?", cfg.Host),
			"Yazıcıyı kapatıp açın, bir dakika bekleyip Tekrar Dene'ye basın. Sorun sürerse IP adresini kontrol edin.",
			err.Error())
		return nil, &r
	}
	return conn, nil
}

func sendNetwork(cfg Config, data []byte) Result {
	conn, ferr := dialPrinter(cfg)
	if ferr != nil {
		return *ferr
	}
	// Yazıcının durumunu sor; kâğıt bitmişse ya da kapak açıksa hiç gönderme.
	r, bad, clean := networkStatus(conn)
	if bad {
		conn.Close()
		return r
	}
	if !clean {
		// Yanıt gelmedi ya da anlaşılmadı: geç gelen bir yanıt bu bağlantıyı
		// bozmasın (okunmamış veri kapanışta bağlantıyı koparır), yenisini aç.
		conn.Close()
		if conn, ferr = dialPrinter(cfg); ferr != nil {
			return *ferr
		}
	}
	tcp, _ := conn.(*net.TCPConn)
	_ = conn.SetDeadline(time.Time{})
	_ = conn.SetWriteDeadline(time.Now().Add(jobWait(len(data))))
	if _, err := conn.Write(data); err != nil {
		// Gönderilemeyen kısım bağlantı kapanınca da yazıcıya gitmesin.
		if tcp != nil {
			_ = tcp.SetLinger(0)
		}
		conn.Close()
		return fail("network", titleFailed, "Yazıcı fişi almadı. Kâğıt bitmiş ya da yazıcı kapanmış olabilir.",
			"Yazıcıyı kontrol edin; yarım çıkan fiş varsa atın, sonra Tekrar Dene'ye basın.", err.Error())
	}
	if tcp != nil {
		_ = tcp.CloseWrite()
	}
	// Yazıcının gönderdiği baytları (durum bilgisi) okuyup bağlantıyı onun
	// kapatmasını bekle; okunmamış veri bırakılırsa kalan fiş yolda kesilebilir.
	drain := 3*time.Second + time.Duration(len(data))*time.Second/(100*1024)
	if drain > 8*time.Second {
		drain = 8 * time.Second
	}
	_ = conn.SetReadDeadline(time.Now().Add(drain))
	_, _ = io.Copy(io.Discard, conn)
	conn.Close()
	return Result{OK: true}
}

// networkStatus ESC/POS gerçek zamanlı durum komutlarıyla (DLE EOT) yazıcıya
// kâğıt ve kapak durumunu sorar. Yanıt vermeyen ya da anlaşılmayan yanıt veren
// yazıcıda baskı engellenmez. bad: baskı yapılmamalı; clean: iki soru da
// zamanında ve geçerli yanıtlandı (bağlantıda bekleyen bayt yok).
func networkStatus(conn net.Conn) (res Result, bad, clean bool) {
	ask := func(n byte) (byte, bool) {
		_ = conn.SetDeadline(time.Now().Add(700 * time.Millisecond))
		if _, err := conn.Write([]byte{0x10, 0x04, n}); err != nil {
			return 0, false
		}
		var b [1]byte
		if _, err := io.ReadFull(conn, b[:]); err != nil {
			return 0, false
		}
		// Geçerli durum baytında 1. ve 4. bitler 1, 0. ve 7. bitler 0'dır.
		if b[0]&0x93 != 0x12 {
			return 0, false
		}
		return b[0], true
	}
	paper, ok := ask(4) // kâğıt sensörü
	if !ok {
		return Result{}, false, false
	}
	if paper&0x60 == 0x60 {
		return fail("paper", titleFailed, "Yazıcıda kâğıt bitmiş.", hintPaper, fmt.Sprintf("DLE EOT 4 = 0x%02x", paper)), true, false
	}
	cause, ok := ask(2) // çevrimdışı nedeni
	if !ok {
		return Result{}, false, false
	}
	switch {
	case cause&0x04 != 0:
		return fail("cover", titleFailed, "Yazıcının kapağı açık.", "Kapağı tam kapatın (tık sesi gelmeli), sonra Tekrar Dene'ye basın.", fmt.Sprintf("DLE EOT 2 = 0x%02x", cause)), true, false
	case cause&0x20 != 0:
		return fail("paper", titleFailed, "Yazıcıda kâğıt bitmiş.", hintPaper, fmt.Sprintf("DLE EOT 2 = 0x%02x", cause)), true, false
	case cause&0x40 != 0:
		return fail("printer_error", titleFailed, "Yazıcı hata veriyor; kesicide kâğıt sıkışmış olabilir.",
			"Yazıcıyı kapatın, kapağı açıp sıkışan kâğıdı çıkarın, açıp Tekrar Dene'ye basın.", fmt.Sprintf("DLE EOT 2 = 0x%02x", cause)), true, false
	}
	return Result{}, false, true
}

// ListPrinters Windows'ta kurulu yazıcıları ve varsayılan yazıcıyı verir.
// Windows dışındaki sistemlerde boş liste döner.
func ListPrinters() ([]PrinterInfo, string) { return listPrinters() }

// GuessThermal listedeki fiş yazıcısını tahmin eder (Koddata, KPS, POS, 80...).
func GuessThermal(list []PrinterInfo) string {
	keys := []string{"koddata", "kps", "pos", "receipt", "thermal", "termal", "80"}
	for _, k := range keys {
		for _, p := range list {
			if strings.Contains(strings.ToLower(p.Name), k) {
				return p.Name
			}
		}
	}
	return ""
}
