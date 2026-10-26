package core

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"halisadisyon/internal/money"
	"halisadisyon/internal/printing"
)

// Desktop pencere ve işletim sistemi işlerini yapar (Windows'ta gerçek, testte sahte).
type Desktop interface {
	Mode() string
	Autostart() (enabled, supported bool)
	SetAutostart(on bool) error
	WindowAction(action string) error
	IsFullscreen() bool
	OpenFolder(path string) error
	// RevealFile dosyayı Windows Gezgini'nde seçili gösterir.
	RevealFile(path string) error
	RemovableDrives() []Drive
	// GoogleDriveDirs bilgisayardaki Google Drive klasörlerini verir ("G:\Drive'ım").
	GoogleDriveDirs() []string
	// ReportsDir Excel raporlarının yazılacağı klasördür (Belgeler\Halis Adisyon Raporları).
	ReportsDir() string
}

// Drive yedek için seçilebilecek çıkarılabilir sürücüdür (USB bellek).
type Drive struct {
	Path  string `json:"path"`
	Label string `json:"label"`
}

// Backuper otomatik yedeği yöneten bileşendir.
type Backuper interface {
	Run(reason string) error
	Status() BackupStatus
	// List geri yüklenebilecek yedekleri yeniden eskiye verir.
	List() []BackupFile
	// PrepareRestore yedeği doğrulayıp program yeniden açılınca yerine konmak üzere
	// hazırlar. maxNo şu ana kadar verilmiş en büyük adisyon numarasıdır.
	PrepareRestore(path string, maxNo int64) error
	// SaveCopy şu anki verinin adlı, ayrı saklanan bir yedeğini alır.
	SaveCopy(label string) (string, error)
}

// BackupStatus yedek durumudur.
type BackupStatus struct {
	Dir, LastAt, LastError string
	SecondError, SecondAt  string // USB bellek / ikinci disk
	DriveError, DriveAt    string // Google Drive klasörü
}

// BackupFile geri yüklenebilecek bir yedektir.
type BackupFile struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Where string `json:"where"`
	At    string `json:"at"`
	Size  int64  `json:"size"`
}

// Options Service'in dış bağımlılıklarıdır.
type Options struct {
	Version  string
	DataDir  string
	Desktop  Desktop
	Backup   Backuper
	Now      func() time.Time
	Send     func(cfg printing.Config, data []byte, doc string) printing.Result
	Printers func() ([]printing.PrinterInfo, string)
	Log      *log.Logger
	// Restart programı kapatıp yeniden açar (yedekten geri yüklemeden sonra).
	Restart func() error
	// Seal ve Unseal gizli bilgiyi (WhatsApp erişim anahtarı) bu bilgisayardaki
	// Windows kullanıcısına bağlı olarak şifreler (DPAPI). Boşsa düz saklanır.
	Seal   func([]byte) ([]byte, error)
	Unseal func([]byte) ([]byte, error)
}

// Service tüm işlemleri sırayla (kilit altında) yürütür.
type Service struct {
	mu      sync.Mutex
	printMu sync.Mutex
	db      *sql.DB
	opt     Options
	set     Settings

	logo    []byte
	logoImg image.Image
	logoRev int64

	undo    map[int64][][]Line
	inPrint map[int64]bool // şu an basılan ya da kapanan adisyonlar
	seq     uint64
	boot    int64

	outboxKick  chan struct{}
	notice      string // kısa süre gösterilecek bilgi (ör. yedekten geri yüklendi)
	noticeUntil time.Time

	// Gün sonu alınmamış günler her ekran yenilemesinde değil, açık gün ya da
	// kapatılan günler değişince yeniden hesaplanır.
	daysVer      int
	tokenLost    bool // şifreli erişim anahtarı bu bilgisayarda açılamadı
	unclosedFor  string
	unclosedVer  int
	unclosedList []string

	unlockedUntil   time.Time
	pinFails        int // art arda yanlış şifre (kalıcı)
	pinBlocks       int // art arda kaç kez bekletildi (kalıcı)
	pinBlockedUntil time.Time
}

