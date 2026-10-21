package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"halisadisyon/internal/money"
	"halisadisyon/internal/remote"
)

// Gün sonu özeti kapanışta sıraya konur; internet varsa hemen, yoksa gelince
// gönderilir. Başarısız gönderim giderek seyrelen aralıklarla (en fazla 3 gün)
// yeniden denenir.

const (
	outboxWhatsApp = "whatsapp"
	outboxSheets   = "sheets"
	outboxMaxTries = 40
)

func backoff(attempts int) time.Duration {
	steps := []time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour}
	if attempts-1 < len(steps) {
		return steps[max(attempts-1, 0)]
	}
	return 2 * time.Hour
}

// whatsAppParams şablondaki {{1}}…{{7}} değişkenleridir.
func whatsAppParams(ds *DaySummary) []string {
	feat := "-"
	if ds.Featured != nil {
		feat = fmt.Sprintf("%d %s", ds.Featured.Qty, ds.Featured.Unit)
	}
	cancel := "Yok"
	if ds.Cancelled > 0 {
		cancel = fmt.Sprintf("%d adisyon · %s", ds.Cancelled, money.TL(ds.CancelledTotal))
	}
	var pays []string
	for _, p := range ds.ByPayment {
		if p.Total > 0 || p.Method != "" {
			pays = append(pays, fmt.Sprintf("%s %s", p.Label, money.TL(p.Total)))
		}
	}
	var ws []string
	for _, w := range ds.ByWaiter {
		ws = append(ws, fmt.Sprintf("%s %d adisyon %s", w.Name, w.Count, money.TL(w.Total)))
	}
	if len(ws) == 0 {
		ws = []string{"-"}
	}
	count := fmt.Sprintf("%d", ds.Paid)
	if ds.Paid > 0 {
		count += " (ortalama " + money.TL(ds.Avg) + ")"
	}
	return []string{
		dateTRLong(ds.Date),
		money.TL(ds.Revenue),
		count,
		feat,
		cancel,
		strings.Join(pays, " · "),
		strings.Join(ws, " · "),
	}
}

func tlNum(k int64) float64 { return float64(k) / 100 }

// sheetsPayload Google Sheets betiğinin beklediği satırlardır.
func sheetsPayload(ds *DaySummary) map[string]any {
	pay := map[string]int64{}
	for _, p := range ds.ByPayment {
		pay[p.Method] = p.Total
	}
	feat, featQty := "", 0
	if ds.Featured != nil {
		feat, featQty = ds.Featured.Name, ds.Featured.Qty
	}
	var ws []string
	for _, w := range ds.ByWaiter {
		ws = append(ws, fmt.Sprintf("%s: %d / %s", w.Name, w.Count, money.TL(w.Total)))
	}
	rng := ""
	if ds.FirstNo > 0 {
		rng = fmt.Sprintf("%d–%d", ds.FirstNo, ds.LastNo)
	}
	closed := ""
	if t, err := time.Parse(time.RFC3339, ds.ClosedAt); err == nil {
		closed = t.Local().Format("02.01.2006 15:04")
	}
	// Sütunlar betikteki başlık sırasıyla aynıdır; yeni sütunlar sona eklenir
	// (eski betikle yazılan tablolarda başlıksız kalır ama kaymaz).
	gun := []any{ds.Date, closed, tlNum(ds.Revenue), tlNum(ds.VAT), ds.Paid, tlNum(ds.Avg), ds.Cancelled, tlNum(ds.CancelledTotal),
		tlNum(pay[PayCash]), tlNum(pay[PayCard]), tlNum(pay[PayTransfer]), feat, featQty, strings.Join(ws, " · "), rng,
		ds.Discounts, tlNum(ds.DiscountTotal), ds.Comps, tlNum(ds.CompValue)}
	adisyonlar := [][]any{}
	for _, r := range ds.Rows {
		adisyonlar = append(adisyonlar, []any{ds.Date, r.No, r.Table, r.Waiter, clockOf(r.OpenedAt), clockOf(r.ClosedAt),
			r.StatusText, r.PaymentLabel, tlNum(r.Total), r.PrintCount, r.CancelReason, r.DiscountPercent, tlNum(r.Discount)})
	}
	urunler := [][]any{}
	for _, p := range ds.Products {
		urunler = append(urunler, []any{ds.Date, p.Name, p.Qty, p.Unit, tlNum(p.Total), p.Comp})
	}
	return map[string]any{
		"type":       "gun_sonu",
		"gun":        map[string]any{"tarih": ds.Date, "satir": gun},
		"adisyonlar": adisyonlar,
		"urunler":    urunler,
	}
}

