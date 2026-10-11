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

const app = spawn(BIN, ['-browser', '-no-open', '-data', OUT + '/data', '-yedek', OUT + '/yedek', '-port', String(PORT)], { env: { ...process.env, HALIS_LOG_STDERR: '1' } });
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
  const prod = name => page.locator('.prod', { hasText: name }).first();
  for (let i = 0; i < 6; i++) await prod('Cağ').click();
  await prod('Ayran').click(); await prod('Ayran').click(); await prod('Ayran').click();
  await prod('Büyük Su').click();
  await prod('Kola').click();
  await sleep(200);
  await shot('07_siparis');
  let total = await page.locator('.total-amount').textContent();
  check('toplam doğru (2.390,00)', total.includes('2.390,00'), total);
  check('KDV satırı doğru (217,27)', (await page.locator('.kdv-row').textContent()).includes('217,27'), await page.locator('.kdv-row').textContent());
  // Kola'yı azalt, geri al
  const kolaLine = page.locator('.line', { hasText: 'Kola' });
  await kolaLine.locator('[data-d="-1"]').click();
  await sleep(150);
  check('adet 0 olunca satır kalkar', await page.locator('.line', { hasText: 'Kola' }).count() === 0);
  await btn('Geri Al');
  await sleep(150);
  check('geri al satırı geri getirir', await page.locator('.line', { hasText: 'Kola' }).count() === 1);
  await kolaLine.locator('[data-d="-1"]').click();
  await sleep(150);
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
  await sleep(200);
  check('basıldıktan sonra değişince uyarı', await page.locator('.note.warn').count() === 1);
  await shot('09_degisti_uyari');
  await btn('Hesap Ödendi');
  await page.waitForSelector('.modal');
  await shot('10_odeme_uyari');
  await page.getByRole('button', { name: 'Tekrar bas ve kapat' }).click();
  await page.waitForSelector('.tiles');
  await sleep(300);
  check('masa kapandı', await page.locator('[data-t="5"] .tile-sub').textContent().then(t => t.includes('Boş')));

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

  // Ayarlar
  await btn('Ayarlar');
  await page.waitForSelector('#s-insta');
  await page.fill('#s-insta', '@halis_cag'); await page.press('#s-insta', 'Tab');
  await sleep(200);
  await page.fill('#s-google', 'https://g.page/r/CbExampleReview/review'); await page.press('#s-google', 'Tab');
  await sleep(300);
  await shot('15_ayarlar');
  await btn('Şimdi yedekle');
  const tb = await toastText('Yedek');
  check('elle yedek', tb.includes('Yedek'), tb);
  // Test fişi (önizleme)
  await page.getByRole('button', { name: 'Fiş önizlemesi' }).click();
  await page.waitForSelector('.receipt-img');
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
  await click('[data-act="open-table"][data-t="7"]');
  await page.waitForSelector('.ticket');
  const small = await page.evaluate(() => [...document.querySelectorAll('.main button, .top button')].filter(b => b.offsetParent && !b.classList.contains('linkish')).map(b => ({ t: (b.getAttribute('aria-label') || b.textContent).trim().slice(0, 30), h: Math.round(b.getBoundingClientRect().height) })).filter(x => x.h < 64));
  check('sipariş ekranında 64 px altı buton yok', small.length === 0, JSON.stringify(small));
  const fonts = await page.evaluate(() => [...document.querySelectorAll('.main *')].filter(e => e.offsetParent && e.childElementCount === 0 && e.textContent.trim()).map(e => parseFloat(getComputedStyle(e).fontSize)).reduce((m, v) => Math.min(m, v), 99));
  check('sipariş ekranında yazılar 18 px ve üstü', fonts >= 18, 'en küçük ' + fonts);
  await shot('19_siparis_7');

  // Farklı ekran boyutları (6 kalemli adisyonla)
  for (const n of ['Yoğurt', 'Salata', 'Soda', 'Kola']) await prod(n).click();
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
  const app2 = spawn(BIN, ['-browser', '-no-open', '-data', OUT + '/data', '-yedek', OUT + '/yedek', '-port', String(PORT)]);
  for (let i = 0; i < 100; i++) { try { await fetch(`http://127.0.0.1:${PORT}/`); break; } catch { await sleep(100); } }
  await page.goto(`http://127.0.0.1:${PORT}/`);
  await page.waitForSelector('.tiles');
  restarting = false;
  check('yeniden açınca açık masalar aynen gelir', await page.locator('.tile.busy').count() === openBefore, openBefore + ' masa');
  app2.kill('SIGTERM');

  check('sayfada JavaScript hatası yok', errors.length === 0, errors.join(' | '));
  await browser.close();
  printer.close();
  fs.writeFileSync(OUT + '/app.log', appLog.join(''));
  const failed = checks.filter(c => !c.ok);
  console.log(`\n${checks.length - failed.length}/${checks.length} kontrol geçti; yazıcıya ${jobs} iş gitti.`);
  for (const c of checks) console.log((c.ok ? 'ok  ' : 'FAIL') + ' ' + c.name + (c.detail && !c.ok ? ' — ' + c.detail : ''));
  process.exit(failed.length ? 1 : 0);
})().catch(e => { console.error(e); fs.writeFileSync(OUT + '/app.log', appLog.join('')); app.kill(); process.exit(2); });
