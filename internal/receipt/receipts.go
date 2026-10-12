package receipt

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"strconv"
	"strings"
	"time"
	"unicode"

	"halisadisyon/internal/money"
)

// Business fişin üstünde ve altında görünen işletme bilgileridir.
type Business struct {
	Name, Slogan       string
	Address1, Address2 string
	Phone              string
	Instagram          string
	Facebook           string
	TikTok             string
	Invite             string // fişin altındaki davet metni
	GoogleURL          string // Google yorum bağlantısı (QR)
	Logo               image.Image
}

// Line adisyon kalemidir.
type Line struct {
	Name, Unit string
	Qty        int
	Price      int64 // birim fiyat, kuruş, KDV dahil
}

// VATGroup bir KDV oranına düşen tutardır.
type VATGroup struct {
	Rate       int
	Gross, VAT int64
}

// Adisyon basılacak müşteri adisyonudur. Total indirim düşülmüş tutardır;
// indirim varsa ara toplam ve indirim satırı da basılır.
type Adisyon struct {
	No              int64
	Table           int
	Waiter          string
	When            time.Time
	Copy            bool
	Lines           []Line
	Subtotal        int64
	DiscountPercent int
	Discount        int64
	Total           int64
	VATs            []VATGroup
}

// TestInfo test fişinde yazacak yazıcı bilgileridir.
type TestInfo struct {
	Connection string
	Width      int
	CutText    string
	When       time.Time
}

// trUpper Türkçe kurallarla büyük harfe çevirir (i → İ, ı → I).
func trUpper(s string) string { return strings.ToUpperSpecial(unicode.TurkishCase, s) }

func scale(width int) float64 {
	if width >= 576 {
		return 1
	}
	return float64(width) / 576
}

func header(c *canvas, b Business, sf float64) {
	if b.Logo != nil {
		c.picture(b.Logo, int(440*sf), int(210*sf))
		c.gap(int(6 * sf))
	} else {
		name := strings.TrimSpace(b.Name)
		if name == "" {
			name = "Adisyon"
		}
		words := strings.Fields(name)
		big := trUpper(words[0])
		c.fitLine(big, bold, 64*sf, center, int(8*sf), int(2*sf))
		if len(words) > 1 {
			rest := trUpper(strings.Join(words[1:], " "))
			c.gap(int(4 * sf))
			c.fitLine(rest, semibold, 22*sf, center, int(4*sf), 0)
		}
	}
	if s := strings.TrimSpace(b.Slogan); s != "" {
		c.gap(int(6 * sf))
		c.para(s, face(italic, 24*sf), center, int(2*sf))
	}
}

func footer(c *canvas, b Business, sf float64, legal string, withQR bool) {
	if s := strings.TrimSpace(b.Invite); s != "" {
		c.para(s, face(regular, 25*sf), center, int(4*sf))
		c.gap(int(6 * sf))
	}
	socials := []struct{ label, v string }{
		{"Instagram", b.Instagram}, {"Facebook", b.Facebook}, {"TikTok", b.TikTok},
	}
	for _, s := range socials {
		if v := strings.TrimSpace(s.v); v != "" {
			c.fitLine(s.label+": "+v, semibold, 24*sf, center, 0, int(4*sf))
		}
	}
	if withQR && strings.TrimSpace(b.GoogleURL) != "" {
		c.gap(int(12 * sf))
		if err := c.qr(strings.TrimSpace(b.GoogleURL), int(240*sf)); err == nil {
			c.gap(int(10 * sf))
			c.para("Kameranızla okutun, Google Haritalar’da yorum yapın", face(regular, 22*sf), center, int(2*sf))
		}
	}
	c.rule(true, 2, int(14*sf), int(12*sf))
	small := face(regular, 22*sf)
	for _, s := range []string{b.Address1, b.Address2} {
		if s = strings.TrimSpace(s); s != "" {
			c.para(s, small, center, int(2*sf))
		}
	}
	if p := strings.TrimSpace(b.Phone); p != "" {
		c.para("Tel: "+p, small, center, int(2*sf))
	}
	c.gap(int(10 * sf))
	c.fitLine(legal, bold, 20*sf, center, int(2*sf), 0)
}

func dateTR(t time.Time) string { return t.Format("02.01.2006") }

