// Package core programın iş kurallarını içerir: masalar, adisyonlar, ürünler,
// ayarlar, yönetici şifresi ve baskı. Arayüz yalnızca bu paketi çağırır.
package core

import (
	"fmt"
	"time"

	"halisadisyon/internal/printing"
)

// Adisyon durumları: Açık → Basıldı → Ödendi; her aşamadan İptal'e geçilebilir.
const (
	StatusOpen      = "open"
	StatusPrinted   = "printed"
	StatusPaid      = "paid"
	StatusCancelled = "cancelled"
)

// BusinessDayCutoffHour: bu saatten önce açılan adisyon bir önceki iş gününe yazılır
// (gece yarısını geçen servis aynı güne sayılır).
const BusinessDayCutoffHour = 5

// Category satış ekranındaki buton grubudur.
type Category struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Sort  int    `json:"sort"`
}

// Product menüdeki üründür. Fiyat kuruş cinsinden ve KDV dahildir.
type Product struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CategoryID int64  `json:"categoryId"`
	Price      int64  `json:"price"`
	VAT        *int   `json:"vat"` // nil: varsayılan oran
	Unit       string `json:"unit"`
	Active     bool   `json:"active"`
	Sort       int    `json:"sort"`
}

// Waiter garsondur.
type Waiter struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// Line adisyon kalemidir; ürün adı, fiyat ve KDV eklendiği andaki haliyle saklanır.
type Line struct {
	ID        int64  `json:"id"`
	ProductID int64  `json:"productId"`
	Name      string `json:"name"`
	Unit      string `json:"unit"`
	Price     int64  `json:"price"`
	VAT       int    `json:"vat"`
	Qty       int    `json:"qty"`
	// PrintedQty son basılan adisyondaki adettir; altına inmek yönetici şifresi ister.
	PrintedQty int `json:"printedQty"`
	// Comp (ikram): kalem adisyonda ve fişte görünür, tutara ve KDV'ye girmez.
	Comp bool `json:"comp"`
}

// Adisyon bir masanın hesabıdır.
type Adisyon struct {
	No           int64  `json:"no"`
	BusinessDate string `json:"businessDate"`
	Table        int    `json:"table"`
	WaiterID     int64  `json:"waiterId"`
	WaiterName   string `json:"waiterName"`
	Status       string `json:"status"`
	OpenedAt     string `json:"openedAt"`
	PrintedAt    string `json:"printedAt,omitempty"`
	ClosedAt     string `json:"closedAt,omitempty"`
	PrintCount   int    `json:"printCount"`
	Changed      bool   `json:"changed"`
	PrintFailed  bool   `json:"printFailed"`
	CancelReason string `json:"cancelReason,omitempty"`
	Total        int64  `json:"total"`
	VATTotal     int64  `json:"vatTotal"`
	// PaymentMethod ödenen adisyonun ödeme şeklidir: nakit, kart, havale.
	PaymentMethod string `json:"paymentMethod,omitempty"`
	// MergedInto birleştirilen (kapanan) adisyonun eklendiği adisyon numarasıdır.
	MergedInto int64 `json:"mergedInto,omitempty"`
	// DiscountPercent adisyonun tamamına uygulanan yüzde indirimdir (0: yok);
	// DiscountAmount bunun kuruş karşılığıdır. Total ve VATTotal indirim düşülmüş hallerdir.
	DiscountPercent int    `json:"discountPercent"`
	DiscountAmount  int64  `json:"discountAmount"`
	Lines           []Line `json:"lines"`
}

func (a *Adisyon) active() bool { return a.Status == StatusOpen || a.Status == StatusPrinted }

// Business fişte görünen işletme bilgileridir.
type Business struct {
	Name      string `json:"name"`
	Slogan    string `json:"slogan"`
	Address1  string `json:"address1"`
	Address2  string `json:"address2"`
	Phone     string `json:"phone"`
	Instagram string `json:"instagram"`
	Facebook  string `json:"facebook"`
	TikTok    string `json:"tiktok"`
	Invite    string `json:"invite"`
	GoogleURL string `json:"googleUrl"`
}

