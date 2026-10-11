// Package remote internet gerektiren gönderimleri yapar: gün sonu özetinin
// WhatsApp Business Platform (Cloud API) şablon mesajıyla gönderilmesi ve
// Google Sheets'e (tabloya eklenen Apps Script web uygulaması) yazılması.
// Program internetsiz de tam çalışır; gönderimler sırada bekler.
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// GraphBase WhatsApp Cloud API adresidir (testte değiştirilir).
var GraphBase = "https://graph.facebook.com"

// DefaultAPIVersion Graph API sürümüdür (ayarlardan değiştirilebilir).
const DefaultAPIVersion = "v25.0"

var client = &http.Client{Timeout: 25 * time.Second}

// Error gönderim hatasıdır; Msg kullanıcıya gösterilir.
type Error struct {
	Msg       string
	Detail    string
	Permanent bool // ayar düzelmeden tekrar denemek boşuna (ör. şablon yok)
}

func (e *Error) Error() string { return e.Msg }

func netErr(err error) *Error {
	return &Error{Msg: "İnternete ulaşılamadı; bağlantı gelince yeniden denenecek.", Detail: err.Error()}
}

var spaces = regexp.MustCompile(`\s{2,}`)

// cleanParam WhatsApp şablon değişkenine uygun metin yapar: satır sonu, sekme
// ve art arda dörtten fazla boşluk kabul edilmez.
func cleanParam(s string) string {
	s = strings.NewReplacer("\r", " ", "\n", " · ", "\t", " ").Replace(s)
	s = spaces.ReplaceAllString(strings.TrimSpace(s), " ")
	if s == "" {
		s = "-"
	}
	if utf8.RuneCountInString(s) > 900 {
		r := []rune(s)
		s = string(r[:900]) + "…"
	}
	return s
}

// WhatsApp Cloud API ile şablon mesajı gönderir.
type WhatsApp struct {
	PhoneNumberID string
	Token         string
	Template      string
	Lang          string
	APIVersion    string
}

var digits = regexp.MustCompile(`\D`)

// NormalizePhone telefon numarasını uluslararası biçime getirir (0532... → 90532...).
func NormalizePhone(p string) string {
	d := digits.ReplaceAllString(p, "")
	switch {
	case strings.HasPrefix(d, "00"):
		d = d[2:]
	case strings.HasPrefix(d, "0") && len(d) == 11:
		d = "90" + d[1:]
	case len(d) == 10 && strings.HasPrefix(d, "5"):
		d = "90" + d
	}
	return d
}

type waError struct {
	Error struct {
		Message      string `json:"message"`
		Code         int    `json:"code"`
		ErrorSubcode int    `json:"error_subcode"`
		ErrorData    struct {
			Details string `json:"details"`
		} `json:"error_data"`
	} `json:"error"`
}

func waFriendly(code int, msg string) (string, bool) {
	switch code {
	case 190:
		return "WhatsApp erişim anahtarı geçersiz ya da süresi dolmuş. Ayarlardan yenisini girin.", true
	case 10, 200, 299:
		return "WhatsApp erişim anahtarının mesaj gönderme izni yok.", true
	case 131030:
		return "Alıcı numara izinli listede değil (Meta test numarası yalnız kayıtlı numaralara gönderir).", true
	case 132001:
		return "WhatsApp şablonu bulunamadı: şablon adı ya da dili yanlış, ya da şablon henüz onaylanmadı.", true
	case 132000:
		return "WhatsApp şablonundaki değişken sayısı uymuyor; şablonda {{1}}–{{7}} olmalı.", true
	case 132015, 132016:
		return "WhatsApp şablonu duraklatılmış ya da devre dışı.", true
	case 131026:
		return "Mesaj teslim edilemedi: numara WhatsApp kullanmıyor olabilir.", false
	case 131056, 130429, 131048, 80007:
		return "WhatsApp gönderim sınırına takıldı; biraz sonra yeniden denenecek.", false
	case 133010:
		return "WhatsApp işletme numarası kayıtlı değil.", true
	}
	return "WhatsApp hatası: " + msg, false
}

