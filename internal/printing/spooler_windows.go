//go:build windows

package printing

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows yazıcı kuyruğuna ham (RAW) ESC/POS verisi gönderir.
// Koddata sürücüsü kurulu olduğunda veri sürücüden değiştirilmeden yazıcıya gider.

var (
	modWinspool            = windows.NewLazySystemDLL("winspool.drv")
	procOpenPrinterW       = modWinspool.NewProc("OpenPrinterW")
	procClosePrinter       = modWinspool.NewProc("ClosePrinter")
	procStartDocPrinterW   = modWinspool.NewProc("StartDocPrinterW")
	procEndDocPrinter      = modWinspool.NewProc("EndDocPrinter")
	procStartPagePrinter   = modWinspool.NewProc("StartPagePrinter")
	procEndPagePrinter     = modWinspool.NewProc("EndPagePrinter")
	procWritePrinter       = modWinspool.NewProc("WritePrinter")
	procAbortPrinter       = modWinspool.NewProc("AbortPrinter")
	procGetPrinterW        = modWinspool.NewProc("GetPrinterW")
	procGetJobW            = modWinspool.NewProc("GetJobW")
	procSetJobW            = modWinspool.NewProc("SetJobW")
	procEnumPrintersW      = modWinspool.NewProc("EnumPrintersW")
	procGetDefaultPrinterW = modWinspool.NewProc("GetDefaultPrinterW")
)

const (
	printerEnumLocal = 0x2

	printerAttributeWorkOffline = 0x400
	printerStatusPaused         = 0x1

	jobStatusPaused           = 0x1
	jobStatusError            = 0x2
	jobStatusDeleting         = 0x4
	jobStatusPrinting         = 0x10
	jobStatusOffline          = 0x20
	jobStatusPaperOut         = 0x40
	jobStatusPrinted          = 0x80
	jobStatusDeleted          = 0x100
	jobStatusBlockedDevq      = 0x200
	jobStatusUserIntervention = 0x400
	jobStatusComplete         = 0x1000

	jobControlDelete = 5

	errInvalidParameter   = windows.Errno(87)
	errInsufficientBuffer = windows.Errno(122)
	errInvalidPrinterName = windows.Errno(1801)
)

type docInfo1 struct {
	DocName    *uint16
	OutputFile *uint16
	Datatype   *uint16
}

type printerInfo2 struct {
	ServerName         *uint16
	PrinterName        *uint16
	ShareName          *uint16
	PortName           *uint16
	DriverName         *uint16
	Comment            *uint16
	Location           *uint16
	DevMode            uintptr
	SepFile            *uint16
	PrintProcessor     *uint16
	Datatype           *uint16
	Parameters         *uint16
	SecurityDescriptor uintptr
	Attributes         uint32
	Priority           uint32
	DefaultPriority    uint32
	StartTime          uint32
	UntilTime          uint32
	Status             uint32
	CJobs              uint32
	AveragePPM         uint32
}

type systemTime struct {
	Year, Month, DayOfWeek, Day, Hour, Minute, Second, Milliseconds uint16
}

type jobInfo1 struct {
	JobID        uint32
	PrinterName  *uint16
	MachineName  *uint16
	UserName     *uint16
	Document     *uint16
	Datatype     *uint16
	StatusText   *uint16
	Status       uint32
	Priority     uint32
	Position     uint32
	TotalPages   uint32
	PagesPrinted uint32
	Submitted    systemTime
}

func utf16(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		p, _ = windows.UTF16PtrFromString("")
	}
	return p
}