// Settings veritabanında saklanan program ayarlarıdır.
type Settings struct {
	Business        Business        `json:"business"`
	Tables          int             `json:"tables"`
	DefaultVAT      int             `json:"defaultVat"`
	LockMinutes     int             `json:"lockMinutes"`
	Printer         printing.Config `json:"printer"`
	SecondBackupDir string          `json:"secondBackupDir"`
	DriveBackupDir  string          `json:"driveBackupDir"`
	Fullscreen      bool            `json:"fullscreen"`
	SetupDone       bool            `json:"setupDone"`
	PinSalt         string          `json:"pinSalt"`
	PinHash         string          `json:"pinHash"`
	// FeaturedProductID istatistikte "satılan cağ" kartının ürünü (0: adı Cağ olan).
	FeaturedProductID int64            `json:"featuredProductId"`
	WhatsApp          WhatsAppSettings `json:"whatsapp"`
	Sheets            SheetsSettings   `json:"sheets"`
	// DiscountNoPIN açıksa indirim yönetici şifresi istemez (varsayılan: ister).
	DiscountNoPIN bool `json:"discountNoPin"`
}

// WhatsAppSettings gün sonu özetinin WhatsApp Business Platform (Cloud API) ile
// gönderilmesi içindir. Mesaj, Meta'da onaylanmış bir şablonla gider.
type WhatsAppSettings struct {
	Enabled       bool     `json:"enabled"`
	PhoneNumberID string   `json:"phoneNumberId"`
	Token         string   `json:"token"`
	Template      string   `json:"template"`
	Lang          string   `json:"lang"`
	APIVersion    string   `json:"apiVersion"`
	Recipients    []string `json:"recipients"`
}

// SheetsSettings gün sonu özetinin ve adisyonların Google Sheets'e yazılması
// içindir (tabloya eklenen Apps Script web uygulaması adresi).
type SheetsSettings struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Secret  string `json:"secret"`
}

// BackupInfo ayarlar ekranında gösterilen yedek durumudur.
type BackupInfo struct {
	Dir         string `json:"dir"`
	SecondDir   string `json:"secondDir"`
	LastAt      string `json:"lastAt"`
	LastError   string `json:"lastError"`
	SecondError string `json:"secondError"`
	SecondAt    string `json:"secondAt"`
	DriveDir    string `json:"driveDir"`
	DriveError  string `json:"driveError"`
	DriveAt     string `json:"driveAt"`
}

// ProgramInfo pencere ve sistem bilgisidir.
type ProgramInfo struct {
	Mode               string `json:"mode"` // "window" (program penceresi) | "browser"
	Fullscreen         bool   `json:"fullscreen"`
	Autostart          bool   `json:"autostart"`
	AutostartSupported bool   `json:"autostartSupported"`
	DataDir            string `json:"dataDir"`
}

// ColorDef kategori rengidir.
type ColorDef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Hex  string `json:"hex"`
}

// Colors kategori için seçilebilecek renklerdir.
var Colors = []ColorDef{
	{"bordo", "Bordo", "#7B2D4F"},
	{"kirmizi", "Kırmızı", "#B42318"},
	{"turuncu", "Turuncu", "#B45309"},
	{"yesil", "Yeşil", "#1C6B3A"},
	{"mavi", "Mavi", "#1D4F86"},
	{"mor", "Mor", "#5B3A8E"},
	{"kahve", "Kahve", "#6B4423"},
	{"gri", "Gri", "#4B5563"},
}

// Units ürün birimleridir.
var Units = []string{"adet", "şiş", "porsiyon", "kase", "bardak", "tabak", "kg"}

