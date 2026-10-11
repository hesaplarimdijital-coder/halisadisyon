// Package receipt fişi 80 mm termal kâğıda uygun siyah-beyaz bir resim olarak çizer.
// Aynı resim hem ekranda önizlenir hem yazıcıya gönderilir; önizleme ile baskı aynıdır.
package receipt

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/skip2/go-qrcode"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"halisadisyon/internal/assets"
)

type style int

const (
	regular style = iota
	italic
	semibold
	bold
)

type align int

const (
	left align = iota
	center
	right
)

var (
	renderMu  sync.Mutex // yazı tipi yüzleri aynı anda tek iş parçacığından kullanılmalı
	fontsOnce sync.Once
	fontsErr  error
	fontFiles = map[style]*opentype.Font{}
	faceCache = map[faceKey]font.Face{}
)

type faceKey struct {
	s    style
	size float64
}

func loadFonts() error {
	fontsOnce.Do(func() {
		paths := map[style]string{
			regular:  assets.FontRegular,
			italic:   assets.FontItalic,
			semibold: assets.FontSemiBold,
			bold:     assets.FontBold,
		}
		for s, p := range paths {
			b, err := assets.Fonts.ReadFile(p)
			if err != nil {
				fontsErr = err
				return
			}
			f, err := opentype.Parse(b)
			if err != nil {
				fontsErr = fmt.Errorf("%s: %w", p, err)
				return
			}
			fontFiles[s] = f
		}
	})
	return fontsErr
}

func face(s style, size float64) font.Face {
	size = float64(int(size*2+0.5)) / 2 // önbellek için yarım piksele yuvarla
	k := faceKey{s, size}
	if f, ok := faceCache[k]; ok {
		return f
	}
	f, err := opentype.NewFace(fontFiles[s], &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err) // gömülü yazı tipleri bozuksa program zaten çalışamaz
	}
	faceCache[k] = f
	return f
}

// canvas yukarıdan aşağı çizilen uzun bir fiş tuvalidir.
type canvas struct {
	img    *image.Gray
	w      int
	margin int
	y      int
}

func newCanvas(w, margin int) *canvas {
	c := &canvas{w: w, margin: margin}
	c.img = image.NewGray(image.Rect(0, 0, w, 2400))
	fillWhite(c.img)
	return c
}

func fillWhite(img *image.Gray) {
	for i := range img.Pix {
		img.Pix[i] = 255
	}
}

func (c *canvas) contentW() int { return c.w - 2*c.margin }

func (c *canvas) grow(need int) {
	h := c.img.Bounds().Dy()
	if c.y+need <= h {
		return
	}
	nh := h * 2
	if nh < c.y+need+400 {
		nh = c.y + need + 400
	}
	n := image.NewGray(image.Rect(0, 0, c.w, nh))
	fillWhite(n)
	copy(n.Pix, c.img.Pix)
	c.img = n
}

func (c *canvas) set(x, y int) {
	if x >= 0 && x < c.w && y >= 0 && y < c.img.Bounds().Dy() {
		c.img.Pix[y*c.img.Stride+x] = 0
	}
}

func (c *canvas) fillRect(x0, y0, x1, y1 int) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c.set(x, y)
		}
	}
}

func metrics(f font.Face) (asc, desc int) {
	m := f.Metrics()
	return m.Ascent.Ceil(), m.Descent.Ceil()
}

func textWidth(f font.Face, s string, spacing int) int {
	if spacing == 0 {
		return font.MeasureString(f, s).Ceil()
	}
	var w fixed.Int26_6
	n := 0
	for _, r := range s {
		adv, _ := f.GlyphAdvance(r)
		w += adv
		n++
	}
	if n == 0 {
		return 0
	}
	return w.Ceil() + spacing*(n-1)
}

func (c *canvas) drawText(s string, f font.Face, x, baseline, spacing int) {
	d := &font.Drawer{Dst: c.img, Src: image.Black, Face: f, Dot: fixed.P(x, baseline)}
	if spacing == 0 {
		d.DrawString(s)
		return
	}
	first := true
	for _, r := range s {
		if !first {
			d.Dot.X += fixed.I(spacing)
		}
		d.DrawString(string(r))
		first = false
	}
}

