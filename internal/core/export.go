package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"halisadisyon/internal/xlsx"
)

func localTime(ts string) time.Time {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}
	}
	return t.Local()
}

func dateOf(d string) time.Time {
	t, _ := parseDate(d)
	return t
}

func changeCell(c StatCard) xlsx.Cell {
	if c.Change == nil {
		return xlsx.Text("—")
	}
	return xlsx.Percent(*c.Change / 100)
}

// excelReport istatistiklerden Excel sayfalarını kurar.
func excelReport(biz string, st *Stats, list []Adisyon, created time.Time) []xlsx.Sheet {
	feat := "Satılan ürün"
	if st.FeaturedName != "" {
		feat = "Satılan " + st.FeaturedName + " (" + st.FeaturedUnit + ")"
	}
	// ---- Özet ----
	ozet := xlsx.Sheet{Name: "Özet", Widths: []float64{30, 18, 18, 12}}
	add := func(cells ...xlsx.Cell) { ozet.Rows = append(ozet.Rows, cells) }
	add(xlsx.Title(biz + " · Satış raporu"))
	add(xlsx.Bold("Dönem"), xlsx.Text(st.Label))
	add(xlsx.Bold("Karşılaştırılan dönem"), xlsx.Text(st.PrevLabel))
	add(xlsx.Bold("Tarih aralığı"), xlsx.Date(dateOf(st.From)), xlsx.Date(dateOf(st.To)))
	add()
	add(xlsx.Header("Gösterge"), xlsx.Header("Bu dönem"), xlsx.Header("Önceki dönem"), xlsx.Header("Değişim"))
	add(xlsx.Text("Ciro (KDV dahil)"), xlsx.Money(st.Revenue.Value), xlsx.Money(st.Revenue.Prev), changeCell(st.Revenue))
	add(xlsx.Text("Adisyon sayısı"), xlsx.Int(st.Count.Value), xlsx.Int(st.Count.Prev), changeCell(st.Count))
	add(xlsx.Text("Ortalama adisyon"), xlsx.Money(st.Avg.Value), xlsx.Money(st.Avg.Prev), changeCell(st.Avg))
	if st.FeaturedID > 0 {
		add(xlsx.Text(feat), xlsx.Int(st.Featured.Value), xlsx.Int(st.Featured.Prev), changeCell(st.Featured))
	}
	add(xlsx.Text("KDV toplamı"), xlsx.Money(st.VAT))
	add(xlsx.Text("Matrah"), xlsx.Money(st.Revenue.Value-st.VAT))
	add(xlsx.Text("İptal edilen adisyon"), xlsx.Int(int64(st.Cancelled)))
	add(xlsx.Text("İptal tutarı"), xlsx.Money(st.CancelledTotal))
	add(xlsx.Text("İndirimli adisyon"), xlsx.Int(int64(st.Discounts)))
	add(xlsx.Text("Yapılan indirim"), xlsx.Money(st.DiscountTotal))
	add()
	add(xlsx.Header("Ödeme şekli"), xlsx.Header("Adisyon"), xlsx.Header("Tutar"), xlsx.Header("Pay"))
	for _, p := range st.ByPayment {
		share := 0.0
		if st.Revenue.Value > 0 {
			share = float64(p.Total) / float64(st.Revenue.Value)
		}
		add(xlsx.Text(p.Label), xlsx.Int(int64(p.Count)), xlsx.Money(p.Total), xlsx.Percent(share))
	}
	add()
	add(xlsx.Header("Garson"), xlsx.Header("Adisyon"), xlsx.Header("Ciro"), xlsx.Header("Ortalama"))
	for _, w := range st.ByWaiter {
		add(xlsx.Text(w.Name), xlsx.Int(int64(w.Count)), xlsx.Money(w.Total), xlsx.Money(avgOf(w.Total, int64(w.Count))))
	}
	add()
	add(xlsx.Text("Rapor tarihi"), xlsx.DateTime(created))
	add(xlsx.Text("Not: Ciro yalnız ödenen adisyonlardan hesaplanır; iş günü sabah 05:00'te başlar."))

	// ---- Günlük (gün × ürün adetleri dahil) ----
	unknown := false
	for _, d := range st.Days {
		if d.Unknown > 0 {
			unknown = true
		}
	}
	gun := xlsx.Sheet{Name: "Günlük", HeaderRow: 1}
	head := []xlsx.Cell{xlsx.Header("Tarih"), xlsx.Header("Gün"), xlsx.Header("Ciro"), xlsx.Header("Adisyon"), xlsx.Header("Ortalama"),
		xlsx.Header("Nakit"), xlsx.Header("Kredi Kartı"), xlsx.Header("Havale")}
	gun.Widths = []float64{12, 11, 15, 9, 13, 14, 14, 14}
	if unknown {
		head = append(head, xlsx.Header("Belirtilmemiş"))
		gun.Widths = append(gun.Widths, 14)
	}
	head = append(head, xlsx.Header("İptal"), xlsx.Header("İndirim"), xlsx.Header("KDV"))
	gun.Widths = append(gun.Widths, 7, 13, 13)
	for _, p := range st.Products {
		head = append(head, xlsx.Header(p.Name+" ("+p.Unit+")"))
		w := float64(len([]rune(p.Name))+len([]rune(p.Unit))) + 4
		if w < 9 {
			w = 9
		}
		gun.Widths = append(gun.Widths, w)
	}
	if len(st.Products) > 0 {
		head = append(head, xlsx.Header("Toplam adet"))
		gun.Widths = append(gun.Widths, 12)
	}
	gun.Rows = append(gun.Rows, head)
	var tot DayPoint
	colTot := make([]int64, len(st.Products))
	for i, d := range st.Days {
		t := dateOf(d.Date)
		row := []xlsx.Cell{xlsx.Date(t), xlsx.Text(gunlerTR[t.Weekday()]), xlsx.Money(d.Revenue), xlsx.Int(int64(d.Count)),
			xlsx.Money(avgOf(d.Revenue, int64(d.Count))), xlsx.Money(d.Cash), xlsx.Money(d.Card), xlsx.Money(d.Transfer)}
		if unknown {
			row = append(row, xlsx.Money(d.Unknown))
		}
		row = append(row, xlsx.Int(int64(d.Cancelled)), xlsx.Money(d.Discount), xlsx.Money(d.VAT))
		var dayQty int64
		for j := range st.Products {
			q := 0
			if i < len(st.Matrix) && j < len(st.Matrix[i]) {
				q = st.Matrix[i][j]
			}
			row = append(row, xlsx.Int(int64(q)))
			colTot[j] += int64(q)
			dayQty += int64(q)
		}
		if len(st.Products) > 0 {
			row = append(row, xlsx.Int(dayQty))
		}
		gun.Rows = append(gun.Rows, row)
		tot.Revenue += d.Revenue
		tot.Count += d.Count
		tot.Cash += d.Cash
		tot.Card += d.Card
		tot.Transfer += d.Transfer
		tot.Unknown += d.Unknown
		tot.Cancelled += d.Cancelled
		tot.Discount += d.Discount
		tot.VAT += d.VAT
	}
	gun.FilterLast = len(gun.Rows)
	row := []xlsx.Cell{xlsx.Bold("Toplam"), xlsx.Empty(), xlsx.MoneyBold(tot.Revenue), {V: int64(tot.Count), Style: xlsx.StyleIntBold},
		xlsx.MoneyBold(avgOf(tot.Revenue, int64(tot.Count))), xlsx.MoneyBold(tot.Cash), xlsx.MoneyBold(tot.Card), xlsx.MoneyBold(tot.Transfer)}
	if unknown {
		row = append(row, xlsx.MoneyBold(tot.Unknown))
	}
	row = append(row, xlsx.Cell{V: int64(tot.Cancelled), Style: xlsx.StyleIntBold}, xlsx.MoneyBold(tot.Discount), xlsx.MoneyBold(tot.VAT))
	var allQty int64
	for _, q := range colTot {
		row = append(row, xlsx.Cell{V: q, Style: xlsx.StyleIntBold})
		allQty += q
	}
	if len(st.Products) > 0 {
		row = append(row, xlsx.Cell{V: allQty, Style: xlsx.StyleIntBold})
	}
	gun.Rows = append(gun.Rows, row)

	// ---- Ürünler ----
	urun := xlsx.Sheet{Name: "Ürünler", HeaderRow: 1, Widths: []float64{24, 11, 10, 16, 11}}
	urun.Rows = append(urun.Rows, []xlsx.Cell{xlsx.Header("Ürün"), xlsx.Header("Birim"), xlsx.Header("Adet"), xlsx.Header("Tutar"), xlsx.Header("Ciro payı")})
	var pq, pt int64
	for _, p := range st.Products {
		share := 0.0
		if st.Revenue.Value > 0 {
			share = float64(p.Total) / float64(st.Revenue.Value)
		}
		urun.Rows = append(urun.Rows, []xlsx.Cell{xlsx.Text(p.Name), xlsx.Text(p.Unit), xlsx.Int(int64(p.Qty)), xlsx.Money(p.Total), xlsx.Percent(share)})
		pq += int64(p.Qty)
		pt += p.Total
	}
	urun.FilterLast = len(urun.Rows)
	urun.Rows = append(urun.Rows, []xlsx.Cell{xlsx.Bold("Toplam"), xlsx.Empty(), {V: pq, Style: xlsx.StyleIntBold}, xlsx.MoneyBold(pt)})

	// ---- Adisyonlar ----
	ad := xlsx.Sheet{Name: "Adisyonlar", HeaderRow: 1, Widths: []float64{8, 12, 7, 14, 17, 17, 16, 14, 14, 10, 13, 12, 8, 34}}
	ad.Rows = append(ad.Rows, []xlsx.Cell{xlsx.Header("No"), xlsx.Header("İş günü"), xlsx.Header("Masa"), xlsx.Header("Garson"),
		xlsx.Header("Açılış"), xlsx.Header("Kapanış"), xlsx.Header("Durum"), xlsx.Header("Ödeme şekli"), xlsx.Header("Tutar"),
		xlsx.Header("İndirim %"), xlsx.Header("İndirim"), xlsx.Header("KDV"), xlsx.Header("Baskı"), xlsx.Header("Not")})
	for i := range list {
		a := &list[i]
		note := a.CancelReason
		ad.Rows = append(ad.Rows, []xlsx.Cell{xlsx.Int(a.No), xlsx.Date(dateOf(a.BusinessDate)), xlsx.Int(int64(a.Table)), xlsx.Text(a.WaiterName),
			xlsx.DateTime(localTime(a.OpenedAt)), xlsx.DateTime(localTime(a.ClosedAt)), xlsx.Text(statusText(a)), xlsx.Text(PaymentLabel(a.PaymentMethod)),
			xlsx.Money(a.Total), xlsx.Int(int64(a.DiscountPercent)), xlsx.Money(a.DiscountAmount), xlsx.Money(a.VATTotal), xlsx.Int(int64(a.PrintCount)), xlsx.Text(note)})
	}
	return []xlsx.Sheet{ozet, gun, urun, ad}
}

