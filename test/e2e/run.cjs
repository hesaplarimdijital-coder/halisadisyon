// Uçtan uca deneme: programı tarayıcı modunda açar, sahte bir ağ yazıcısı (TCP 9100 benzeri)
// dinler ve garsonun/yöneticinin yapacağı işleri sırayla tıklar.
//   BIN=./halis OUT=/tmp/e2e node test/e2e/run.cjs
const { chromium } = require('playwright');
const net = require('net');
const fs = require('fs');
const { spawn } = require('child_process');

const OUT = process.env.OUT || '/tmp/e2e';
const BIN = process.env.BIN;
const PORT = 18080, PRN = 19100, DEAD = 19199;
const sleep = ms => new Promise(r => setTimeout(r, ms));
// Yeniden başlatmada program kendini yeni bir süreç olarak açar: o süreci (ve önceki
// yarım kalmış denemelerden kalanları) veri klasörü argümanından bulup kapatır.
function killAppsUsing(dir) {
  try {
    const out = require('child_process').execFileSync('ps', ['-eo', 'pid,args']).toString();
    for (const line of out.split('\n')) {
      const m = line.trim().match(/^(\d+)\s+(.*)$/);
      if (m && m[2].includes(dir) && /-data/.test(m[2]) && !/\bnode\b/.test(m[2]) && Number(m[1]) !== process.pid) {
        try { process.kill(Number(m[1]), 'SIGTERM'); } catch (e) { }
      }
    }
  } catch (e) { }
}
// Wine altında program Windows yolu döndürür (C:\... ya da sürücüsüz \tmp\...);
// dosyaya buradan bakabilmek için Linux yoluna çevir.
function hostPath(p) {
  if (!p) return p;
  const m = p.match(/^([A-Za-z]):\\(.*)$/);
  if (m) {
    const rest = m[2].replace(/\\/g, '/');
    if (m[1].toLowerCase() === 'z') return '/' + rest;
    return process.env.WINEPREFIX ? process.env.WINEPREFIX + '/drive_' + m[1].toLowerCase() + '/' + rest : p;
  }
  return /^\\[^\\]/.test(p) ? p.replace(/\\/g, '/') : p;
}
killAppsUsing((process.env.OUT || '/tmp/e2e') + '/data');
fs.rmSync(OUT, { recursive: true, force: true });
for (const d of ['shots', 'jobs']) fs.mkdirSync(OUT + '/' + d, { recursive: true });

let jobs = 0;
const printer = net.createServer(sock => {
  const chunks = [];
  sock.on('data', d => {
    // DLE EOT n (durum sorusu): sağlıklı yazıcı gibi 0x12 ile yanıt ver.
    if (d.length === 3 && d[0] === 0x10 && d[1] === 0x04) { sock.write(Buffer.from([0x12])); return; }
    chunks.push(d);
  });
  sock.on('end', () => { jobs++; fs.writeFileSync(`${OUT}/jobs/job_${jobs}.bin`, Buffer.concat(chunks)); });
});
printer.listen(PRN, '127.0.0.1');

const ENV = { ...process.env, HALIS_LOG_STDERR: '1', XDG_DATA_HOME: OUT + '/xdg' };
const app = spawn(BIN, ['-browser', '-no-open', '-data', OUT + '/data', '-yedek', OUT + '/yedek', '-port', String(PORT)], { env: ENV });
const appLog = [];
app.stdout.on('data', d => appLog.push(String(d)));
app.stderr.on('data', d => appLog.push(String(d)));

const checks = [];
function check(name, ok, detail) { checks.push({ name, ok: !!ok, detail: detail || '' }); if (!ok) console.log('FAIL:', name, detail || ''); }