// fit yazı tek satıra sığmazsa boyutu küçültür.
func fit(s string, st style, size float64, spacing, maxW int) (font.Face, int) {
	min := size * 0.6
	for size > min {
		f := face(st, size)
		if textWidth(f, s, spacing) <= maxW {
			return f, spacing
		}
		size *= 0.94
		if spacing > 0 {
			spacing = spacing * 9 / 10
		}
	}
	return face(st, min), 0
}

// line tek satır yazı çizer.
func (c *canvas) line(s string, f font.Face, al align, spacing, lead int) {
	asc, desc := metrics(f)
	c.grow(asc + desc + lead)
	w := textWidth(f, s, spacing)
	x := c.margin
	switch al {
	case center:
		x = (c.w - w) / 2
	case right:
		x = c.w - c.margin - w
	}
	if x < 0 {
		x = 0
	}
	c.drawText(s, f, x, c.y+asc, spacing)
	c.y += asc + desc + lead
}

// fitLine tek satıra sığacak biçimde (gerekirse küçülterek) yazar.
func (c *canvas) fitLine(s string, st style, size float64, al align, spacing, lead int) {
	f, sp := fit(s, st, size, spacing, c.contentW())
	c.line(s, f, al, sp, lead)
}

// para yazıyı sarar ve satır satır çizer.
func (c *canvas) para(s string, f font.Face, al align, lead int) {
	for _, ln := range wrap(s, f, c.contentW()) {
		c.line(ln, f, al, 0, lead)
	}
}

// row sola yaslı metin ile sağa yaslı tutarı aynı satıra yazar; sol metin uzunsa alt satıra kayar.
func (c *canvas) row(l string, fl font.Face, r string, fr font.Face, spacing, lead int) {
	rw := textWidth(fr, r, spacing)
	if rw > c.contentW()*2/3 {
		// Sağdaki yazı satırı paylaşamayacak kadar uzun: üst üste binmesin, alt alta yaz.
		for _, ln := range wrap(l, fl, c.contentW()) {
			c.line(ln, fl, left, spacing, 0)
		}
		rl := wrap(r, fr, c.contentW())
		for i, ln := range rl {
			ld := 0
			if i == len(rl)-1 {
				ld = lead
			}
			c.line(ln, fr, right, spacing, ld)
		}
		return
	}
	avail := c.contentW() - rw - 18
	if avail < c.contentW()/3 {
		avail = c.contentW() / 3
	}
	lines := wrap(l, fl, avail)
	if len(lines) == 0 {
		lines = []string{""}
	}
	ascL, descL := metrics(fl)
	ascR, descR := metrics(fr)
	asc, desc := ascL, descL
	if ascR > asc {
		asc = ascR
	}
	if descR > desc {
		desc = descR
	}
	c.grow(asc + desc + lead)
	base := c.y + asc
	c.drawText(lines[0], fl, c.margin, base, spacing)
	c.drawText(r, fr, c.w-c.margin-rw, base, spacing)
	c.y = base + desc + lead
	for _, ln := range lines[1:] {
		c.line(ln, fl, left, spacing, lead)
	}
}

func (c *canvas) gap(px int) {
	if px <= 0 {
		return
	}
	c.grow(px)
	c.y += px
}

// rule yatay çizgi çizer (kesikli ya da düz).
func (c *canvas) rule(dashed bool, thick, padTop, padBottom int) {
	c.gap(padTop)
	c.grow(thick)
	for t := 0; t < thick; t++ {
		for x := c.margin; x < c.w-c.margin; x++ {
			if !dashed || (x-c.margin)%14 < 8 {
				c.set(x, c.y+t)
			}
		}
	}
	c.y += thick
	c.gap(padBottom)
}

