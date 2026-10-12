# Halis Adisyon 2.1.0

Halis Erzurum Cağ Kebabı için Windows 11'de çalışan, internetsiz adisyon programı.
Garson masayı açar, ürünlere tıklar, adisyonu Koddata KPS80 (80 mm termal, ESC/POS)
yazıcıdan basar; müşteri ödeyince ödeme şeklini (Nakit, Kredi Kartı, Havale) seçip
masayı kapatır. Yönetici gün sonunu alır, istatistikleri ve Excel raporunu görür.

## 2.1.0'da gelenler

- **Yüzde indirim:** sipariş ekranında "İndirim" (hazır yüzdeler ya da elle, %100 = ikram). Yönetici şifresi ister; Ayarlar'dan kapatılabilir (`discountNoPin`). İndirim adisyonun tamamına, KDV oranı başına yarım yukarı yuvarlanarak uygulanır; `total` ve `vat_total` indirimli tutardır (`discount_percent`, `discount_amount` sütunları, şema 3). Fişte "Ara toplam / İndirim (%x) / TOPLAM"; gün sonu, istatistik, Excel ve Sheets'te indirim toplamları.
- **Ödenmiş adisyonu düzeltme:** Gün Sonu › adisyon ayrıntısında "Düzeltmek için yeniden aç" (`reopen`: sebep + şifre; adisyon masaya `printed`/`open` olarak döner, ödeme kaydı kalkar, numara aynı kalır) ve "İptal et" (ödenmiş adisyon için `cancel`). Yalnız kapatılmamış günlerde.
- **Günlük yedekler 90 gün** saklanır (önceden 60). Veri süresizdir; Ayarlar'daki açıklama buna göre düzeltildi.
- Üst çubuk uzun gün adlarında (Pazartesi, Çarşamba) alt satıra kaymıyor.

## 2.0.0'da gelenler (MVP 2, 3, 4)

- **Ödeme şekli:** "Hesap Ödendi" penceresinde Nakit / Kredi Kartı / Havale (klavyede 1-2-3).
  - Fişe basılmaz; adisyon fişi ödemeden önce basılır.
  - Adisyona kaydedilir ve gün sonu, istatistik, Excel, WhatsApp ve Sheets'te görünür.
  - Yanlış seçim gün kapanmadan şifreyle düzeltilir.
- **Gün sonu:**
  - Kontrol: numara atlaması, basılmadan kapatılanlar, basıldıktan sonra iptal, açık masa.
  - Ciro, KDV (oranlara göre), ödeme şekli ve garson toplamları, ürün adetleri.
  - Adisyon listesi, ayrıntı ve işlem geçmişi.
  - Termal gün sonu fişi. Günü kapatma: açık masa varken kapanmaz, kapatılan gün değişmez, kapanışta yedek alınır.
- **Masa taşıma ve birleştirme.** Birleşen adisyon "birleşti" olarak kayıtta kalır; iptal sayılmaz.
- **Toplu zam:** yüzde ve yuvarlama, önce önizleme, işlem kaydına yazılır.
- **İşlem kaydı ekranı:** tarih, tür ve adisyon numarasına göre süzülür.
- **Ayarlarda canlı fiş önizlemesi:** kaydetmeden çizilir; örnek adisyon basılabilir.
- **İstatistik:** gün, ay, yıl ya da aralık seçilir. Önceki dönemle karşılaştıran kartlar, elle çizilen SVG grafikler ve sıralanabilir gün × ürün tablosu var.
- **Excel:** Özet, Günlük, Ürünler ve Adisyonlar sayfaları. Bağımlılıksız XLSX yazıcıyla üretilir (`internal/xlsx`).
- **Yedek:**
  - Google Drive klasörüne kopya; son kopyanın saati gösterilir.
  - Program içinden geri yükleme: önce şu anki veri yedeklenir, yedek doğrulanır, program yeniden açılırken yerine konur, adisyon numarası geriye düşmez.
- **Gün sonu gönderimleri:** WhatsApp Business Platform (Cloud API) şablon mesajı ve Google Sheets (Apps Script web uygulaması). İnternet yoksa `outbox` tablosunda sırada bekler.
- **Güncelleme:** 1.0.0 verisi otomatik geçiş yapar (şema 1 → 2); geçişten önce yedek alınır.

## MVP 1 kapsamı (1.0.0)

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
  - 24 saatlik ve 90 günlük yedek tutulur (veri süresiz; yedekler kopyadır)
  - isteğe bağlı USB'ye kopya
- Kurulum sihirbazı (yönetici izni istemez), masaüstü simgesi, isteğe bağlı otomatik başlatma

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
- **İş günü ve numara:**
  - 05:00'ten önce açılan adisyon önceki iş gününe sayılır.
  - Gün sonu alınıp kapatılan güne yeni adisyon yazılmaz; ilk kapatılmamış sonraki güne yazılır.
  - Numara `MAX(no)+1` ile verilir; silinmez, tekrar kullanılmaz.
  - Yedekten geri yüklemede `settings.no_floor` en büyük numarayı saklar, numaralar onun üstünden devam eder.
