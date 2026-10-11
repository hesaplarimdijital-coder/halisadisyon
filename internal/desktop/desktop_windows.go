//go:build windows

// Package desktop programın Windows'taki penceresini (WebView2) ve sistem işlerini yönetir:
// tek kopya çalışma, tam ekran, Windows açılınca başlama, klasör açma, USB bellek listesi.
package desktop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"halisadisyon/internal/core"
)

// AppTitle pencere başlığıdır (kurulum programı da bu adla pencereyi bulur).
const AppTitle = "Halis Adisyon"

const (
	runKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue = "HalisAdisyon"
)

var (
	user32                            = windows.NewLazySystemDLL("user32.dll")
	procShowWindow                    = user32.NewProc("ShowWindow")
	procSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	procFindWindowW                   = user32.NewProc("FindWindowW")
	procIsIconic                      = user32.NewProc("IsIconic")
	procGetWindowLongPtrW             = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW             = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos                  = user32.NewProc("SetWindowPos")
	procGetWindowPlacement            = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement            = user32.NewProc("SetWindowPlacement")
	procMonitorFromWindow             = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procPeekMessageW                  = user32.NewProc("PeekMessageW")
	procCallWindowProcW               = user32.NewProc("CallWindowProcW")
)

const (
	swShowNormal            = 1
	swMaximize              = 3
	swMinimize              = 6
	swRestore               = 9
	wsOverlappedWindow      = 0x00CF0000
	swpNoSize               = 0x0001
	swpNoMove               = 0x0002
	swpNoZOrder             = 0x0004
	swpFrameChanged         = 0x0020
	swpNoOwnerZOrder        = 0x0200
	monitorDefaultToNearest = 2
	mbOK                    = 0x0
	mbIconError             = 0x10
	mbIconInfo              = 0x40
	driveRemovable          = 2
)

var (
	gwlStyle    = ^uintptr(15) // GWL_STYLE = -16
	gwlpWndProc = ^uintptr(3)  // GWLP_WNDPROC = -4
)

const (
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	pmRemove          = 0x0001
)

type rect struct{ Left, Top, Right, Bottom int32 }
type point struct{ X, Y int32 }
type windowPlacement struct {
	Length         uint32
	Flags          uint32
	ShowCmd        uint32
	MinPosition    point
	MaxPosition    point
	NormalPosition rect
}
type monitorInfo struct {
	CbSize  uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

// ErrNoWebView WebView2 penceresi açılamadığında döner.
var ErrNoWebView = errors.New("WebView2 penceresi açılamadı")

// Desktop core.Desktop arayüzünü Windows için uygular.
type Desktop struct {
	mu          sync.Mutex
	w           webview2.WebView
	hwnd        uintptr
	fullscreen  bool
	savedStyle  uintptr
	savedPlace  windowPlacement
	browserMode bool
}

// New masaüstü yardımcısını kurar.
func New() *Desktop { return &Desktop{} }

var _ core.Desktop = (*Desktop)(nil)

// PrepareProcess pencereler açılmadan önce yüksek DPI desteğini açar (keskin yazı).
func PrepareProcess() {
	if procSetProcessDpiAwarenessContext.Find() == nil {
		// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
		if r, _, _ := procSetProcessDpiAwarenessContext.Call(^uintptr(3)); r != 0 {
			return
		}
	}
	if procSetProcessDPIAware.Find() == nil {
		procSetProcessDPIAware.Call()
	}
}

func utf16(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}

// DataDir verilerin tutulduğu klasördür: %LOCALAPPDATA%\HalisAdisyon
func DataDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	return filepath.Join(base, "HalisAdisyon")
}

// BackupDir otomatik yedeklerin klasörüdür: Belgeler\Halis Adisyon Yedekleri
func BackupDir() string {
	docs, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0)
	if err != nil || docs == "" {
		home, _ := os.UserHomeDir()
		docs = filepath.Join(home, "Documents")
	}
	return filepath.Join(docs, "Halis Adisyon Yedekleri")
}

// Alert hata penceresi gösterir.
func Alert(title, msg string) {
	windows.MessageBox(0, utf16(msg), utf16(title), mbOK|mbIconError)
}

// Info bilgi penceresi gösterir.
func Info(title, msg string) {
	windows.MessageBox(0, utf16(msg), utf16(title), mbOK|mbIconInfo)
}

func findAppWindow() uintptr {
	h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(utf16("webview"))), uintptr(unsafe.Pointer(utf16(AppTitle))))
	return h
}