// boxed çerçeveli, ortalanmış bir etiket çizer (örneğin KOPYA).
func (c *canvas) boxed(s string, f font.Face, spacing, border int) {
	asc, desc := metrics(f)
	pad := 6
	h := asc + desc + 2*pad
	c.grow(h)
	x0, x1 := c.margin, c.w-c.margin
	c.fillRect(x0, c.y, x1, c.y+border)
	c.fillRect(x0, c.y+h-border, x1, c.y+h)
	c.fillRect(x0, c.y, x0+border, c.y+h)
	c.fillRect(x1-border, c.y, x1, c.y+h)
	w := textWidth(f, s, spacing)
	c.drawText(s, f, (c.w-w)/2, c.y+pad+asc, spacing)
	c.y += h
}

// qr bağlantının QR kodunu ortalayarak çizer (hata düzeltme seviyesi M).
func (c *canvas) qr(data string, size int) error {
	q, err := qrcode.New(data, qrcode.Medium)
	if err != nil {
		return err
	}
	q.DisableBorder = true
	bm := q.Bitmap()
	n := len(bm)
	if n == 0 {
		return fmt.Errorf("boş QR")
	}
	mod := size / n
	if mod < 3 {
		mod = 3
	}
	total := n * mod
	if total > c.contentW() {
		mod = c.contentW() / n
		total = n * mod
	}
	x0 := (c.w - total) / 2
	c.grow(total)
	for y, rowBits := range bm {
		for x, black := range rowBits {
			if black {
				c.fillRect(x0+x*mod, c.y+y*mod, x0+(x+1)*mod, c.y+(y+1)*mod)
			}
		}
	}
	c.y += total
	return nil
}

// picture logoyu beyaz zemin üzerine siyah-beyaz olarak ortalar.
func (c *canvas) picture(src image.Image, maxW, maxH int) {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return
	}
	scale := 1.0
	if float64(maxW)/float64(sw) < scale {
		scale = float64(maxW) / float64(sw)
	}
	if float64(maxH)/float64(sh) < scale {
		scale = float64(maxH) / float64(sh)
	}
	dw, dh := int(float64(sw)*scale+0.5), int(float64(sh)*scale+0.5)
	if dw < 1 || dh < 1 {
		return
	}
	rgba := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.Draw(rgba, rgba.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(rgba, rgba.Bounds(), src, b, draw.Over, nil)
	x0 := (c.w - dw) / 2
	c.grow(dh)
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			g := color.GrayModel.Convert(rgba.At(x, y)).(color.Gray)
			if g.Y < 140 {
				c.set(x0+x, c.y+y)
			}
		}
	}
	c.y += dh
}

// finish fişi gerçek yüksekliğine kırpar ve keskin siyah-beyaza çevirir.
func (c *canvas) finish(bottom int) *image.Gray {
	c.gap(bottom)
	h := c.y
	out := image.NewGray(image.Rect(0, 0, c.w, h))
	for y := 0; y < h; y++ {
		src := c.img.Pix[y*c.img.Stride : y*c.img.Stride+c.w]
		dst := out.Pix[y*out.Stride : y*out.Stride+c.w]
		for x, v := range src {
			if v < 150 {
				dst[x] = 0
			} else {
				dst[x] = 255
			}
		}
	}
	return out
}

// wrap yazıyı verilen genişliğe göre satırlara böler.
func wrap(s string, f font.Face, maxW int) []string {
	var out []string
	for _, para := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		words := strings.Fields(para)
		cur := ""
		for _, w := range words {
			cand := w
			if cur != "" {
				cand = cur + " " + w
			}
			if textWidth(f, cand, 0) <= maxW {
				cur = cand
				continue
			}
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			for textWidth(f, w, 0) > maxW {
				cut := fitPrefix(w, f, maxW)
				out = append(out, w[:cut])
				w = w[cut:]
			}
			cur = w
		}
		if cur != "" {
			out = append(out, cur)
		}
	}
	return out
}

func fitPrefix(s string, f font.Face, maxW int) int {
	last := 0
	for i := range s {
		if i == 0 {
			continue
		}
		if textWidth(f, s[:i], 0) > maxW {
			break
		}
		last = i
	}
	if last == 0 {
		_, size := utf8.DecodeRuneInString(s)
		return size
	}
	return last
}
