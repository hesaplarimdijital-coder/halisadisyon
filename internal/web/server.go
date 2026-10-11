// Package web arayüzü (HTML ekranlar) ve arayüzün çağırdığı yerel API'yi sunar.
// Sunucu yalnızca bu bilgisayardan (127.0.0.1) erişilebilir; her API isteği
// program açılırken üretilen gizli anahtarı taşımak zorundadır.
package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"halisadisyon/internal/assets"
	"halisadisyon/internal/core"
	"halisadisyon/internal/printing"
)

//go:embed ui/*
var uiFS embed.FS

// Server yerel HTTP sunucusudur.
type Server struct {
	svc      *core.Service
	log      *log.Logger
	token    string
	version  string
	srv      *http.Server
	ln       net.Listener
	URL      string
	lastPing atomic.Int64
	quit     chan struct{}
	quitOnce atomic.Bool
}

// Start sunucuyu addr üzerinde başlatır (örnek "127.0.0.1:0" boş port seçer).
func Start(svc *core.Service, addr, version string, logger *log.Logger) (*Server, error) {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		ln.Close()
		return nil, err
	}
	s := &Server{svc: svc, log: logger, token: hex.EncodeToString(b), version: version, ln: ln, quit: make(chan struct{})}
	s.URL = "http://" + ln.Addr().String() + "/"
	s.lastPing.Store(time.Now().UnixMilli())
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleStatic)
	mux.HandleFunc("/api/", s.handleAPI)
	s.srv = &http.Server{Handler: s.guard(mux), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Printf("sunucu durdu: %v", err)
		}
	}()
	return s, nil
}

// Shutdown sunucuyu kapatır.
func (s *Server) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.srv.Shutdown(ctx)
}

// LastPing arayüzün son haber verdiği zamandır (tarayıcı modunda kapanışı anlamak için).
func (s *Server) LastPing() time.Time { return time.UnixMilli(s.lastPing.Load()) }

// QuitRequested tarayıcı modunda “Programı kapat” denince kapanır.
func (s *Server) QuitRequested() <-chan struct{} { return s.quit }

// RequestQuit tarayıcı modunda programı kapatır.
func (s *Server) RequestQuit() {
	if s.quitOnce.CompareAndSwap(false, true) {
		close(s.quit)
	}
}