// SingleInstance program zaten açıksa onun penceresini öne getirir ve false döner.
func SingleInstance(dataDir string) (release func(), ok bool) {
	h, err := windows.CreateMutex(nil, false, utf16(`Local\HalisAdisyon-tek-kopya`))
	if err == windows.ERROR_ALREADY_EXISTS {
		if h != 0 {
			windows.CloseHandle(h)
		}
		// Diğer kopya henüz açılıyor ya da kapanıyor olabilir: penceresini, adresini
		// ya da kapanıp kilidi bırakmasını birkaç saniye bekle.
		for i := 0; i < 60; i++ {
			if i > 0 {
				h2, err2 := windows.CreateMutex(nil, false, utf16(`Local\HalisAdisyon-tek-kopya`))
				if err2 == nil && h2 != 0 {
					return func() { windows.CloseHandle(h2) }, true // önceki kopya kapandı
				}
				if h2 != 0 {
					windows.CloseHandle(h2)
				}
			}
			if hwnd := findAppWindow(); hwnd != 0 {
				if r, _, _ := procIsIconic.Call(hwnd); r != 0 {
					procShowWindow.Call(hwnd, swRestore)
				}
				procSetForegroundWindow.Call(hwnd)
				return nil, false
			}
			if b, err := os.ReadFile(filepath.Join(dataDir, "calisiyor.txt")); err == nil && len(b) > 0 {
				// Tarayıcı modunda çalışıyor: aynı adresi yeniden aç.
				openURL(strings.TrimSpace(string(b)))
				return nil, false
			}
			time.Sleep(250 * time.Millisecond)
		}
		Info(AppTitle, "Halis Adisyon zaten açık ya da açılıyor.\nBirkaç saniye bekleyin; açılmazsa bilgisayarı yeniden başlatın.")
		return nil, false
	}
	if err != nil && h == 0 {
		return func() {}, true // kilit alınamadı; yine de çalış
	}
	return func() { windows.CloseHandle(h) }, true
}

// RunOptions pencere seçenekleridir.
type RunOptions struct {
	Fullscreen bool
	DataDir    string
	// OnEndSession Windows kapanırken/oturum kapanırken çağrılır; ardından
	// Windows programı sonlandırır. Kısa sürmelidir.
	OnEndSession func()
}

// Run programın penceresini açar ve kapanana kadar bekler. Ana iş parçacığından çağrılmalıdır.
func (d *Desktop) Run(url string, opt RunOptions) error {
	v, err := webviewloader.GetInstalledVersion()
	if err != nil || v == "" {
		if err == nil {
			err = errors.New("WebView2 kurulu değil")
		}
		return fmt.Errorf("%w: %v", ErrNoWebView, err)
	}
	// WebView2 açılırken program çökerse (kitaplık bazı hatalarda programı kapatır)
	// bir sonraki açılışta tarayıcı moduna geçilir ve WebView2 verisi yenilenir.
	// Aynı WebView2 sürümüyle iki kez çökerse, sürüm güncellenene kadar hep
	// tarayıcı modunda açılır (her sabah bir çökme, bir açılış olmasın).
	marker := filepath.Join(opt.DataDir, "webview2_aciliyor.txt")
	stateFile := filepath.Join(opt.DataDir, "webview2_durum.txt")
	profile := filepath.Join(opt.DataDir, "webview2")
	failVer, fails := "", 0
	if b, err := os.ReadFile(stateFile); err == nil {
		parts := strings.Fields(string(b))
		if len(parts) == 2 {
			failVer = parts[0]
			fmt.Sscan(parts[1], &fails)
		}
	}
	if failVer != v {
		failVer, fails = v, 0
	}
	if _, err := os.Stat(marker); err == nil {
		os.Remove(marker)
		os.RemoveAll(profile + ".eski")
		os.Rename(profile, profile+".eski")
		fails++
		os.WriteFile(stateFile, []byte(fmt.Sprintf("%s %d", failVer, fails)), 0o644)
		return fmt.Errorf("%w: önceki açılışta WebView2 %s başlatılamadı (%d. kez)", ErrNoWebView, v, fails)
	}
	if fails >= 2 {
		return fmt.Errorf("%w: WebView2 %s bu bilgisayarda açılamıyor; güncellenince yeniden denenecek", ErrNoWebView, v)
	}
	os.WriteFile(marker, []byte(time.Now().Format(time.RFC3339)), 0o644)
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		DataPath:  filepath.Join(opt.DataDir, "webview2"),
		WindowOptions: webview2.WindowOptions{
			Title:  AppTitle,
			Width:  1280,
			Height: 760,
			IconId: 1,
			Center: true,
		},
	})
	os.Remove(marker)
	if w != nil && fails > 0 {
		os.Remove(stateFile)
	}
	if w == nil {
		// Kitaplık pencereyi WebView2'yi yerleştirmeden önce açar; boş kalmasın.
		if hwnd := findAppWindow(); hwnd != 0 {
			procDestroyWindow.Call(hwnd)
		}
		drainMessages()
		return ErrNoWebView
	}
	d.mu.Lock()
	d.w = w
	d.hwnd = uintptr(w.Window())
	d.mu.Unlock()
	w.SetSize(1000, 640, webview2.HintMin)
	if opt.OnEndSession != nil {
		hookEndSession(d.hwnd, opt.OnEndSession)
	}
	w.Navigate(url)
	if opt.Fullscreen {
		d.setFullscreen(true)
	} else {
		procShowWindow.Call(d.hwnd, swMaximize)
	}
	w.Run()
	d.mu.Lock()
	d.w = nil
	d.mu.Unlock()
	return nil
}

