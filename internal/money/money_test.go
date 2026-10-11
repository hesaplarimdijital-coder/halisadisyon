package money

import "testing"

func TestVATExampleFromPlan(t *testing.T) {
	// Planlama dokümanındaki örnek: 2.390 ₺ ve %10 → KDV 217,27 ₺, matrah 2.172,73 ₺
	gross := int64(239000)
	vat := VAT(gross, 10)
	if vat != 21727 {
		t.Fatalf("KDV = %d, beklenen 21727", vat)
	}
	if gross-vat != 217273 {
		t.Fatalf("matrah = %d, beklenen 217273", gross-vat)
	}
}

func TestVATRates(t *testing.T) {
	cases := []struct {
		gross int64
		rate  int
		want  int64
	}{
		{0, 10, 0},
		{100, 0, 0},
		{11000, 10, 1000},
		{12000, 20, 2000},
		{10100, 1, 100},
		{3, 20, 1},        // 0,5 kuruş yukarı yuvarlanır
		{34000, 10, 3091}, // 3090,909… → 3091
	}
	for _, c := range cases {
		if got := VAT(c.gross, c.rate); got != c.want {
			t.Errorf("VAT(%d,%d)=%d, beklenen %d", c.gross, c.rate, got, c.want)
		}
	}
}

func TestFormat(t *testing.T) {
	cases := map[int64]string{
		0:         "0,00",
		5:         "0,05",
		250:       "2,50",
		34000:     "340,00",
		239000:    "2.390,00",
		100000000: "1.000.000,00",
		123456789: "1.234.567,89",
		-239050:   "-2.390,50",
	}
	for k, want := range cases {
		if got := Format(k); got != want {
			t.Errorf("Format(%d)=%q, beklenen %q", k, got, want)
		}
	}
	if TL(239000) != "2.390,00 ₺" {
		t.Errorf("TL biçimi yanlış: %q", TL(239000))
	}
}
