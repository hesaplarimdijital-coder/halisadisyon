package core

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"halisadisyon/internal/money"
)

// AuditRow işlem kaydı ekranındaki bir satırdır.
type AuditRow struct {
	ID     int64  `json:"id"`
	At     string `json:"at"`
	Action string `json:"action"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	No     int64  `json:"no,omitempty"`
	Text   string `json:"text"`
}

// AuditKinds işlem kaydı filtresindeki gruplardır.
var AuditKinds = []struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}{
	{"", "Tümü"},
	{"iptal", "İptaller"},
	{"fiyat", "Fiyat ve ürün"},
	{"baski", "Baskılar"},
	{"odeme", "Ödemeler"},
	{"masa", "Masa ve garson"},
	{"kalem", "Kalem değişiklikleri"},
	{"gun", "Gün sonu ve yedek"},
	{"ayar", "Ayarlar ve şifre"},
}

var auditKindOf = map[string]string{
	"iptal": "iptal", "bos_kapatildi": "iptal", "yeniden_acildi": "iptal",
	"indirim": "odeme", "indirim_kaldirildi": "odeme", "ikram": "kalem", "ikram_kaldirildi": "kalem",
	"urun_eklendi": "fiyat", "urun_degisti": "fiyat", "toplu_fiyat": "fiyat", "toplu_zam": "fiyat",
	"basildi": "baski", "basilamadi": "baski", "test_fisi": "baski", "gun_sonu_fisi": "baski",
	"odendi": "odeme", "odeme_degisti": "odeme",
	"veresiye_tahsil": "odeme", "veresiye_tahsil_iptal": "odeme", "veresiye_musteri": "odeme", "veresiye_musteri_degisti": "odeme",
	"adisyon_acildi": "masa", "masa_tasindi": "masa", "masa_birlesti": "masa", "garson_degisti": "masa",
	"kalem": "kalem", "geri_al": "kalem",
	"gun_kapatildi": "gun", "gun_yeniden_acildi": "gun", "gun_duzeltildi": "gun", "is_gunu_degisti": "gun", "kayma_yoksayildi": "gun", "gun_sonu_gecisi": "gun", "yedek_geri_yuklendi": "gun", "geri_yukleme_hazirlandi": "gun", "gonderim": "gun", "rapor_alindi": "gun",
	"ayar_degisti": "ayar", "logo": "ayar", "kategori_eklendi": "ayar", "kategori_degisti": "ayar",
	"kategori_silindi": "ayar", "garson_eklendi": "ayar", "garson_kaldirildi": "ayar", "otomatik_baslat": "ayar",
	"kurulum_tamamlandi": "ayar", "ilk_acilis": "ayar",
	"sifre_cok_hatali": "ayar", "sifre_degisti": "ayar", "sifre_sifirlandi": "ayar",
}

var auditLabels = map[string]string{
	"iptal": "İptal", "bos_kapatildi": "Boş masa kapatıldı", "yeniden_acildi": "Düzeltmek için yeniden açıldı",
	"indirim": "İndirim uygulandı", "indirim_kaldirildi": "İndirim kaldırıldı", "ikram": "İkram", "ikram_kaldirildi": "İkram kaldırıldı",
	"urun_eklendi": "Ürün eklendi", "urun_degisti": "Ürün değişti", "toplu_fiyat": "Toplu zam", "toplu_zam": "Toplu zam uygulandı",
	"basildi": "Baskı", "basilamadi": "Baskı yapılamadı", "test_fisi": "Test fişi", "gun_sonu_fisi": "Gün sonu fişi",
	"odendi": "Hesap ödendi", "odeme_degisti": "Ödeme şekli değişti",
	"veresiye_tahsil": "Veresiye tahsilatı", "veresiye_tahsil_iptal": "Tahsilat iptal edildi",
	"veresiye_musteri": "Veresiye müşterisi açıldı", "veresiye_musteri_degisti": "Veresiye müşterisi değişti",
	"adisyon_acildi": "Masa açıldı", "masa_tasindi": "Masa taşındı", "masa_birlesti": "Masalar birleşti", "garson_degisti": "Garson değişti",
	"kalem": "Kalem", "geri_al": "Geri alındı",
	"gun_kapatildi": "Gün kapatıldı", "gun_yeniden_acildi": "Gün yeniden açıldı", "gun_duzeltildi": "Kaymış adisyonlar düzeltildi",
	"is_gunu_degisti": "İş günü değişti", "kayma_yoksayildi": "Kayma böyle bırakıldı", "yedek_geri_yuklendi": "Yedekten geri yüklendi", "gonderim": "Gönderim",
	"geri_yukleme_hazirlandi": "Geri yükleme başlatıldı", "rapor_alindi": "Excel raporu", "gun_sonu_gecisi": "Sürüm geçişi",
	"ayar_degisti": "Ayar değişti", "logo": "Logo", "kategori_eklendi": "Kategori eklendi", "kategori_degisti": "Kategori değişti",
	"kategori_silindi": "Kategori silindi", "garson_eklendi": "Garson eklendi", "garson_kaldirildi": "Garson kaldırıldı",
	"otomatik_baslat": "Otomatik başlatma", "kurulum_tamamlandi": "Kurulum tamamlandı", "ilk_acilis": "İlk açılış",
	"sifre_cok_hatali": "Çok yanlış şifre", "sifre_degisti": "Şifre değişti", "sifre_sifirlandi": "Şifre sıfırlandı",
}

func dStr(d map[string]any, k string) string {
	switch v := d[k].(type) {
	case string:
		return v
	case float64:
		return fmt.Sprint(int64(v))
	case bool:
		if v {
			return "evet"
		}
		return "hayır"
	case nil:
		return ""
	case []any:
		parts := make([]string, 0, len(v))
		for _, x := range v {
			parts = append(parts, fmt.Sprint(x))
		}
		return strings.Join(parts, ", ")
	}
	return fmt.Sprint(d[k])
}

func dNum(d map[string]any, k string) (int64, bool) {
	v, ok := d[k].(float64)
	return int64(v), ok
}

func dMoney(d map[string]any, k string) string {
	if v, ok := dNum(d, k); ok {
		return money.TL(v)
	}
	return ""
}

func vatText(d map[string]any, k string) string {
	if v, ok := dNum(d, k); ok {
		return fmt.Sprintf("%%%d", v)
	}
	return "varsayılan"
}

// describeAudit kayıttaki ayrıntıyı okunur bir cümleye çevirir.
func describeAudit(action string, no int64, d map[string]any) string {
	switch action {
	case "adisyon_acildi":
		return fmt.Sprintf("Masa %s · garson %s", dStr(d, "masa"), dStr(d, "garson"))
	case "kalem":
		f, _ := dNum(d, "fark")
		sign := "+"
		if f < 0 {
			sign = ""
		}
		t := fmt.Sprintf("%s %s%d", dStr(d, "urun"), sign, f)
		if a, ok := dNum(d, "adet"); ok {
			t += fmt.Sprintf(" (adet %d)", a)
		}
		if b, ok := dNum(d, "basilan_adet"); ok {
			t += fmt.Sprintf(" · basılan adet %d idi, şifreyle düşüldü", b)
		}
		return t
	case "geri_al":
		return "Son kalem değişikliği geri alındı"
	case "iptal":
		t := fmt.Sprintf("Sebep: %s · tutar %s · Masa %s", dStr(d, "sebep"), dMoney(d, "tutar"), dStr(d, "masa"))
		if dStr(d, "eski_durum") == StatusPaid {
			t += " · ödenmişti"
			if p := dStr(d, "eski_parcalar"); p != "" {
				t += " (" + p + ")"
			} else if m := dStr(d, "eski_odeme"); m != "" {
				t += " (" + PaymentLabel(m) + ")"
			}
			if m := dStr(d, "musteri"); m != "" {
				t += " · veresiye " + m + " borcundan düşüldü"
			}
		}
		return t
	case "yeniden_acildi":
		t := fmt.Sprintf("Sebep: %s · %s", dStr(d, "sebep"), dMoney(d, "tutar"))
		if p := dStr(d, "eski_parcalar"); p != "" {
			t += " · ödeme (" + p + ") kaldırıldı"
		} else if m := dStr(d, "eski_odeme"); m != "" {
			t += " · ödeme " + PaymentLabel(m) + " kaldırıldı"
		}
		if m := dStr(d, "musteri"); m != "" {
			t += " · veresiye " + m + " borcundan düşüldü"
		}
		if o, n := dStr(d, "eski_masa"), dStr(d, "masa"); o != n && o != "" {
			t += fmt.Sprintf(" · Masa %s → Masa %s", o, n)
		} else {
			t += " · Masa " + n
		}
		return t
	case "indirim":
		return fmt.Sprintf("%%%s · %s indirim · toplam %s → %s", dStr(d, "yuzde"), dMoney(d, "tutar"), dMoney(d, "ara_toplam"), dMoney(d, "toplam"))
	case "indirim_kaldirildi":
		return fmt.Sprintf("%%%s indirim kaldırıldı · toplam %s", dStr(d, "eski_yuzde"), dMoney(d, "toplam"))
	case "ikram", "ikram_kaldirildi":
		return fmt.Sprintf("%s × %s · %s değerinde", dStr(d, "urun"), dStr(d, "adet"), dMoney(d, "deger"))
	case "bos_kapatildi":
		if b, _ := dNum(d, "baski"); b > 0 {
			return fmt.Sprintf("Masa %s · basılmıştı (%d baskı), ürünler şifreyle silindi", dStr(d, "masa"), b)
		}
		return fmt.Sprintf("Masa %s · ürünler silinip masa boşaltıldı", dStr(d, "masa"))
	case "odendi":
		t := fmt.Sprintf("%s · Masa %s", dMoney(d, "tutar"), dStr(d, "masa"))
		if p := dStr(d, "parcalar"); p != "" {
			t += " · " + p
		} else if m := dStr(d, "odeme"); m != "" {
			t += " · " + PaymentLabel(m)
		}
		if m := dStr(d, "musteri"); m != "" {
			t += " · veresiye: " + m
		}
		if dStr(d, "basildi") == "hayır" {
			t += " · basılmadan"
		} else if dStr(d, "degisti") == "evet" {
			t += " · son hali basılmadan"
		}
		return t
	case "odeme_degisti":
		o, n := dStr(d, "eski_metin"), dStr(d, "yeni_metin")
		if o == "" {
			o, n = PaymentLabel(dStr(d, "eski")), PaymentLabel(dStr(d, "yeni"))
		}
		if m := dStr(d, "eski_musteri"); m != "" {
			o += " (" + m + ")"
		}
		if m := dStr(d, "musteri"); m != "" {
			n += " (" + m + ")"
		}
		return o + " → " + n
	case "veresiye_tahsil":
		t := fmt.Sprintf("%s · %s · %s · kalan borç %s", dStr(d, "musteri"), dMoney(d, "tutar"), PaymentLabel(dStr(d, "odeme")), dMoney(d, "kalan"))
		if n := dStr(d, "not"); n != "" {
			t += " · " + n
		}
		return t
	case "veresiye_tahsil_iptal":
		return fmt.Sprintf("%s · %s %s · sebep: %s · borç %s", dStr(d, "musteri"), dMoney(d, "tutar"), PaymentLabel(dStr(d, "odeme")), dStr(d, "sebep"), dMoney(d, "kalan"))
	case "veresiye_musteri":
		t := dStr(d, "ad")
		if p := dStr(d, "telefon"); p != "" {
			t += " · " + p
		}
		return t
	case "veresiye_musteri_degisti":
		if o, n := dStr(d, "eski_ad"), dStr(d, "ad"); o != n {
			return o + " → " + n
		}
		return dStr(d, "ad") + " · bilgiler güncellendi"
	case "basildi":
		t := fmt.Sprintf("%s. baskı · %s", dStr(d, "sira"), dMoney(d, "tutar"))
		if dStr(d, "kopya") == "evet" {
			t += " · KOPYA"
		}
		return t
	case "basilamadi":
		return "Yazıcı hatası: " + dStr(d, "kod")
	case "test_fisi":
		if dStr(d, "ornek") == "evet" {
			if dStr(d, "ok") == "evet" {
				return "Örnek adisyon basıldı · " + dStr(d, "baglanti")
			}
			return "Örnek adisyon basılamadı (" + dStr(d, "kod") + ")"
		}
		if dStr(d, "ok") == "evet" {
			return "Basıldı · " + dStr(d, "baglanti")
		}
		return "Basılamadı (" + dStr(d, "kod") + ") · " + dStr(d, "baglanti")
	case "gun_sonu_fisi":
		return "Tarih " + dateTRLong(dStr(d, "tarih"))
	case "masa_tasindi":
		return fmt.Sprintf("Masa %s → Masa %s", dStr(d, "eski_masa"), dStr(d, "yeni_masa"))
	case "masa_birlesti":
		return fmt.Sprintf("No %s (Masa %s, %s) → bu adisyona (Masa %s)", dStr(d, "kaynak_no"), dStr(d, "kaynak_masa"), dMoney(d, "tutar"), dStr(d, "hedef_masa"))
	case "garson_degisti":
		if no > 0 {
			return fmt.Sprintf("%s → %s", dStr(d, "eski"), dStr(d, "yeni"))
		}
		return fmt.Sprintf("Garson adı: %s → %s", dStr(d, "eski"), dStr(d, "yeni"))
	case "urun_eklendi":
		return fmt.Sprintf("%s · %s · KDV %s", dStr(d, "ad"), dMoney(d, "fiyat"), vatText(d, "kdv"))
	case "urun_degisti":
		var parts []string
		if old, nw := dStr(d, "eski_ad"), dStr(d, "ad"); old != "" && old != nw {
			parts = append(parts, fmt.Sprintf("ad %s → %s", old, nw))
		}
		if o, _ := dNum(d, "eski_fiyat"); true {
			if n, _ := dNum(d, "yeni_fiyat"); o != n {
				parts = append(parts, fmt.Sprintf("fiyat %s → %s", money.TL(o), money.TL(n)))
			}
		}
		if vatText(d, "eski_kdv") != vatText(d, "yeni_kdv") {
			parts = append(parts, fmt.Sprintf("KDV %s → %s", vatText(d, "eski_kdv"), vatText(d, "yeni_kdv")))
		}
		if dStr(d, "satista") == "hayır" {
			parts = append(parts, "satıştan kaldırıldı")
		}
		if len(parts) == 0 {
			parts = append(parts, "bilgiler kaydedildi")
		}
		return dStr(d, "ad") + ": " + strings.Join(parts, " · ")
	case "toplu_fiyat":
		return fmt.Sprintf("%s: %s → %s (%%%s)", dStr(d, "ad"), dMoney(d, "eski_fiyat"), dMoney(d, "yeni_fiyat"), trimFloat(d["yuzde"]))
	case "toplu_zam":
		return fmt.Sprintf("%%%s · %s ürün", trimFloat(d["yuzde"]), dStr(d, "urun"))
	case "gun_kapatildi":
		return fmt.Sprintf("%s · ciro %s · %s adisyon · %s iptal", dateTRLong(dStr(d, "tarih")), dMoney(d, "ciro"), dStr(d, "adisyon"), dStr(d, "iptal"))
	case "gun_yeniden_acildi":
		t := fmt.Sprintf("%s · sebep: %s", dateTRLong(dStr(d, "tarih")), dStr(d, "sebep"))
		if k := dStr(d, "kapanis"); k != "" {
			if at, err := time.Parse(time.RFC3339, k); err == nil {
				t += " · " + at.In(time.Local).Format("02.01 15:04") + "'te kapatılmıştı"
			}
		}
		return t + fmt.Sprintf(" · ciro %s, %s adisyon", dMoney(d, "ciro"), dStr(d, "adisyon"))
	case "gun_duzeltildi":
		t := dStr(d, "metin") + " · sebep: " + dStr(d, "sebep")
		if v, ok := d["acilan"].([]any); ok && len(v) > 0 {
			t += " · yeniden açılan: " + dateList(v)
		}
		if v, ok := d["yeniden_hesaplanan"].([]any); ok && len(v) > 0 {
			t += " · özeti yeniden hesaplanan: " + dateList(v)
		}
		return t
	case "is_gunu_degisti":
		return fmt.Sprintf("%s → %s · sebep: %s", dateTRLong(dStr(d, "eski")), dateTRLong(dStr(d, "yeni")), dStr(d, "sebep"))
	case "kayma_yoksayildi":
		return fmt.Sprintf("%s'de açılıp %s'e yazılan adisyonlar böyle bırakıldı", dateTRLong(dStr(d, "dogru")), dateTRLong(dStr(d, "eski")))
	case "yedek_geri_yuklendi":
		t := "Dosya: " + dStr(d, "dosya")
		if n, ok := dNum(d, "en_buyuk_no"); ok && n > 0 {
			t += fmt.Sprintf(" · numaralar No %d'den sonra devam eder", n)
		}
		return t
	case "geri_yukleme_hazirlandi":
		return fmt.Sprintf("Dosya: %s · program yeniden başlatıldı", dStr(d, "dosya"))
	case "rapor_alindi":
		return fmt.Sprintf("%s – %s · %s", dateTRLong(dStr(d, "bas")), dateTRLong(dStr(d, "son")), dStr(d, "dosya"))
	case "gonderim":
		return dStr(d, "metin")
	case "ayar_degisti":
		if v, ok := d["degisiklik"].([]any); ok && len(v) > 0 {
			parts := make([]string, 0, len(v))
			for _, x := range v {
				parts = append(parts, fmt.Sprint(x))
			}
			return strings.Join(parts, " · ")
		}
		return "Değişen: " + dStr(d, "bolum")
	case "gun_sonu_gecisi":
		return fmt.Sprintf("1.0.0 verisindeki %s geçmiş gün kapatılmış sayıldı", dStr(d, "gun"))
	case "logo":
		if dStr(d, "var") == "evet" {
			return "Logo eklendi"
		}
		return "Logo kaldırıldı"
	case "kategori_eklendi", "kategori_silindi", "garson_eklendi", "garson_kaldirildi":
		return dStr(d, "ad")
	case "kategori_degisti":
		return dStr(d, "ad")
	case "otomatik_baslat":
		if dStr(d, "acik") == "evet" {
			return "Açıldı"
		}
		return "Kapatıldı"
	case "kurulum_tamamlandi":
		return fmt.Sprintf("%s masa · %s garson · %s", dStr(d, "masa"), dStr(d, "garson"), dStr(d, "yazici"))
	case "sifre_cok_hatali":
		if m := dStr(d, "bekleme_dk"); m != "" {
			return m + " dakika bekletildi"
		}
		return "Bekletildi"
	}
	return ""
}

func trimFloat(v any) string {
	f, ok := v.(float64)
	if !ok {
		return fmt.Sprint(v)
	}
	s := fmt.Sprintf("%.2f", f)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return strings.Replace(s, ".", ",", 1)
}

func auditRow(id int64, at, action string, no sql.NullInt64, detail sql.NullString) AuditRow {
	r := AuditRow{ID: id, At: at, Action: action, Kind: auditKindOf[action], Label: auditLabels[action]}
	if r.Label == "" {
		r.Label = action
	}
	if no.Valid {
		r.No = no.Int64
	}
	d := map[string]any{}
	if detail.Valid {
		_ = json.Unmarshal([]byte(detail.String), &d)
	}
	r.Text = describeAudit(action, r.No, d)
	return r
}

// AuditQuery işlem kaydı filtresidir. Tarihler takvim günüdür (YYYY-AA-GG).
type AuditQuery struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Kind   string `json:"kind"`
	No     int64  `json:"no"`
	Before int64  `json:"before"` // sayfalama: bu kayıttan eskiler
}

// AuditPage işlem kaydı sayfasıdır.
type AuditPage struct {
	Rows []AuditRow `json:"rows"`
	More bool       `json:"more"`
}

// AuditLog işlem kaydını (yeniden eskiye) verir.
func (s *Service) AuditLog(q AuditQuery) (*AuditPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	where := []string{"1=1"}
	var args []any
	if q.From != "" {
		if !validDate(q.From) {
			return nil, uerr("invalid", "Başlangıç tarihi anlaşılamadı.")
		}
		where = append(where, "substr(at,1,10) >= ?")
		args = append(args, q.From)
	}
	if q.To != "" {
		if !validDate(q.To) {
			return nil, uerr("invalid", "Bitiş tarihi anlaşılamadı.")
		}
		where = append(where, "substr(at,1,10) <= ?")
		args = append(args, q.To)
	}
	if q.No > 0 {
		where = append(where, "adisyon_no = ?")
		args = append(args, q.No)
	}
	if q.Kind != "" {
		var acts []string
		for a, k := range auditKindOf {
			if k == q.Kind {
				acts = append(acts, "'"+a+"'")
			}
		}
		if len(acts) == 0 {
			return nil, uerr("invalid", "Geçersiz işlem türü.")
		}
		where = append(where, "action IN ("+strings.Join(acts, ",")+")")
	}
	if q.Before > 0 {
		where = append(where, "id < ?")
		args = append(args, q.Before)
	}
	const pageSize = 200
	rows, err := s.db.Query(`SELECT id, at, action, adisyon_no, detail FROM audit_log WHERE `+strings.Join(where, " AND ")+
		` ORDER BY id DESC LIMIT ?`, append(args, pageSize+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := &AuditPage{Rows: []AuditRow{}}
	for rows.Next() {
		var id int64
		var at, action string
		var no sql.NullInt64
		var detail sql.NullString
		if err := rows.Scan(&id, &at, &action, &no, &detail); err != nil {
			return nil, err
		}
		if len(page.Rows) == pageSize {
			page.More = true
			break
		}
		page.Rows = append(page.Rows, auditRow(id, at, action, no, detail))
	}
	return page, rows.Err()
}

// AdisyonDetail bir adisyonun tüm bilgileri ve işlem geçmişidir (gün sonu ekranı).
type AdisyonDetail struct {
	Adisyon   *Adisyon   `json:"adisyon"`
	VATs      []VATGroup `json:"vats"`
	History   []AuditRow `json:"history"`
	DayClosed bool       `json:"dayClosed"`
}

// Detail adisyonun ayrıntısını verir.
func (s *Service) Detail(no int64) (*AdisyonDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	a, err := loadAdisyon(s.db, no)
	if err != nil {
		return nil, err
	}
	d := &AdisyonDetail{Adisyon: a, History: []AuditRow{}}
	d.VATs, _, _, _, _ = Breakdown(a.Lines, a.DiscountPercent)
	if d.DayClosed, _, err = dayClosed(s.db, a.BusinessDate); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id, at, action, adisyon_no, detail FROM audit_log WHERE adisyon_no=? ORDER BY id`, no)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var at, action string
		var n sql.NullInt64
		var detail sql.NullString
		if err := rows.Scan(&id, &at, &action, &n, &detail); err != nil {
			return nil, err
		}
		d.History = append(d.History, auditRow(id, at, action, n, detail))
	}
	return d, rows.Err()
}

