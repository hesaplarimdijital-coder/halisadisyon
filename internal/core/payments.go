package core

// Ödeme şekilleri. Veritabanında kısa kod, ekranda ve raporda Türkçe ad kullanılır.
const (
	PayCash     = "nakit"
	PayCard     = "kart"
	PayTransfer = "havale"
)

// PaymentMethod ödeme şeklidir (arayüz için).
type PaymentMethod struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// PaymentMethods ekranda gösterilen sırayla ödeme şekilleridir.
var PaymentMethods = []PaymentMethod{
	{PayCash, "Nakit"},
	{PayCard, "Kredi Kartı"},
	{PayTransfer, "Havale"},
}

func validPayment(m string) bool {
	for _, p := range PaymentMethods {
		if p.ID == m {
			return true
		}
	}
	return false
}

// PaymentLabel ödeme şeklinin Türkçe adını verir ("" → "").
func PaymentLabel(m string) string {
	for _, p := range PaymentMethods {
		if p.ID == m {
			return p.Label
		}
	}
	if m == "" {
		return ""
	}
	return "Bilinmiyor"
}
