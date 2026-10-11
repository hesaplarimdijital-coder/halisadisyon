// Package escpos fiş resmini ESC/POS komutlarına çevirir.
//
// Fiş, yazıcının kendi yazı tipleri yerine resim (raster) olarak basılır;
// böylece Türkçe karakterler, logo ve QR her yazıcıda aynı görünür.
package escpos

import (
	"errors"
	"image"
)

const (
	bandRows = 256 // tek komutta gönderilen en fazla satır

	// SyncBytes her işin başına konan boş (NUL) bayt sayısıdır. Önceki bir iş
	// yarıda kesildiyse (kâğıt bitti, iş silindi) yazıcı yarım kalan resim bandının
	// geri kalanını bekler; bu baytlar o bandı boş satırla tamamlar, artanları
	// yazıcı yok sayar. Böylece sonraki fiş bozuk çıkmaz. En geniş kâğıt (72 bayt)
	// ve en uzun bant için yeterlidir.
	SyncBytes = 72 * bandRows
)

// Options baskı sonu davranışıdır.
type Options struct {
	Cut     bool
	FullCut bool // false: kısmi kesme
}

// Encode siyah-beyaz fiş resmini (0 = siyah, 255 = beyaz) ESC/POS baytlarına çevirir.
// Resim genişliği 8'in katı olmalıdır.
func Encode(img *image.Gray, opt Options) ([]byte, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, errors.New("boş resim")
	}
	if w%8 != 0 {
		return nil, errors.New("resim genişliği 8'in katı olmalı")
	}
	if w > 72*8 {
		return nil, errors.New("resim yazıcı için fazla geniş")
	}
	bpr := w / 8
	out := make([]byte, SyncBytes, SyncBytes+16+h*bpr+(h/bandRows+1)*8+16)
	out = append(out, 0x1B, 0x40) // ESC @ : yazıcıyı sıfırla
	for y0 := 0; y0 < h; y0 += bandRows {
		bh := bandRows
		if y0+bh > h {
			bh = h - y0
		}
		// GS v 0 m xL xH yL yH d1...dk : raster bit resim
		out = append(out, 0x1D, 0x76, 0x30, 0x00, byte(bpr), byte(bpr>>8), byte(bh), byte(bh>>8))
		for y := y0; y < y0+bh; y++ {
			row := img.Pix[(y)*img.Stride : (y)*img.Stride+w]
			for x := 0; x < w; x += 8 {
				var v byte
				for bit := 0; bit < 8; bit++ {
					if row[x+bit] < 128 {
						v |= 0x80 >> bit
					}
				}
				out = append(out, v)
			}
		}
	}
	if opt.Cut {
		m := byte(66) // GS V B n: kesme konumuna kadar ilerle, kısmi kes
		if opt.FullCut {
			m = 65 // GS V A n: tam kes
		}
		out = append(out, 0x1D, 0x56, m, 0x00)
	} else {
		out = append(out, 0x1B, 0x64, 0x05) // ESC d 5: 5 satır ilerle (elle koparmak için)
	}
	return out, nil
}

// Decoded, Encode çıktısının geri çözülmüş halidir (testler ve kontrol için).
type Decoded struct {
	Image   *image.Gray
	Cut     bool
	FullCut bool
	Reset   bool
}

// Decode Encode'un ürettiği bayt dizisini resme geri çevirir.
func Decode(data []byte) (*Decoded, error) {
	d := &Decoded{}
	var rows [][]byte
	width := 0
	i := 0
	for i < len(data) {
		switch {
		case data[i] == 0x00: // eşitleme baytları
			i++
		case i+2 < len(data) && data[i] == 0x10 && data[i+1] == 0x04: // DLE EOT n: durum sorusu
			i += 3
		case i+1 < len(data) && data[i] == 0x1B && data[i+1] == 0x40:
			d.Reset = true
			i += 2
		case i+7 < len(data) && data[i] == 0x1D && data[i+1] == 0x76 && data[i+2] == 0x30:
			bpr := int(data[i+4]) | int(data[i+5])<<8
			bh := int(data[i+6]) | int(data[i+7])<<8
			i += 8
			if width == 0 {
				width = bpr * 8
			} else if width != bpr*8 {
				return nil, errors.New("bantların genişliği farklı")
			}
			need := bpr * bh
			if i+need > len(data) {
				return nil, errors.New("eksik raster verisi")
			}
			for r := 0; r < bh; r++ {
				rows = append(rows, data[i+r*bpr:i+(r+1)*bpr])
			}
			i += need
		case i+3 < len(data) && data[i] == 0x1D && data[i+1] == 0x56:
			d.Cut = true
			d.FullCut = data[i+2] == 65 || data[i+2] == 0 || data[i+2] == 48
			i += 4
		case i+2 < len(data) && data[i] == 0x1B && data[i+1] == 0x64:
			i += 3
		default:
			return nil, errors.New("tanınmayan komut")
		}
	}
	if width == 0 {
		return nil, errors.New("resim yok")
	}
	img := image.NewGray(image.Rect(0, 0, width, len(rows)))
	for y, row := range rows {
		for x := 0; x < width; x++ {
			if row[x/8]&(0x80>>(x%8)) != 0 {
				img.Pix[y*img.Stride+x] = 0
			} else {
				img.Pix[y*img.Stride+x] = 255
			}
		}
	}
	d.Image = img
	return d, nil
}
