package printing

import (
	"bytes"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

// fakePrinter 9100 portu gibi davranan sahte bir ağ yazıcısıdır. status nil ise
// durum sorularına yanıt vermez; değilse DLE EOT n sorusuna status[n] ile yanıt verir.
func fakePrinter(t *testing.T, status map[byte]byte) (host string, port int, got chan []byte) {
	return fakePrinterDelay(t, status, 0)
}

// fakePrinterDelay durum sorularına delay kadar geç yanıt veren sahte yazıcıdır.
func fakePrinterDelay(t *testing.T, status map[byte]byte, delay time.Duration) (host string, port int, got chan []byte) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got = make(chan []byte, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				var job []byte
				buf := make([]byte, 64*1024)
				for {
					n, err := c.Read(buf)
					chunk := buf[:n]
					for len(chunk) >= 3 && chunk[0] == 0x10 && chunk[1] == 0x04 && status != nil {
						if b, ok := status[chunk[2]]; ok {
							if delay > 0 {
								go func(b byte) { time.Sleep(delay); c.Write([]byte{b}) }(b)
							} else {
								c.Write([]byte{b})
							}
						}
						chunk = chunk[3:]
					}
					job = append(job, chunk...)
					if err == io.EOF || err != nil {
						if len(job) > 0 || len(got) == 0 {
							got <- job
						}
						return
					}
				}
			}(c)
		}
	}()
	a := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", a.Port, got
}

func cfgFor(host string, port int) Config {
	c := DefaultConfig()
	c.Mode = ModeNetwork
	c.Host = host
	c.Port = port
	return c
}

func TestNetworkPrintHealthy(t *testing.T) {
	host, port, got := fakePrinter(t, map[byte]byte{4: 0x12, 2: 0x12})
	data := bytes.Repeat([]byte{0x00}, 1000)
	data = append(data, 0x1B, 0x40)
	start := time.Now()
	r := Send(cfgFor(host, port), data, "test")
	if !r.OK {
		t.Fatalf("baskı başarısız: %+v", r)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("sağlıklı yazıcıda baskı çok uzun sürdü: %s", time.Since(start))
	}
	select {
	case job := <-got:
		if !bytes.Equal(job, data) {
			t.Fatalf("yazıcıya giden veri farklı: %d bayt", len(job))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("yazıcıya iş gelmedi")
	}
}

func TestNetworkPrintPaperOut(t *testing.T) {
	host, port, got := fakePrinter(t, map[byte]byte{4: 0x72, 2: 0x32})
	r := Send(cfgFor(host, port), []byte{0x1B, 0x40}, "test")
	if r.OK || r.Code != "paper" {
		t.Fatalf("kâğıt bitti hatası bekleniyordu: %+v", r)
	}
	select {
	case job := <-got:
		if len(job) != 0 {
			t.Fatalf("kâğıt yokken %d bayt gönderildi", len(job))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("bağlantı kapanmadı")
	}
}

func TestNetworkPrintCoverOpen(t *testing.T) {
	host, port, _ := fakePrinter(t, map[byte]byte{4: 0x12, 2: 0x16})
	r := Send(cfgFor(host, port), []byte{0x1B, 0x40}, "test")
	if r.OK || r.Code != "cover" {
		t.Fatalf("kapak açık hatası bekleniyordu: %+v", r)
	}
}

// Durum sorusuna yanıt vermeyen yazıcıda baskı yine yapılmalı.
func TestNetworkPrintSilentPrinter(t *testing.T) {
	host, port, got := fakePrinter(t, nil)
	data := []byte{0x1B, 0x40, 0x1B, 0x64, 0x05}
	r := Send(cfgFor(host, port), data, "test")
	if !r.OK {
		t.Fatalf("baskı başarısız: %+v", r)
	}
	for {
		select {
		case job := <-got:
			if bytes.Equal(job, []byte{0x10, 0x04, 0x04}) {
				continue // yanıtsız kalan durum sorusunun bağlantısı
			}
			if !bytes.Equal(job, data) {
				t.Fatalf("iş eksik geldi: % x", job)
			}
			return
		case <-time.After(5 * time.Second):
			t.Fatal("yazıcıya iş gelmedi")
		}
	}
}

// Anlamsız yanıt (sabit bitleri tutmayan bayt) baskıyı engellememeli.
func TestNetworkPrintGarbageStatusIgnored(t *testing.T) {
	host, port, _ := fakePrinter(t, map[byte]byte{4: 0xFF, 2: 0xFF})
	if r := Send(cfgFor(host, port), []byte{0x1B, 0x40}, "test"); !r.OK {
		t.Fatalf("baskı engellendi: %+v", r)
	}
}

func TestNetworkPrintUnreachable(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	r := Send(cfgFor("127.0.0.1", port), []byte{0x1B, 0x40}, "test")
	if r.OK || r.Code != "network" {
		t.Fatalf("ulaşılamıyor hatası bekleniyordu: %+v", r)
	}
	if r.Message == "" || r.Hint == "" {
		t.Fatal("kullanıcı mesajı boş")
	}
}

func TestJobWaitGrowsWithSize(t *testing.T) {
	small, big := jobWait(10*1024), jobWait(400*1024)
	if !(small >= 6*time.Second && big > small && big <= 20*time.Second) {
		t.Fatalf("bekleme süreleri yanlış: %s %s", small, big)
	}
}

func TestNormalize(t *testing.T) {
	c := Config{Mode: "network", Host: " 192.168.1.87 ", Width: 999}
	if err := c.Normalize(); err != nil {
		t.Fatal(err)
	}
	if c.Host != "192.168.1.87" || c.Port != 9100 || c.Width != 576 || c.CutMode != "partial" {
		t.Fatalf("eksik alanlar doldurulmadı: %+v", c)
	}
	bad := Config{Mode: "network", Host: "1.2.3.4", Port: 70000}
	if bad.Normalize() == nil {
		t.Fatal("geçersiz port kabul edildi")
	}
	_ = strconv.Itoa
}

// Yazıcı durum sorusuna geç yanıt verirse (önceki fişi basarken) geç gelen bayt
// bağlantıyı koparıp fişin sonunu kesmemeli.
func TestNetworkPrintLateStatusReply(t *testing.T) {
	host, port, got := fakePrinterDelay(t, map[byte]byte{4: 0x12, 2: 0x12}, time.Second)
	data := bytes.Repeat([]byte{0xAA}, 400*1024)
	r := Send(cfgFor(host, port), data, "test")
	if !r.OK {
		t.Fatalf("baskı başarısız: %+v", r)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case job := <-got:
			if len(job) == 0 {
				continue // durum sorusu sorulan ilk bağlantı
			}
			if !bytes.Equal(job, data) {
				t.Fatalf("fiş eksik geldi: %d/%d bayt", len(job), len(data))
			}
			return
		case <-deadline:
			t.Fatal("yazıcıya iş gelmedi")
		}
	}
}
