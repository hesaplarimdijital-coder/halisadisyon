package core

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"image"
	"strings"

	"halisadisyon/internal/escpos"
	"halisadisyon/internal/printing"
	"halisadisyon/internal/receipt"
)

func receiptBusiness(set Settings, logo image.Image) receipt.Business {
	b := set.Business
	return receipt.Business{
		Name: b.Name, Slogan: b.Slogan, Address1: b.Address1, Address2: b.Address2, Phone: b.Phone,
		Instagram: b.Instagram, Facebook: b.Facebook, TikTok: b.TikTok, Invite: b.Invite, GoogleURL: b.GoogleURL,
		Logo: logo,
	}
}

func (s *Service) receiptAdisyon(a *Adisyon, isCopy bool) receipt.Adisyon {
	groups, total, _ := VATBreakdown(a.Lines)
	r := receipt.Adisyon{No: a.No, Table: a.Table, Waiter: a.WaiterName, When: s.now(), Copy: isCopy, Total: total}
	for _, l := range a.Lines {
		r.Lines = append(r.Lines, receipt.Line{Name: l.Name, Unit: l.Unit, Qty: l.Qty, Price: l.Price})
	}
	for _, g := range groups {
		r.VATs = append(r.VATs, receipt.VATGroup{Rate: g.Rate, Gross: g.Gross, VAT: g.VAT})
	}
	return r
}

func pngDataURL(img *image.Gray) (string, error) {
	b, err := receipt.PNG(img)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b), nil
}

func renderFailed(err error) printing.Result {
	return printing.Result{Code: "render", Title: "Fiş hazırlanamadı",
		Message: "Fiş hazırlanırken bir sorun oldu.", Hint: "Programı kapatıp açın; sorun sürerse yöneticiye haber verin.",
		Detail: err.Error()}
}

// send fiş resmini yazıcıya gönderir; önizleme modunda resmi geri verir.
func (s *Service) send(cfg printing.Config, img *image.Gray, doc string) (printing.Result, string) {
	if cfg.Mode == printing.ModeNone {
		url, err := pngDataURL(img)
		if err != nil {
			return renderFailed(err), ""
		}
		return printing.Result{OK: true}, url
	}
	data, err := escpos.Encode(img, escpos.Options{Cut: cfg.Cut, FullCut: cfg.CutMode == "full"})
	if err != nil {
		return renderFailed(err), ""
	}
	s.printMu.Lock()
	res := s.opt.Send(cfg, data, doc)
	s.printMu.Unlock()
	return res, ""
}

func sameLines(a, b []Line) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.ProductID != y.ProductID || x.Price != y.Price || x.VAT != y.VAT || x.Qty != y.Qty || x.Name != y.Name {
			return false
		}
	}
	return true
}

// Print adisyonu basar. İlk baskıdan sonra değişmeden yapılan baskıya “KOPYA” yazılır.
// Baskı olmazsa adisyon kaybolmaz; “basılamadı” olarak işaretlenir.
func (s *Service) Print(no int64) (*PrintOutcome, *State, error) {
	s.mu.Lock()
	if s.inPrint[no] {
		s.mu.Unlock()
		return nil, nil, errBusy(no)
	}
	s.inPrint[no] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.inPrint, no)
		s.mu.Unlock()
	}()
	return s.print(no)
}

// print basma işini yapar; çağıran inPrint[no] işaretini tutar (bu sürede
// adisyona ürün eklenemez, hesap kapatılamaz). Ödeme şekli fişe yazılmaz:
// adisyon fişi ödemeden önce basılır, ödeme şekli yalnız programda saklanır.
func (s *Service) print(no int64) (*PrintOutcome, *State, error) {
	s.mu.Lock()
	a, err := loadActiveOrFail(s.db, no)
	if err == nil && len(a.Lines) == 0 {
		err = uerr("invalid", "Adisyonda ürün yok; önce ürün ekleyin.")
	}
	if err != nil {
		s.mu.Unlock()
		return nil, nil, err
	}
	isCopy := a.PrintCount > 0 && !a.Changed
	set := s.set
	logo := s.logoImg
	s.mu.Unlock()

	out := &PrintOutcome{No: no, Copy: isCopy}
	img, rerr := receipt.RenderAdisyon(receiptBusiness(set, logo), s.receiptAdisyon(a, isCopy), set.Printer.Width)
	if rerr != nil {
		out.Result = renderFailed(rerr)
	} else {
		out.Result, out.Preview = s.send(set.Printer, img, fmt.Sprintf("Adisyon %d", no))
	}
	if out.OK {
		s.opt.Log.Printf("adisyon %d basıldı (kopya=%v, mod=%s)", no, isCopy, set.Printer.Mode)
	} else {
		s.opt.Log.Printf("adisyon %d basılamadı: %s (%s)", no, out.Code, out.Detail)
	}

	s.mu.Lock()
	err = s.withTx(func(tx *sql.Tx) error {
		cur, err := loadAdisyon(tx, no)
		if err != nil {
			return err
		}
		if !cur.active() {
			return nil // bu arada kapanmış
		}
		now := s.now()
		if out.OK {
			changed := 0
			if !sameLines(cur.Lines, a.Lines) {
				changed = 1
			}
			if _, err := tx.Exec(`UPDATE adisyons SET status='printed', print_count=print_count+1, printed_at=?,
				changed=?, print_failed=0 WHERE no=?`, stamp(now), changed, no); err != nil {
				return err
			}
			// Basılan adetler: bunların altına inmek yönetici şifresi ister.
			for _, l := range a.Lines {
				if _, err := tx.Exec(`UPDATE adisyon_lines SET printed_qty=? WHERE id=? AND adisyon_no=?`, l.Qty, l.ID, no); err != nil {
					return err
				}
			}
			_, printedTotal, _ := VATBreakdown(a.Lines)
			audit(tx, now, "basildi", no, map[string]any{"kopya": isCopy, "sira": cur.PrintCount + 1, "tutar": printedTotal, "mod": set.Printer.Mode})
		} else {
			if _, err := tx.Exec(`UPDATE adisyons SET print_failed=1 WHERE no=?`, no); err != nil {
				return err
			}
			audit(tx, now, "basilamadi", no, map[string]any{"kod": out.Code, "ayrinti": out.Detail})
		}
		return nil
	})
	if err != nil {
		s.mu.Unlock()
		return nil, nil, err
	}
	if out.OK {
		// Basılan hal müşteriye verildi: “Geri Al” bundan önceki adımlara dönmez.
		delete(s.undo, no)
	}
	st, err := s.stateLocked()
	s.mu.Unlock()
	return out, st, err
}

