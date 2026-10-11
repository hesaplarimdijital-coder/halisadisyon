package receipt

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sampleBusiness() Business {
	return Business{
		Name:      "Halis Erzurum Cağ Kebabı",
		Slogan:    "Odun ateşinde, yatay şişte",
		Address1:  "Merkez: Akdeniz Sanayi 5009 Sk.",
		Address2:  "Şube: Uncalı Mh. Çamlık Cd.",
		Phone:     "0242 000 00 00",
		Instagram: "@halis_cag",
		Invite:    "Afiyet olsun! Memnun kaldıysanız Google’daki yorumunuz bizim için çok değerli.",
		GoogleURL: "https://g.page/r/CbExampleReview/review",
	}
}

func sampleAdisyon() Adisyon {
	lines := []Line{
		{Name: "Cağ", Unit: "şiş", Qty: 6, Price: 34000},
		{Name: "Ayran", Unit: "adet", Qty: 3, Price: 5000},
		{Name: "Büyük Su", Unit: "adet", Qty: 1, Price: 5000},
		{Name: "Çok uzun adlı bir ürün: ızgara köz biber ve domates", Unit: "porsiyon", Qty: 2, Price: 12000},
	}
	var total int64
	for _, l := range lines {
		total += l.Price * int64(l.Qty)
	}
	return Adisyon{
		No: 1234, Table: 17, Waiter: "Garson 2", When: time.Date(2026, 10, 11, 14, 5, 0, 0, time.Local),
		Lines: lines, Total: total, VATs: []VATGroup{{Rate: 10, Gross: total, VAT: 25000}},
	}
}

func save(t *testing.T, name string, img *image.Gray) {
	dir := os.Getenv("RECEIPT_OUT")
	if dir == "" {
		return
	}
	b, err := PNG(img)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func blackShare(img *image.Gray) float64 {
	n := 0
	for _, v := range img.Pix {
		if v == 0 {
			n++
		}
	}
	return float64(n) / float64(len(img.Pix))
}

func TestRenderAdisyonWidths(t *testing.T) {
	for _, w := range []int{576, 512, 384} {
		a := sampleAdisyon()
		a.Copy = w == 512
		img, err := RenderAdisyon(sampleBusiness(), a, w)
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != w {
			t.Fatalf("genişlik %d, beklenen %d", img.Bounds().Dx(), w)
		}
		h := img.Bounds().Dy()
		if h < 600 || h > 4000 {
			t.Fatalf("yükseklik makul değil: %d", h)
		}
		for _, v := range img.Pix {
			if v != 0 && v != 255 {
				t.Fatal("resim tam siyah-beyaz değil")
			}
		}
		if s := blackShare(img); s < 0.02 || s > 0.5 {
			t.Fatalf("siyah oranı tuhaf: %.3f", s)
		}
		save(t, "adisyon_"+itoa(w)+".png", img)
	}
}

func TestRenderTestReceipt(t *testing.T) {
	img, err := RenderTest(sampleBusiness(), TestInfo{Connection: "Windows: KPS80", Width: 576, CutText: "kısmi", When: time.Now()}, 576)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 576 {
		t.Fatal("genişlik yanlış")
	}
	save(t, "test_576.png", img)
	b := sampleBusiness()
	b.GoogleURL = ""
	img, err = RenderTest(b, TestInfo{Connection: "Önizleme", Width: 384, CutText: "kapalı", When: time.Now()}, 384)
	if err != nil {
		t.Fatal(err)
	}
	save(t, "test_384.png", img)
}

func TestWrapAndUpper(t *testing.T) {
	if trUpper("Halis Erzurum Cağ Kebabı") != "HALİS ERZURUM CAĞ KEBABI" {
		t.Fatalf("Türkçe büyük harf yanlış: %s", trUpper("Halis Erzurum Cağ Kebabı"))
	}
	if err := loadFonts(); err != nil {
		t.Fatal(err)
	}
	f := face(regular, 24)
	lines := wrap("Afiyet olsun! Memnun kaldıysanız Google’daki yorumunuz bizim için çok değerli.", f, 300)
	if len(lines) < 2 {
		t.Fatalf("satır sarma çalışmadı: %v", lines)
	}
	for _, l := range lines {
		if textWidth(f, l, 0) > 300 {
			t.Fatalf("satır taşıyor: %q", l)
		}
	}
	long := wrap("Çokçokçokçokçokçokçokçokçokçokçokçokuzunkelime", f, 120)
	for _, l := range long {
		if textWidth(f, l, 0) > 120 {
			t.Fatalf("uzun kelime bölünmedi: %q", l)
		}
	}
}

func itoa(i int) string {
	return string(rune('0'+i/100%10)) + string(rune('0'+i/10%10)) + string(rune('0'+i%10))
}

// Sağdaki yazı çok uzunsa soldakinin üstüne binmemeli, alt satıra inmeli.
func TestRowLongRightTextDoesNotOverlap(t *testing.T) {
	if err := loadFonts(); err != nil {
		t.Fatal(err)
	}
	renderMu.Lock()
	defer renderMu.Unlock()
	f := face(regular, 24)
	asc, desc := metrics(f)
	c := newCanvas(576, 12)
	start := c.y
	c.row("Bağlantı", f, "Windows: KODDATA KPS80 Termal Fiş Yazıcısı (USB001) kopya 2", f, 0, 0)
	if c.y-start < 2*(asc+desc) {
		t.Fatal("uzun sağ yazı alt satıra inmedi")
	}
	c2 := newCanvas(576, 12)
	s2 := c2.y
	c2.row("Bağlantı", f, "Windows: KPS80", f, 0, 0)
	if c2.y-s2 >= 2*(asc+desc) {
		t.Fatal("kısa sağ yazı gereksiz yere alt satıra indi")
	}
}
