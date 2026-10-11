# Halis Adisyon 1.0.0 (MVP 1)

Halis Erzurum Cağ Kebabı için Windows 11'de çalışan, internetsiz adisyon programı.
Garson masayı açar, ürünlere tıklar, adisyonu Koddata KPS80 (80 mm termal, ESC/POS)
yazıcıdan basar, hesap ödenince masayı kapatır.

## MVP 1 kapsamı

- Masa kartları, kategori sekmeli ürün düğmeleri, KDV dahil fiyatlı adisyon (2.390,00 ₺)
- +/− düğmeleri, geri al, basıldıktan sonra değişince "tekrar basın" uyarısı
- Tek tıkla baskı, ikinci baskıda KOPYA, sade hata mesajı ve Tekrar Dene, "Basılamadı" işareti
- Fişte logo, slogan, adresler, Google yorum QR'ı ve yasal not
- Ürün, kategori (renk, sıra), garson yönetimi; ilk açılış sihirbazı (12 ürünün fiyatı)
- Yönetici şifresi (4 hane, otomatik kilit). Şunlar her seferinde ayrıca şifre ister:
  - iptal (sebep yazılır)
  - basılmış adisyondan ürün düşmek
  - basılmış adisyonu boşaltmak
- Anında kayıt (SQLite, WAL, synchronous=FULL); otomatik yedek:
  - açılışta, saatte bir ve kapanışta alınır, .zip olarak saklanır
  - 24 saatlik ve 60 günlük yedek tutulur
  - isteğe bağlı USB'ye kopya
- Kurulum sihirbazı (yönetici izni istemez), masaüstü simgesi, isteğe bağlı otomatik başlatma

MVP 1'de olmayanlar: gün sonu, masa taşıma, istatistik ve Excel, WhatsApp ve Google Sheets.

## Mimari

```
HalisAdisyon.exe (Go, tek dosya, CGO yok)
 ├─ yerel HTTP sunucu 127.0.0.1:<rastgele port>  (internal/web)
 │    arayüz (internal/web/ui) + JSON API: POST /api/<ad>, X-Halis-Token başlığı
 ├─ pencere: WebView2 (internal/desktop)  ─ olmazsa Edge --app / varsayılan tarayıcı
 ├─ iş kuralları (internal/core)  ─ tek kilit, her işlem kendi transaction'ında
 ├─ veritabanı (internal/store)   ─ SQLite (ncruces/go-sqlite3, wasm), WAL
 ├─ yedek (internal/backup)       ─ ayrı okuma bağlantısından VACUUM INTO → zip
 ├─ fiş çizimi (internal/receipt) ─ 576/512/384 nokta 1-bit resim, IBM Plex Sans
 ├─ ESC/POS (internal/escpos)     ─ GS v 0 raster bantları, kesme
 └─ yazıcı (internal/printing)    ─ Windows kuyruğu (RAW) ya da TCP 9100
```

### Önemli kararlar

- **Para:** kuruş olarak tamsayıyla tutulur ve KDV dahildir. KDV her oran için ayrı hesaplanır, yarım kuruş yukarı yuvarlanır (`internal/money`).
- **İş günü ve numara:** 05:00'ten önce açılan adisyon önceki iş gününe sayılır. Numara `MAX(no)+1` ile verilir; silinmez, tekrar kullanılmaz.
- **Fiş resim olarak basılır.** Türkçe harfler ve logo her yazıcıda aynı çıkar.
  - Her işin başında 72×256 adet NUL baytı gider. Önceki iş yarıda kesildiyse yazıcıda kalan yarım resim bandı bunlarla tamamlanır.
- **Windows kuyruğu:**
  - Yazıcı "çevrimdışı kullan" ya da duraklatılmışsa iş hiç gönderilmez.
  - Gönderilen iş izlenir; hata verirse ya da süresi dolarsa kuyruktan silinir.
  - Yazıcı takılsa bile en geç birkaç saniyede sonuç döner.
- **Ağ yazıcısı:** göndermeden önce DLE EOT ile kâğıt ve kapak durumu sorulur. Yanıt vermeyen yazıcıda baskı engellenmez.
- **Arayüz ve istek sırası:**
  - Sipariş istekleri tıklama sırasıyla gider.
  - Baskı ve hesap kapatma kendinden önceki tıklamaları bekler ama sonrakileri bekletmez; böylece yazıcı takılsa da diğer masalar çalışır.
  - Basılan ya da kapanan adisyona o sırada ürün eklenemez.
  - Durum kopyalarında `seq` ve `boot` alanları vardır; arayüz geç gelen eski bir yanıtı yeninin üstüne yazmaz.
- **Güvenlik:**
  - Sunucu yalnızca 127.0.0.1'de dinler. Her istekte rastgele bir oturum anahtarı gerekir.
  - Host ve Origin başlıkları denetlenir, CSP uygulanır.
  - Şifre PBKDF2-SHA256 ile saklanır. Yanlış denemelerde bekleme süresi uzar ve veritabanında tutulur, yani programı kapatıp açmak sıfırlamaz.

## Klasörler

