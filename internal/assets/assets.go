// Package assets programın içine gömülü dosyaları (yazı tipleri) tutar.
// Aynı yazı tipleri hem ekranda (arayüz) hem fişte kullanılır.
package assets

import "embed"

//go:embed fonts/*.ttf fonts/OFL-IBM-Plex.txt
var Fonts embed.FS

const (
	FontRegular  = "fonts/IBMPlexSans-Regular.ttf"
	FontItalic   = "fonts/IBMPlexSans-Italic.ttf"
	FontSemiBold = "fonts/IBMPlexSans-SemiBold.ttf"
	FontBold     = "fonts/IBMPlexSans-Bold.ttf"
)
