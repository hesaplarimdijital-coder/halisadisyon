// Halis Adisyon: kasadaki bilgisayardan termal yazıcıya adisyon basan program.
//
// Program açılınca yalnızca bu bilgisayardan erişilebilen küçük bir yerel sunucu
// başlatır ve ekranları Windows'un kendi tarayıcı motoruyla (WebView2) kendi
// penceresinde gösterir. Veriler %LOCALAPPDATA%\HalisAdisyon\adisyon.db dosyasındadır.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"halisadisyon/internal/backup"
	"halisadisyon/internal/core"
	"halisadisyon/internal/desktop"
	"halisadisyon/internal/printing"
	"halisadisyon/internal/store"
	"halisadisyon/internal/web"
)

var version = "2.2.1"

func init() {
	// Pencere ve mesaj döngüsü aynı işletim sistemi iş parçacığında kalmalı.
	runtime.LockOSThread()
}

func main() {
	if restart := run(); restart {
		// Yedekten geri yükleme: veriler kapandıktan sonra program yeniden açılır.
		if exe, err := os.Executable(); err == nil {
			cmd := exec.Command(exe, os.Args[1:]...)
			cmd.Env = os.Environ()
			_ = cmd.Start()
		}
	}
}

// run programı çalıştırır; program yeniden başlatılmak üzere kapandıysa true döner.
func run() (restart bool) {
	dataFlag := flag.String("data", "", "veri klasörü (varsayılan: %LOCALAPPDATA%\\HalisAdisyon)")
	backupFlag := flag.String("yedek", "", "yedek klasörü (varsayılan: Belgeler\\Halis Adisyon Yedekleri)")
	browser := flag.Bool("browser", false, "pencere yerine tarayıcı modunda çalış")
	noOpen := flag.Bool("no-open", false, "tarayıcı modunda tarayıcıyı açma (test)")
	port := flag.Int("port", 0, "yerel sunucu portu (0: boş port)")
	showVersion := flag.Bool("version", false, "sürümü yaz")
	flag.Parse()
	if *showVersion {
		fmt.Println("Halis Adisyon", version)
		return false
	}

	desktop.PrepareProcess()

	dataDir := *dataFlag
	if dataDir == "" {
		dataDir = desktop.DataDir()
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		desktop.Alert("Halis Adisyon", "Veri klasörü oluşturulamadı:\n"+dataDir+"\n\n"+err.Error())
		os.Exit(1)
	}
	logger, closeLog := openLog(dataDir)
	defer closeLog()
	printing.Logf = logger.Printf
	defer func() {
		if r := recover(); r != nil {
			logger.Printf("beklenmeyen hata: %v\n%s", r, debug.Stack())
			desktop.Alert("Halis Adisyon", fmt.Sprintf("Beklenmeyen bir hata oldu ve program kapanacak.\nVeriler kayıtlı; programı yeniden açabilirsiniz.\n\nAyrıntı: %v", r))
			os.Exit(2)
		}
	}()

	releaseLock, ok := desktop.SingleInstance(dataDir)
	if !ok {
		return false // program zaten açık; penceresi öne getirildi
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(releaseLock) }
	defer release()
	// Önceki açılıştan kalmış (Windows kapanırken sonlanmış) adres dosyası yanıltmasın.
	running := filepath.Join(dataDir, "calisiyor.txt")
	os.Remove(running)

	backupDir := *backupFlag
	if backupDir == "" {
		backupDir = desktop.BackupDir()
	}

	// Yedekten geri yükleme hazırlandıysa veritabanı açılmadan önce yerine konur.
	restored, rerr := backup.ApplyPending(dataDir)
	if rerr != nil {
		logger.Printf("yedekten geri yüklenemedi: %v", rerr)
		desktop.Alert("Halis Adisyon", "Seçilen yedek geri yüklenemedi; program şu anki verilerle açılıyor.\n\nAyrıntı: "+rerr.Error())
	} else if restored != nil {
		logger.Printf("yedek geri yüklendi: %s (eski veri: %s)", restored.File, restored.OldDir)
	}

	st, err := store.Open(filepath.Join(dataDir, "adisyon.db"))
	if err != nil {
		fatal(logger, "Veritabanı açılamadı.", err)
	}
	defer st.Close()

	var svc *core.Service
	bm := backup.New(st, backupDir, func() []backup.Target {
		if svc == nil {
			return nil
		}
		set := svc.Settings()
		return []backup.Target{{Key: "usb", Dir: set.SecondBackupDir}, {Key: "drive", Dir: set.DriveBackupDir, Keep: 30}}
	}, logger)
	if err := st.Migrate(bm.BeforeUpgrade); err != nil {
		fatal(logger, "Veritabanı güncellenemedi.", err)
	}

	d := desktop.New()
	var srv *web.Server
	var restartReq atomic.Bool
	svc, err = core.New(st.DB, core.Options{
		Version: version,
		DataDir: dataDir,
		Desktop: d,
		Backup:  backupAdapter{bm},
		Log:     logger,
		Seal:    desktop.Seal,
		Unseal:  desktop.Unseal,
		Restart: func() error {
			if !restartReq.CompareAndSwap(false, true) {
				return nil
			}
			go func() {
				time.Sleep(700 * time.Millisecond) // yanıt ekrana ulaşsın
				if err := d.WindowAction("quit"); err != nil && srv != nil {
					srv.RequestQuit()
				}
			}()
			return nil
		},
	})
	if err != nil {
		fatal(logger, "Program verileri okunamadı.", err)
	}
	if restored != nil {
		svc.RecordRestore(restored.File, restored.MaxNo)
	}

	// Şifre unutulursa: veri klasörüne "sifre_sifirla.txt" adlı boş bir dosya koyup programı açın.
	resetFile := filepath.Join(dataDir, "sifre_sifirla.txt")
	if _, err := os.Stat(resetFile); err == nil {
		if err := svc.ResetPin("1234"); err == nil {
			os.Remove(resetFile)
			logger.Println("yönetici şifresi 1234 olarak sıfırlandı")
			desktop.Info("Halis Adisyon", "Yönetici şifresi 1234 olarak sıfırlandı.\nAyarlar › Yönetici şifresi bölümünden yeni şifre belirleyin.")
		}
	}

	srv, err = web.Start(svc, fmt.Sprintf("127.0.0.1:%d", *port), version, logger)
	if err != nil {
		fatal(logger, "Program başlatılamadı (yerel sunucu).", err)
	}
	os.WriteFile(running, []byte(srv.URL), 0o644)
	defer os.Remove(running)

	// Açılış yedeği pencere açıldıktan sonra arka planda alınır.
	bm.Start(4 * time.Second)
	// Gün sonu özetlerini (WhatsApp, Google Sheets) internet olunca gönderir.
	outCtx, stopOutbox := context.WithCancel(context.Background())
	defer stopOutbox()
	svc.StartOutbox(outCtx)
	logger.Printf("Halis Adisyon %s açıldı (%s)", version, srv.URL)

	// Windows kapanırken program sonlandırılır; veritabanını düzgün kapat.
	onEndSession := func() {
		logger.Println("Windows kapanıyor; veriler kapatılıyor")
		_, _ = st.DB.Exec(`PRAGMA wal_checkpoint(PASSIVE)`)
		os.Remove(running)
	}

	if !*browser {
		err := d.Run(srv.URL, desktop.RunOptions{Fullscreen: svc.Settings().Fullscreen, DataDir: dataDir, OnEndSession: onEndSession})
		if err != nil {
			logger.Printf("program penceresi açılamadı: %v; tarayıcı moduna geçiliyor", err)
			d.RunBrowser(srv.URL, dataDir, srv.LastPing, srv.QuitRequested(), *noOpen)
		}
	} else {
		d.RunBrowser(srv.URL, dataDir, srv.LastPing, srv.QuitRequested(), *noOpen)
	}

	logger.Println("program kapanıyor")
	stopOutbox()
	// Önce adres dosyası ve tek kopya kilidi bırakılır: kapanış yedeği sürerken
	// program yeniden açılırsa ölü bir adres açılmaz, yeni kopya hemen başlar.
	os.Remove(running)
	srv.Shutdown()
	if restartReq.Load() {
		// Yeniden başlatmada kapanış yedeği alınmaz (geri yüklemeden önce şu anki
		// verinin yedeği zaten alındı); süren yedek beklenir, kilit veritabanı
		// kapandıktan sonra bırakılır.
		bm.Halt(30 * time.Second)
		logger.Println("program yeniden başlatılıyor")
		return true
	}
	release()
	bm.Stop(30 * time.Second)
	printing.Logf = func(string, ...any) {}
	return false
}