// RenderAdisyon müşteri adisyonunu çizer.
func RenderAdisyon(b Business, a Adisyon, width int) (*image.Gray, error) {
	renderMu.Lock()
	defer renderMu.Unlock()
	if err := loadFonts(); err != nil {
		return nil, err
	}
	sf := scale(width)
	c := newCanvas(width, int(12*sf))
	c.gap(int(8 * sf))
	header(c, b, sf)
	c.rule(true, 2, int(14*sf), int(12*sf))

	big := face(bold, 32*sf)
	c.row("ADİSYON", big, "No "+strconv.FormatInt(a.No, 10), big, 0, int(4*sf))
	if a.Copy {
		c.gap(int(4 * sf))
		c.boxed("KOPYA", face(bold, 26*sf), int(10*sf), 3)
		c.gap(int(2 * sf))
	}
	c.gap(int(6 * sf))
	c.row("Masa "+strconv.Itoa(a.Table), face(semibold, 27*sf), a.Waiter, face(regular, 26*sf), 0, int(2*sf))
	c.row(dateTR(a.When), face(regular, 24*sf), "Saat "+a.When.Format("15:04"), face(regular, 24*sf), 0, 0)
	c.rule(true, 2, int(12*sf), int(10*sf))

	head := face(bold, 20*sf)
	c.row("ÜRÜN", head, "TUTAR", head, int(3*sf), int(6*sf))
	itemF := face(semibold, 27*sf)
	subF := face(regular, 23*sf)
	for _, l := range a.Lines {
		c.row(l.Name, itemF, money.Format(l.Price*int64(l.Qty)), itemF, 0, 0)
		c.line(fmt.Sprintf("%d %s × %s", l.Qty, l.Unit, money.Format(l.Price)), subF, left, 0, int(8*sf))
	}
	c.rule(true, 2, int(4*sf), int(12*sf))

	if a.DiscountPercent > 0 {
		mid := face(semibold, 25*sf)
		c.row("Ara toplam", mid, money.Format(a.Subtotal), mid, 0, int(3*sf))
		c.row(fmt.Sprintf("İndirim (%%%d)", a.DiscountPercent), mid, "−"+money.Format(a.Discount), mid, 0, int(8*sf))
	}
	c.row("TOPLAM", face(bold, 32*sf), money.TL(a.Total), face(bold, 44*sf), 0, int(4*sf))
	small := face(regular, 22*sf)
	c.line("Fiyatlara KDV dahildir", small, left, 0, int(2*sf))
	var vatSum int64
	for _, v := range a.VATs {
		c.row(fmt.Sprintf("KDV (%%%d)", v.Rate), small, money.Format(v.VAT), small, 0, int(2*sf))
		vatSum += v.VAT
	}
	c.row("Matrah", small, money.Format(a.Total-vatSum), small, 0, 0)
	c.rule(false, 3, int(14*sf), int(16*sf))

	footer(c, b, sf, "BİLGİ FİŞİDİR · MALİ DEĞERİ YOKTUR", true)
	return c.finish(int(24 * sf)), nil
}

// RenderTest yazıcı ayarını denemek için test fişi çizer:
// Türkçe karakterler, kenar çerçevesi, siyahlık ve QR.
func RenderTest(b Business, t TestInfo, width int) (*image.Gray, error) {
	renderMu.Lock()
	defer renderMu.Unlock()
	if err := loadFonts(); err != nil {
		return nil, err
	}
	sf := scale(width)
	c := newCanvas(width, int(12*sf))
	c.gap(int(8 * sf))
	header(c, b, sf)
	c.rule(true, 2, int(14*sf), int(14*sf))

	c.fitLine("YAZICI TESTİ", bold, 38*sf, center, int(4*sf), int(6*sf))
	c.line("Türkçe karakterler doğru çıkmalı:", face(regular, 22*sf), center, 0, int(4*sf))
	c.fitLine("ÇĞİÖŞÜ çğıöşü ₺", semibold, 40*sf, center, 0, int(6*sf))
	c.rule(true, 2, int(8*sf), int(10*sf))

	f := face(regular, 23*sf)
	c.row("Bağlantı", f, t.Connection, f, 0, int(2*sf))
	c.row("Genişlik", f, fmt.Sprintf("%d nokta (%d mm)", t.Width, t.Width*254/2030), f, 0, int(2*sf))
	c.row("Kesme", f, t.CutText, f, 0, int(2*sf))
	c.row("Tarih", f, dateTR(t.When)+" "+t.When.Format("15:04"), f, 0, 0)
	c.rule(true, 2, int(10*sf), int(12*sf))

	// Kenar kontrolü: kâğıdın iki ucuna kadar çerçeve ve santim çizgileri.
	c.line("KENAR KONTROLÜ", face(bold, 20*sf), center, int(3*sf), int(6*sf))
	boxH := int(70 * sf)
	c.grow(boxH)
	y0 := c.y
	c.fillRect(0, y0, c.w, y0+3)
	c.fillRect(0, y0+boxH-3, c.w, y0+boxH)
	c.fillRect(0, y0, 3, y0+boxH)
	c.fillRect(c.w-3, y0, c.w, y0+boxH)
	cm := 80 // 203 dpi'da yaklaşık 10 mm
	tickF := face(semibold, 20*sf)
	for i := 1; i*cm < c.w-4; i++ {
		x := i * cm
		c.fillRect(x-1, y0, x+1, y0+boxH/3)
		label := strconv.Itoa(i)
		w := textWidth(tickF, label, 0)
		asc, _ := metrics(tickF)
		c.drawText(label, tickF, x-w/2, y0+boxH/3+asc+2, 0)
	}
	c.y += boxH
	c.gap(int(6 * sf))
	c.para("Çerçevenin sol ve sağ kenarı görünüyorsa genişlik doğru.", face(regular, 22*sf), center, int(2*sf))
	c.gap(int(8 * sf))
	c.grow(int(26 * sf))
	c.fillRect(c.margin, c.y, c.w-c.margin, c.y+int(26*sf))
	c.y += int(26 * sf)
	c.gap(int(4 * sf))
	c.para("Siyah şerit koyu ve düzgün olmalı.", face(regular, 22*sf), center, int(2*sf))
	c.rule(true, 2, int(12*sf), int(12*sf))

	qrData := strings.TrimSpace(b.GoogleURL)
	caption := "Telefon kamerasıyla okutun: Google yorum sayfası açılmalı."
	if qrData == "" {
		qrData = "HALIS ADISYON TEST"
		caption = "Telefon kamerasıyla okutun: “HALIS ADISYON TEST” yazısı görünmeli."
	}
	if err := c.qr(qrData, int(240*sf)); err == nil {
		c.gap(int(10 * sf))
		c.para(caption, face(regular, 22*sf), center, int(2*sf))
	}
	c.rule(true, 2, int(14*sf), int(12*sf))
	c.fitLine("TEST FİŞİDİR · MALİ DEĞERİ YOKTUR", bold, 20*sf, center, int(2*sf), 0)
	return c.finish(int(24 * sf)), nil
}

// PNG fiş resmini PNG olarak kodlar (önizleme için).
func PNG(img *image.Gray) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
