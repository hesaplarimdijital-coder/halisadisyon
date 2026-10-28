package core

import (
	"strings"
	"testing"
)

func (e *env) payParts(no int64, credit *CreditRef, parts ...PaymentPart) (*PayOutcome, error) {
	out, _, err := e.svc.PayParts(no, parts, credit, -1)
	return out, err
}

// Parçalı ödeme: parçaların toplamı hesaba eşit olmalı; gün sonunda her parça
// kendi ödeme şekline sayılır, adisyon bir kez sayılır.
func TestSplitPayment(t *testing.T) {
	e := newEnv(t)
	e.setup()
	no := e.add(1, "Cağ")
	e.add(1, "Cağ")
	e.add(1, "Ayran") // 730,00
	if _, err := e.payParts(no, nil, PaymentPart{Method: PayCash, Amount: 50000}, PaymentPart{Method: PayCard, Amount: 20000}); code(err) != "invalid" || !strings.Contains(err.Error(), "30,00") {
		t.Fatalf("eksik parça kabul edildi: %v", err)
	}
	if _, err := e.payParts(no, nil, PaymentPart{Method: PayCash, Amount: 50000}, PaymentPart{Method: PayCard, Amount: 30000}); code(err) != "invalid" || !strings.Contains(err.Error(), "fazla") {
		t.Fatalf("fazla parça kabul edildi: %v", err)
	}
	out, err := e.payParts(no, nil, PaymentPart{Method: PayCard, Amount: 23000}, PaymentPart{Method: PayCash, Amount: 50000}, PaymentPart{Method: PayTransfer, Amount: 0})
	if err != nil {
		t.Fatal(err)
	}
	if out.Method != PaySplit || len(out.Parts) != 2 || out.Parts[0].Method != PayCash || out.MethodLabel != "Nakit 500,00 ₺ + Kredi Kartı 230,00 ₺" {
		t.Fatalf("sonuç: %+v", out)
	}
	a := e.adisyon(no)
	if a.Status != StatusPaid || a.PaymentMethod != PaySplit || len(a.Payments) != 2 {
		t.Fatalf("adisyon: %+v", a)
	}
	n2 := e.add(2, "Ayran")
	e.pay(n2, PayCash)
	e.unlock()
	ds, err := e.svc.DaySummary("")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]PayTotal{}
	for _, p := range ds.ByPayment {
		got[p.Method] = p
	}
	if got[PayCash].Total != 55000 || got[PayCash].Count != 2 || got[PayCard].Total != 23000 || ds.Revenue != 78000 || ds.Paid != 2 || ds.Split != 1 {
		t.Fatalf("gün sonu: %+v ciro %d split %d", ds.ByPayment, ds.Revenue, ds.Split)
	}
	if _, ok := got[PayCredit]; ok {
		t.Fatal("veresiye yokken listede görünmemeli")
	}
	if ds.Rows[0].PaymentLabel != "Nakit 500,00 ₺ + Kredi Kartı 230,00 ₺" {
		t.Fatalf("liste etiketi: %s", ds.Rows[0].PaymentLabel)
	}
	// İstatistik ve gün noktaları parçalardan.
	st, err := e.svc.Stats(StatsQuery{Mode: "day"})
	if err != nil {
		t.Fatal(err)
	}
	if d := st.Days[0]; d.Cash != 55000 || d.Card != 23000 || d.Unknown != 0 {
		t.Fatalf("istatistik günü: %+v", d)
	}
	// Ödeme düzeltme: parçalı → tek şekil (şifreyle).
	if _, err := e.svc.SetPayments(no, []PaymentPart{{Method: PayCard}}, nil, ""); code(err) != "pin_required" {
		t.Fatalf("şifresiz değişti: %v", err)
	}
	if _, err := e.svc.SetPayments(no, []PaymentPart{{Method: PayCard}}, nil, "2468"); err != nil {
		t.Fatal(err)
	}
	if a = e.adisyon(no); a.PaymentMethod != PayCard || len(a.Payments) != 1 || a.Payments[0].Amount != 73000 {
		t.Fatalf("düzeltilmiş ödeme: %+v", a.Payments)
	}
}