// ChangePayment ödenmiş adisyonun ödemesini düzeltir (yönetici şifresi): tek
// şekil (method) ya da parçalar (parts) ve veresiye müşterisi. Kapatılmış günün
// adisyonu değiştirilemez. Veresiye kaldırılırken borç eksiye düşmemelidir.
func (s *Service) ChangePayment(no int64, method, pin string) (*State, error) {
	return s.SetPayments(no, []PaymentPart{{Method: method}}, nil, pin)
}

// SetPayments ChangePayment'ın parçalı ve veresiye destekli hâlidir.
func (s *Service) SetPayments(no int64, parts []PaymentPart, credit *CreditRef, pin string) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := loadAdisyon(s.db, no)
	if err != nil {
		return nil, err
	}
	if a.Status != StatusPaid {
		return nil, uerr("invalid", "Yalnız ödenmiş adisyonun ödeme şekli değişir.")
	}
	if closed, _, err := dayClosed(s.db, a.BusinessDate); err != nil {
		return nil, err
	} else if closed {
		return nil, uerr("closed", "Bu adisyonun günü kapatılmış; değiştirilemez.")
	}
	ps, err := normalizeParts(parts, a.Total)
	if err != nil {
		return nil, err
	}
	old := partsOf(a)
	same := len(old) == len(ps)
	for i := range ps {
		if same && (old[i].Method != ps[i].Method || old[i].Amount != ps[i].Amount) {
			same = false
		}
	}
	if same && (creditPart(ps) == 0 || credit == nil || credit.ID == a.CreditCustomerID ||
		(credit.ID == 0 && trFold(credit.Name) == trFold(a.CreditCustomer))) {
		return s.stateLocked()
	}
	if err := s.confirmPin(pin, "Ödeme şeklini değiştirmek yönetici şifresi ister."); err != nil {
		return nil, err
	}
	err = s.withTx(func(tx *sql.Tx) error {
		now := s.now()
		var cid int64
		var name string
		if creditPart(ps) > 0 {
			ref := credit
			if ref == nil && a.CreditCustomerID > 0 {
				ref = &CreditRef{ID: a.CreditCustomerID}
			}
			if cid, name, err = resolveCustomer(tx, ref, now); err != nil {
				return err
			}
		}
		keep := int64(0)
		if cid == a.CreditCustomerID {
			keep = creditPart(ps)
		}
		if err := checkCreditRemoval(tx, a, keep); err != nil {
			return err
		}
		if err := writePayments(tx, no, ps, cid); err != nil {
			return err
		}
		d := map[string]any{"eski": a.PaymentMethod, "yeni": methodOf(ps), "tutar": a.Total,
			"eski_metin": PaymentText(old, a.PaymentMethod), "yeni_metin": PaymentText(ps, "")}
		if a.CreditCustomer != "" {
			d["eski_musteri"] = a.CreditCustomer
		}
		if name != "" {
			d["musteri"] = name
		}
		audit(tx, now, "odeme_degisti", no, d)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.stateLocked()
}

func dateList(v []any) string {
	parts := make([]string, 0, len(v))
	for _, x := range v {
		parts = append(parts, dateTRLong(fmt.Sprint(x)))
	}
	return strings.Join(parts, ", ")
}
