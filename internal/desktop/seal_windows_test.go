//go:build windows

package desktop

import (
	"bytes"
	"testing"
)

func TestSealRoundTrip(t *testing.T) {
	plain := []byte("EAAG-deneme-anahtari-çğış")
	sealed, err := Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, plain) {
		t.Fatal("şifreli veri düz metni içeriyor")
	}
	back, err := Unseal(sealed)
	if err != nil || !bytes.Equal(back, plain) {
		t.Fatalf("çözülemedi: %v %q", err, back)
	}
}