// guard yalnızca bu bilgisayardan ve programın kendi sayfasından gelen istekleri kabul eder.
func (s *Server) guard(next http.Handler) http.Handler {
	port := ""
	if _, p, err := net.SplitHostPort(s.ln.Addr().String()); err == nil {
		port = p
	}
	okHost := map[string]bool{"127.0.0.1:" + port: true, "localhost:" + port: true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !okHost[r.Host] {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !okHost[strings.TrimPrefix(o, "http://")] {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	p := r.URL.Path
	switch {
	case p == "/" || p == "/index.html":
		b, err := uiFS.ReadFile("ui/index.html")
		if err != nil {
			http.Error(w, "missing", http.StatusInternalServerError)
			return
		}
		b = bytes.ReplaceAll(b, []byte("__TOKEN__"), []byte(s.token))
		b = bytes.ReplaceAll(b, []byte("__VERSION__"), []byte(s.version))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; font-src 'self'; connect-src 'self'; frame-ancestors 'none'")
		w.Write(b)
	case p == "/logo.png":
		b := s.svc.LogoPNG()
		if len(b) == 0 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", http.DetectContentType(b))
		w.Header().Set("Cache-Control", "no-store")
		w.Write(b)
	case strings.HasPrefix(p, "/fonts/"):
		name := path.Base(p)
		b, err := assets.Fonts.ReadFile("fonts/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "font/ttf")
		w.Header().Set("Cache-Control", "max-age=86400")
		w.Write(b)
	default:
		name := strings.TrimPrefix(path.Clean(p), "/")
		if strings.Contains(name, "/") || name == "" {
			http.NotFound(w, r)
			return
		}
		b, err := fs.ReadFile(uiFS, "ui/"+name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		ct := mime.TypeByExtension(path.Ext(name))
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(b)
	}
}

type apiResp struct {
	OK    bool        `json:"ok"`
	Data  any         `json:"data,omitempty"`
	State *core.State `json:"state,omitempty"`
	Error string      `json:"error,omitempty"`
	Code  string      `json:"code,omitempty"`
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(apiResp{Error: "method", Code: "method"})
		return
	}
	if r.Header.Get("X-Halis-Token") != s.token {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(apiResp{Error: "Oturum geçersiz. Programı kapatıp yeniden açın.", Code: "token"})
		return
	}
	s.lastPing.Store(time.Now().UnixMilli())
	name := strings.TrimPrefix(r.URL.Path, "/api/")
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(apiResp{Error: "istek okunamadı", Code: "bad_request"})
		return
	}
	if len(bytes.TrimSpace(body)) == 0 {
		body = []byte("{}")
	}
	resp := s.dispatch(name, body)
	json.NewEncoder(w).Encode(resp)
}

func decode[T any](body []byte) (T, error) {
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		return v, &core.UserError{Code: "bad_request", Msg: "İstek okunamadı."}
	}
	return v, nil
}

func (s *Server) dispatch(name string, body []byte) (resp apiResp) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Printf("API %s panik: %v", name, rec)
			resp = apiResp{Error: "Beklenmeyen bir hata oldu. İşlemi tekrar deneyin; sürerse programı kapatıp açın.", Code: "internal"}
		}
	}()
	data, st, err := s.call(name, body)
	if err != nil {
		var ue *core.UserError
		if errors.As(err, &ue) {
			r := apiResp{Error: ue.Msg, Code: ue.Code}
			// Ekran eskimişse (kalem/adisyon bu arada değişmiş) güncel hali de gönder.
			switch ue.Code {
			case "not_found", "closed", "busy", "changed":
				if fresh, ferr := s.svc.State(); ferr == nil {
					r.State = fresh
				}
			}
			return r
		}
		s.log.Printf("API %s hata: %v", name, err)
		if diskFull(err) {
			return apiResp{Error: "Bilgisayarın diskinde yer kalmadı; işlem kaydedilemedi. Yöneticiye haber verin: gereksiz dosyaları silip yer açın.", Code: "disk_full"}
		}
		return apiResp{Error: "Beklenmeyen bir hata oldu. İşlemi tekrar deneyin; sürerse programı kapatıp açın.", Code: "internal"}
	}
	return apiResp{OK: true, Data: data, State: st}
}

type noReq struct {
	No int64 `json:"no"`
}

// diskFull disk dolduğu için yazılamayan durumları tanır.
func diskFull(err error) bool {
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "disk is full") || strings.Contains(m, "not enough space") ||
		strings.Contains(m, "no space left") || strings.Contains(m, "sqlite_full")
}