(async () => {
  for (let i = 0; i < 100; i++) { try { await fetch(`http://127.0.0.1:${PORT}/`); break; } catch { await sleep(100); } }
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1366, height: 768 } });
  const errors = [];
  let restarting = false; // program yeniden açılırken bağlantı hatası beklenir
  page.on('console', m => { if (m.type() === 'error' && !(restarting && /ERR_CONNECTION_REFUSED/.test(m.text()))) errors.push('console: ' + m.text()); });
  page.on('pageerror', e => errors.push('pageerror: ' + e.message));
  const shot = async name => page.screenshot({ path: `${OUT}/shots/${name}.png` });
  const click = sel => page.locator(sel).first().click();
  const btn = name => page.getByRole('button', { name, exact: true }).first().click();
  const toastText = async (want) => {
    try { await page.waitForFunction(w => [...document.querySelectorAll('.toast')].some(t => !w || t.textContent.includes(w)), want || '', { timeout: 10000 }); } catch (e) { /* aşağıda raporlanır */ }
    const all = await page.locator('.toast').allTextContents();
    return (want ? all.filter(t => t.includes(want)).pop() : all.pop()) || all.join(' | ');
  };

  // Program yavaş yanıt verse de (ör. Wine) koşul sağlanana kadar bekle.
  const until = (pred, arg, ms = 8000) => page.waitForFunction(pred, arg, { timeout: ms }).catch(() => {});

  await page.goto(`http://127.0.0.1:${PORT}/`);
  await page.waitForSelector('.wizard');
  await shot('01_kurulum_isletme');
  check('kurulum açılır', await page.locator('.step.on').textContent().then(t => t.includes('İşletme')));
  await page.fill('#w-slogan', 'Odun ateşinde, yatay şişte');
  await page.fill('#w-phone', '0242 000 00 00');
  await btn('İleri');

  await page.waitForSelector('#w-tables');
  await page.fill('#w-tables', '20');
  await page.fill('#w-wt-0', 'Ahmet');
  await btn('Garson ekle');
  await page.fill('#w-wt-1', 'Mehmet');
  await shot('02_kurulum_masalar');
  await btn('İleri');

  await page.waitForSelector('input[name="wz-mode"]');
  await page.locator('input[name="wz-mode"][value="network"]').check({ force: true });
  await page.waitForSelector('#wz-host');
  await page.fill('#wz-host', '127.0.0.1'); await page.press('#wz-host', 'Tab');
  await page.fill('#wz-port', String(PRN)); await page.press('#wz-port', 'Tab');
  await sleep(200);
  await btn('Test fişi bas');
  await page.waitForSelector('.note.good, .note.bad', { timeout: 10000 });
  await sleep(300);
  await shot('03_kurulum_yazici_test');
  check('test fişi yazıcıya gitti', jobs === 1, 'iş sayısı ' + jobs);
  await btn('İleri');

  const prices = { 'Cağ': '340', 'Şiş': '300', 'Yoğurt': '70', 'Tatlı': '150', 'Salata': '90', 'Ayran': '50', 'Şalgam': '60', 'Kola': '150', 'Soda': '50', 'Büyük Su': '50', 'Küçük Su': '25', 'Çorba': '120' };
  for (const [n, v] of Object.entries(prices)) await page.getByLabel(n + ' fiyatı', { exact: true }).fill(v);
  await shot('04_kurulum_fiyatlar');
  await btn('İleri');
  await page.fill('#w-pin', '2468'); await page.fill('#w-pin2', '2468');
  await btn('Kurulumu bitir');
  await page.waitForSelector('.tiles');
  await sleep(300);
  await shot('05_masalar_bos');
  check('masa sayısı 20', await page.locator('.tile').count() === 20);

  // Masa 5: garson seç, sipariş gir
  await click('[data-act="open-table"][data-t="5"]');
  await page.waitForSelector('.pick');
  await shot('06_garson_sec');
  await page.getByRole('button', { name: 'Ahmet' }).click();
  await page.waitForSelector('.prods');
  // Ürün butonu adıyla tam eşleşir (“Şiş” birim adı “şiş” olan Cağ'a da uymasın).
  const prod = name => page.locator('.prod', { has: page.locator('.prod-name', { hasText: new RegExp('^' + name + '$') }) }).first();
  for (let i = 0; i < 6; i++) await prod('Cağ').click();
  await prod('Ayran').click(); await prod('Ayran').click(); await prod('Ayran').click();
  await prod('Büyük Su').click();
  await prod('Kola').click();
  await until(() => (document.querySelector('.total-amount') || {}).textContent?.includes('2.390,00'));
  await sleep(200);
  await shot('07_siparis');
  let total = await page.locator('.total-amount').textContent();
  check('toplam doğru (2.390,00)', total.includes('2.390,00'), total);
  check('KDV satırı doğru (217,27)', (await page.locator('.kdv-row').textContent()).includes('217,27'), await page.locator('.kdv-row').textContent());
  // Kola'yı azalt, geri al
  const kolaLine = page.locator('.line', { hasText: 'Kola' });
  await kolaLine.locator('[data-d="-1"]').click();
  await until(() => ![...document.querySelectorAll('.line')].some(l => l.textContent.includes('Kola')));
  check('adet 0 olunca satır kalkar', await page.locator('.line', { hasText: 'Kola' }).count() === 0);
  await btn('Geri Al');
  await until(() => [...document.querySelectorAll('.line')].some(l => l.textContent.includes('Kola')));
  check('geri al satırı geri getirir', await page.locator('.line', { hasText: 'Kola' }).count() === 1);
  await kolaLine.locator('[data-d="-1"]').click();
  await until(() => (document.querySelector('.total-amount') || {}).textContent?.includes('2.240,00'));
  total = await page.locator('.total-amount').textContent();
  check('azaltınca toplam düşer (2.240,00)', total.includes('2.240,00'), total);

  // Bas
  const before = jobs;
  await btn('Adisyonu Bas');
  const t1 = await toastText('basıldı');
  check('baskı bildirimi', t1.includes('basıldı'), t1);
  await sleep(300);
  check('adisyon yazıcıya gitti', jobs === before + 1, 'iş ' + jobs);
  await shot('08_basildi');
  await page.getByRole('button', { name: 'Tekrar Bas' }).click();
  const t2 = await toastText('kopya');
  check('ikinci baskı kopya', t2.includes('kopya'), t2);
  await prod('Çorba').click();
  await until(() => !!document.querySelector('.note.warn'));
  await sleep(100);
  check('basıldıktan sonra değişince uyarı', await page.locator('.note.warn').count() === 1);
  await shot('09_degisti_uyari');
  await btn('Hesap Ödendi');
  await page.waitForSelector('.pay-methods');
  await shot('10_odeme_popup_uyari');
  check('ödeme penceresinde üç ödeme şekli', await page.locator('.pay-btn').count() === 3);
  check('ödeme penceresi değişiklik uyarısı', (await page.locator('.modal .note.warn').textContent()).includes('değişti'));
  // Fiş ödemeden önce basılır: pencereden “Tekrar Bas”, sonra ödeme şekli.
  const beforePay = jobs;
  await page.locator('.modal [data-act="pay-print"]').click();
  await page.waitForFunction(() => document.querySelector('.pay-methods') && !document.querySelector('.modal .note.warn'), null, { timeout: 10000 }).catch(() => {});
  await sleep(300);
  check('pencereden basınca fiş gitti ve pencere geri geldi', jobs === beforePay + 1 && await page.locator('.pay-methods').count() === 1, 'iş ' + (jobs - beforePay));
  await shot('10b_odeme_popup');
  await page.locator('.pay-btn', { hasText: 'Kredi Kartı' }).click();
  const tPay = await toastText('kapatıldı');
  check('hesap kredi kartıyla kapandı', tPay.includes('Kredi Kartı'), tPay);
  await page.waitForSelector('.tiles');
  await sleep(300);
  check('hesap kapatılırken fiş basılmadı', jobs === beforePay + 1, 'iş ' + (jobs - beforePay));
  check('masa kapandı', await page.locator('[data-t="5"] .tile-sub').textContent().then(t => t.includes('Boş')));

  // Masa 4: basılmadan kapatma uyarısı, klavyeden 1 = Nakit
  await click('[data-act="open-table"][data-t="4"]');
  await page.getByRole('button', { name: 'Mehmet' }).click();
  await page.waitForSelector('.prods');
  await prod('Şiş').click(); await prod('Ayran').click();
  await until(() => (document.querySelector('.total-amount') || {}).textContent?.includes('350,00'));
  await btn('Hesap Ödendi');
  await page.waitForSelector('.pay-methods');
  check('basılmadı uyarısı', (await page.locator('.modal .note.warn').textContent()).includes('basılmadı'));
  await page.keyboard.press('1');
  const tCash = await toastText('Nakit');
  check('klavyeden 1 ile nakit kapandı', tCash.includes('Nakit') && tCash.includes('350,00'), tCash);
  await page.waitForSelector('.tiles');

  // Masa 9: iptal (şifre + sebep)
  await click('[data-act="open-table"][data-t="9"]');
  await page.getByRole('button', { name: 'Mehmet' }).click();
  await prod('Şiş').click(); await prod('Salata').click();
  await sleep(150);
  await click('[data-act="cancel"]');
  await page.waitForSelector('input[name="reason"]');
  await page.fill('#reason-other', 'Müşteri masayı değiştirdi');
  await shot('12_iptal_sebep');
  await page.getByRole('button', { name: 'Adisyonu iptal et' }).click();
  await page.waitForSelector('.keypad');
  await shot('11_sifre');
  await page.keyboard.type('1111');
  await sleep(400);
  check('yanlış şifre uyarısı', (await page.locator('#pin-err').textContent()).includes('yanlış'));
  await page.keyboard.type('2468');
  await page.waitForSelector('.tiles');
  check('iptal edilen masa boşaldı', await page.locator('[data-t="9"] .tile-sub').textContent().then(t => t.includes('Boş')));

  // Masa 14: basılmış adisyonu “−” ile boşaltmak şifre ister
  await click('[data-act="open-table"][data-t="14"]');
  await page.getByRole('button', { name: 'Ahmet' }).click();
  await prod('Ayran').click();
  await btn('Adisyonu Bas');
  await toastText('basıldı');
  await page.locator('.line', { hasText: 'Ayran' }).locator('[data-d="-1"]').click();
  await page.waitForSelector('.keypad', { timeout: 5000 }).catch(() => {});
  check('basılmış üründen düşmek şifre ister', await page.locator('.keypad').count() === 1);
  await page.keyboard.type('2468');
  await page.waitForSelector('[data-act="empty-close"]', { timeout: 5000 });
  await btn('Masayı boşalt');
  await page.waitForSelector('.keypad', { timeout: 5000 }).catch(() => {});
  check('basılmış adisyonu boşaltmak şifre ister', await page.locator('.keypad').count() === 1);
  await page.keyboard.type('2468');
  await page.waitForSelector('.tiles');

  // Masa 12: ürün sil ve boşalt
  await click('[data-act="open-table"][data-t="12"]');
  await page.getByRole('button', { name: 'Ahmet' }).click();
  await prod('Kola').click();
  await page.locator('.line', { hasText: 'Kola' }).locator('[data-d="-1"]').click();
  await sleep(150);
  await btn('Masayı boşalt');
  await page.waitForSelector('.tiles');

  // Masa 3 ve 7 açık kalsın, 3'ün baskısı hata versin
  await click('[data-act="open-table"][data-t="7"]');
  await page.getByRole('button', { name: 'Mehmet' }).click();
  for (let i = 0; i < 4; i++) await prod('Cağ').click();
  await prod('Şalgam').click(); await prod('Şalgam').click();
  await btn('Adisyonu Bas');
  await toastText('basıldı');
  await btn('Masalar');
  await page.waitForSelector('.tiles');

  // Masa taşıma: 7 → 8 (boş masa), sonra 10 → 8 birleştirme
  await click('[data-act="open-table"][data-t="7"]');
  await page.waitForSelector('.ticket');
  await click('[data-act="move-table"]');
  await page.waitForSelector('.move-grid');
  await shot('12b_masa_tasi');
  await click('[data-act="move-to"][data-to="8"]');
  const tMove = await toastText('taşındı');
  check('masa taşındı', tMove.includes('Masa 8'), tMove);
  await page.waitForSelector('.ticket');
  check('taşınan masanın ekranı açıldı', (await page.locator('.order-head h1').textContent()).includes('Masa 8'));
  await btn('Masalar');
  await page.waitForSelector('.tiles');
  check('eski masa boş, yeni masa dolu', (await page.locator('[data-t="7"] .tile-sub').textContent()).includes('Boş') && (await page.locator('[data-t="8"]').getAttribute('class')).includes('busy'));
  await click('[data-act="open-table"][data-t="10"]');
  await page.getByRole('button', { name: 'Ahmet' }).click();
  await prod('Cağ').click(); await prod('Kola').click();
  await sleep(150);
  await click('[data-act="move-table"]');
  await page.waitForSelector('.move-grid');
  await click('[data-act="move-to"][data-to="8"]');
  await page.waitForSelector('.modal .warn-list');
  await shot('12c_birlestir_onay');
  await page.getByRole('button', { name: 'Birleştir', exact: true }).click();
  const tMerge = await toastText('birleşti');
  check('masalar birleşti', tMerge.includes('birleşti'), tMerge);
  await page.waitForSelector('.ticket');
  const mergedTotal = await page.locator('.total-amount').textContent();
  check('birleşen adisyonun toplamı (1.970,00)', mergedTotal.includes('1.970,00'), mergedTotal);
  check('birleşince aynı ürün toplandı (Cağ 5)', (await page.locator('.line', { hasText: 'Cağ' }).locator('.qty').textContent()).trim() === '5');
  check('birleşen basılı adisyonda tekrar bas uyarısı', (await page.locator('.ticket .note.warn').count()) === 1);
  await btn('Masalar');
  await page.waitForSelector('.tiles');
  check('birleşen masa boşaldı', (await page.locator('[data-t="10"] .tile-sub').textContent()).includes('Boş'));

  // Ürünler: iptal şifresi yönetici girişini açmaz; Ürünler şifre ister
  await click('[data-act="nav"][data-to="urunler"]');
  await page.waitForSelector('.keypad');
  check('iptal sonrası yönetici girişi kapalı', await page.locator('.keypad').count() === 1);
  await page.keyboard.type('2468');
  await page.waitForSelector('.data');
  await shot('13_urunler');
  await page.locator('tr', { hasText: 'Cağ' }).getByRole('button', { name: 'Düzenle' }).click();
  await page.fill('#f-price', '360,00');
  await page.locator('label.radio', { hasText: '%20' }).click();
  await page.getByRole('button', { name: 'Kaydet' }).click();
  await sleep(300);
  check('fiyat güncellendi', (await page.locator('tr', { hasText: 'Cağ' }).first().textContent()).includes('360,00'));
  await page.fill('#cat-new', 'Mezeler');
  await click('[data-act="c-add"]');
  await sleep(300);
  await shot('14_urunler_duzenle');
  // Toplu zam: %10, 5 ₺ yuvarlama, önizle → uygula
  await click('[data-act="bulk-price"]');
  await page.waitForSelector('#bulk-pct');
  await page.fill('#bulk-pct', '10');
  await page.locator('label.radio', { hasText: '5 ₺' }).click();
  await page.getByRole('button', { name: 'Yeni fiyatları göster' }).click();
  await page.waitForSelector('.modal .data');
  await shot('14b_toplu_zam');
  const cagRow = await page.locator('.modal tr', { hasText: 'Cağ' }).textContent();
  check('toplu zam önizleme (360 → 395)', cagRow.includes('360,00') && cagRow.includes('395,00'), cagRow);
  check('önizlemede fiyat değişmedi', (await page.locator('.main tr', { hasText: 'Cağ' }).first().textContent()).includes('360,00'));
  await page.getByRole('button', { name: 'Uygula' }).click();
  const tBulk = await toastText('fiyatı güncellendi');
  check('toplu zam uygulandı', tBulk.includes('güncellendi'), tBulk);
  await sleep(200);
  check('yeni fiyat listede', (await page.locator('.main tr', { hasText: 'Cağ' }).first().textContent()).includes('395,00'));

  // Ayarlar
  await btn('Ayarlar');
  await page.waitForSelector('#s-insta');
  await page.fill('#s-insta', '@halis_cag'); await page.press('#s-insta', 'Tab');
  await sleep(200);
  await page.fill('#s-google', 'https://g.page/r/CbExampleReview/review'); await page.press('#s-google', 'Tab');
  await sleep(300);
  await page.waitForSelector('#sample-img', { timeout: 10000 }).catch(() => {});
  const src1 = await page.locator('#sample-img').getAttribute('src').catch(() => '');
  check('ayarlarda canlı fiş önizlemesi var', (src1 || '').startsWith('data:image/png'));
  await page.fill('#s-slogan', 'Odun ateşinde, yatay şişte · 1987');
  await page.waitForFunction(s => { const i = document.getElementById('sample-img'); return i && i.src !== s; }, src1, { timeout: 10000 }).catch(() => {});
  check('yazdıkça önizleme güncellenir (kaydetmeden)', (await page.locator('#sample-img').getAttribute('src')) !== src1);
  await page.press('#s-slogan', 'Tab');
  await sleep(300);
  // WhatsApp: eksik bilgiyle açılmaz; erişim anahtarı ekrana geri gelmez
  await page.locator('label[for="wa-enabled"]').click();
  const tWa = await toastText('WhatsApp');
  check('eksik WhatsApp ayarıyla gönderim açılmaz', tWa.includes('girin'), tWa);
  await page.fill('#wa-pnid', '123456789012345');
  await page.fill('#wa-token', 'EAAG-gizli-deneme-anahtari');
  await page.fill('#wa-template', 'gun_sonu_ozeti');
  await page.fill('#wa-rcpt', '0532 123 45 67');
  await click('[data-act="wa-save"]');
  const tWa2 = await toastText('WhatsApp ayarı');
  check('WhatsApp ayarı kaydedildi', tWa2.includes('WhatsApp'), tWa2);
  await sleep(200);
  check('WhatsApp anahtarı ekrana gelmez', !(await page.content()).includes('EAAG-gizli') && (await page.getAttribute('#wa-token', 'placeholder')).includes('Kayıtlı'));
  check('alıcı numara uluslararası biçimde', (await page.inputValue('#wa-rcpt')).includes('+905321234567'));
  await page.fill('#gs-url', 'https://example.com/exec');
  await click('[data-act="gs-save"]');
  const tGs = await toastText('Web uygulaması');
  check('geçersiz Sheets adresi reddedilir', tGs.includes('script.google.com'), tGs);
  await shot('15_ayarlar');
  await btn('Şimdi yedekle');
  const tb = await toastText('Yedek');
  check('elle yedek', tb.includes('Yedek'), tb);
  // Test fişi (önizleme)
  await page.getByRole('button', { name: 'Test fişi önizlemesi' }).click();
  await page.waitForSelector('.modal .receipt-img');
  await shot('16_ayarlar_onizleme');
  await page.keyboard.press('Escape');

  // Yazıcı bozuk: portu ölü porta çevir
  await page.fill('#set-port', String(DEAD)); await page.press('#set-port', 'Tab');
  await sleep(300);
  await btn('Masalar');
  await click('[data-act="open-table"][data-t="3"]');
  await page.getByRole('button', { name: 'Ahmet' }).click();
  await prod('Yoğurt').click();
  await btn('Adisyonu Bas');
  await page.waitForSelector('.err-box', { timeout: 10000 });
  await shot('17_baski_hatasi');
  check('hata mesajı sade', (await page.locator('.err-box').textContent()).includes('ulaşılamıyor'));
  await page.getByRole('button', { name: 'Kapat', exact: true }).last().click();
  await btn('Masalar');
  await sleep(200);
  check('masada Basılamadı işareti', (await page.locator('[data-t="3"]').textContent()).includes('Basılamadı'));
  await shot('18_masalar_dolu');

  // Yazıcı yanıt vermezken (ağda kayıp) diğer işlemler beklememeli
  await btn('Ayarlar');
  await page.waitForSelector('#set-host');
  await page.fill('#set-host', '10.255.255.1'); await page.press('#set-host', 'Tab');
  await sleep(200);
  await page.fill('#set-port', '9100'); await page.press('#set-port', 'Tab');
  await sleep(300);
  await btn('Masalar');
  await click('[data-act="open-table"][data-t="3"]');
  await page.waitForSelector('.ticket');
  await click('[data-act="print"]');
  await sleep(300);
  check('basılan adisyona o sırada ürün eklenemez', await page.locator('.prod').first().isDisabled());
  const t0 = Date.now();
  await btn('Masalar');
  await click('[data-act="open-table"][data-t="11"]');
  await page.locator('.pick', { hasText: 'Ahmet' }).click();
  await prod('Kola').click();
  await page.waitForFunction(() => [...document.querySelectorAll('.ticket .line')].some(l => l.textContent.includes('Kola')), null, { timeout: 10000 }).catch(() => {});
  const took = Date.now() - t0;
  check('yazıcı beklerken başka masada sipariş alınır', took < 2500, took + ' ms');
  await page.waitForSelector('.err-box', { timeout: 20000 });
  check('yanıt vermeyen yazıcı için sade mesaj', (await page.locator('.err-box').textContent()).includes('ulaşılamıyor'));
  await page.getByRole('button', { name: 'Kapat', exact: true }).last().click();
  await btn('Ayarlar');
  await page.waitForSelector('#set-host');
  await page.fill('#set-host', '127.0.0.1'); await page.press('#set-host', 'Tab');
  await sleep(200);
  await page.fill('#set-port', String(PRN)); await page.press('#set-port', 'Tab');
  await sleep(300);
  await btn('Masalar');
  await page.waitForSelector('.tiles');

  // Kilitle → Ürünler şifre ister
  await page.getByRole('button', { name: 'Kilitle' }).click();
  await sleep(200);
  await click('[data-act="nav"][data-to="urunler"]');
  await sleep(200);
  check('kilitli ekran şifre ister', await page.locator('.keypad').count() === 1);
  await page.keyboard.press('Escape');

  // Büyük buton ölçüleri (H8.4): garson ekranlarında 64 px altı buton olmasın.
  await click('[data-act="open-table"][data-t="8"]');
  await page.waitForSelector('.ticket');
  const small = await page.evaluate(() => [...document.querySelectorAll('.main button, .top button')].filter(b => b.offsetParent && !b.classList.contains('linkish')).map(b => ({ t: (b.getAttribute('aria-label') || b.textContent).trim().slice(0, 30), h: Math.round(b.getBoundingClientRect().height) })).filter(x => x.h < 64));
  check('sipariş ekranında 64 px altı buton yok', small.length === 0, JSON.stringify(small));
  const fonts = await page.evaluate(() => [...document.querySelectorAll('.main *')].filter(e => e.offsetParent && e.childElementCount === 0 && e.textContent.trim()).map(e => parseFloat(getComputedStyle(e).fontSize)).reduce((m, v) => Math.min(m, v), 99));
  check('sipariş ekranında yazılar 18 px ve üstü', fonts >= 18, 'en küçük ' + fonts);
  await shot('19_siparis_8');

  // Farklı ekran boyutları (6 kalemli adisyonla; basılmış, uyarısız)
  for (const n of ['Yoğurt', 'Salata', 'Soda', 'Kola']) await prod(n).click();
  await sleep(200);
  await btn('Tekrar Bas');
  await toastText('basıldı');
  await sleep(200);
  for (const [w, h] of [[1366, 768], [1280, 720], [1536, 864], [1920, 1080], [1024, 768]]) {
    await page.setViewportSize({ width: w, height: h });
    await sleep(250);
    await shot(`20_siparis_${w}x${h}`);
    const m = await page.evaluate(() => {
      const b = document.querySelector('.ticket .btn-xl'); const box = document.querySelector('.ticket .lines');
      const r = b ? b.getBoundingClientRect() : null;
      const lb = box ? box.getBoundingClientRect() : null;
      const shown = lb ? [...box.querySelectorAll('.line')].filter(li => { const q = li.getBoundingClientRect(); return q.top >= lb.top - 1 && q.bottom <= lb.bottom + 1; }).length : 0;
      return { printVisible: !!r && r.bottom <= window.innerHeight && r.top >= 0, shown, total: box ? box.querySelectorAll('.line').length : 0, wide: document.documentElement.scrollWidth <= window.innerWidth };
    });
    check(`Adisyonu Bas ${w}x${h} ekranda görünür`, m.printVisible);
    check(`${w}x${h}: yatay kaydırma yok`, m.wide);
    const want = h < 768 ? 2 : h <= 800 ? 3 : h <= 900 ? 4 : m.total;
    check(`${w}x${h}: adisyonda en az ${want} kalem birden görünür`, m.shown >= want, m.shown + '/' + m.total);
  }
  await page.setViewportSize({ width: 1366, height: 768 });
  await btn('Masalar');
  await sleep(200);
  await page.setViewportSize({ width: 1920, height: 1080 });
  await shot('21_masalar_1080');

  // Programı yeniden aç: açık masalar duruyor mu?
  await page.setViewportSize({ width: 1366, height: 768 });
  const openBefore = await page.locator('.tile.busy').count();
  restarting = true;
  app.kill('SIGTERM');
  await sleep(800);
  const app2 = spawn(BIN, ['-browser', '-no-open', '-data', OUT + '/data', '-yedek', OUT + '/yedek', '-port', String(PORT)], { env: ENV });
  app2.stdout.on('data', d => appLog.push(String(d)));
  app2.stderr.on('data', d => appLog.push(String(d)));
  for (let i = 0; i < 100; i++) { try { await fetch(`http://127.0.0.1:${PORT}/`); break; } catch { await sleep(100); } }
  await page.goto(`http://127.0.0.1:${PORT}/`);
  await page.waitForSelector('.tiles');
  restarting = false;
  check('yeniden açınca açık masalar aynen gelir', await page.locator('.tile.busy').count() === openBefore, openBefore + ' masa');

  // ---- Gün sonu ----
  await click('[data-act="nav"][data-to="gunsonu"]');
  await page.waitForSelector('.keypad');
  await page.keyboard.type('2468');
  await page.waitForSelector('.kpis');
  await sleep(300);
  await shot('22_gunsonu_acik');
  const kpiText = await page.locator('.kpis').textContent();
  check('gün sonu cirosu (2.710,00)', kpiText.includes('2.710,00'), kpiText.slice(0, 200));
  const payText = await page.locator('section', { hasText: 'Ödeme şekilleri' }).first().textContent();
  check('gün sonunda ödeme şekli dağılımı', payText.includes('Kredi Kartı') && payText.includes('2.360,00') && payText.includes('350,00'), payText);
  check('açık masa uyarısı', (await page.locator('.warn-list').textContent()).includes('hâlâ açık'));
  const rowText = await page.locator('table.data', { hasText: 'Açılış' }).first().textContent();
  check('iptaller ayrı tabloda', (await page.locator('table.data', { hasText: 'Sebep' }).first().textContent()).includes('Müşteri masayı değiştirdi'));
  check('adisyon listesinde ödeme şekli', rowText.includes('Kredi Kartı') && rowText.includes('Nakit'));
  check('listede birleşen adisyon', rowText.includes('Birleşti'));
  await page.getByRole('button', { name: 'Günü kapat' }).click();
  await page.waitForSelector('.modal .err-box');
  check('açık masa varken gün kapanmaz', (await page.locator('.modal').textContent()).includes('Açık masa'));
  await page.keyboard.press('Escape');
  // Adisyon ayrıntısı ve ödeme şeklini düzeltme (şifreyle)
  await page.locator('tr.click', { hasText: 'Kredi Kartı' }).first().click();
  await page.waitForSelector('.modal .hist');
  await shot('23_adisyon_detay');
  check('ayrıntıda ödeme şekli ve geçmiş', (await page.locator('.modal').textContent()).includes('Kredi Kartı') && (await page.locator('.modal .hist').textContent()).includes('Hesap ödendi'));
  await page.locator('.modal [data-act="change-pay"]').click();
  await page.locator('.pay-btn', { hasText: 'Havale' }).click();
  await page.waitForSelector('.keypad');
  await page.keyboard.type('2468');
  const tChg = await toastText('ödeme şekli');
  check('ödeme şekli şifreyle değişti', tChg.includes('Havale'), tChg);
  await page.waitForSelector('.modal .hist');
  check('değişiklik geçmişte', (await page.locator('.modal .hist').textContent()).includes('Ödeme şekli değişti'));
  await page.keyboard.press('Escape');
  // Açık masaları kapat, günü kapat
  await btn('Masalar');
  let dbl = false;
  for (const t of [3, 8, 11]) {
    if (!(await page.locator(`[data-t="${t}"]`).getAttribute('class')).includes('busy')) continue;
    await click(`[data-act="open-table"][data-t="${t}"]`);
    await page.waitForSelector('.ticket');
    if (!dbl) {
      // Çift tıklama: ilk tıklama ödeme penceresini açar; aynı noktaya gelen ikinci tıklama
      // (detail 2) pencerede bir şeye denk gelse de (burada “Vazgeç”) yok sayılır.
      dbl = true;
      const bb = await page.getByRole('button', { name: 'Hesap Ödendi', exact: true }).first().boundingBox();
      const cx = bb.x + bb.width / 2, cy = bb.y + bb.height / 2;
      await page.mouse.click(cx, cy);
      await page.waitForSelector('.pay-methods');
      await page.evaluate(([x, y]) => {
        const b = [...document.querySelectorAll('.modal button')].find(e => e.textContent.trim() === 'Vazgeç');
        b.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, detail: 2, clientX: x, clientY: y }));
      }, [cx, cy]);
      await sleep(500);
      check('çift tıklamanın ikinci tıklaması açılan pencerede bir şey seçmez', await page.locator('.modal .pay-methods').count() === 1);
    } else {
      await btn('Hesap Ödendi');
      await page.waitForSelector('.pay-methods');
    }
    await page.locator('.pay-btn', { hasText: 'Nakit' }).click();
    await toastText('kapatıldı');
    await page.waitForSelector('.tiles');
  }
  check('tüm masalar kapandı', await page.locator('.tile.busy').count() === 0);
  await click('[data-act="nav"][data-to="gunsonu"]');
  await page.waitForSelector('.kpis');
  await sleep(300);
  const jobsBeforeClose = jobs;
  await page.getByRole('button', { name: 'Günü kapat' }).click();
  await page.waitForSelector('#close-print');
  await shot('24_gun_kapat_onay');
  await page.locator('.modal-actions .btn-primary').click();
  const tClose = await toastText('kapatıldı');
  check('gün kapatıldı', tClose.includes('kapatıldı'), tClose);
  await sleep(1500);
  check('gün sonu fişi basıldı', jobs === jobsBeforeClose + 1, 'iş ' + (jobs - jobsBeforeClose));
  check('kapatılan gün kilitli', (await page.locator('.main').textContent()).includes('değiştirilemez'));
  await shot('25_gun_kapandi', true);
  const dayCopies = fs.existsSync(OUT + '/yedek/ozel') ? fs.readdirSync(OUT + '/yedek/ozel').filter(n => n.includes('gun_sonu')) : [];
  check('gün sonu yedeği alındı', dayCopies.length === 1, dayCopies.join(','));

  // ---- İstatistik ----
  await click('[data-act="nav"][data-to="istatistik"]');
  await page.waitForSelector('.kpis');
  await page.waitForSelector('.chart svg', { timeout: 10000 }).catch(() => {});
  await sleep(400);
  await shot('26_istatistik', true);
  const stText = await page.locator('.main').textContent();
  check('istatistik ödeme şekilleri', stText.includes('Ödeme şekilleri') && stText.includes('Havale'));
  check('istatistikte grafik çizildi', await page.locator('.chart svg').count() >= 2);
  await page.getByRole('button', { name: 'Excel’e aktar' }).click();
  await page.waitForSelector('.modal .kv', { timeout: 15000 }).catch(() => {});
  const xl = (await page.locator('.modal .kv dd').first().textContent().catch(() => '')).trim();
  const xlHost = hostPath(xl);
  check('Excel dosyası yazıldı', xl.endsWith('.xlsx') && fs.existsSync(xlHost), xl);
  if (xl && fs.existsSync(xlHost)) fs.copyFileSync(xlHost, OUT + '/rapor.xlsx');
  await page.keyboard.press('Escape');

  // ---- İşlem kaydı ----
  await click('[data-act="nav"][data-to="gunsonu"]');
  await page.waitForSelector('.kpis');
  await page.locator('[data-act="nav"][data-to="kayit"]').first().click();
  await page.waitForSelector('.data', { timeout: 10000 });
  const logText = await page.locator('.data').textContent();
  check('işlem kaydında ödeme, zam ve gün sonu', logText.includes('Hesap ödendi') && logText.includes('Toplu zam') && logText.includes('Gün kapatıldı'), logText.slice(0, 120));
  await shot('27_islem_kaydi');

  // ---- Yedekten geri yükleme (program yeniden başlar) ----
  // Gün kapandıktan sonra yeni güne bir adisyon açılır; gün sonu yedeği geri yüklenince
  // o adisyon ekrandan kalkar ama numarası tekrar kullanılmaz.
  await btn('Masalar');
  await click('[data-act="open-table"][data-t="1"]');
  await page.getByRole('button', { name: 'Ahmet' }).click();
  await prod('Ayran').click();
  await sleep(200);
  await btn('Masalar');
  const token = () => page.evaluate(() => document.querySelector('meta[name="halis-token"]').content);
  const nextBefore = (await (await fetch(`http://127.0.0.1:${PORT}/api/state`, { method: 'POST', headers: { 'X-Halis-Token': await token() } })).json()).state.nextNo;
  await btn('Ayarlar');
  await page.waitForSelector('[data-act="restore-open"]');
  await click('[data-act="restore-open"]');
  await page.waitForSelector('[data-act="restore-pick"]');
  await page.locator('.modal details', { hasText: 'Gün sonu' }).locator('summary').click();
  await shot('28_geri_yukle_liste');
  await page.locator('.modal details', { hasText: 'Gün sonu' }).locator('[data-act="restore-pick"]').first().click();
  await page.getByRole('button', { name: 'Geri yükle', exact: true }).click();
  await page.waitForSelector('.keypad');
  restarting = true;
  await page.keyboard.type('2468');
  await page.waitForSelector('.restart', { timeout: 15000 }).catch(() => {});
  check('geri yüklemede yeniden başlatma ekranı', await page.locator('.restart').count() === 1);
  // Önce eski program kapanır, sonra yenisi açılır.
  for (let i = 0; i < 100; i++) { try { await fetch(`http://127.0.0.1:${PORT}/`); await sleep(100); } catch { break; } }
  let up = false;
  for (let i = 0; i < 150; i++) { await sleep(200); try { const r = await fetch(`http://127.0.0.1:${PORT}/`); if (r.ok) { up = true; break; } } catch { } }
  await sleep(500);
  await page.goto(`http://127.0.0.1:${PORT}/`);
  await page.waitForSelector('.tiles', { timeout: 15000 }).catch(() => {});
  restarting = false;
  await shot('29_geri_yuklendi');
  check('program yeniden açıldı', up && await page.locator('.tiles').count() === 1);
  check('geri yükleme bildirimi', (await page.locator('.note.info').textContent().catch(() => '')).includes('Yedek geri yüklendi'));
  const st2 = (await (await fetch(`http://127.0.0.1:${PORT}/api/state`, { method: 'POST', headers: { 'X-Halis-Token': await token() } })).json()).state;
  check('gün sonundan sonraki adisyon geri yüklemeyle kalktı', st2.open.length === 0, st2.open.length + ' açık');
  check('geri yüklemeden sonra numara geri düşmez', st2.nextNo >= nextBefore, st2.nextNo + ' / ' + nextBefore);
  check('eski veri kenara alındı', fs.existsSync(OUT + '/data/eski_veri') && fs.readdirSync(OUT + '/data/eski_veri').length === 1);
  check('geri yükleme öncesi yedeği var', fs.readdirSync(OUT + '/yedek/ozel').some(n => n.includes('geri_yukleme_oncesi')));
  killAppsUsing(OUT + '/data');
  app2.kill('SIGTERM');

  check('sayfada JavaScript hatası yok', errors.length === 0, errors.join(' | '));
  await browser.close();
  printer.close();
  fs.writeFileSync(OUT + '/app.log', appLog.join(''));
  const failed = checks.filter(c => !c.ok);
  console.log(`\n${checks.length - failed.length}/${checks.length} kontrol geçti; yazıcıya ${jobs} iş gitti.`);
  for (const c of checks) console.log((c.ok ? 'ok  ' : 'FAIL') + ' ' + c.name + (c.detail && !c.ok ? ' — ' + c.detail : ''));
  process.exit(failed.length ? 1 : 0);
})().catch(e => { console.error(e); fs.writeFileSync(OUT + '/app.log', appLog.join('')); app.kill(); killAppsUsing(OUT + '/data'); process.exit(2); });