func clockOf(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}
	return t.Local().Format("15:04")
}

// enqueueDayReports kapanan günün özetini gönderim sırasına koyar. s.mu tutulurken çağrılır.
func (s *Service) enqueueDayReports(ds *DaySummary) { s.enqueueReports(ds, true) }

// enqueueReports raporları sıraya koyar; withWhatsApp false ise yalnız Sheets.
func (s *Service) enqueueReports(ds *DaySummary, withWhatsApp bool) {
	now := s.now()
	add := func(kind, target string, payload any) {
		b, _ := json.Marshal(payload)
		_, err := s.db.Exec(`INSERT INTO outbox(kind,target,ref,payload,created_at,next_at) VALUES(?,?,?,?,?,?)`,
			kind, target, ds.Date, string(b), stamp(now), stamp(now))
		if err != nil {
			s.opt.Log.Printf("gönderim sıraya konamadı (%s): %v", kind, err)
		}
	}
	wa := s.set.WhatsApp
	if wa.Enabled && withWhatsApp {
		params := whatsAppParams(ds)
		for _, to := range wa.Recipients {
			if to = remote.NormalizePhone(to); to != "" {
				add(outboxWhatsApp, to, map[string]any{"params": params})
			}
		}
	}
	if s.set.Sheets.Enabled && strings.TrimSpace(s.set.Sheets.URL) != "" {
		add(outboxSheets, "", sheetsPayload(ds))
	}
}

// kickOutbox gönderimciyi hemen çalıştırır.
func (s *Service) kickOutbox() {
	if s.outboxKick == nil {
		return
	}
	select {
	case s.outboxKick <- struct{}{}:
	default:
	}
}

// StartOutbox gönderimciyi başlatır (program kapanınca ctx iptal edilir).
func (s *Service) StartOutbox(ctx context.Context) {
	s.outboxKick = make(chan struct{}, 1)
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		select {
		case <-time.After(10 * time.Second): // açılışta biraz bekle
		case <-ctx.Done():
			return
		}
		for {
			s.processOutbox(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			case <-s.outboxKick:
			}
		}
	}()
}

type outboxItem struct {
	id       int64
	kind     string
	target   string
	payload  string
	attempts int
}

func (s *Service) processOutbox(ctx context.Context) {
	for i := 0; i < 50; i++ {
		if ctx.Err() != nil {
			return
		}
		var it outboxItem
		err := s.db.QueryRow(`SELECT id, kind, target, payload, attempts FROM outbox
			WHERE status='pending' AND next_at <= ? ORDER BY id LIMIT 1`, stamp(s.now())).Scan(&it.id, &it.kind, &it.target, &it.payload, &it.attempts)
		if err != nil {
			return // sıra boş ya da okunamadı
		}
		s.mu.Lock()
		set := s.set
		s.mu.Unlock()
		sendErr := s.sendOutbox(ctx, set, it)
		now := s.now()
		if sendErr == nil {
			if _, err := s.db.Exec(`UPDATE outbox SET status='sent', sent_at=?, attempts=attempts+1, last_error=NULL WHERE id=?`, stamp(now), it.id); err != nil {
				// Gönderildi ama kaydedilemedi: aynı mesaj tekrar tekrar gitmesin diye dur.
				s.opt.Log.Printf("gönderildi ama kaydedilemedi (%s %s): %v", it.kind, it.target, err)
				return
			}
			s.opt.Log.Printf("gönderildi: %s %s", it.kind, it.target)
			continue
		}
		attempts := it.attempts + 1
		status := "pending"
		if attempts >= outboxMaxTries {
			status = "failed"
		}
		next := now.Add(backoff(attempts))
		if _, err := s.db.Exec(`UPDATE outbox SET status=?, attempts=?, next_at=?, last_error=? WHERE id=?`,
			status, attempts, stamp(next), sendErr.Error(), it.id); err != nil {
			s.opt.Log.Printf("gönderim durumu kaydedilemedi: %v", err)
			return
		}
		s.opt.Log.Printf("gönderilemedi (%s %s, %d. deneme): %s", it.kind, it.target, attempts, remote.Detail(sendErr))
		if !remote.IsPermanent(sendErr) {
			return // internet yoksa sıradakileri de deneme
		}
	}
}

