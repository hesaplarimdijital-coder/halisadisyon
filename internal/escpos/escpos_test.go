package escpos

import (
	"bytes"
	"image"
	"math/rand"
	"testing"
)

func randomImage(w, h int, seed int64) *image.Gray {
	r := rand.New(rand.NewSource(seed))
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		if r.Intn(3) == 0 {
			img.Pix[i] = 0
		} else {
			img.Pix[i] = 255
		}
	}
	return img
}

func TestRoundTripMultiBand(t *testing.T) {
	img := randomImage(576, 2100, 1) // birden çok banda bölünür
	data, err := Encode(img, Options{Cut: true})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data[:SyncBytes], make([]byte, SyncBytes)) {
		t.Fatal("eşitleme baytları yok")
	}
	if !bytes.HasPrefix(data[SyncBytes:], []byte{0x1B, 0x40}) {
		t.Fatal("eşitlemeden sonra ESC @ gelmiyor")
	}
	if !bytes.HasSuffix(data, []byte{0x1D, 0x56, 66, 0}) {
		t.Fatal("kısmi kesme komutu yok")
	}
	if n := bytes.Count(data, []byte{0x1D, 0x76, 0x30, 0x00}); n < 3 {
		t.Fatalf("bant sayısı %d, en az 3 olmalı", n)
	}
	d, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Reset || !d.Cut || d.FullCut {
		t.Fatalf("komutlar yanlış: %+v", d)
	}
	if !bytes.Equal(d.Image.Pix, img.Pix) {
		t.Fatal("geri çözülen resim farklı")
	}
}

func TestFullCutAndNoCut(t *testing.T) {
	img := randomImage(384, 50, 2)
	data, _ := Encode(img, Options{Cut: true, FullCut: true})
	if !bytes.HasSuffix(data, []byte{0x1D, 0x56, 65, 0}) {
		t.Fatal("tam kesme komutu yok")
	}
	data, _ = Encode(img, Options{Cut: false})
	if !bytes.HasSuffix(data, []byte{0x1B, 0x64, 0x05}) {
		t.Fatal("kesmesiz ilerleme komutu yok")
	}
	if bytes.Contains(data, []byte{0x1D, 0x56}) {
		t.Fatal("kesme kapalıyken kesme komutu var")
	}
}

func TestRejectsBadWidth(t *testing.T) {
	if _, err := Encode(image.NewGray(image.Rect(0, 0, 570, 10)), Options{}); err == nil {
		t.Fatal("8'in katı olmayan genişlik kabul edildi")
	}
}

// printerSim basit bir ESC/POS yazıcısıdır: komut kipinde tanımadığı baytları
// "çöp yazı" sayar, raster kipinde beklenen bayt sayısı dolana kadar satır toplar.
type printerSim struct {
	rows    [][]byte // son ESC @'tan beri basılan satırlar
	garbage int      // komut sanılıp yazı olarak basılan bayt sayısı
	cuts    int
}

func (p *printerSim) feed(data []byte) {
	i := 0
	for i < len(data) {
		switch {
		case data[i] == 0x00:
			i++
		case i+1 < len(data) && data[i] == 0x1B && data[i+1] == 0x40:
			p.rows = nil
			i += 2
		case i+7 < len(data) && data[i] == 0x1D && data[i+1] == 0x76 && data[i+2] == 0x30:
			bpr := int(data[i+4]) | int(data[i+5])<<8
			bh := int(data[i+6]) | int(data[i+7])<<8
			i += 8
			for r := 0; r < bh; r++ {
				row := make([]byte, bpr)
				for x := 0; x < bpr; x++ {
					if i < len(data) {
						row[x] = data[i]
						i++
					}
				}
				p.rows = append(p.rows, row)
			}
		case i+3 < len(data) && data[i] == 0x1D && data[i+1] == 0x56:
			p.cuts++
			i += 4
		case i+2 < len(data) && data[i] == 0x1B && data[i+1] == 0x64:
			i += 3
		default:
			p.garbage++
			i++
		}
	}
}

// Kâğıt bitip iş yarıda silinse bile sonraki fiş eksiksiz çıkmalı.
func TestInterruptedJobDoesNotCorruptNext(t *testing.T) {
	first, _ := Encode(randomImage(576, 1500, 3), Options{Cut: true})
	secondImg := randomImage(576, 700, 4)
	second, _ := Encode(secondImg, Options{Cut: true})
	want, _ := Decode(second)
	r := rand.New(rand.NewSource(5))
	for n := 0; n < 200; n++ {
		cut := SyncBytes + 2 + r.Intn(len(first)-SyncBytes-10)
		stream := append(append([]byte{}, first[:cut]...), second...)
		p := &printerSim{}
		p.feed(stream)
		if p.garbage != 0 {
			t.Fatalf("kesim %d: %d bayt çöp yazı basıldı", cut, p.garbage)
		}
		if p.cuts != 1 {
			t.Fatalf("kesim %d: %d kez kesildi", cut, p.cuts)
		}
		if len(p.rows) != secondImg.Rect.Dy() {
			t.Fatalf("kesim %d: ikinci fiş %d satır, %d olmalı", cut, len(p.rows), secondImg.Rect.Dy())
		}
		got := image.NewGray(secondImg.Rect)
		for y, row := range p.rows {
			for x := 0; x < 576; x++ {
				if row[x/8]&(0x80>>(x%8)) != 0 {
					got.Pix[y*got.Stride+x] = 0
				} else {
					got.Pix[y*got.Stride+x] = 255
				}
			}
		}
		if !bytes.Equal(got.Pix, want.Image.Pix) {
			t.Fatalf("kesim %d: ikinci fiş bozuk çıktı", cut)
		}
	}
}