// New servisi kurar; boş veritabanına başlangıç ürünlerini ve ayarlarını yazar.
func New(db *sql.DB, opt Options) (*Service, error) {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.Send == nil {
		opt.Send = printing.Send
	}
	if opt.Printers == nil {
		opt.Printers = printing.ListPrinters
	}
	if opt.Log == nil {
		opt.Log = log.New(io.Discard, "", 0)
	}
	if opt.Desktop == nil {
		opt.Desktop = nullDesktop{}
	}
	if opt.Restart == nil {
		opt.Restart = func() error { return errors.New("yeniden başlatma desteklenmiyor") }
	}
	s := &Service{db: db, opt: opt, undo: map[int64][][]Line{}, inPrint: map[int64]bool{}, boot: time.Now().UnixMilli()}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) now() time.Time { return s.opt.Now() }

func defaultSettings() Settings {
	return Settings{
		Business: Business{
			Name:     "Halis Erzurum Cağ Kebabı",
			Address1: "Merkez: Akdeniz Sanayi 5009 Sk.",
			Address2: "Şube: Uncalı Mh. Çamlık Cd.",
			Invite:   "Afiyet olsun! Memnun kaldıysanız Google’daki yorumunuz bizim için çok değerli.",
		},
		Tables:      24,
		DefaultVAT:  10,
		LockMinutes: 5,
		Printer:     printing.DefaultConfig(),
		Fullscreen:  true,
	}
}

// Kâğıt adisyondaki 12 ürün; fiyatlar ilk açılışta sorulur.
var defaultCategories = []struct{ name, color string }{
	{"Yemekler", "bordo"}, {"İçecekler", "mavi"}, {"Tatlı ve Diğer", "turuncu"},
}

var defaultProducts = []struct {
	name, unit string
	cat        int
}{
	{"Cağ", "şiş", 0}, {"Şiş", "porsiyon", 0}, {"Yoğurt", "kase", 0}, {"Tatlı", "porsiyon", 2},
	{"Salata", "porsiyon", 0}, {"Ayran", "adet", 1}, {"Şalgam", "adet", 1}, {"Kola", "adet", 1},
	{"Soda", "adet", 1}, {"Büyük Su", "adet", 1}, {"Küçük Su", "adet", 1}, {"Çorba", "kase", 0},
}

func (s *Service) init() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var raw string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key='settings'`).Scan(&raw)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		s.set = defaultSettings()
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM categories`).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			ts := stamp(s.now())
			catIDs := make([]int64, len(defaultCategories))
			for i, c := range defaultCategories {
				res, err := tx.Exec(`INSERT INTO categories(name,color,sort) VALUES(?,?,?)`, c.name, c.color, i)
				if err != nil {
					return err
				}
				catIDs[i], _ = res.LastInsertId()
			}
			for i, p := range defaultProducts {
				if _, err := tx.Exec(`INSERT INTO products(name,category_id,price,vat,unit,active,sort,created_at,updated_at)
					VALUES(?,?,0,NULL,?,1,?,?,?)`, p.name, catIDs[p.cat], p.unit, i, ts, ts); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(`INSERT INTO waiters(name,active,sort) VALUES('Garson 1',1,0)`); err != nil {
				return err
			}
		}
		if err := s.saveSettingsTx(tx, s.set); err != nil {
			return err
		}
		audit(tx, s.now(), "ilk_acilis", 0, nil)
		if err := tx.Commit(); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		s.set = defaultSettings()
		if err := json.Unmarshal([]byte(raw), &s.set); err != nil {
			return err
		}
		if err := s.set.Printer.Normalize(); err != nil {
			s.set.Printer = printing.DefaultConfig()
		}
		s.openSecrets()
	}
	s.loadPinGuard()
	if err := s.adoptPastDays(); err != nil {
		s.opt.Log.Printf("geçmiş günler aktarılamadı: %v", err)
	}
	var logo string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key='logo'`).Scan(&logo); err == nil && logo != "" {
		if b, err := base64.StdEncoding.DecodeString(logo); err == nil {
			if img, _, err := image.Decode(bytes.NewReader(b)); err == nil {
				s.logo, s.logoImg = b, img
				s.logoRev = s.now().UnixMilli()
			}
		}
	}
	return nil
}

// openSecrets şifreli saklanan erişim anahtarını açar. Başka bir bilgisayardan
// (yedekten) gelmişse açılamaz: anahtar boşaltılır, ayarlarda yeniden istenir.
func (s *Service) openSecrets() {
	t := s.set.WhatsApp.Token
	if !strings.HasPrefix(t, sealedPrefix) {
		return
	}
	s.set.WhatsApp.Token = ""
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(t, sealedPrefix))
	if err == nil && s.opt.Unseal != nil {
		var plain []byte
		if plain, err = s.opt.Unseal(raw); err == nil {
			s.set.WhatsApp.Token = string(plain)
			return
		}
	}
	s.tokenLost = true
	s.opt.Log.Printf("WhatsApp erişim anahtarı bu bilgisayarda açılamadı; yeniden girilmeli: %v", err)
}

// adoptPastDays 1.0.0'dan gelen veride gün sonu yoktu: ilk açılışta, açık masası
// olmayan geçmiş günler kapatılmış sayılır (yoksa her eski gün "gün sonu
// alınmadı" diye hatırlatılır ve tek tek kapatılması gerekirdi). Bir kez yapılır.
func (s *Service) adoptPastDays() error {
	var done string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key='gun_sonu_gecisi'`).Scan(&done)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	now := s.now()
	today := businessDate(now)
	return s.withTx(func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT OR IGNORE INTO business_days(date, closed_at, summary)
			SELECT DISTINCT business_date, ?, '{"gecis":true}' FROM adisyons
			WHERE business_date < ? AND business_date NOT IN
				(SELECT business_date FROM adisyons WHERE status IN ('open','printed'))`, stamp(now), today)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES('gun_sonu_gecisi',?)`, stamp(now)); err != nil {
			return err
		}
		if n > 0 {
			audit(tx, now, "gun_sonu_gecisi", 0, map[string]any{"gun": n, "bugun": today})
		}
		return nil
	})
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