func sendWindows(cfg Config, data []byte, docName string, wait time.Duration) Result {
	if len(data) == 0 {
		return fail("error", titleFailed, msgUnreach, hintUnreach, "empty data")
	}
	var h windows.Handle
	r, _, e := procOpenPrinterW.Call(uintptr(unsafe.Pointer(utf16(cfg.Name))), uintptr(unsafe.Pointer(&h)), 0)
	if r == 0 {
		if e == errInvalidPrinterName {
			return fail("not_found", titleFailed,
				fmt.Sprintf("“%s” adlı yazıcı bu bilgisayarda bulunamadı.", cfg.Name),
				"Yöneticiden Ayarlar › Yazıcı bölümünden doğru yazıcıyı seçmesini isteyin.", "OpenPrinter: "+e.Error())
		}
		return fail("error", titleFailed, msgUnreach, hintUnreach, "OpenPrinter: "+e.Error())
	}
	defer procClosePrinter.Call(uintptr(h))

	// Windows'ta elle kapatılmış ya da duraklatılmış yazıcıyı baştan yakala.
	queued := uint32(0)
	if info, ok := printerInfo(h); ok {
		if info.Attributes&printerAttributeWorkOffline != 0 {
			return fail("offline", titleFailed,
				"Yazıcı Windows'ta “çevrimdışı kullan” durumunda.",
				"Başlat › Ayarlar › Yazıcılar bölümünde yazıcıyı açıp “Yazıcıyı çevrimdışı kullan” işaretini kaldırın.",
				"PRINTER_ATTRIBUTE_WORK_OFFLINE")
		}
		if info.Status&printerStatusPaused != 0 {
			return fail("paused", titleFailed,
				"Yazıcı Windows'ta duraklatılmış.",
				"Başlat › Ayarlar › Yazıcılar bölümünde yazıcıyı açıp “Yazdırmayı sürdür”ü seçin.",
				"PRINTER_STATUS_PAUSED")
		}
		queued = info.Jobs
	}

	di := docInfo1{DocName: utf16(docName), Datatype: utf16("RAW")}
	r, _, e = procStartDocPrinterW.Call(uintptr(h), 1, uintptr(unsafe.Pointer(&di)))
	runtime.KeepAlive(&di)
	if r == 0 {
		return fail("error", titleFailed, msgUnreach, hintUnreach, "StartDocPrinter: "+e.Error())
	}
	job := uint32(r)
	procStartPagePrinter.Call(uintptr(h))
	var written uint32
	wr, _, we := procWritePrinter.Call(uintptr(h), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(unsafe.Pointer(&written)))
	runtime.KeepAlive(data)
	if wr == 0 || int(written) != len(data) {
		// Yarım işi kuyruğa bırakmadan sil.
		procAbortPrinter.Call(uintptr(h))
		return fail("error", titleFailed, msgUnreach, hintUnreach,
			fmt.Sprintf("WritePrinter: %v (%d/%d bayt)", we, written, len(data)))
	}
	procEndPagePrinter.Call(uintptr(h))
	if r, _, e := procEndDocPrinter.Call(uintptr(h)); r == 0 {
		if !deleteJob(h, job) {
			return stuckJob(fmt.Sprintf("EndDocPrinter: %v", e))
		}
		return fail("error", titleFailed, msgUnreach, hintUnreach, fmt.Sprintf("EndDocPrinter: %v", e))
	}
	return waitJob(h, job, wait, queued)
}

// stuckJob silinemeyen iş için uyarıdır: yazıcı düzelince fiş yine de çıkabilir.
func stuckJob(detail string) Result {
	return fail("stuck", titleFailed,
		"Fiş Windows yazıcı kuyruğunda bekliyor ve iptal edilemedi. Yazıcı düzelince kendiliğinden çıkabilir.",
		"Önce yazıcıyı kontrol edin. Fiş çıkmazsa Windows'ta yazıcı kuyruğunu açıp bekleyen belgeleri silin, sonra Tekrar Dene'ye basın.",
		detail)
}