// drainMessages bekleyen pencere iletilerini (ör. WM_QUIT) atar; yoksa sonraki
// bir uyarı penceresi açılır açılmaz kapanır.
func drainMessages() {
	var msg [48]byte
	for i := 0; i < 1000; i++ {
		if r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0, pmRemove); r == 0 {
			return
		}
	}
}

var (
	endSessionMu   sync.Mutex
	endSessionFn   func()
	endSessionPrev uintptr
	endSessionOnce sync.Once
	endSessionProc uintptr
)

// hookEndSession pencerenin ileti işleyicisinin önüne geçerek Windows kapanırken
// verilerin düzgün kapatılmasını sağlar.
func hookEndSession(hwnd uintptr, fn func()) {
	endSessionOnce.Do(func() {
		endSessionProc = windows.NewCallback(func(h, msg, wp, lp uintptr) uintptr {
			switch msg {
			case wmQueryEndSession:
				return 1 // kapanmaya izin ver
			case wmEndSession:
				if wp != 0 {
					endSessionMu.Lock()
					f := endSessionFn
					endSessionFn = nil
					endSessionMu.Unlock()
					if f != nil {
						f()
					}
				}
				return 0
			}
			r, _, _ := procCallWindowProcW.Call(endSessionPrev, h, msg, wp, lp)
			return r
		})
	})
	endSessionMu.Lock()
	endSessionFn = fn
	endSessionMu.Unlock()
	if cur, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlpWndProc); cur == endSessionProc {
		return // zaten bağlı
	}
	prev, _, _ := procSetWindowLongPtrW.Call(hwnd, gwlpWndProc, endSessionProc)
	endSessionPrev = prev
}

func (d *Desktop) setFullscreen(on bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hwnd == 0 || on == d.fullscreen {
		return
	}
	h := d.hwnd
	if on {
		style, _, _ := procGetWindowLongPtrW.Call(h, gwlStyle)
		d.savedStyle = style
		d.savedPlace = windowPlacement{Length: uint32(unsafe.Sizeof(windowPlacement{}))}
		procGetWindowPlacement.Call(h, uintptr(unsafe.Pointer(&d.savedPlace)))
		mi := monitorInfo{CbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
		mon, _, _ := procMonitorFromWindow.Call(h, monitorDefaultToNearest)
		if r, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
			return
		}
		procSetWindowLongPtrW.Call(h, gwlStyle, style&^wsOverlappedWindow)
		procSetWindowPos.Call(h, 0, uintptr(mi.Monitor.Left), uintptr(mi.Monitor.Top),
			uintptr(mi.Monitor.Right-mi.Monitor.Left), uintptr(mi.Monitor.Bottom-mi.Monitor.Top),
			swpNoOwnerZOrder|swpFrameChanged)
		d.fullscreen = true
		return
	}
	style := d.savedStyle
	if style == 0 {
		style, _, _ = procGetWindowLongPtrW.Call(h, gwlStyle)
		style |= wsOverlappedWindow
	}
	procSetWindowLongPtrW.Call(h, gwlStyle, style)
	if d.savedPlace.Length != 0 {
		procSetWindowPlacement.Call(h, uintptr(unsafe.Pointer(&d.savedPlace)))
	}
	procSetWindowPos.Call(h, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoOwnerZOrder|swpFrameChanged)
	d.fullscreen = false
}