func (s *Server) call(name string, body []byte) (any, *core.State, error) {
	svc := s.svc
	switch name {
	case "state":
		st, err := svc.State()
		return nil, st, err
	case "ping":
		return map[string]any{"time": time.Now().UnixMilli()}, nil, nil
	case "touch":
		svc.Touch()
		return nil, nil, nil

	// ---- sipariş ----
	case "addItem":
		req, err := decode[struct {
			Table     int   `json:"table"`
			ProductID int64 `json:"productId"`
			WaiterID  int64 `json:"waiterId"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, no, err := svc.AddItem(req.Table, req.ProductID, req.WaiterID)
		return map[string]any{"no": no}, st, err
	case "changeQty":
		req, err := decode[struct {
			No     int64  `json:"no"`
			LineID int64  `json:"lineId"`
			Delta  int    `json:"delta"`
			Pin    string `json:"pin"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.ChangeQty(req.No, req.LineID, req.Delta, req.Pin)
		return nil, st, err
	case "undo":
		req, err := decode[noReq](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.Undo(req.No)
		return nil, st, err
	case "print":
		req, err := decode[noReq](body)
		if err != nil {
			return nil, nil, err
		}
		out, st, err := svc.Print(req.No)
		return out, st, err
	case "pay":
		req, err := decode[struct {
			No     int64  `json:"no"`
			Method string `json:"method"`
			Total  *int64 `json:"total"` // ekranda gösterilen toplam
		}](body)
		if err != nil {
			return nil, nil, err
		}
		expected := int64(-1)
		if req.Total != nil {
			expected = *req.Total
		}
		out, st, err := svc.PayChecked(req.No, req.Method, expected)
		return out, st, err
	case "moveTable":
		req, err := decode[struct {
			No     int64 `json:"no"`
			Target int   `json:"target"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		out, st, err := svc.MoveTable(req.No, req.Target)
		return out, st, err
	case "closeEmpty":
		req, err := decode[struct {
			No  int64  `json:"no"`
			Pin string `json:"pin"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.CloseEmpty(req.No, req.Pin)
		return nil, st, err
	case "changeWaiter":
		req, err := decode[struct {
			No       int64 `json:"no"`
			WaiterID int64 `json:"waiterId"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.ChangeWaiter(req.No, req.WaiterID)
		return nil, st, err
	case "preview":
		req, err := decode[noReq](body)
		if err != nil {
			return nil, nil, err
		}
		img, err := svc.Preview(req.No)
		return map[string]string{"image": img}, nil, err
	case "cancel":
		req, err := decode[struct {
			No     int64  `json:"no"`
			Reason string `json:"reason"`
			Pin    string `json:"pin"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.Cancel(req.No, req.Reason, req.Pin)
		return nil, st, err

	// ---- yönetici ----
	case "unlock":
		req, err := decode[struct {
			Pin string `json:"pin"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.Unlock(req.Pin)
		return nil, st, err
	case "lock":
		st, err := svc.Lock()
		return nil, st, err
	case "changePin":
		req, err := decode[struct {
			Old string `json:"old"`
			New string `json:"new"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.ChangePin(req.Old, req.New)
		return nil, st, err
	case "saveProduct":
		req, err := decode[core.ProductInput](body)
		if err != nil {
			return nil, nil, err
		}
		st, id, err := svc.SaveProduct(req)
		return map[string]any{"id": id}, st, err
	case "moveProduct":
		req, err := decode[struct {
			ID  int64 `json:"id"`
			Dir int   `json:"dir"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.MoveProduct(req.ID, clampDir(req.Dir))
		return nil, st, err
	case "saveCategory":
		req, err := decode[core.CategoryInput](body)
		if err != nil {
			return nil, nil, err
		}
		st, id, err := svc.SaveCategory(req)
		return map[string]any{"id": id}, st, err
	case "moveCategory":
		req, err := decode[struct {
			ID  int64 `json:"id"`
			Dir int   `json:"dir"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.MoveCategory(req.ID, clampDir(req.Dir))
		return nil, st, err
	case "deleteCategory":
		req, err := decode[struct {
			ID int64 `json:"id"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.DeleteCategory(req.ID)
		return nil, st, err
	case "saveWaiter":
		req, err := decode[struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.SaveWaiter(req.ID, req.Name)
		return nil, st, err
	case "removeWaiter":
		req, err := decode[struct {
			ID int64 `json:"id"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.RemoveWaiter(req.ID)
		return nil, st, err
	case "saveSettings":
		req, err := decode[core.SettingsPatch](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.SaveSettings(req)
		return nil, st, err
	case "setLogo":
		req, err := decode[struct {
			Image string `json:"image"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.SetLogo(req.Image)
		return nil, st, err
	case "printers":
		list, def, guess := svc.Printers()
		return map[string]any{"printers": list, "default": def, "guess": guess}, nil, nil
	case "testPrint":
		req, err := decode[struct {
			Printer *printing.Config `json:"printer"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		out, err := svc.TestPrint(req.Printer)
		return out, nil, err
	case "testPreview":
		req, err := decode[struct {
			Printer *printing.Config `json:"printer"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		img, err := svc.TestPreview(req.Printer)
		return map[string]string{"image": img}, nil, err
	case "backupNow":
		st, err := svc.BackupNow()
		return nil, st, err
	case "openFolder":
		req, err := decode[struct {
			Which string `json:"which"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, svc.OpenFolder(req.Which)
	case "driveDirs":
		d, err := svc.DriveDirs()
		return map[string]any{"dirs": d}, nil, err
	case "drives":
		d, err := svc.Drives()
		return map[string]any{"drives": d}, nil, err
	case "setAutostart":
		req, err := decode[struct {
			On bool `json:"on"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.SetAutostart(req.On)
		return nil, st, err
	case "window":
		req, err := decode[struct {
			Action string `json:"action"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		err = svc.Window(req.Action)
		if err != nil && req.Action == "quit" {
			// Tarayıcı modunda pencere yok: programı sunucu üzerinden kapat.
			s.RequestQuit()
			return nil, nil, nil
		}
		return nil, nil, err
	case "samplePreview":
		req, err := decode[struct {
			Business *core.Business `json:"business"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		img, err := svc.SamplePreview(req.Business)
		return map[string]string{"image": img}, nil, err
	case "samplePrint":
		out, err := svc.SamplePrint()
		return out, nil, err

	// ---- gün sonu ----
	case "daySummary":
		req, err := decode[struct {
			Date string `json:"date"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		ds, err := svc.DaySummary(req.Date)
		return ds, nil, err
	case "closeDay":
		req, err := decode[struct {
			Date string `json:"date"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		ds, st, err := svc.CloseDay(req.Date)
		return ds, st, err
	case "dayPreview":
		req, err := decode[struct {
			Date string `json:"date"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		img, err := svc.DayReportPreview(req.Date)
		return map[string]string{"image": img}, nil, err
	case "printDay":
		req, err := decode[struct {
			Date string `json:"date"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		out, err := svc.PrintDayReport(req.Date)
		return out, nil, err
	case "detail":
		req, err := decode[noReq](body)
		if err != nil {
			return nil, nil, err
		}
		d, err := svc.Detail(req.No)
		return d, nil, err
	case "changePayment":
		req, err := decode[struct {
			No     int64  `json:"no"`
			Method string `json:"method"`
			Pin    string `json:"pin"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.ChangePayment(req.No, req.Method, req.Pin)
		return nil, st, err
	case "bulkPrice":
		req, err := decode[core.BulkPriceInput](body)
		if err != nil {
			return nil, nil, err
		}
		res, st, err := svc.BulkPrice(req)
		return res, st, err
	case "auditLog":
		req, err := decode[core.AuditQuery](body)
		if err != nil {
			return nil, nil, err
		}
		page, err := svc.AuditLog(req)
		return page, nil, err
	case "backups":
		list, err := svc.Backups()
		return map[string]any{"backups": list}, nil, err
	case "restore":
		req, err := decode[struct {
			Path string `json:"path"`
			Pin  string `json:"pin"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, svc.Restore(req.Path, req.Pin)

	// ---- istatistik ----
	case "stats":
		req, err := decode[core.StatsQuery](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.Stats(req)
		return st, nil, err
	case "exportExcel":
		req, err := decode[core.StatsQuery](body)
		if err != nil {
			return nil, nil, err
		}
		path, err := svc.ExportExcel(req)
		return map[string]string{"path": path}, nil, err

	// ---- gönderimler ----
	case "retryOutbox":
		st, err := svc.RetryOutbox()
		return nil, st, err
	case "testRemote":
		req, err := decode[struct {
			Kind string `json:"kind"`
		}](body)
		if err != nil {
			return nil, nil, err
		}
		msg, err := svc.TestRemote(req.Kind)
		return map[string]string{"message": msg}, nil, err

	case "finishSetup":
		req, err := decode[core.SetupInput](body)
		if err != nil {
			return nil, nil, err
		}
		st, err := svc.FinishSetup(req)
		return nil, st, err
	}
	return nil, nil, &core.UserError{Code: "unknown", Msg: "Bilinmeyen işlem: " + name}
}

func clampDir(d int) int {
	if d < 0 {
		return -1
	}
	return 1
}