| Yol | İçerik |
|---|---|
| `cmd/halisadisyon` | Giriş noktası (bayraklar, kayıt dosyası, açılış ve kapanış sırası) |
| `internal/core` | Masalar, adisyonlar, baskı, ürünler, ayarlar, şifre |
| `internal/store` | Şema (`migrations`), açma, yedek kopyası |
| `internal/backup` | Otomatik yedek ve saklama |
| `internal/receipt`, `internal/escpos`, `internal/printing` | Fiş çizimi, ESC/POS, yazıcıya gönderme |
| `internal/desktop` | WebView2 penceresi, tek kopya, tam ekran, otomatik başlatma |
| `internal/web` | Yerel sunucu ve gömülü arayüz (`ui/`) |
| `build/windows` | Simge, manifest, sürüm bilgisi, NSIS kurulum betiği, kullanım rehberi |
| `test/e2e` | Playwright uçtan uca testi |
| `tools/escpos2png` | Yakalanan ESC/POS işini resme çevirir (yazıcısız kontrol) |

## Derleme

Gerekenler:

- Go 1.24 ya da üstü.
- Kurulum dosyası için ayrıca `x86_64-w64-mingw32-windres` (binutils-mingw-w64) ve `makensis` (NSIS 3).

```sh
go test ./...
./scripts/build-windows.sh            # dist/HalisAdisyon_Kurulum_1.0.0.exe
VERSION=1.0.1 ./scripts/build-windows.sh
```

Yalnızca exe için (Windows'ta da çalışır; simge ve manifest hazır `.syso` dosyasından gelir):

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.version=1.0.0" -o HalisAdisyon.exe ./cmd/halisadisyon
```

`go.mod` içindeki `replace` satırları golang.org/x paketlerini GitHub aynalarından alır. Bunlar yalnızca golang.org'a erişimin kısıtlı olduğu ortamlar için gerekli; normal ortamda silinebilir.

## Çalıştırma ve test

```sh
# Tarayıcı modu (Linux'ta geliştirme): http://127.0.0.1:18080/
HALIS_LOG_STDERR=1 go run ./cmd/halisadisyon -browser -port 18080 -data /tmp/halis -yedek /tmp/halis-yedek

# Uçtan uca test: sahte bir ağ yazıcısı (TCP 19100) açar, sihirbazdan ödemeye kadar her şeyi tıklar
go build -o /tmp/halis ./cmd/halisadisyon
NODE_PATH=$(npm root -g) BIN=/tmp/halis OUT=/tmp/e2e node test/e2e/run.cjs

# Yakalanan bir yazıcı işini resme çevir
go run ./tools/escpos2png /tmp/e2e/jobs/job_2.bin /tmp/fis.png
```

Bayraklar:

| Bayrak | Anlamı |
|---|---|
| `-data` | Veri klasörü |
| `-yedek` | Yedek klasörü |
| `-browser` | Pencere yerine tarayıcıda aç |
| `-no-open` | Tarayıcıyı açma |
| `-port` | Yerel sunucunun portu |
| `-version` | Sürümü yaz |

## Veriler (Windows)

| Ne | Nerede |
|---|---|
| Veritabanı | `%LOCALAPPDATA%\HalisAdisyon\adisyon.db` (yanında `-wal` ve `-shm`) |
| Kayıt dosyası | `%LOCALAPPDATA%\HalisAdisyon\kayitlar\halis.log` (5 MB'ta döner) |
| Yedekler | `Belgeler\Halis Adisyon Yedekleri\{saatlik,gunluk}\adisyon_*.zip` |
| Şema güncellemesi öncesi yedek | `Belgeler\Halis Adisyon Yedekleri\guncelleme_oncesi_*.db` |

Yönetici şifresini sıfırlamak için veri klasörüne `sifre_sifirla.txt` adlı bir dosya koyup programı açın. Şifre 1234 olur ve yanlış deneme beklemesi kalkar.

Yedekten geri dönmek için:

1. Programı kapatın.
2. `adisyon.db`, `adisyon.db-wal` ve `adisyon.db-shm` dosyalarını üçü birlikte başka bir yere taşıyın.
3. Yedek zip'teki `adisyon.db` dosyasını veri klasörüne koyun.

Eski `-wal` dosyası yerinde kalırsa veritabanı bozulur.

## Doğrulananlar ve sınırlar

**Doğrulananlar:**

- Birim testleri Linux'ta ve Windows test programı olarak Wine 9 altında geçiyor.
- Uçtan uca test, Windows exe'si Wine altında çalışırken de 45/45 geçiyor.
- Wine'da Windows yazıcı kuyruğuna RAW iş gönderildi; çıkan baytlar doğru fişe çözüldü.
- Kurulum, yükseltme ve kaldırma Wine'da denendi.

**Gerçek Windows ve donanımda denenmemiş olanlar:**

- WebView2 penceresi.
- Koddata sürücüsünün kuyruk durum bitleri.
- KPS80'in DLE EOT yanıtları.

**İmza:** Program imzasız olduğu için SmartScreen uyarısı çıkar ("Ek bilgi › Yine de çalıştır"). Kod imzalama sertifikası alınırsa `signtool` ile exe ve kurulum dosyası imzalanmalı.

## Sonraki adımlar (MVP 2)

- Gün sonu ekranı (iş gününe göre satış, KDV, iptal listesi, değiştirilen basılmış adisyonlar)
- Masa taşıma ve birleştirme
- İstatistik ve Excel dışa aktarma
- Yedekten geri dönme düğmesi
