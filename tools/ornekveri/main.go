// ornekveri test ve tanıtım için geçmiş günleri olan örnek bir veritabanı üretir:
// her gün onlarca adisyon (baskı, ödeme şekli, iptal, masa birleştirme) ve gün sonu.
//
//	go run ./tools/ornekveri -db /tmp/ornek/adisyon.db -gun 45
package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"halisadisyon/internal/core"
	"halisadisyon/internal/printing"
	"halisadisyon/internal/store"
)

func main() {
	dbPath := flag.String("db", "", "oluşturulacak veritabanı (var olan silinir)")
	days := flag.Int("gun", 45, "geçmiş gün sayısı")
	seed := flag.Int64("tohum", 7, "rastgele tohum")
	flag.Parse()
	if *dbPath == "" {
		log.Fatal("-db gerekli")
	}
	os.MkdirAll(filepath.Dir(*dbPath), 0o755)
	for _, suf := range []string{"", "-wal", "-shm"} {
		os.Remove(*dbPath + suf)
	}
	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(nil); err != nil {
		log.Fatal(err)
	}
	rnd := rand.New(rand.NewSource(*seed))
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	clock := today.AddDate(0, 0, -*days).Add(9 * time.Hour)
	svc, err := core.New(st.DB, core.Options{Version: "örnek", Now: func() time.Time { return clock },
		Send: func(printing.Config, []byte, string) printing.Result { return printing.Result{OK: true} }})
	if err != nil {
		log.Fatal(err)
	}
	s0, _ := svc.State()
	prices := map[string]int64{"Cağ": 35000, "Şiş": 32000, "Yoğurt": 6000, "Tatlı": 9000, "Salata": 7000, "Ayran": 4000,
		"Şalgam": 4000, "Kola": 6000, "Soda": 3500, "Büyük Su": 2500, "Küçük Su": 1500, "Çorba": 9000}
	pm := map[string]int64{}
	ids := map[string]int64{}
	for _, p := range s0.Products {
		pm[strconv.FormatInt(p.ID, 10)] = prices[p.Name]
		ids[p.Name] = p.ID
	}
	if _, err := svc.FinishSetup(core.SetupInput{Business: core.Business{Name: "Halis Erzurum Cağ Kebabı", Address1: "Merkez: Akdeniz Sanayi 5009 Sk.", Address2: "Şube: Uncalı Mh. Çamlık Cd."},
		Tables: 24, Waiters: []string{"Ahmet", "Mehmet", "Ayşe"}, Printer: printing.Config{Mode: printing.ModeNone, Width: 576, Cut: true},
		Prices: pm, Pin: "1234"}); err != nil {
		log.Fatal(err)
	}
	st1, _ := svc.State()
	waiters := st1.Waiters
	type pick struct {
		name string
		w    int
	}
	menu := []pick{{"Cağ", 40}, {"Şiş", 12}, {"Ayran", 22}, {"Şalgam", 8}, {"Kola", 8}, {"Yoğurt", 6}, {"Salata", 8}, {"Tatlı", 6}, {"Çorba", 6}, {"Büyük Su", 5}, {"Küçük Su", 6}, {"Soda", 3}}
	total := 0
	for _, m := range menu {
		total += m.w
	}
	choose := func() int64 {
		r := rnd.Intn(total)
		for _, m := range menu {
			if r < m.w {
				return ids[m.name]
			}
			r -= m.w
		}
		return ids["Cağ"]
	}
	methods := []string{core.PayCash, core.PayCard, core.PayTransfer}
	count := 0
	for d := 0; d <= *days; d++ {
		day := today.AddDate(0, 0, d-*days)
		isToday := d == *days
		n := 35 + rnd.Intn(30)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			n += 25
		}
		if isToday {
			n = 12
		}
		// Gün içinde masa açılış saatleri (11:00–23:30), sıralı.
		var opens []time.Duration
		for i := 0; i < n; i++ {
			opens = append(opens, time.Duration(11*60+rnd.Intn(12*60+30))*time.Minute)
		}
		for i := range opens {
			for j := i + 1; j < len(opens); j++ {
				if opens[j] < opens[i] {
					opens[i], opens[j] = opens[j], opens[i]
				}
			}
		}
		type openTable struct {
			table int
			no    int64
			close time.Time
		}
		var active []openTable
		used := map[int]bool{}
		closeDue := func(until time.Time, all bool) {
			var keep []openTable
			for _, a := range active {
				if !all && a.close.After(until) {
					keep = append(keep, a)
					continue
				}
				if a.close.After(clock) {
					clock = a.close
				}
				r := rnd.Intn(100)
				switch {
				case r < 3:
					svc.Unlock("1234")
					if _, err := svc.Cancel(a.no, "Müşteri vazgeçti", "1234"); err != nil {
						log.Fatal(err)
					}
				default:
					if r < 93 {
						if _, _, err := svc.Print(a.no); err != nil {
							log.Fatal(err)
						}
					}
					m := methods[0]
					switch x := rnd.Intn(100); {
					case x < 50:
						m = core.PayCard
					case x < 94:
						m = core.PayCash
					default:
						m = core.PayTransfer
					}
					if _, _, err := svc.Pay(a.no, m); err != nil {
						log.Fatal(err)
					}
				}
				delete(used, a.table)
			}
			active = keep
		}
		for i, o := range opens {
			t := day.Add(o)
			if isToday && t.After(now) {
				break
			}
			clock = t
			closeDue(t, false)
			table := 1 + rnd.Intn(24)
			for used[table] {
				table = 1 + rnd.Intn(24)
			}
			used[table] = true
			w := waiters[rnd.Intn(len(waiters))]
			_, no, err := svc.AddItem(table, choose(), w.ID)
			if err != nil {
				log.Fatal(err)
			}
			items := 1 + rnd.Intn(6)
			for k := 0; k < items; k++ {
				clock = clock.Add(time.Duration(rnd.Intn(90)) * time.Second)
				if _, _, err := svc.AddItem(table, choose(), w.ID); err != nil {
					log.Fatal(err)
				}
			}
			count++
			stay := time.Duration(25+rnd.Intn(70)) * time.Minute
			active = append(active, openTable{table: table, no: no, close: t.Add(stay)})
			// Ara sıra masa birleştirme.
			if i%37 == 5 && len(active) > 1 {
				src, dst := active[len(active)-1], active[0]
				if _, _, err := svc.MoveTable(src.no, dst.table); err == nil {
					delete(used, src.table)
					active = active[:len(active)-1]
				}
			}
		}
		if isToday {
			break // bugünün masaları açık kalır
		}
		closeDue(day.Add(26*time.Hour), true)
		clock = day.Add(24*time.Hour + 30*time.Minute)
		if _, err := svc.Unlock("1234"); err != nil {
			log.Fatal(err)
		}
		if d < *days-2 { // son iki günün gün sonu alınmamış kalsın
			if _, _, err := svc.CloseDay(day.Format("2006-01-02")); err != nil {
				log.Fatal(err)
			}
		}
	}
	fmt.Printf("%d gün, %d adisyon yazıldı: %s (şifre 1234)\n", *days+1, count, *dbPath)
}