// waitJob işin yazıcıya gidip gitmediğini izler. Hata veren ya da süresi dolan iş
// kuyruktan silinir; böylece yazıcı sonradan beklenmedik bir fiş basmaz. Yarıda
// kalan işin yazıcıda bıraktığı yarım resim bandını sonraki fişin başındaki
// eşitleme baytları temizler.
func waitJob(h windows.Handle, job uint32, wait time.Duration, queued uint32) Result {
	start := time.Now()
	soft := start.Add(wait)
	hard := start.Add(jobHardLimit)
	var last uint32
	seen := false
	var lastErr error
	for {
		st, state, err := jobStatus(h, job)
		switch state {
		case jobGone:
			return Result{OK: true}
		case jobUnknown:
			lastErr = err
		case jobPresent:
			seen = true
			last = st
			switch {
			case st&(jobStatusPrinted|jobStatusComplete) != 0:
				return Result{OK: true}
			case st&(jobStatusDeleted|jobStatusDeleting) != 0:
				return fail("cancelled", titleFailed, "Baskı Windows'ta iptal edildi.", "Tekrar Dene'ye basın.", fmt.Sprintf("job status 0x%x", st))
			case st&jobStatusPaperOut != 0:
				return removeJob(h, job, fail("paper", titleFailed, msgPaper, hintPaper, fmt.Sprintf("job status 0x%x", st)))
			case st&(jobStatusError|jobStatusOffline|jobStatusUserIntervention|jobStatusBlockedDevq) != 0:
				return removeJob(h, job, fail("offline", titleFailed, msgUnreach, hintUnreach, fmt.Sprintf("job status 0x%x", st)))
			}
		}
		now := time.Now()
		// Yazıcı veriyi alıp basıyorsa (hata yokken) uzun fişe biraz daha süre tanı.
		printing := seen && last&jobStatusPrinting != 0 && last&jobStatusPaused == 0
		if now.After(hard) || (now.After(soft) && !printing) {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	waited := time.Since(start).Round(100 * time.Millisecond)
	if !seen && lastErr != nil {
		return removeJob(h, job, fail("unknown", titleFailed,
			"Windows yazıcı kuyruğundan bilgi alınamadı; fişin çıkıp çıkmadığı bilinmiyor.",
			"Fiş çıktıysa tekrar basmayın. Çıkmadıysa yazıcıyı kontrol edip Tekrar Dene'ye basın.",
			fmt.Sprintf("GetJob: %v (%s)", lastErr, waited)))
	}
	if queued > 0 {
		return removeJob(h, job, fail("queue", titleFailed,
			"Yazıcı kuyruğunda daha önce gönderilmiş bekleyen belgeler var; fiş sırada kaldı.",
			"Windows'ta Ayarlar › Yazıcılar › yazıcı › “Yazdırma kuyruğunu aç” ile bekleyen belgeleri silin, sonra Tekrar Dene'ye basın.",
			fmt.Sprintf("job waited %s, status 0x%x, %d job(s) ahead", waited, last, queued)))
	}
	return removeJob(h, job, fail("timeout", titleFailed, msgUnreach, hintUnreach,
		fmt.Sprintf("job waited %s, status 0x%x", waited, last)))
}

// removeJob işi kuyruktan siler. İş bu arada bitmişse baskı başarılı sayılır;
// silinemezse kullanıcı uyarılır (yoksa iki fiş çıkabilir).
func removeJob(h windows.Handle, job uint32, res Result) Result {
	r, _, e := procSetJobW.Call(uintptr(h), uintptr(job), 0, 0, jobControlDelete)
	if r != 0 {
		return res
	}
	if jobMissing(e, 0) { // iş son anda bitip kuyruktan çıkmış
		if res.Code == "timeout" || res.Code == "queue" {
			return Result{OK: true}
		}
		return res
	}
	return stuckJob(fmt.Sprintf("%s; SetJob(DELETE): %v", res.Detail, e))
}

type jobState int

const (
	jobGone    jobState = iota // iş kuyrukta yok (bitti ya da silindi)
	jobPresent                 // iş kuyrukta, durumu okundu
	jobUnknown                 // kuyruk sorgulanamadı
)

// jobMissing işin kuyrukta olmadığını gösteren hatadır. Windows geçersiz iş
// numarası için ERROR_INVALID_PARAMETER döner; bazı ortamlar (ör. Wine) hata
// kodu koymadan başarısız olur.
func jobMissing(e error, needed uint32) bool {
	return e == errInvalidParameter || (e == windows.Errno(0) && needed == 0)
}

func jobStatus(h windows.Handle, job uint32) (uint32, jobState, error) {
	var needed uint32
	r, _, e := procGetJobW.Call(uintptr(h), uintptr(job), 1, 0, 0, uintptr(unsafe.Pointer(&needed)))
	if r == 0 {
		if jobMissing(e, needed) {
			return 0, jobGone, nil
		}
		if e != errInsufficientBuffer || needed == 0 {
			return 0, jobUnknown, e
		}
	}
	if needed < uint32(unsafe.Sizeof(jobInfo1{})) {
		needed = uint32(unsafe.Sizeof(jobInfo1{}))
	}
	buf := make([]byte, needed+64)
	r, _, e = procGetJobW.Call(uintptr(h), uintptr(job), 1, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&needed)))
	if r == 0 {
		if jobMissing(e, 0) {
			return 0, jobGone, nil
		}
		return 0, jobUnknown, e
	}
	ji := (*jobInfo1)(unsafe.Pointer(&buf[0]))
	st := ji.Status
	runtime.KeepAlive(buf)
	return st, jobPresent, nil
}