// backupAdapter yedek bileşenini programın beklediği biçime uydurur.
type backupAdapter struct{ m *backup.Manager }

func (b backupAdapter) Run(reason string) error { return b.m.Run(reason) }

func (b backupAdapter) Status() core.BackupStatus {
	s := b.m.Status()
	return core.BackupStatus{Dir: s.Dir, LastAt: s.LastAt, LastError: s.LastError,
		SecondError: s.Extra["usb"].Error, SecondAt: s.Extra["usb"].At,
		DriveError: s.Extra["drive"].Error, DriveAt: s.Extra["drive"].At}
}

func (b backupAdapter) List() []core.BackupFile {
	list := b.m.List()
	out := make([]core.BackupFile, 0, len(list))
	for _, f := range list {
		out = append(out, core.BackupFile{Path: f.Path, Name: f.Name, Where: f.Where, At: f.At.Format(time.RFC3339), Size: f.Size})
	}
	return out
}

func (b backupAdapter) PrepareRestore(path string, maxNo int64) error { return b.m.PrepareRestore(path, maxNo) }
func (b backupAdapter) SaveCopy(label string) (string, error)        { return b.m.SaveCopy(label) }

func fatal(logger *log.Logger, msg string, err error) {
	logger.Printf("%s %v", msg, err)
	desktop.Alert("Halis Adisyon", msg+"\n\n"+err.Error())
	os.Exit(1)
}

// openLog kayıt dosyasını açar; 5 MB'ı geçince eskisini .1 olarak saklar.
func openLog(dataDir string) (*log.Logger, func()) {
	dir := filepath.Join(dataDir, "kayitlar")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "halis.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 5<<20 {
		os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return log.New(os.Stderr, "", log.LstdFlags), func() {}
	}
	var w io.Writer = f
	if os.Getenv("HALIS_LOG_STDERR") == "1" {
		w = io.MultiWriter(f, os.Stderr)
	}
	// Kitaplıkların kendi kayıtları ve beklenmedik çökmeler de bu dosyaya düşsün.
	log.SetOutput(w)
	log.SetFlags(log.LstdFlags)
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
	return log.New(w, "", log.LstdFlags), func() { f.Close() }
}