func (s *Service) sendOutbox(ctx context.Context, set Settings, it outboxItem) error {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	switch it.kind {
	case outboxWhatsApp:
		var p struct {
			Params []string `json:"params"`
		}
		if err := json.Unmarshal([]byte(it.payload), &p); err != nil {
			return &remote.Error{Msg: "Gönderim verisi bozuk.", Permanent: true}
		}
		w := set.WhatsApp
		_, err := remote.WhatsApp{PhoneNumberID: w.PhoneNumberID, Token: w.Token, Template: w.Template, Lang: w.Lang, APIVersion: w.APIVersion}.
			SendTemplate(cctx, it.target, p.Params)
		return err
	case outboxSheets:
		var p map[string]any
		if err := json.Unmarshal([]byte(it.payload), &p); err != nil {
			return &remote.Error{Msg: "Gönderim verisi bozuk.", Permanent: true}
		}
		return remote.Sheets{URL: set.Sheets.URL, Secret: set.Sheets.Secret}.Post(cctx, p)
	}
	return &remote.Error{Msg: "Bilinmeyen gönderim türü.", Permanent: true}
}

// OutboxStatus ayarlar ekranındaki gönderim durumudur.
type OutboxStatus struct {
	Pending   int    `json:"pending"`
	Failed    int    `json:"failed"`
	LastSent  string `json:"lastSent,omitempty"`
	LastError string `json:"lastError,omitempty"`
}

func (s *Service) outboxStatus(kind string) OutboxStatus {
	var st OutboxStatus
	_ = s.db.QueryRow(`SELECT
		COALESCE(SUM(status='pending'),0), COALESCE(SUM(status='failed'),0), COALESCE(MAX(sent_at),'')
		FROM outbox WHERE kind=?`, kind).Scan(&st.Pending, &st.Failed, &st.LastSent)
	var le sql.NullString
	_ = s.db.QueryRow(`SELECT last_error FROM outbox WHERE kind=? AND status IN ('pending','failed') AND last_error IS NOT NULL
		ORDER BY id DESC LIMIT 1`, kind).Scan(&le)
	st.LastError = le.String
	return st
}

// RetryOutbox bekleyen ve başarısız gönderimleri hemen yeniden dener.
func (s *Service) RetryOutbox() (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`UPDATE outbox SET status='pending', next_at=?, attempts=CASE WHEN status='failed' THEN 0 ELSE attempts END
		WHERE status IN ('pending','failed')`, stamp(s.now())); err != nil {
		return nil, err
	}
	s.kickOutbox()
	return s.stateLocked()
}

// TestRemote WhatsApp ya da Google Sheets ayarını hemen dener (açık günün ara özetiyle).
func (s *Service) TestRemote(kind string) (string, error) {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return "", err
	}
	set := s.set
	day, err := openBusinessDay(s.db, s.now())
	var ds *DaySummary
	if err == nil {
		ds, err = s.daySummary(s.db, day)
	}
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch kind {
	case outboxWhatsApp:
		w := set.WhatsApp
		if len(w.Recipients) == 0 {
			return "", uerr("invalid", "En az bir alıcı numarası girin.")
		}
		params := whatsAppParams(ds)
		params[0] = "TEST · " + params[0]
		var sent []string
		for _, to := range w.Recipients {
			if _, err := (remote.WhatsApp{PhoneNumberID: w.PhoneNumberID, Token: w.Token, Template: w.Template, Lang: w.Lang, APIVersion: w.APIVersion}).
				SendTemplate(ctx, to, params); err != nil {
				s.opt.Log.Printf("WhatsApp testi başarısız (%s): %s", to, remote.Detail(err))
				return "", uerr("remote", "%s", err.Error())
			}
			sent = append(sent, remote.NormalizePhone(to))
		}
		return "Test mesajı gönderildi: " + strings.Join(sent, ", "), nil
	case outboxSheets:
		p := map[string]any{"type": "test"}
		if err := (remote.Sheets{URL: set.Sheets.URL, Secret: set.Sheets.Secret}).Post(ctx, p); err != nil {
			s.opt.Log.Printf("Google Sheets testi başarısız: %s", remote.Detail(err))
			return "", uerr("remote", "%s", err.Error())
		}
		return "Google Sheets bağlantısı çalışıyor.", nil
	}
	return "", uerr("invalid", "Geçersiz deneme.")
}
