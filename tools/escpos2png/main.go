// escpos2png yazıcıya giden ESC/POS dosyasını PNG resmine çevirir (kontrol aracı).
//
//	go run ./tools/escpos2png job.bin cikti.png
package main

import (
	"fmt"
	"image/png"
	"os"

	"halisadisyon/internal/escpos"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "kullanım: escpos2png girdi.bin cikti.png")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	d, err := escpos.Decode(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "çözülemedi:", err)
		os.Exit(1)
	}
	f, err := os.Create(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, d.Image); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cut := "yok"
	if d.Cut {
		cut = "kısmi"
		if d.FullCut {
			cut = "tam"
		}
	}
	fmt.Printf("%dx%d nokta, sıfırlama=%v, kesme=%s\n", d.Image.Bounds().Dx(), d.Image.Bounds().Dy(), d.Reset, cut)
}
