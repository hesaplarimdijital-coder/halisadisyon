//go:build !windows

package printing

import "time"

func sendWindows(cfg Config, data []byte, docName string, wait time.Duration) Result {
	return fail("not_found", titleFailed,
		"Windows yazıcısı yalnızca Windows bilgisayarda kullanılabilir.",
		"Ayarlar › Yazıcı bölümünden ağ yazıcısı ya da önizleme seçin.", "not windows")
}

func listPrinters() ([]PrinterInfo, string) { return nil, "" }