// Mode "window" ya da "browser" döner.
func (d *Desktop) Mode() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.browserMode {
		return "browser"
	}
	return "window"
}

// WindowAction tam ekran, küçültme ve kapatma işlemlerini yapar.
func (d *Desktop) WindowAction(action string) error {
	d.mu.Lock()
	w, h := d.w, d.hwnd
	d.mu.Unlock()
	if w == nil || h == 0 {
		return errors.New("pencere yok")
	}
	switch action {
	case "fullscreen":
		w.Dispatch(func() {
			d.mu.Lock()
			on := !d.fullscreen
			d.mu.Unlock()
			d.setFullscreen(on)
		})
	case "minimize":
		procShowWindow.Call(h, swMinimize)
	case "quit":
		w.Destroy()
	}
	return nil
}

// IsFullscreen pencerenin tam ekran olup olmadığını verir.
func (d *Desktop) IsFullscreen() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.fullscreen
}

func exePath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p
}

// Autostart Windows açılınca başlama ayarını okur.
func (d *Desktop) Autostart() (bool, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false, true
	}
	defer k.Close()
	v, _, err := k.GetStringValue(runValue)
	return err == nil && v != "", true
}

// SetAutostart Windows açılınca başlamayı açar ya da kapatır.
func (d *Desktop) SetAutostart(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if err := k.DeleteValue(runValue); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	exe := exePath()
	if exe == "" {
		return errors.New("program yolu bulunamadı")
	}
	return k.SetStringValue(runValue, `"`+exe+`"`)
}

// OpenFolder klasörü Windows Gezgini'nde açar.
func (d *Desktop) OpenFolder(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return exec.Command("explorer.exe", path).Start()
}

// RemovableDrives takılı USB bellekleri listeler.
func (d *Desktop) RemovableDrives() []core.Drive {
	var out []core.Drive
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		if windows.GetDriveType(utf16(root)) != driveRemovable {
			continue
		}
		label := make([]uint16, 261)
		_ = windows.GetVolumeInformation(utf16(root), &label[0], uint32(len(label)), nil, nil, nil, nil, 0)
		out = append(out, core.Drive{Path: root, Label: windows.UTF16ToString(label)})
	}
	return out
}

func findEdge() string {
	var cands []string
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LOCALAPPDATA"} {
		if base := os.Getenv(env); base != "" {
			cands = append(cands, filepath.Join(base, `Microsoft\Edge\Application\msedge.exe`))
		}
	}
	for _, c := range cands {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

func openURL(url string) {
	if url == "" {
		return
	}
	if edge := findEdge(); edge != "" {
		exec.Command(edge, "--app="+url).Start()
		return
	}
	windows.ShellExecute(0, utf16("open"), utf16(url), nil, nil, swShowNormal)
}

// RunBrowser WebView2 açılamazsa programı Edge'in uygulama penceresinde açar ve
// pencere kapanana (ya da arayüz 2 dakika haber vermeyene) kadar bekler.
func (d *Desktop) RunBrowser(url, dataDir string, lastPing func() time.Time, quit <-chan struct{}, noOpen bool) {
	d.mu.Lock()
	d.browserMode = true
	d.mu.Unlock()
	if !noOpen {
		opened := false
		if edge := findEdge(); edge != "" {
			cmd := exec.Command(edge, "--app="+url, "--user-data-dir="+filepath.Join(dataDir, "edge"),
				"--no-first-run", "--no-default-browser-check", "--start-maximized")
			if err := cmd.Start(); err == nil {
				opened = true
				started := time.Now()
				done := make(chan struct{})
				go func() { cmd.Wait(); close(done) }()
				select {
				case <-done:
					// Edge penceresi kapatıldı: program da kapanır. Edge hemen çıktıysa
					// adresi zaten açık bir Edge'e devretmiştir; o durumda arayüzün
					// sinyaline göre çalışmayı sürdür.
					if time.Since(started) > 15*time.Second {
						return
					}
				case <-quit:
					cmd.Process.Kill()
					return
				}
			}
		}
		if !opened {
			windows.ShellExecute(0, utf16("open"), utf16(url), nil, nil, swShowNormal)
		}
	}
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-quit:
			return
		case <-t.C:
			if time.Since(lastPing()) > 2*time.Minute {
				return
			}
		}
	}
}