// ExportExcel seçilen dönemin raporunu Excel dosyası olarak kaydeder ve dosyayı
// Windows Gezgini'nde gösterir. Dosya yolunu verir.
func (s *Service) ExportExcel(q StatsQuery) (string, error) {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return "", err
	}
	st, err := s.stats(q)
	var list []Adisyon
	if err == nil {
		list, err = s.periodAdisyons(st.From, st.To)
	}
	biz := s.set.Business.Name
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := xlsx.Write(&buf, excelReport(biz, st, list, s.now())); err != nil {
		return "", err
	}
	// Önce Belgeler\Halis Adisyon Raporları; oraya yazılamazsa (Belgeler başka yere
	// taşınmış, ağ klasörü erişilemiyor vb.) programın veri klasöründeki Raporlar.
	dirs := []string{}
	if d := s.opt.Desktop.ReportsDir(); d != "" {
		dirs = append(dirs, d)
	}
	if s.opt.DataDir != "" {
		dirs = append(dirs, filepath.Join(s.opt.DataDir, "Raporlar"))
	}
	if len(dirs) == 0 {
		return "", uerr("invalid", "Rapor klasörü bulunamadı.")
	}
	dir := ""
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			s.opt.Log.Printf("rapor klasörü oluşturulamadı: %v", err)
			continue
		}
		dir = d
		break
	}
	if dir == "" {
		return "", uerr("invalid", "Rapor klasörü oluşturulamadı (%s). Belgeler klasörüne yazma izni var mı?", dirs[0])
	}
	base := "Halis_Adisyon_" + st.From
	if st.To != st.From {
		base += "_" + st.To
	}
	path := filepath.Join(dir, base+".xlsx")
	tmp := path + ".yaziliyor"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		os.Remove(tmp)
		s.opt.Log.Printf("rapor yazılamadı: %v", err)
		if strings.Contains(strings.ToLower(err.Error()), "space") {
			return "", uerr("invalid", "Rapor yazılamadı: diskte yer kalmadı.")
		}
		return "", uerr("invalid", "Rapor dosyası yazılamadı (%s). Klasöre yazma izni var mı?", dir)
	}
	// Aynı adlı rapor Excel'de açıksa üzerine yazılamaz: yeni adla kaydet.
	if err := os.Rename(tmp, path); err != nil {
		for i := 2; i < 100; i++ {
			path = filepath.Join(dir, fmt.Sprintf("%s (%d).xlsx", base, i))
			if _, serr := os.Stat(path); serr == nil {
				continue
			}
			if err = os.Rename(tmp, path); err == nil {
				break
			}
		}
		if err != nil {
			os.Remove(tmp)
			return "", uerr("invalid", "Rapor kaydedilemedi. Aynı adlı rapor Excel'de açıksa kapatıp tekrar deneyin.")
		}
	}
	s.mu.Lock()
	audit(s.db, s.now(), "rapor_alindi", 0, map[string]any{"bas": st.From, "son": st.To, "dosya": filepath.Base(path)})
	s.mu.Unlock()
	_ = s.opt.Desktop.RevealFile(path)
	return path, nil
}