// SendTemplate şablon mesajını gönderir; başarıda mesaj kimliğini verir.
func (w WhatsApp) SendTemplate(ctx context.Context, to string, params []string) (string, error) {
	if strings.TrimSpace(w.PhoneNumberID) == "" || strings.TrimSpace(w.Token) == "" || strings.TrimSpace(w.Template) == "" {
		return "", &Error{Msg: "WhatsApp ayarları eksik: telefon numarası kimliği, erişim anahtarı ve şablon adı girilmeli.", Permanent: true}
	}
	to = NormalizePhone(to)
	if len(to) < 10 {
		return "", &Error{Msg: "Alıcı telefon numarası geçersiz.", Permanent: true}
	}
	ver := strings.TrimSpace(w.APIVersion)
	if ver == "" {
		ver = DefaultAPIVersion
	}
	lang := strings.TrimSpace(w.Lang)
	if lang == "" {
		lang = "tr"
	}
	ps := make([]map[string]string, len(params))
	for i, p := range params {
		ps[i] = map[string]string{"type": "text", "text": cleanParam(p)}
	}
	body := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to,
		"type":              "template",
		"template": map[string]any{
			"name":       strings.TrimSpace(w.Template),
			"language":   map[string]string{"code": lang},
			"components": []any{map[string]any{"type": "body", "parameters": ps}},
		},
	}
	b, _ := json.Marshal(body)
	u := fmt.Sprintf("%s/%s/%s/messages", strings.TrimRight(GraphBase, "/"), url.PathEscape(ver), url.PathEscape(strings.TrimSpace(w.PhoneNumberID)))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return "", &Error{Msg: "WhatsApp isteği hazırlanamadı.", Detail: err.Error(), Permanent: true}
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(w.Token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", netErr(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 == 2 {
		var ok struct {
			Messages []struct {
				ID string `json:"id"`
			} `json:"messages"`
		}
		if json.Unmarshal(raw, &ok) == nil && len(ok.Messages) > 0 {
			return ok.Messages[0].ID, nil
		}
		return "", &Error{Msg: "WhatsApp beklenmeyen yanıt verdi.", Detail: string(raw)}
	}
	var we waError
	if json.Unmarshal(raw, &we) == nil && we.Error.Message != "" {
		msg, perm := waFriendly(we.Error.Code, we.Error.Message)
		detail := fmt.Sprintf("HTTP %d · kod %d · %s %s", resp.StatusCode, we.Error.Code, we.Error.Message, we.Error.ErrorData.Details)
		return "", &Error{Msg: msg, Detail: detail, Permanent: perm}
	}
	return "", &Error{Msg: fmt.Sprintf("WhatsApp hatası (HTTP %d).", resp.StatusCode), Detail: string(raw), Permanent: resp.StatusCode == 400 || resp.StatusCode == 401 || resp.StatusCode == 403}
}

// Sheets Google Sheets'teki Apps Script web uygulamasına veri gönderir.
type Sheets struct {
	URL    string
	Secret string
}

// Post veriyi (gizli anahtarla birlikte) gönderir; betik {"ok":true} dönmelidir.
func (s Sheets) Post(ctx context.Context, payload map[string]any) error {
	u := strings.TrimSpace(s.URL)
	if !strings.HasPrefix(u, "https://script.google.com/") && !strings.HasPrefix(u, "http://127.0.0.1") {
		return &Error{Msg: "Google Sheets adresi geçersiz: https://script.google.com/macros/s/.../exec biçiminde olmalı.", Permanent: true}
	}
	body := map[string]any{}
	for k, v := range payload {
		body[k] = v
	}
	body["secret"] = s.Secret
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return &Error{Msg: "Google Sheets isteği hazırlanamadı.", Detail: err.Error(), Permanent: true}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return netErr(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var r struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		snippet := string(raw)
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		perm := resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 404
		msg := "Google Sheets betiği beklenen yanıtı vermedi. Web uygulamasının “Herkes” erişimiyle dağıtıldığını kontrol edin."
		if resp.StatusCode/100 == 5 {
			msg, perm = "Google Sheets şu an yanıt vermiyor; yeniden denenecek.", false
		}
		return &Error{Msg: msg, Detail: fmt.Sprintf("HTTP %d · %s", resp.StatusCode, snippet), Permanent: perm}
	}
	if !r.OK {
		return &Error{Msg: "Google Sheets: " + r.Error, Detail: string(raw), Permanent: true}
	}
	return nil
}

// IsPermanent hatanın ayar düzelmeden geçmeyeceğini söyler.
func IsPermanent(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Permanent
}

// Detail hatanın teknik ayrıntısını verir (kayıt için).
func Detail(err error) string {
	var e *Error
	if errors.As(err, &e) && e.Detail != "" {
		return e.Detail
	}
	return err.Error()
}