// Preview adisyonun fiş önizlemesini verir (hiçbir şey değişmez).
func (s *Service) Preview(no int64) (string, error) {
	s.mu.Lock()
	a, err := loadAdisyon(s.db, no)
	set := s.set
	logo := s.logoImg
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	isCopy := a.PrintCount > 0 && !a.Changed
	img, err := receipt.RenderAdisyon(receiptBusiness(set, logo), s.receiptAdisyon(a, isCopy), set.Printer.Width)
	if err != nil {
		return "", err
	}
	return pngDataURL(img)
}

func connectionText(cfg printing.Config) string {
	switch cfg.Mode {
	case printing.ModeWindows:
		if strings.TrimSpace(cfg.Name) == "" {
			return "Windows (seçilmedi)"
		}
		return "Windows: " + cfg.Name
	case printing.ModeNetwork:
		return fmt.Sprintf("Ağ: %s:%d", cfg.Host, cfg.Port)
	default:
		return "Önizleme (yazıcı yok)"
	}
}

func cutText(cfg printing.Config) string {
	if !cfg.Cut {
		return "kapalı"
	}
	if cfg.CutMode == "full" {
		return "tam"
	}
	return "kısmi"
}

func (s *Service) testImage(cfg *printing.Config) (*image.Gray, printing.Config, error) {
	s.mu.Lock()
	set := s.set
	logo := s.logoImg
	s.mu.Unlock()
	c := set.Printer
	if cfg != nil {
		c = *cfg
		if err := c.Normalize(); err != nil {
			return nil, c, uerr("invalid", "%s", err.Error())
		}
	}
	info := receipt.TestInfo{Connection: connectionText(c), Width: c.Width, CutText: cutText(c), When: s.now()}
	img, err := receipt.RenderTest(receiptBusiness(set, logo), info, c.Width)
	return img, c, err
}

// TestPrint test fişi basar. cfg verilirse (henüz kaydedilmemiş ayar) onunla denenir.
// Sonuçla birlikte önizleme resmi de döner.
func (s *Service) TestPrint(cfg *printing.Config) (*PrintOutcome, error) {
	s.mu.Lock()
	err := s.requireUnlocked()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	img, c, err := s.testImage(cfg)
	if err != nil {
		return nil, err
	}
	out := &PrintOutcome{}
	out.Result, out.Preview = s.send(c, img, "Halis Adisyon test fişi")
	if out.Preview == "" {
		out.Preview, _ = pngDataURL(img)
	}
	if !out.OK && out.Code != "" {
		out.Title = "Test fişi basılamadı"
	}
	s.opt.Log.Printf("test fişi: ok=%v kod=%s ayrıntı=%s", out.OK, out.Code, out.Detail)
	s.mu.Lock()
	audit(s.db, s.now(), "test_fisi", 0, map[string]any{"ok": out.OK, "kod": out.Code, "baglanti": connectionText(c)})
	s.mu.Unlock()
	return out, nil
}

// TestPreview test fişinin önizlemesini verir.
func (s *Service) TestPreview(cfg *printing.Config) (string, error) {
	img, _, err := s.testImage(cfg)
	if err != nil {
		return "", err
	}
	return pngDataURL(img)
}

// Printers Windows'taki yazıcıları, varsayılanı ve fiş yazıcısı tahminini verir.
func (s *Service) Printers() ([]printing.PrinterInfo, string, string) {
	list, def := s.opt.Printers()
	if list == nil {
		list = []printing.PrinterInfo{}
	}
	return list, def, printing.GuessThermal(list)
}