// Veresiye: müşteri adı ister, aynı ad (büyük/küçük harf) aynı müşteridir;
// ciroya yazılır, kasaya girmez; tahsilat kısmi alınır ve alındığı günün
// kasasına girer; borçtan fazla tahsil edilmez; tahsilat iptali şifre ister.
func TestCredit(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.at(19, 12, 0)
	n1 := e.add(1, "Cağ") // 340,00
	if _, err := e.payParts(n1, nil, PaymentPart{Method: PayCredit}); code(err) != "credit_name" {
		t.Fatalf("adsız veresiye: %v", err)
	}
	out, err := e.payParts(n1, &CreditRef{Name: " İsmail  Işık ", Phone: "0532 000 00 00"}, PaymentPart{Method: PayCredit})
	if err != nil {
		t.Fatal(err)
	}
	if out.Customer != "İsmail Işık" || out.MethodLabel != "Veresiye" {
		t.Fatalf("veresiye sonucu: %+v", out)
	}
	n2 := e.add(2, "Cağ")
	e.add(2, "Ayran") // 390,00: 100 nakit + 290 veresiye
	if _, err := e.payParts(n2, &CreditRef{Name: "ismail ışık"}, PaymentPart{Method: PayCash, Amount: 10000}, PaymentPart{Method: PayCredit, Amount: 29000}); err != nil {
		t.Fatal(err)
	}
	cl, err := e.svc.Credits()
	if err != nil {
		t.Fatal(err)
	}
	if len(cl.Customers) != 1 || cl.Balance != 63000 || cl.Customers[0].Count != 2 || cl.Customers[0].Phone != "0532 000 00 00" {
		t.Fatalf("veresiye listesi: %+v", cl)
	}
	id := cl.Customers[0].ID
	ds, _ := e.svc.DaySummary("")
	if ds.Revenue != 73000 || ds.CreditGivenTotal != 63000 || len(ds.CreditGiven) != 2 || ds.DrawerTotal != 10000 {
		t.Fatalf("gün sonu: ciro %d veresiye %d kasa %d", ds.Revenue, ds.CreditGivenTotal, ds.DrawerTotal)
	}
	if st := e.state(); st.CreditBalance != 63000 || len(st.CreditCustomers) != 1 {
		t.Fatalf("durum: %d %+v", st.CreditBalance, st.CreditCustomers)
	}
	e.at(19, 23, 0)
	e.closeDay("2026-10-19")

	// Ertesi gün kısmi tahsilat.
	e.at(20, 13, 0)
	if _, _, err := e.svc.CollectCredit(id, 70000, PayCash, ""); code(err) != "invalid" {
		t.Fatalf("borçtan fazla tahsil: %v", err)
	}
	if _, _, err := e.svc.CollectCredit(id, 1000, PayCredit, ""); code(err) != "invalid" {
		t.Fatalf("veresiyeyle tahsil: %v", err)
	}
	d, _, err := e.svc.CollectCredit(id, 40000, PayCash, "maaş günü")
	if err != nil {
		t.Fatal(err)
	}
	if d.Customer.Balance != 23000 || d.Items[0].Kind != "tahsilat" || d.Items[0].Balance != 23000 || len(d.Items) != 3 {
		t.Fatalf("hesap dökümü: %+v", d)
	}
	ds20, _ := e.svc.DaySummary("2026-10-20")
	if ds20.Revenue != 0 || ds20.CollectedTotal != 40000 || ds20.DrawerTotal != 40000 || ds20.Drawer[0].Total != 40000 {
		t.Fatalf("tahsilat günü: ciro %d tahsilat %d kasa %d", ds20.Revenue, ds20.CollectedTotal, ds20.DrawerTotal)
	}
	// Kapalı günün veresiyesi değiştirilemez; tahsilat varken veresiye kaldırılamaz.
	n3 := e.add(3, "Ayran")
	if _, err := e.payParts(n3, &CreditRef{ID: id}, PaymentPart{Method: PayCredit}); err != nil {
		t.Fatal(err)
	}
	if d, _ = e.svc.CreditAccount(id); d.Customer.Balance != 28000 {
		t.Fatalf("üçüncü veresiye: %d", d.Customer.Balance)
	}
	// 280 borç: 50 bugünkü veresiye, 230 dünkü. Bugünkü adisyonu iptal etmek borcu 230'a düşürür.
	if _, err := e.svc.Cancel(n3, "yanlış", "2468"); err != nil {
		t.Fatal(err)
	}
	if d, _ = e.svc.CreditAccount(id); d.Customer.Balance != 23000 {
		t.Fatalf("iptal sonrası borç: %d", d.Customer.Balance)
	}
	// Tahsilat iptali: şifre ve sebep; bugünün tahsilatı.
	pid := d.Items[0].ID
	if d.Items[0].Kind != "tahsilat" {
		t.Fatalf("ilk hareket tahsilat olmalı: %+v", d.Items[0])
	}
	if _, _, err := e.svc.CancelCollection(pid, "yanlış tutar", ""); code(err) != "pin_required" {
		t.Fatalf("şifresiz tahsilat iptali: %v", err)
	}
	if d, _, err = e.svc.CancelCollection(pid, "yanlış tutar", "2468"); err != nil || d.Customer.Balance != 63000 {
		t.Fatalf("tahsilat iptali: %v %+v", err, d)
	}
	// Tamamını kapat; borç sıfır. Tahsilattan sonra (aynı gün) veresiyeli bir
	// adisyon yeniden açılırsa borç eksiye düşeceği için engellenir.
	n4 := e.add(4, "Ayran")
	if _, err := e.payParts(n4, &CreditRef{ID: id}, PaymentPart{Method: PayCredit}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.CollectCredit(id, 68000, PayCard, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Reopen(n4, 0, "adet", "2468"); code(err) != "credit_paid" {
		t.Fatalf("tahsil edilmiş veresiye geri açıldı: %v", err)
	}
	if _, err := e.svc.SetPayments(n4, []PaymentPart{{Method: PayCash}}, nil, "2468"); code(err) != "credit_paid" {
		t.Fatalf("tahsil edilmiş veresiye nakde çevrildi: %v", err)
	}
	cl, _ = e.svc.Credits()
	if cl.Balance != 0 || cl.Debtors != 0 {
		t.Fatalf("borç kapanmadı: %+v", cl)
	}
	// Müşteri adı düzeltilir; başka müşteriyle aynı ad olamaz.
	if _, _, err := e.svc.SaveCreditCustomer(id, "İsmail Işık (Kuyumcu)", "", "esnaf"); err != nil {
		t.Fatal(err)
	}
	n5 := e.add(5, "Ayran")
	if _, err := e.payParts(n5, &CreditRef{Name: "Veli"}, PaymentPart{Method: PayCredit}); err != nil {
		t.Fatal(err)
	}
	cl, _ = e.svc.Credits()
	var vid int64
	for _, c := range cl.Customers {
		if c.Name == "Veli" {
			vid = c.ID
		}
	}
	if _, _, err := e.svc.SaveCreditCustomer(vid, "İSMAİL IŞIK (kuyumcu)", "", ""); code(err) != "invalid" {
		t.Fatalf("aynı ad kabul edildi: %v", err)
	}
	if cl.Customers[0].Name != "Veli" {
		t.Fatalf("borçlu müşteri önce gelmeli: %+v", cl.Customers)
	}
	// İşlem kaydı.
	log, err := e.svc.AuditLog(AuditQuery{From: "2026-10-01", To: "2026-10-30"})
	if err != nil {
		t.Fatal(err)
	}
	var tahsil, iptal, veresiyeOdeme bool
	for _, r := range log.Rows {
		switch r.Action {
		case "veresiye_tahsil":
			tahsil = tahsil || strings.Contains(r.Text, "maaş günü")
		case "veresiye_tahsil_iptal":
			iptal = strings.Contains(r.Text, "yanlış tutar")
		case "odendi":
			veresiyeOdeme = veresiyeOdeme || strings.Contains(r.Text, "Nakit 100,00 ₺ + Veresiye 290,00 ₺")
		}
	}
	if !tahsil || !iptal || !veresiyeOdeme {
		t.Fatalf("işlem kaydı: tahsil=%v iptal=%v parçalı=%v", tahsil, iptal, veresiyeOdeme)
	}
	// Kapalı günün tahsilatı iptal edilemez.
	e.at(20, 23, 30)
	e.closeDay("2026-10-20")
	e.at(21, 12, 0)
	d, _ = e.svc.CreditAccount(id)
	for _, it := range d.Items {
		if it.Kind == "tahsilat" && !it.Cancelled {
			if _, _, err := e.svc.CancelCollection(it.ID, "x", "2468"); code(err) != "closed" {
				t.Fatalf("kapalı günün tahsilatı iptal edildi: %v", err)
			}
		}
	}
}

// Gün öğlen kapatıldıktan sonra alınan veresiye tahsilatı ertesi güne yazılır;
// gün düzeltmesi tahsilatı da doğru güne taşır (o günün kasası doğru çıkar).
func TestShiftedCollection(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.at(19, 10, 0)
	n := e.add(1, "Cağ")
	e.payParts(n, &CreditRef{Name: "Veli"}, PaymentPart{Method: PayCredit})
	e.at(19, 12, 0)
	e.closeDay("2026-10-19")
	e.at(19, 15, 0)
	cl, _ := e.svc.Credits()
	if _, _, err := e.svc.CollectCredit(cl.Customers[0].ID, 10000, PayCash, ""); err != nil {
		t.Fatal(err)
	}
	ds, _ := e.svc.DaySummary("")
	if len(ds.Shifted) != 1 || ds.Shifted[0].Collections != 1 || ds.Shifted[0].Count != 0 || ds.Shifted[0].CollectedTotal != 10000 {
		t.Fatalf("tahsilat kayması görünmüyor: %+v", ds.Shifted)
	}
	n2 := e.add(2, "Ayran")
	e.pay(n2, PayCash)
	ds, _ = e.svc.DaySummary("")
	if len(ds.Shifted) != 1 || ds.Shifted[0].Count != 1 || ds.Shifted[0].Collections != 1 {
		t.Fatalf("adisyon ve tahsilat aynı grupta olmalı: %+v", ds.Shifted)
	}
	res, _, err := e.svc.FixShifted([]ShiftPair{{ds.Shifted[0].From, ds.Shifted[0].To}}, "öğlen kapatıldı", "2468")
	if err != nil {
		t.Fatal(err)
	}
	if res.Moved != 1 || res.MovedCollections != 1 {
		t.Fatalf("sonuç: %+v", res)
	}
	d19, _ := e.svc.DaySummary("2026-10-19")
	d20, _ := e.svc.DaySummary("2026-10-20")
	if d19.CollectedTotal != 10000 || d19.Drawer[0].Total != 15000 || d20.CollectedTotal != 0 {
		t.Fatalf("19 Ekim tahsilat %d kasa nakit %d · 20 Ekim tahsilat %d", d19.CollectedTotal, d19.Drawer[0].Total, d20.CollectedTotal)
	}
}

// Satış olmayan ama tahsilat alınan gün de "gün sonu alınmamış" hatırlatmasına
// girer (tahsilat gün sonu raporuna o gün kapatılınca girer). Veresiye listesi
// durum içinde önbellekte tutulur, ödeme ve tahsilatta yenilenir.
func TestCollectionOnlyDayAndCache(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.at(19, 12, 0)
	n := e.add(1, "Cağ")
	e.payParts(n, &CreditRef{Name: "Veli"}, PaymentPart{Method: PayCredit})
	if st := e.state(); st.CreditBalance != 34000 {
		t.Fatalf("ödemeden sonra alacak: %d", st.CreditBalance)
	}
	e.at(19, 23, 0)
	e.closeDay("2026-10-19")
	e.at(20, 12, 0) // lokanta kapalı; yalnız tahsilat
	cl, _ := e.svc.Credits()
	_, st, err := e.svc.CollectCredit(cl.Customers[0].ID, 34000, PayCash, "")
	if err != nil {
		t.Fatal(err)
	}
	if st.CreditBalance != 0 || st.CreditCustomers[0].Balance != 0 {
		t.Fatalf("tahsilattan sonra durum eski kaldı: %d", st.CreditBalance)
	}
	e.at(21, 12, 0)
	if st = e.state(); len(st.UnclosedDays) != 1 || st.UnclosedDays[0] != "2026-10-20" {
		t.Fatalf("yalnız tahsilat olan gün hatırlatılmadı: %v", st.UnclosedDays)
	}
	e.closeDay("2026-10-20")
	if st = e.state(); len(st.UnclosedDays) != 0 {
		t.Fatalf("kapatılan gün hâlâ listede: %v", st.UnclosedDays)
	}
}
