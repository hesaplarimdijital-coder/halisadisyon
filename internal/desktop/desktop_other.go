//go:build !windows

// Package desktop Windows dışında (geliştirme ve test) programı tarayıcı modunda çalıştırır.
package desktop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"halisadisyon/internal/core"
)

// AppTitle pencere başlığıdır.
const AppTitle = "Halis Adisyon"

// ErrNoWebView bu sistemde program penceresi olmadığını bildirir.
var ErrNoWebView = errors.New("bu sistemde program penceresi yok")

// Desktop tarayıcı modunda çalışan sahte masaüstüdür.
type Desktop struct{}

// New masaüstü yardımcısını kurar.
func New() *Desktop { return &Desktop{} }

var _ core.Desktop = (*Desktop)(nil)

// RunOptions pencere seçenekleridir.
type RunOptions struct {
	Fullscreen   bool
	DataDir      string
	OnEndSession func()
}

func PrepareProcess() {}

func DataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "halis-adisyon")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "halis-adisyon")
}

func BackupDir() string { return filepath.Join(DataDir(), "yedekler") }

func Alert(title, msg string) { fmt.Fprintf(os.Stderr, "%s: %s\n", title, msg) }
func Info(title, msg string)  { fmt.Fprintf(os.Stderr, "%s: %s\n", title, msg) }

func SingleInstance(dataDir string) (func(), bool) { return func() {}, true }

func (d *Desktop) Run(url string, opt RunOptions) error { return ErrNoWebView }

func (d *Desktop) Mode() string                  { return "browser" }
func (d *Desktop) Autostart() (bool, bool)       { return false, false }
func (d *Desktop) SetAutostart(bool) error       { return errors.New("bu sistemde desteklenmiyor") }
func (d *Desktop) WindowAction(string) error     { return errors.New("pencere yok") }
func (d *Desktop) IsFullscreen() bool            { return false }
func (d *Desktop) RemovableDrives() []core.Drive { return nil }
func (d *Desktop) OpenFolder(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return exec.Command("xdg-open", path).Start()
}

// RevealFile dosyanın klasörünü açar.
func (d *Desktop) RevealFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return exec.Command("xdg-open", filepath.Dir(path)).Start()
}

// ReportsDir Excel raporlarının klasörüdür.
func (d *Desktop) ReportsDir() string { return filepath.Join(DataDir(), "raporlar") }

// GoogleDriveDirs ev klasöründeki Google Drive klasörlerini verir (geliştirme için).
func (d *Desktop) GoogleDriveDirs() []string {
	home, _ := os.UserHomeDir()
	var out []string
	for _, n := range []string{"Google Drive", "My Drive", "Drive'ım"} {
		p := filepath.Join(home, n)
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

// RunBrowser adresi yazar ve program kapatılana kadar bekler.
func (d *Desktop) RunBrowser(url, dataDir string, lastPing func() time.Time, quit <-chan struct{}, noOpen bool) {
	fmt.Println("Halis Adisyon çalışıyor:", url)
	if !noOpen {
		exec.Command("xdg-open", url).Start()
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-quit:
	case <-sig:
	}
}

// Seal Windows dışında (geliştirme) veriyi olduğu gibi bırakır.
func Seal(b []byte) ([]byte, error) { return append([]byte(nil), b...), nil }

// Unseal Seal'in tersidir.
func Unseal(b []byte) ([]byte, error) { return append([]byte(nil), b...), nil }