// deleteJob işi siler; iş zaten kuyrukta yoksa da başarılı sayılır.
func deleteJob(h windows.Handle, job uint32) bool {
	r, _, e := procSetJobW.Call(uintptr(h), uintptr(job), 0, 0, jobControlDelete)
	return r != 0 || jobMissing(e, 0)
}

type printerState struct {
	Attributes, Status, Jobs uint32
	Port, Driver             string
}

func printerInfo(h windows.Handle) (printerState, bool) {
	var needed uint32
	procGetPrinterW.Call(uintptr(h), 2, 0, 0, uintptr(unsafe.Pointer(&needed)))
	if needed == 0 {
		return printerState{}, false
	}
	buf := make([]byte, needed+64)
	r, _, _ := procGetPrinterW.Call(uintptr(h), 2, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&needed)))
	if r == 0 {
		return printerState{}, false
	}
	pi := (*printerInfo2)(unsafe.Pointer(&buf[0]))
	out := printerState{
		Attributes: pi.Attributes,
		Status:     pi.Status,
		Jobs:       pi.CJobs,
		Port:       windows.UTF16PtrToString(pi.PortName),
		Driver:     windows.UTF16PtrToString(pi.DriverName),
	}
	runtime.KeepAlive(buf)
	return out, true
}

func defaultPrinter() string {
	var n uint32
	procGetDefaultPrinterW.Call(0, uintptr(unsafe.Pointer(&n)))
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	r, _, _ := procGetDefaultPrinterW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

func listPrinters() ([]PrinterInfo, string) {
	def := defaultPrinter()
	var needed, returned uint32
	procEnumPrintersW.Call(printerEnumLocal, 0, 2, 0, 0, uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if needed == 0 {
		return nil, def
	}
	buf := make([]byte, needed+64)
	r, _, _ := procEnumPrintersW.Call(printerEnumLocal, 0, 2, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
		uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if r == 0 {
		return nil, def
	}
	size := unsafe.Sizeof(printerInfo2{})
	out := make([]PrinterInfo, 0, returned)
	for i := uintptr(0); i < uintptr(returned); i++ {
		if (i+1)*size > uintptr(len(buf)) {
			break
		}
		pi := (*printerInfo2)(unsafe.Pointer(&buf[i*size]))
		name := windows.UTF16PtrToString(pi.PrinterName)
		if name == "" {
			continue
		}
		out = append(out, PrinterInfo{
			Name:    name,
			Port:    windows.UTF16PtrToString(pi.PortName),
			Driver:  windows.UTF16PtrToString(pi.DriverName),
			Default: name == def,
		})
	}
	runtime.KeepAlive(buf)
	return out, def
}