// State arayüzün ekranı çizmek için ihtiyaç duyduğu her şeydir.
type State struct {
	Version      string          `json:"version"`
	SetupDone    bool            `json:"setupDone"`
	Business     Business        `json:"business"`
	HasLogo      bool            `json:"hasLogo"`
	LogoRev      int64           `json:"logoRev"`
	Tables       int             `json:"tables"`
	DefaultVAT   int             `json:"defaultVat"`
	LockMinutes  int             `json:"lockMinutes"`
	Unlocked     bool            `json:"unlocked"`
	UnlockLeftMs int64           `json:"unlockLeftMs"`
	Categories   []Category      `json:"categories"`
	Products     []Product       `json:"products"`
	Waiters      []Waiter        `json:"waiters"`
	Open         []Adisyon       `json:"open"`
	UndoCounts   map[int64]int   `json:"undoCounts"`
	NextNo       int64           `json:"nextNo"`
	BusinessDate string          `json:"businessDate"` // yeni adisyonların yazıldığı (açık) iş günü
	ClockDate    string          `json:"clockDate"`    // saate göre bugünün iş günü
	Printer      printing.Config `json:"printer"`
	Backup       BackupInfo      `json:"backup"`
	Program      ProgramInfo     `json:"program"`
	Units        []string        `json:"units"`
	Colors       []ColorDef      `json:"colors"`
	ServerTime   string          `json:"serverTime"`
	// Seq her durum kopyasında artar; arayüz eski bir yanıtı yenisinin üstüne yazmaz.
	// Boot programın açılış anıdır (program yeniden açılınca Seq baştan başlar).
	Seq  uint64 `json:"seq"`
	Boot int64  `json:"boot"`

	PaymentMethods    []PaymentMethod `json:"paymentMethods"`
	UnclosedDays      []string        `json:"unclosedDays"` // gün sonu alınmamış önceki günler
	Notice            string          `json:"notice,omitempty"`
	FeaturedProductID int64           `json:"featuredProductId"`
	WhatsApp          WhatsAppView    `json:"whatsapp"`
	Sheets            SheetsView      `json:"sheets"`
	DiscountNoPIN     bool            `json:"discountNoPin"`
	DiscountPresets   []int           `json:"discountPresets"`
}

// WhatsAppView ayarlar ekranındaki WhatsApp bilgisidir (erişim anahtarı gönderilmez).
type WhatsAppView struct {
	Enabled       bool         `json:"enabled"`
	PhoneNumberID string       `json:"phoneNumberId"`
	HasToken      bool         `json:"hasToken"`
	TokenHint     string       `json:"tokenHint"`
	TokenLost     bool         `json:"tokenLost"` // yedekten başka bilgisayara geldi; yeniden girilmeli
	Template      string       `json:"template"`
	Lang          string       `json:"lang"`
	APIVersion    string       `json:"apiVersion"`
	Recipients    []string     `json:"recipients"`
	Status        OutboxStatus `json:"status"`
}

// SheetsView ayarlar ekranındaki Google Sheets bilgisidir.
type SheetsView struct {
	Enabled bool         `json:"enabled"`
	URL     string       `json:"url"`
	Secret  string       `json:"secret,omitempty"` // yalnız yönetici girişi açıkken
	Status  OutboxStatus `json:"status"`
}

// PrintOutcome bir baskı denemesinin arayüze dönen sonucudur.
type PrintOutcome struct {
	printing.Result
	No      int64  `json:"no"`
	Copy    bool   `json:"copy"`
	Preview string `json:"preview,omitempty"` // önizleme resmi (data URL)
}

// PayOutcome hesap kapatma sonucudur.
type PayOutcome struct {
	Paid        bool   `json:"paid"`
	No          int64  `json:"no"`
	Total       int64  `json:"total"` // kapanan tutar
	Method      string `json:"method"`
	MethodLabel string `json:"methodLabel"`
}

// UserError garsona ya da yöneticiye gösterilecek açıklamalı hatadır.
type UserError struct {
	Code string
	Msg  string
}

func (e *UserError) Error() string { return e.Msg }

func uerr(code, format string, a ...any) error {
	return &UserError{Code: code, Msg: fmt.Sprintf(format, a...)}
}

// errLocked yönetici şifresi gereken işlemler için döner.
var errLocked = &UserError{Code: "locked", Msg: "Bu işlem yönetici şifresi ister."}

func businessDate(t time.Time) string {
	if t.Hour() < BusinessDayCutoffHour {
		t = t.AddDate(0, 0, -1)
	}
	return t.Format("2006-01-02")
}

func stamp(t time.Time) string { return t.Format(time.RFC3339) }