type querier interface {
	execer
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

const sealedPrefix = "dpapi:"

// saveSettingsTx ayarları yazar; WhatsApp erişim anahtarı (varsa) şifrelenerek
// saklanır, böylece yedek dosyalarında düz metin olarak bulunmaz.
func (s *Service) saveSettingsTx(q execer, set Settings) error {
	if t := set.WhatsApp.Token; t != "" && s.opt.Seal != nil {
		sealed, err := s.opt.Seal([]byte(t))
		if err != nil {
			return err
		}
		set.WhatsApp.Token = sealedPrefix + base64.StdEncoding.EncodeToString(sealed)
	}
	b, err := json.Marshal(set)
	if err != nil {
		return err
	}
	_, err = q.Exec(`INSERT INTO settings(key,value) VALUES('settings',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(b))
	return err
}

func audit(q execer, at time.Time, action string, no int64, detail any) {
	var d sql.NullString
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			d = sql.NullString{String: string(b), Valid: true}
		}
	}
	var n sql.NullInt64
	if no > 0 {
		n = sql.NullInt64{Int64: no, Valid: true}
	}
	_, _ = q.Exec(`INSERT INTO audit_log(at,action,adisyon_no,detail) VALUES(?,?,?,?)`, stamp(at), action, n, d)
}

// ---- okuma yardımcıları ----

func loadCategories(q querier) ([]Category, error) {
	rows, err := q.Query(`SELECT id,name,color,sort FROM categories ORDER BY sort,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Color, &c.Sort); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func loadProducts(q querier) ([]Product, error) {
	rows, err := q.Query(`SELECT id,name,category_id,price,vat,unit,active,sort FROM products ORDER BY sort,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Product{}
	for rows.Next() {
		var p Product
		var vat sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Name, &p.CategoryID, &p.Price, &vat, &p.Unit, &p.Active, &p.Sort); err != nil {
			return nil, err
		}
		if vat.Valid {
			v := int(vat.Int64)
			p.VAT = &v
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func loadProduct(q querier, id int64) (*Product, error) {
	ps, err := loadProducts(q)
	if err != nil {
		return nil, err
	}
	for i := range ps {
		if ps[i].ID == id {
			return &ps[i], nil
		}
	}
	return nil, uerr("not_found", "Ürün bulunamadı.")
}

func loadWaiters(q querier, activeOnly bool) ([]Waiter, error) {
	query := `SELECT id,name,active FROM waiters ORDER BY sort,id`
	if activeOnly {
		query = `SELECT id,name,active FROM waiters WHERE active=1 ORDER BY sort,id`
	}
	rows, err := q.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Waiter{}
	for rows.Next() {
		var w Waiter
		if err := rows.Scan(&w.ID, &w.Name, &w.Active); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

const adisyonCols = `no,business_date,table_no,COALESCE(waiter_id,0),waiter_name,status,opened_at,
	COALESCE(printed_at,''),COALESCE(closed_at,''),print_count,changed,print_failed,COALESCE(cancel_reason,''),total,vat_total,
	COALESCE(payment_method,''),COALESCE(merged_into,0),discount_percent,discount_amount,COALESCE(credit_customer_id,0),
	COALESCE((SELECT name FROM credit_customers c WHERE c.id=adisyons.credit_customer_id),'')`

func scanAdisyon(sc interface{ Scan(...any) error }) (*Adisyon, error) {
	var a Adisyon
	err := sc.Scan(&a.No, &a.BusinessDate, &a.Table, &a.WaiterID, &a.WaiterName, &a.Status, &a.OpenedAt,
		&a.PrintedAt, &a.ClosedAt, &a.PrintCount, &a.Changed, &a.PrintFailed, &a.CancelReason, &a.Total, &a.VATTotal,
		&a.PaymentMethod, &a.MergedInto, &a.DiscountPercent, &a.DiscountAmount, &a.CreditCustomerID, &a.CreditCustomer)
	if err != nil {
		return nil, err
	}
	a.Lines = []Line{}
	return &a, nil
}

func loadLines(q querier, no int64) ([]Line, error) {
	rows, err := q.Query(`SELECT id,COALESCE(product_id,0),name,unit,price,vat,qty,printed_qty,comp FROM adisyon_lines WHERE adisyon_no=? ORDER BY pos,id`, no)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Line{}
	for rows.Next() {
		var l Line
		if err := rows.Scan(&l.ID, &l.ProductID, &l.Name, &l.Unit, &l.Price, &l.VAT, &l.Qty, &l.PrintedQty, &l.Comp); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func loadAdisyon(q querier, no int64) (*Adisyon, error) {
	a, err := scanAdisyon(q.QueryRow(`SELECT `+adisyonCols+` FROM adisyons WHERE no=?`, no))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, uerr("not_found", "Adisyon %d bulunamadı.", no)
	}
	if err != nil {
		return nil, err
	}
	if a.Lines, err = loadLines(q, no); err != nil {
		return nil, err
	}
	if a.Status == StatusPaid {
		if a.Payments, err = loadPayments(q, no); err != nil {
			return nil, err
		}
		a.Payments = partsOf(a)
	}
	return a, nil
}

func loadActive(q querier) ([]Adisyon, error) {
	rows, err := q.Query(`SELECT ` + adisyonCols + ` FROM adisyons WHERE status IN ('open','printed') ORDER BY table_no,no`)
	if err != nil {
		return nil, err
	}
	var out []Adisyon
	for rows.Next() {
		a, err := scanAdisyon(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Lines, err = loadLines(q, out[i].No); err != nil {
			return nil, err
		}
	}
	if out == nil {
		out = []Adisyon{}
	}
	return out, nil
}

func activeForTable(q querier, table int) (*Adisyon, error) {
	var no int64
	err := q.QueryRow(`SELECT no FROM adisyons WHERE table_no=? AND status IN ('open','printed') ORDER BY no DESC LIMIT 1`, table).Scan(&no)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return loadAdisyon(q, no)
}

// nextNo sıradaki adisyon numarasıdır. Yedekten geri yüklemede kâğıtta verilmiş
// numaralar tekrar kullanılmasın diye "no_floor" ayarının altına inilmez.
func nextNo(q querier) (int64, error) {
	var n int64
	err := q.QueryRow(`SELECT MAX(COALESCE((SELECT MAX(no) FROM adisyons),0),
		COALESCE((SELECT CAST(value AS INTEGER) FROM settings WHERE key='no_floor'),0))+1`).Scan(&n)
	return n, err
}

// VATGroup bir KDV oranına düşen KDV dahil (indirim düşülmüş) tutar, içindeki
// KDV ve o orana düşen indirimdir.
type VATGroup struct {
	Rate     int   `json:"rate"`
	Gross    int64 `json:"gross"`
	VAT      int64 `json:"vat"`
	Discount int64 `json:"discount,omitempty"`
}

// discountOf tutarın yüzdesini kuruşa yuvarlar (yarım yukarı).
func discountOf(gross int64, pct int) int64 {
	if pct <= 0 || gross <= 0 {
		return 0
	}
	if pct >= 100 {
		return gross
	}
	return (gross*int64(pct) + 50) / 100
}

// Breakdown kalemleri KDV oranına göre gruplar; yüzde indirim her orana kendi
// tutarı üzerinden uygulanır ve KDV indirimli tutardan hesaplanır. Böylece fiş,
// gün sonu ve istatistik aynı kuruşları verir. İkram kalemleri tutara girmez.
func Breakdown(lines []Line, pct int) (groups []VATGroup, subtotal, discount, total, vat int64) {
	by := map[int]int64{}
	for _, l := range lines {
		if l.Comp {
			continue
		}
		g := l.Price * int64(l.Qty)
		by[l.VAT] += g
		subtotal += g
	}
	rates := make([]int, 0, len(by))
	for r := range by {
		rates = append(rates, r)
	}
	sort.Ints(rates)
	for _, r := range rates {
		d := discountOf(by[r], pct)
		net := by[r] - d
		v := money.VAT(net, r)
		groups = append(groups, VATGroup{Rate: r, Gross: net, VAT: v, Discount: d})
		discount += d
		total += net
		vat += v
	}
	return
}

// VATBreakdown indirimsiz döküm (eski çağıranlar için).
func VATBreakdown(lines []Line) (groups []VATGroup, total, vat int64) {
	groups, _, _, total, vat = Breakdown(lines, 0)
	return
}

func recalc(q querier, no int64) error {
	lines, err := loadLines(q, no)
	if err != nil {
		return err
	}
	var pct int
	if err := q.QueryRow(`SELECT discount_percent FROM adisyons WHERE no=?`, no).Scan(&pct); err != nil {
		return err
	}
	_, _, discount, total, vat := Breakdown(lines, pct)
	_, err = q.Exec(`UPDATE adisyons SET total=?, vat_total=?, discount_amount=? WHERE no=?`, total, vat, discount, no)
	return err
}

// ---- durum ----

// State ekranın güncel halini verir.
func (s *Service) State() (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateLocked()
}

func (s *Service) stateLocked() (*State, error) {
	cats, err := loadCategories(s.db)
	if err != nil {
		return nil, err
	}
	prods, err := loadProducts(s.db)
	if err != nil {
		return nil, err
	}
	waiters, err := loadWaiters(s.db, true)
	if err != nil {
		return nil, err
	}
	open, err := loadActive(s.db)
	if err != nil {
		return nil, err
	}
	nn, err := nextNo(s.db)
	if err != nil {
		return nil, err
	}
	now := s.now()
	s.seq++
	st := &State{
		Seq:          s.seq,
		Boot:         s.boot,
		Version:      s.opt.Version,
		SetupDone:    s.set.SetupDone,
		Business:     s.set.Business,
		HasLogo:      len(s.logo) > 0,
		LogoRev:      s.logoRev,
		Tables:       s.set.Tables,
		DefaultVAT:   s.set.DefaultVAT,
		LockMinutes:  s.set.LockMinutes,
		Categories:   cats,
		Products:     prods,
		Waiters:      waiters,
		Open:         open,
		UndoCounts:   s.undoCounts(),
		NextNo:       nn,
		BusinessDate: businessDate(now),
		Printer:      s.set.Printer,
		Units:        Units,
		Colors:       Colors,
		ServerTime:   stamp(now),
	}
	if !s.set.SetupDone {
		st.Unlocked = true
	} else if now.Before(s.unlockedUntil) {
		st.Unlocked = true
		st.UnlockLeftMs = s.unlockedUntil.Sub(now).Milliseconds()
	}
	if s.opt.Backup != nil {
		b := s.opt.Backup.Status()
		st.Backup = BackupInfo{Dir: b.Dir, SecondDir: s.set.SecondBackupDir, LastAt: b.LastAt, LastError: b.LastError,
			SecondError: b.SecondError, SecondAt: b.SecondAt, DriveDir: s.set.DriveBackupDir, DriveError: b.DriveError, DriveAt: b.DriveAt}
	} else {
		st.Backup.SecondDir = s.set.SecondBackupDir
		st.Backup.DriveDir = s.set.DriveBackupDir
	}
	st.PaymentMethods = PaymentMethods
	st.CollectMethods = CollectMethods
	if list, total, err := creditBriefs(s.db); err == nil {
		st.CreditCustomers, st.CreditBalance = list, total
	} else {
		st.CreditCustomers = []CreditBrief{}
	}
	st.ClockDate = businessDate(now)
	if day, err := openBusinessDay(s.db, now); err == nil {
		st.BusinessDate = day
		if s.unclosedFor != day || s.unclosedVer != s.daysVer || s.unclosedList == nil {
			if list, err := unclosedDays(s.db, day); err == nil {
				s.unclosedFor, s.unclosedVer, s.unclosedList = day, s.daysVer, list
			}
		}
		st.UnclosedDays = append([]string(nil), s.unclosedList...)
	}
	if st.UnclosedDays == nil {
		st.UnclosedDays = []string{}
	}
	if s.notice != "" && now.Before(s.noticeUntil) {
		st.Notice = s.notice
	}
	st.FeaturedProductID = s.set.FeaturedProductID
	st.DiscountNoPIN = s.set.DiscountNoPIN
	st.DiscountPresets = DiscountPresets
	wa := s.set.WhatsApp
	st.WhatsApp = WhatsAppView{Enabled: wa.Enabled, PhoneNumberID: wa.PhoneNumberID, HasToken: wa.Token != "",
		Template: wa.Template, Lang: wa.Lang, APIVersion: wa.APIVersion, Recipients: wa.Recipients}
	if n := len(wa.Token); n > 4 {
		st.WhatsApp.TokenHint = "…" + wa.Token[n-4:]
	}
	st.WhatsApp.TokenLost = s.tokenLost && wa.Token == ""
	if st.WhatsApp.Recipients == nil {
		st.WhatsApp.Recipients = []string{}
	}
	st.Sheets = SheetsView{Enabled: s.set.Sheets.Enabled, URL: s.set.Sheets.URL}
	if st.Unlocked {
		st.Sheets.Secret = s.set.Sheets.Secret
		st.WhatsApp.Status = s.outboxStatus(outboxWhatsApp)
		st.Sheets.Status = s.outboxStatus(outboxSheets)
	}
	auto, sup := s.opt.Desktop.Autostart()
	st.Program = ProgramInfo{
		Mode:               s.opt.Desktop.Mode(),
		Fullscreen:         s.set.Fullscreen,
		Autostart:          auto,
		AutostartSupported: sup,
		DataDir:            s.opt.DataDir,
	}
	return st, nil
}

// Settings ayarların bir kopyasını verir.
func (s *Service) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.set
}

// LogoPNG yüklenen logoyu verir (yoksa nil).
func (s *Service) LogoPNG() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.logo
}

// ---- metin doğrulama ----

func clean(s string, max int, field string) (string, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r", ""))
	if !utf8.ValidString(s) {
		return "", uerr("invalid", "%s geçersiz karakter içeriyor.", field)
	}
	if utf8.RuneCountInString(s) > max {
		return "", uerr("invalid", "%s en fazla %d karakter olabilir.", field, max)
	}
	return s, nil
}

func required(s string, max int, field string) (string, error) {
	s, err := clean(s, max, field)
	if err != nil {
		return "", err
	}
	if s == "" {
		return "", uerr("invalid", "%s boş bırakılamaz.", field)
	}
	return s, nil
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

type nullDesktop struct{}

func (nullDesktop) Mode() string              { return "browser" }
func (nullDesktop) Autostart() (bool, bool)   { return false, false }
func (nullDesktop) SetAutostart(bool) error   { return errors.New("desteklenmiyor") }
func (nullDesktop) WindowAction(string) error { return errors.New("desteklenmiyor") }
func (nullDesktop) IsFullscreen() bool        { return false }
func (nullDesktop) OpenFolder(string) error   { return errors.New("desteklenmiyor") }
func (nullDesktop) RevealFile(string) error   { return errors.New("desteklenmiyor") }
func (nullDesktop) RemovableDrives() []Drive  { return nil }
func (nullDesktop) GoogleDriveDirs() []string { return nil }
func (nullDesktop) ReportsDir() string        { return "" }
