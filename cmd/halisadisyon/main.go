// Halis Adisyon: kasadaki bilgisayardan termal yazıcıya adisyon basan program.
//
// Program açılınca yalnızca bu bilgisayardan erişilebilen küçük bir yerel sunucu
// başlatır ve ekranları Windows'un kendi tarayıcı motoruyla (WebView2) kendi
// penceresinde gösterir. Veriler %LOCALAPPDATA%\HalisAdisyon\adisyon.db dosyasındadır.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"halisadisyon/internal/backup"
	"halisadisyon/internal/core"
	"halisadisyon/internal/desktop"
	"halisadisyon/internal/printing"
	"halisadisyon/internal/store"
	"halisadisyon/internal/web"
)

var version = "1.0.0"

func init() {
	// Pencere ve mesaj döngüsü aynı işletim sistemi iş parçacığında kalmalı.
	runtime.LockOSThread()
}

func main() {
	dataFlag := flag.String("data", "", "veri klasörü (varsayılan: %LOCALAPPDATA%\\HalisAdisyon)")
	backupFlag := flag.String("yedek", "", "yedek klasörü (varsayılan: Belgeler\\Halis Adisyon Yedekleri)")
	browser := flag.Bool("browser", false, "pencere yerine tarayıcı modunda çalış")
	noOpen := flag.Bool("no-open", false, "tarayıcı modunda tarayıcıyı açma (test)")
	port := flag.Int("port", 0, "yerel sunucu portu (0: boş port)")
	showVersion := flag.Bool("version", false, "sürümü yaz")
	flag.Parse()
	if *showVersion {
		fmt.Println("Halis Adisyon", version)
		return
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
		return // program zaten açık; penceresi öne getirildi
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

	st, err := store.Open(filepath.Join(dataDir, "adisyon.db"))
	if err != nil {
		fatal(logger, "Veritabanı açılamadı.", err)
	}
	defer st.Close()

	var svc *core.Service
	bm := backup.New(st, backupDir, func() string {
		if svc == nil {
			return ""
		}
		return svc.SecondBackupDir()
	}, logger)
	if err := st.Migrate(bm.BeforeUpgrade); err != nil {
		fatal(logger, "Veritabanı güncellenemedi.", err)
	}

	d := desktop.New()
	svc, err = core.New(st.DB, core.Options{
		Version: version,
		DataDir: dataDir,
		Desktop: d,
		Backup:  bm,
		Log:     logger,
	})
	if err != nil {
		fatal(logger, "Program verileri okunamadı.", err)
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

	srv, err := web.Start(svc, fmt.Sprintf("127.0.0.1:%d", *port), version, logger)
	if err != nil {
		fatal(logger, "Program başlatılamadı (yerel sunucu).", err)
	}
	os.WriteFile(running, []byte(srv.URL), 0o644)
	defer os.Remove(running)

	// Açılış yedeği pencere açıldıktan sonra arka planda alınır.
	bm.Start(4 * time.Second)
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
	// Önce adres dosyası ve tek kopya kilidi bırakılır: kapanış yedeği sürerken
	// program yeniden açılırsa ölü bir adres açılmaz, yeni kopya hemen başlar.
	os.Remove(running)
	srv.Shutdown()
	release()
	bm.Stop(30 * time.Second)
	printing.Logf = func(string, ...any) {}
}

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
