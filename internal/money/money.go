// Package money kuruş cinsinden tam sayı tutarlarla çalışır.
// Tüm tutarlar KDV dahildir; KDV, dahil tutardan hesaplanır:
//
//	KDV = tutar × oran ÷ (100 + oran)   (yarım kuruş yukarı yuvarlanır)
package money

import (
	"strconv"
	"strings"
)

// VAT, KDV dahil brüt tutarın (kuruş) içindeki KDV'yi verir.
// Örnek: VAT(239000, 10) = 21727 → 217,27 ₺
func VAT(gross int64, rate int) int64 {
	if gross <= 0 || rate <= 0 {
		return 0
	}
	r := int64(rate)
	return (gross*r*2 + (100 + r)) / (2 * (100 + r))
}

// Format kuruşu Türkçe biçimde yazar: 239000 → "2.390,00".
func Format(k int64) string {
	neg := k < 0
	if neg {
		k = -k
	}
	lira := strconv.FormatInt(k/100, 10)
	kurus := k % 100

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	first := len(lira) % 3
	if first == 0 {
		first = 3
	}
	b.WriteString(lira[:first])
	for i := first; i < len(lira); i += 3 {
		b.WriteByte('.')
		b.WriteString(lira[i : i+3])
	}
	b.WriteByte(',')
	if kurus < 10 {
		b.WriteByte('0')
	}
	b.WriteString(strconv.FormatInt(kurus, 10))
	return b.String()
}

// TL, tutarı lira işaretiyle yazar: "2.390,00 ₺".
func TL(k int64) string { return Format(k) + " ₺" }