- **Ödeme şekli:** `adisyons.payment_method` alanındadır (`nakit`, `kart`, `havale`; 1.0.0'dan kalanlarda boş). Fiş çizimi bu alanı hiç okumaz.
- **İstatistik:** yalnız ödenen adisyonlar sayılır. Dönem bugünü içeriyorsa bugüne kadar alınır ve önceki dönemin aynı sayıda günüyle karşılaştırılır.
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
| `internal/core` | Masalar, adisyonlar, ödeme, baskı, ürünler, ayarlar, şifre, gün sonu (`days.go`), istatistik (`stats.go`), Excel (`export.go`), gönderim sırası (`outbox.go`), geri yükleme |
| `internal/store` | Şema (`migrations`), açma, yedek kopyası, yedek doğrulama |
| `internal/backup` | Otomatik yedek, saklama, USB ve Google Drive kopyası, geri yükleme hazırlığı |
| `internal/remote` | WhatsApp Cloud API ve Google Sheets istemcisi |
| `internal/xlsx` | Excel (.xlsx) yazıcı |
| `internal/receipt`, `internal/escpos`, `internal/printing` | Fiş çizimi, ESC/POS, yazıcıya gönderme |
| `internal/desktop` | WebView2 penceresi, tek kopya, tam ekran, otomatik başlatma |
| `internal/web` | Yerel sunucu ve gömülü arayüz (`ui/`) |
| `build/windows` | Simge, manifest, sürüm bilgisi, NSIS kurulum betiği, kullanım rehberi |
| `test/e2e` | Playwright uçtan uca testi |
| `tools/escpos2png` | Yakalanan ESC/POS işini resme çevirir (yazıcısız kontrol) |
| `tools/ornekveri` | Geçmiş günleri olan örnek veritabanı üretir (gün sonu ve istatistik denemesi için) |

## Derleme

Gerekenler:

- Go 1.24 ya da üstü.
- Kurulum dosyası için ayrıca `x86_64-w64-mingw32-windres` (binutils-mingw-w64) ve `makensis` (NSIS 3).

```sh
go test ./...
VERSION=2.1.0 ./scripts/build-windows.sh   # dist/HalisAdisyon_Kurulum_2.1.0.exe
```

Yalnızca exe için (Windows'ta da çalışır; simge ve manifest hazır `.syso` dosyasından gelir):

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.version=2.1.0" -o HalisAdisyon.exe ./cmd/halisadisyon
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

# 45 günlük örnek veriyle aç (şifre 1234)
go run ./tools/ornekveri -db /tmp/ornek/adisyon.db -gun 45
go run ./cmd/halisadisyon -browser -port 18080 -data /tmp/ornek -yedek /tmp/ornek-yedek
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
| Yedekler | `Belgeler\Halis Adisyon Yedekleri\{saatlik,gunluk,ozel}\adisyon_*.zip` |
| Şema güncellemesi öncesi yedek | `Belgeler\Halis Adisyon Yedekleri\guncelleme_oncesi_*.db` |
| Excel raporları | `Belgeler\Halis Adisyon Raporları\Halis_Adisyon_<başlangıç>_<bitiş>.xlsx` (Belgeler'e yazılamazsa `%LOCALAPPDATA%\HalisAdisyon\Raporlar\`) |
| Geri yüklemede kenara alınan veri | `%LOCALAPPDATA%\HalisAdisyon\eski_veri\<tarih>\` |

Yönetici şifresini sıfırlamak için veri klasörüne `sifre_sifirla.txt` adlı bir dosya koyup programı açın. Şifre 1234 olur ve yanlış deneme beklemesi kalkar.

Yedekten geri yükleme Ayarlar › Yedek › "Yedekten geri yükle" ile yapılır:

1. Şu anki verinin yedeği alınır (`ozel/..._geri_yukleme_oncesi.zip`).
2. Seçilen yedek veri klasörüne `geri_yukle.db` olarak çıkarılır ve doğrulanır (`integrity_check`, şema sürümü, tablolar).
3. Program kendini kapatıp yeniden açar.
4. Açılışta, veritabanı açılmadan önce `adisyon.db`, `-wal` ve `-shm` üçü birlikte `eski_veri\<tarih>` klasörüne taşınır ve yedek yerine konur (`backup.ApplyPending`).

Elle yapılacaksa aynı sıra izlenmeli. Eski `-wal` dosyası yerinde kalırsa veritabanı bozulur.

## Doğrulananlar ve sınırlar

**Doğrulananlar:**

- Birim testleri Linux'ta ve Windows test programı olarak Wine 9 altında geçiyor.
- Uçtan uca test (sihirbazdan gün sonu, istatistik, Excel ve yedekten geri yüklemeye kadar) Linux'ta ve Windows exe'si Wine altında çalışırken geçiyor.
- Wine'da Windows yazıcı kuyruğuna RAW iş gönderildi; çıkan baytlar doğru fişe çözüldü.
- Kurulum, yükseltme ve kaldırma Wine'da denendi.

**Gerçek Windows ve donanımda denenmemiş olanlar:**

- WebView2 penceresi.
- Koddata sürücüsünün kuyruk durum bitleri.
- KPS80'in DLE EOT yanıtları.
- Gerçek WhatsApp Cloud API ve Google Apps Script uçları. İstemciler sahte sunucularla denendi; Meta ve Google hesabıyla bir kez "Test mesajı gönder" / "Bağlantıyı dene" yapılmalı.
- Google Drive masaüstü programının klasör adları ("Drive'ım", "My Drive"); bulunamazsa klasör elle yazılabilir.

**İmza:** Program imzasız olduğu için SmartScreen uyarısı çıkar ("Ek bilgi › Yine de çalıştır"). Kod imzalama sertifikası alınırsa `signtool` ile exe ve kurulum dosyası imzalanmalı.

## Sonraki adımlar

Planlama belgesinin "Sonra · Fikir havuzu" bölümündeki maddeler (ör. H1.7 ikram/indirim) henüz yapılmadı.
