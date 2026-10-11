package remote

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizePhone(t *testing.T) {
	cases := map[string]string{
		"0532 123 45 67":    "905321234567",
		"+90 (532) 123-4567": "905321234567",
		"5321234567":        "905321234567",
		"0090 532 1234567":  "905321234567",
		"+49 151 2345 6789": "4915123456789",
	}
	for in, want := range cases {
		if got := NormalizePhone(in); got != want {
			t.Errorf("%q → %q, beklenen %q", in, got, want)
		}
	}
}

func TestCleanParam(t *testing.T) {
	if got := cleanParam("Ahmet 3 adisyon\nMehmet\t2    adisyon "); got != "Ahmet 3 adisyon · Mehmet 2 adisyon" {
		t.Fatalf("temizlenmedi: %q", got)
	}
	if cleanParam("  ") != "-" {
		t.Fatal("boş değişken '-' olmalı")
	}
}

func TestWhatsAppErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":{"message":"(#132001) Template name does not exist in the translation","code":132001}}`))
	}))
	defer srv.Close()
	old := GraphBase
	GraphBase = srv.URL
	defer func() { GraphBase = old }()
	_, err := WhatsApp{PhoneNumberID: "1", Token: "x", Template: "yok"}.SendTemplate(context.Background(), "05321234567", []string{"a"})
	if err == nil || !IsPermanent(err) || !strings.Contains(err.Error(), "şablon") {
		t.Fatalf("hata çevrilmedi: %v", err)
	}
	if _, err := (WhatsApp{}).SendTemplate(context.Background(), "05321234567", nil); !IsPermanent(err) {
		t.Fatal("eksik ayar kalıcı hata olmalı")
	}
}

func TestSheetsPost(t *testing.T) {
	if err := (Sheets{URL: "https://example.com/exec"}).Post(context.Background(), map[string]any{}); !IsPermanent(err) {
		t.Fatal("yabancı adres kabul edildi")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":false,"error":"Gizli anahtar uyuşmuyor."}`))
	}))
	defer srv.Close()
	err := (Sheets{URL: srv.URL, Secret: "x"}).Post(context.Background(), map[string]any{"type": "test"})
	if err == nil || !strings.Contains(err.Error(), "Gizli anahtar") {
		t.Fatalf("betik hatası iletilmedi: %v", err)
	}
	// HTML yanıt (yanlış dağıtım): anlaşılır mesaj.
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`<html>Sign in</html>`))
	}))
	defer html.Close()
	if err := (Sheets{URL: html.URL}).Post(context.Background(), map[string]any{}); err == nil || !IsPermanent(err) || !strings.Contains(err.Error(), "Herkes") {
		t.Fatalf("yanlış dağıtım hatası: %v", err)
	}
}
