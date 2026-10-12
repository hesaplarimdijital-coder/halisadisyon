/* Halis Adisyon · arayüz
   Tüm veriler programın yerel sunucusundadır (SQLite). Bu dosya yalnızca ekranı çizer
   ve her dokunuşu sunucuya iletir; “Kaydet” butonu yoktur. */
(() => {
'use strict';

const TOKEN = document.querySelector('meta[name="halis-token"]').content;
const VERSION = document.querySelector('meta[name="halis-version"]').content;

// ---------- Biçimlendirme ----------
const nf2 = new Intl.NumberFormat('tr-TR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
const TL = k => nf2.format(k / 100) + '\u00a0₺';
const N2 = k => nf2.format(k / 100);
const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const pad = n => String(n).padStart(2, '0');
const hm = iso => { if (!iso) return ''; const d = new Date(iso); return pad(d.getHours()) + ':' + pad(d.getMinutes()); };
const AY = ['Ocak', 'Şubat', 'Mart', 'Nisan', 'Mayıs', 'Haziran', 'Temmuz', 'Ağustos', 'Eylül', 'Ekim', 'Kasım', 'Aralık'];
const GUN = ['Pazar', 'Pazartesi', 'Salı', 'Çarşamba', 'Perşembe', 'Cuma', 'Cumartesi'];
const clockText = () => { const d = new Date(); return GUN[d.getDay()] + ', ' + d.getDate() + ' ' + AY[d.getMonth()] + ' · ' + pad(d.getHours()) + ':' + pad(d.getMinutes()); };
// "2026-10-11" → Date (yerel), "11 Ekim 2026 Pazar"
const parseDay = d => { const [y, m, dd] = String(d).split('-').map(Number); return new Date(y, (m || 1) - 1, dd || 1); };
const dayStr = dt => dt.getFullYear() + '-' + pad(dt.getMonth() + 1) + '-' + pad(dt.getDate());
const addDays = (d, n) => { const t = parseDay(d); t.setDate(t.getDate() + n); return dayStr(t); };
const dateLong = d => { if (!d) return ''; const t = parseDay(d); return t.getDate() + ' ' + AY[t.getMonth()] + ' ' + t.getFullYear() + ' ' + GUN[t.getDay()]; };
const dateShort = d => { if (!d) return ''; const t = parseDay(d); return pad(t.getDate()) + '.' + pad(t.getMonth() + 1) + '.' + t.getFullYear(); };
const dmShort = d => { const t = parseDay(d); return t.getDate() + ' ' + AY[t.getMonth()].slice(0, 3); };
const stampText = iso => { if (!iso) return ''; const d = new Date(iso); return pad(d.getDate()) + '.' + pad(d.getMonth() + 1) + '.' + d.getFullYear() + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes()); };
const nf0 = new Intl.NumberFormat('tr-TR', { maximumFractionDigits: 0 });
const nf1 = new Intl.NumberFormat('tr-TR', { minimumFractionDigits: 1, maximumFractionDigits: 1 });
const TL0 = k => nf0.format(Math.round(k / 100)) + '\u00a0₺';
const payLabel = m => { const p = (S && S.paymentMethods || []).find(x => x.id === m); return p ? p.label : (m ? m : 'Belirtilmemiş'); };
const sinceText = iso => { const m = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60000)); return m < 1 ? 'az önce' : m < 60 ? m + ' dk' : Math.floor(m / 60) + ' sa ' + (m % 60) + ' dk'; };
// Sayıya Türkçe ek: 9’a, 12’ye (son okunan kelimeye göre)
const SFX1 = [null, ['e', 'den'], ['ye', 'den'], ['e', 'ten'], ['e', 'ten'], ['e', 'ten'], ['ya', 'dan'], ['ye', 'den'], ['e', 'den'], ['a', 'dan']];
const SFX10 = [null, ['a', 'dan'], ['ye', 'den'], ['a', 'dan'], ['a', 'tan'], ['ye', 'den'], ['a', 'tan'], ['e', 'ten'], ['e', 'den'], ['a', 'dan']];
const numSfx = n => { n = Math.abs(Math.round(Number(n) || 0)); if (n % 10) return SFX1[n % 10]; if (n % 100) return SFX10[(n % 100) / 10]; if (n === 0) return ['a', 'dan']; if (n % 1e6) return ['e', 'den']; return ['a', 'dan']; };
const dat = n => n + '’' + numSfx(n)[0];
// Bulunma eki: 8’de, 6’da, 3’te (ayrılma ekinin “n”siz hali).
const loc = n => n + '’' + numSfx(n)[1].slice(0, -1);
// "340", "340,50", "1.250", "1.250,00", "12.5" → kuruş
const parsePrice = str => {
  let s = String(str == null ? '' : str).trim().replace(/\s|₺|tl/gi, '');
  if (!s) return NaN;
  if (s.includes(',')) s = s.replace(/\./g, '').replace(',', '.');
  else if (/^\d{1,3}(\.\d{3})+$/.test(s)) s = s.replace(/\./g, '');
  if (!/^\d+(\.\d{1,2})?$/.test(s)) return NaN;
  return Math.round(Number(s) * 100);
};
const priceText = k => (k > 0 ? nf2.format(k / 100) : '');
const val = id => { const el = document.getElementById(id); return el ? el.value : ''; };
const checked = id => { const el = document.getElementById(id); return !!(el && el.checked); };

// ---------- Simgeler ----------
const ICONS = {
  grid: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
  list: '<path d="M9 6h11M9 12h11M9 18h11M4 6h.01M4 12h.01M4 18h.01"/>',
  sliders: '<path d="M4 7h9M17 7h3M4 17h3M11 17h9"/><circle cx="15" cy="7" r="2"/><circle cx="9" cy="17" r="2"/>',
  lock: '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
  unlock: '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 7.5-2"/>',
  printer: '<path d="M7 9V3h10v6"/><rect x="3" y="9" width="18" height="8" rx="2"/><path d="M7 14h10v7H7z"/>',
  back: '<path d="M19 12H5M11 18l-6-6 6-6"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  minus: '<path d="M5 12h14"/>',
  undo: '<path d="M9 14L4 9l5-5"/><path d="M4 9h10a6 6 0 0 1 0 12h-3"/>',
  check: '<path d="M20 6L9 17l-5-5"/>',
  alert: '<path d="M12 3l9.5 17h-19z"/><path d="M12 10v4M12 17.5v.01"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8v.01"/>',
  x: '<path d="M6 6l12 12M18 6L6 18"/>',
  user: '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>',
  cash: '<rect x="2" y="6" width="20" height="12" rx="2"/><circle cx="12" cy="12" r="3"/>',
  upload: '<path d="M12 21V9M7 14l5-5 5 5M5 3h14"/>',
  up: '<path d="M6 15l6-6 6 6"/>',
  down: '<path d="M6 9l6 6 6-6"/>',
  receipt: '<path d="M6 3h12v18l-3-2-3 2-3-2-3 2z"/><path d="M9 8h6M9 12h6"/>',
  folder: '<path d="M3 6a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
  refresh: '<path d="M20 11a8 8 0 1 0-2.3 5.7"/><path d="M20 4v7h-7"/>',
  min: '<path d="M5 18h14"/>',
  expand: '<path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5"/>',
  power: '<path d="M12 3v9"/><path d="M6.3 7.1a8 8 0 1 0 11.4 0"/>',
  trash: '<path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3"/>',
  eye: '<path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>',
  save: '<path d="M5 3h11l3 3v15H5z"/><path d="M8 3v6h8V3M8 21v-7h8v7"/>',
  chart: '<path d="M4 20V10M10 20V4M16 20v-7M22 20H2"/>',
  moon: '<path d="M20 14.5A8 8 0 0 1 9.5 4 8 8 0 1 0 20 14.5z"/>',
  card: '<rect x="2" y="5" width="20" height="14" rx="2"/><path d="M2 10h20M6 15h4"/>',
  bank: '<path d="M3 10l9-6 9 6M5 10v8M10 10v8M14 10v8M19 10v8M3 20h18"/>',
  move: '<path d="M7 7h13M16 3l4 4-4 4"/><path d="M17 17H4M8 13l-4 4 4 4"/>',
  percent: '<path d="M19 5L5 19"/><circle cx="7" cy="7" r="2.5"/><circle cx="17" cy="17" r="2.5"/>',
  history: '<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5M12 7v5l3 3"/>',
  download: '<path d="M12 3v12M7 10l5 5 5-5M5 21h14"/>',
  left: '<path d="M15 18l-6-6 6-6"/>',
  right: '<path d="M9 18l6-6-6-6"/>',
  send: '<path d="M22 2L11 13"/><path d="M22 2l-7 20-4-9-9-4z"/>',
  copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/>',
  cloud: '<path d="M7 18a5 5 0 1 1 .9-9.9A6 6 0 0 1 19 10a4 4 0 0 1 0 8z"/>'
};
const ic = (n, s = 22, sw = 2, cls = '') => '<svg class="ic ' + cls + '" width="' + s + '" height="' + s + '" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="' + sw + '" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' + (ICONS[n] || '') + '</svg>';

// ---------- Durum ----------
let S = null;                 // programdan gelen güncel durum
let unlockDeadline = 0;
const LOCKED = new Set(['gunsonu', 'istatistik', 'kayit', 'urunler', 'ayarlar']);
const PAY_ICON = { nakit: 'cash', kart: 'card', havale: 'bank' };
const REASONS = ['Yanlış masa', 'Müşteri vazgeçti', 'Hatalı giriş'];
const ui = {
  route: 'masalar', table: null,
  drafts: {},                 // masa → {waiterId, openedAt}: ilk ürün eklenene kadar
  catTab: 'all',
  busy: null,                 // "print:NO" | "pay:NO"
  editProduct: null,          // null | 'new' | id
  prodFilter: 'all',
  printers: null,             // {printers, default, guess}
  testResult: null,           // son test fişi sonucu
  setup: null,                // kurulum sihirbazı verisi
  lastTouch: 0,
  day: { date: '', data: null, loading: false, filter: 'all' },     // gün sonu
  stats: { mode: 'month', from: '', to: '', data: null, loading: false, product: 0, sort: { key: 'date', dir: 1 } },
  audit: { from: '', to: '', kind: '', no: '', rows: null, more: false, loading: false },
  sample: { image: '', timer: 0, seq: 0 }, // ayarlardaki canlı fiş önizlemesi
  bulk: null                  // toplu zam formu
};

function setState(st) {
  S = st;
  if (!st.setupDone) unlockDeadline = Infinity;
  else unlockDeadline = st.unlocked ? Date.now() + (st.unlockLeftMs || 0) : 0;
}
const isUnlocked = () => Date.now() < unlockDeadline;
const shortName = () => ((S && S.business.name) || 'Adisyon').trim().split(/\s+/)[0];
const catById = id => S.categories.find(c => c.id === id);
const colorHex = id => (S.colors.find(c => c.id === id) || S.colors[0]).hex;
const productById = id => S.products.find(p => p.id === id);
const activeFor = t => S.open.find(a => a.table === t);
const waiterName = id => { const w = S.waiters.find(x => x.id === id); return w ? w.name : ''; };

// Programdaki Breakdown ile aynı kuruşlar: indirim orana düşen tutardan yarım
// yukarı yuvarlanarak düşülür, KDV indirimli tutardan hesaplanır.
const discountOf = (gross, pct) => !(pct > 0) || gross <= 0 ? 0 : pct >= 100 ? gross : Math.floor((gross * pct + 50) / 100);
const vatOf = (gross, r) => gross <= 0 || r <= 0 ? 0 : Math.floor((gross * r * 2 + (100 + r)) / (2 * (100 + r)));
function vatBreakdown(lines, pct) {
  const by = {};
  for (const l of lines) by[l.vat] = (by[l.vat] || 0) + l.price * l.qty;
  return Object.keys(by).map(Number).sort((a, b) => a - b).map(r => { const d = discountOf(by[r], pct || 0); return { rate: r, gross: by[r] - d, discount: d, vat: vatOf(by[r] - d, r) }; });
}

// ---------- Program ile konuşma ----------
class ApiError extends Error { constructor(r) { super(r.error || 'Bir sorun oldu.'); this.code = r.code || ''; } }
// Sipariş işlemleri tıklama sırasıyla gider (zincir). Baskı ve hesap kapatma kendinden
// önceki tıklamaları bekler ama sonrakileri bekletmez; yazıcı takılsa da diğer masalar
// çalışır. Durum, bağlantı ve pencere istekleri hiç beklemez.
let chain = Promise.resolve();
const AFTER_PRIOR = new Set(['print', 'pay', 'testPrint', 'backupNow', 'printDay', 'samplePrint', 'exportExcel', 'restore', 'testRemote']);
// Yalnız okuyan istekler sırayı beklemez.
const FREE = new Set(['ping', 'touch', 'window', 'state', 'daySummary', 'stats', 'auditLog', 'detail', 'backups', 'dayPreview', 'samplePreview', 'driveDirs', 'preview', 'printers']);
function request(name, args) {
  const ctl = new AbortController();
  const timer = setTimeout(() => ctl.abort(), AFTER_PRIOR.has(name) ? 90000 : 25000);
  return fetch('/api/' + name, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Halis-Token': TOKEN },
    body: JSON.stringify(args || {}),
    signal: ctl.signal
  }).then(r => r.json()).finally(() => clearTimeout(timer));
}
function rawApi(name, args) {
  if (FREE.has(name)) return request(name, args);
  const p = chain.then(() => request(name, args));
  if (!AFTER_PRIOR.has(name)) chain = p.catch(() => {});
  return p;
}
let offline = false;
let restarting = false;       // yedekten geri yükleme: program yeniden açılıyor
let lastSeq = 0, lastBoot = 0;
// Geç gelen eski bir yanıt ekrandaki yeni durumun üstüne yazılmasın.
function applyState(st) {
  if (st.boot !== lastBoot) { lastBoot = st.boot; lastSeq = 0; }
  if (st.seq && st.seq < lastSeq) return false;
  lastSeq = st.seq || lastSeq;
  setState(st);
  return true;
}
async function api(name, args, opts) {
  opts = opts || {};
  let res;
  try { res = await rawApi(name, args); }
  catch (e) {
    if (e && e.name === 'AbortError') {
      // İşlem programda yine de yapılmış olabilir: ekranı tazele, kullanıcı tekrar basmadan baksın.
      request('state').then(r => { if (r && r.state && applyState(r.state)) render(); }).catch(() => {});
      throw new ApiError({ error: 'Program geç yanıt verdi. İşlem yapılmış olabilir; tekrar basmadan önce ekrandaki adisyonu kontrol edin.', code: 'timeout' });
    }
    connectionLost(); throw new ApiError({ error: 'Programla bağlantı kesildi.', code: 'offline' });
  }
  if (offline) connectionBack();
  if (res.state && applyState(res.state) && !res.ok) render();
  if (!res.ok) {
    if ((res.code === 'locked' || res.code === 'pin_required') && !opts.noPin) {
      // locked: yönetici girişi açılıp işlem tekrarlanır.
      // pin_required: bu tek işlem için şifre sorulur (iptal gibi); giriş açılmaz.
      const single = res.code === 'pin_required';
      return new Promise((resolve, reject) => {
        // Giriş açıldıktan sonra işlem, girilen şifreyle tekrarlanır: işlem ayrıca
        // şifre istiyorsa (geri yükleme gibi) ikinci kez sorulmaz.
        askPin((r, pin) => single ? resolve(r) : api(name, pin ? Object.assign({}, args, { pin }) : args, Object.assign({}, opts, { noPin: true })).then(resolve, reject),
          single ? res.error : (opts.why || 'Bu işlem yönetici şifresi ister.'),
          () => reject(new ApiError({ error: '', code: 'pin_cancelled' })),
          single ? pin => api(name, Object.assign({}, args, { pin }), Object.assign({}, opts, { noPin: true })) : null);
      });
    }
    throw new ApiError(res);
  }
  return res;
}
// Aynı düğmeye iki kez basılınca işlem iki kez yapılmasın (kaydet, ekle...).
const pending = new Set();
async function once(key, fn) {
  if (pending.has(key)) return null;
  pending.add(key);
  try { return await fn(); } finally { pending.delete(key); }
}
// act hata olursa kısa bir bildirim gösterir ve null döner.
async function act(name, args, opts) {
  try { return await api(name, args, opts); }
  catch (e) {
    if (e instanceof ApiError && e.code !== 'pin_cancelled' && e.code !== 'offline' && e.message) toast(e.message, 'bad');
    return null;
  }
}
function connectionLost() {
  if (offline || restarting) return;
  offline = true;
  const d = document.createElement('div');
  d.className = 'offline'; d.id = 'offline';
  d.innerHTML = '<div class="modal"><div class="modal-head"><h2>Programla bağlantı kesildi</h2></div><div class="modal-body"><p>Program kapanmış olabilir. Masalar ve adisyonlar kayıtlı; kaybolan bir şey yok.</p><p class="muted">Programı masaüstündeki <strong>Halis Adisyon</strong> simgesinden yeniden açın.</p></div><div class="modal-actions"><button type="button" class="btn btn-primary" data-act="reload">Yeniden dene</button></div></div>';
  document.body.appendChild(d);
}
function connectionBack() {
  offline = false;
  const d = document.getElementById('offline'); if (d) d.remove();
}

// ---------- Pencere ve bildirim ----------
let modal = null;
// Pencere açılıp kapanırken çift tıklamanın ikinci tıklaması yanlış yere düşmesin.
// Çift tıklama koruması: bir pencereyi açan/kapatan tıklamanın hemen ardından AYNI
// noktaya gelen ikinci tıklama (çift tıklamanın devamı) yok sayılır. Başka bir yere
// yapılan hızlı tıklamalar (ör. ürüne art arda basmak) etkilenmez.
let lastClick = { x: -99, y: -99, t: 0 };
let clickGuard = null;
const guardClicks = () => {
  const now = Date.now();
  clickGuard = now - lastClick.t < 1500 ? { until: now + 380, x: lastClick.x, y: lastClick.y } : null;
};
function openModal(o) {
  guardClicks();
  const root = document.getElementById('modal-root');
  const prevFocus = modal ? modal.returnFocus : document.activeElement;
  // Açık pencerenin yerine yenisi geliyorsa eskisi “vazgeçildi” sayılır (bekleyen işlem takılı kalmasın).
  if (modal && modal.onClose) { const f = modal.onClose; modal.onClose = null; setTimeout(f, 0); }
  modal = { actions: o.actions || [], onClose: o.onClose, returnFocus: prevFocus, kind: o.kind || '' };
  const acts = modal.actions.map((a, i) => '<button type="button" class="btn ' + (a.kind || '') + '" data-act="modal-action" data-i="' + i + '"' + (a.disabled ? ' disabled' : '') + (a.id ? ' id="' + a.id + '"' : '') + '>' + (a.icon ? ic(a.icon) : '') + esc(a.label) + '</button>').join('');
  root.innerHTML = '<div class="scrim" data-act="modal-scrim"><div class="modal' + (o.wide ? ' wide' : '') + '" role="dialog" aria-modal="true" aria-labelledby="modal-title"><div class="modal-head"><h2 id="modal-title">' + esc(o.title) + '</h2><button type="button" class="icon-btn" data-act="modal-close" aria-label="Kapat">' + ic('x') + '</button></div><div class="modal-body">' + o.body + '</div>' + (acts ? '<div class="modal-actions">' + acts + '</div>' : '') + '</div></div>';
  root.hidden = false;
  document.body.classList.add('modal-open');
  const first = o.focusClose ? root.querySelector('[data-act="modal-close"]') : (root.querySelector('[data-autofocus]') || root.querySelector('.modal-actions .btn-primary') || root.querySelector('.modal-body button, .modal-body input') || root.querySelector('button'));
  if (first) first.focus({ preventScroll: true });
  const box = root.querySelector('.modal'); if (box && !root.querySelector('[data-autofocus]')) box.scrollTop = 0;
}
function closeModal(silent) {
  if (!modal) return;
  guardClicks();
  const m = modal; modal = null;
  const root = document.getElementById('modal-root'); root.innerHTML = ''; root.hidden = true;
  document.body.classList.remove('modal-open');
  if (!silent && m.onClose) m.onClose();
  if (m.returnFocus && document.contains(m.returnFocus)) m.returnFocus.focus({ preventScroll: true });
}
function toast(msg, kind) {
  if (!msg) return;
  const box = document.getElementById('toasts'); const el = document.createElement('div');
  el.className = 'toast' + (kind ? ' ' + kind : ''); el.textContent = msg; box.appendChild(el);
  while (box.children.length > 2) box.firstChild.remove();
  setTimeout(() => { el.classList.add('out'); setTimeout(() => el.remove(), 300); }, kind === 'bad' ? 4200 : 2200);
}

// ---------- Yönetici şifresi ----------
function askPin(onOk, why, onCancel, verify) {
  let val = ''; let busy = false;
  const dots = () => [0, 1, 2, 3].map(i => '<span class="pin-dot' + (i < val.length ? ' on' : '') + '"></span>').join('');
  const keys = [1, 2, 3, 4, 5, 6, 7, 8, 9].map(n => '<button type="button" class="key" data-act="pin-key" data-k="' + n + '">' + n + '</button>').join('');
  openModal({
    title: 'Yönetici şifresi',
    kind: 'pin',
    body: '<p class="muted">' + esc(why || 'Bu ekran yönetici şifresiyle açılır.') + '</p><div class="pin-dots" id="pin-dots">' + dots() + '</div><p class="pin-err" id="pin-err" role="alert"></p><div class="keypad">' + keys + '<button type="button" class="key quiet" data-act="pin-key" data-k="C">Temizle</button><button type="button" class="key" data-act="pin-key" data-k="0" data-autofocus>0</button><button type="button" class="key quiet" data-act="pin-key" data-k="B" aria-label="Son rakamı sil">' + ic('back') + '</button></div><p class="muted small center">Klavyeden de yazabilirsiniz.</p>',
    actions: [{ label: 'Vazgeç', onClick: () => closeModal() }],
    onClose: onCancel
  });
  modal.pin = {
    get: () => val,
    set: async v => {
      if (busy) return;
      val = v;
      const d = document.getElementById('pin-dots'); if (d) d.innerHTML = dots();
      if (val.length < 4) return;
      busy = true;
      try {
        const res = verify ? await verify(val) : await api('unlock', { pin: val }, { noPin: true });
        const entered = val;
        closeModal(true);
        render();
        onOk(res, entered);
      } catch (e) {
        const er = document.getElementById('pin-err'); if (er) er.textContent = e.message || 'Şifre yanlış.';
        val = '';
        setTimeout(() => { const d2 = document.getElementById('pin-dots'); if (d2) d2.innerHTML = dots(); }, 200);
      } finally { busy = false; }
    }
  };
}

// ---------- Gezinme ----------
function go(name, table) {
  if (LOCKED.has(name) && !isUnlocked()) { askPin(() => go(name, table)); return; }
  if (ui.route === 'siparis' && !(name === 'siparis' && table === ui.table)) dropEmptyDraft(ui.table);
  const prev = ui.route;
  ui.route = name; ui.table = table == null ? null : table;
  if (name === 'ayarlar' && !ui.printers) loadPrinters();
  if (name === 'gunsonu') loadDay(table || (prev === name || prev === 'kayit' ? ui.day.date : ''));
  if (name === 'istatistik' && (prev !== name || !ui.stats.data)) loadStats();
  if (name === 'kayit') { if (table) { ui.audit.no = String(table); ui.audit.from = ''; ui.audit.to = ''; } loadAudit(); }
  if (name === 'ayarlar' && prev !== name) { ui.sample.image = ''; setTimeout(refreshSample, 0); }
  render();
  window.scrollTo(0, 0);
}
function dropEmptyDraft(t) { if (t != null && ui.drafts[t] && !activeFor(t)) delete ui.drafts[t]; }

// ---------- Ekranlar ----------
function header() {
  if (!S.setupDone) {
    return '<header class="top" id="top"><div class="top-in"><div class="brand"><span class="brand-name">' + esc(shortName()) + '</span><span class="brand-sub">Adisyon · İlk kurulum</span></div>' + winButtons() + '</div></header>';
  }
  const items = [['masalar', 'grid', 'Masalar'], ['gunsonu', 'moon', 'Gün Sonu'], ['istatistik', 'chart', 'İstatistik'], ['urunler', 'list', 'Ürünler'], ['ayarlar', 'sliders', 'Ayarlar']];
  const nav = items.map(([k, i, l]) => {
    const on = ui.route === k || (k === 'masalar' && ui.route === 'siparis') || (k === 'gunsonu' && ui.route === 'kayit');
    const lock = LOCKED.has(k) && !isUnlocked() ? ic('lock', 18, 2, 'lockic') : '';
    return '<button type="button" class="nav-btn' + (on ? ' on' : '') + '" data-act="nav" data-to="' + k + '" data-fk="nav-' + k + '"' + (on ? ' aria-current="page"' : '') + (lock ? ' aria-label="' + l + ', yönetici şifresi gerekir"' : '') + '>' + ic(i) + '<span>' + l + '</span>' + lock + '</button>';
  }).join('');
  const lockNow = isUnlocked() ? '<button type="button" class="nav-btn quiet" data-act="lock-now" data-fk="lock-now">' + ic('unlock', 20) + '<span>Kilitle</span></button>' : '';
  return '<header class="top" id="top"><div class="top-in"><div class="brand"><span class="brand-name">' + esc(shortName()) + '</span><span class="brand-sub">Adisyon</span><span class="clock" id="clock">' + clockText() + '</span></div><nav class="nav" aria-label="Ana menü">' + nav + lockNow + '</nav>' + winButtons() + '</div></header>';
}
function winButtons() {
  const w = S.program.mode === 'window';
  return '<div class="win">' +
    (w ? '<button type="button" class="win-btn" data-act="win" data-a="minimize" title="Küçült" aria-label="Pencereyi küçült">' + ic('min', 24) + '</button><button type="button" class="win-btn" data-act="win" data-a="fullscreen" title="Tam ekran (F11)" aria-label="Tam ekranı aç ya da kapat">' + ic('expand', 24) + '</button>' : '') +
    '<button type="button" class="win-btn close" data-act="quit" title="Programı kapat" aria-label="Programı kapat">' + ic('power', 24) + '</button></div>';
}
function screen() {
  if (!S.setupDone) return scSetup();
  switch (ui.route) {
    case 'siparis': return scOrder(ui.table);
    case 'gunsonu': return scDay();
    case 'istatistik': return scStats();
    case 'kayit': return scAudit();
    case 'urunler': return scProducts();
    case 'ayarlar': return scSettings();
    default: return scTables();
  }
}
function render() {
  if (!S) return;
  const app = document.getElementById('app');
  const ae = document.activeElement;
  const fk = ae && ae.dataset ? ae.dataset.fk : null;
  // Yazılmakta olan kutu yeniden çizimde silinmesin (ör. yazıcı listesi gelince).
  let typing = null;
  if (ae && ae.id && !modal && app.contains(ae) && (ae.tagName === 'TEXTAREA' || (ae.tagName === 'INPUT' && !/^(checkbox|radio|file|button|submit)$/.test(ae.type)))) {
    typing = { id: ae.id, value: ae.value, s: ae.selectionStart, e: ae.selectionEnd };
  }
  const oldLines = app.querySelector('.ticket .lines');
  const linesKey = oldLines ? oldLines.dataset.t : null; const linesTop = oldLines ? oldLines.scrollTop : 0;
  app.innerHTML = header() + '<main class="main" id="main">' + screen() + '</main>';
  const newLines = app.querySelector('.ticket .lines');
  if (newLines && newLines.dataset.t === linesKey) newLines.scrollTop = linesTop;
  if (fk && !modal) { const el = app.querySelector('[data-fk="' + CSS.escape(fk) + '"]'); if (el) el.focus({ preventScroll: true }); }
  if (typing) {
    const el = document.getElementById(typing.id);
    if (el && (el.tagName === 'TEXTAREA' || el.tagName === 'INPUT')) {
      if (el.value !== typing.value) el.value = typing.value;
      el.focus({ preventScroll: true });
      try { el.setSelectionRange(typing.s, typing.e); } catch (e) { /* bazı kutu türlerinde seçim yok */ }
    }
  }
  setLayoutVars();
  drawCharts();
}
function setLayoutVars() {
  const top = document.getElementById('top');
  if (top) document.documentElement.style.setProperty('--top-h', top.offsetHeight + 'px');
  const order = document.querySelector('.order');
  if (order) document.documentElement.style.setProperty('--ticket-top', Math.round(order.getBoundingClientRect().top + window.scrollY) + 'px');
}

// ---- Masalar ----
function scTables() {
  const byTable = new Map(S.open.map(a => [a.table, a]));
  const openSum = S.open.reduce((s, a) => s + a.total, 0);
  const printed = S.open.filter(a => a.status === 'printed').length;
  const maxT = Math.max(S.tables, ...S.open.map(a => a.table));
  let tiles = '';
  for (let t = 1; t <= maxT; t++) {
    const a = byTable.get(t);
    if (!a) {
      tiles += '<button type="button" class="tile" data-act="open-table" data-t="' + t + '" data-fk="t-' + t + '"><span class="tile-name">Masa ' + t + '</span><span class="tile-sub">' + (ui.drafts[t] ? 'Açılıyor' : 'Boş') + '</span></button>';
      continue;
    }
    let pill = '';
    if (a.printFailed) pill = '<span class="pill bad">' + ic('alert', 15, 2.4) + 'Basılamadı</span>';
    else if (a.status === 'printed' && a.changed) pill = '<span class="pill warn">' + ic('printer', 15, 2.2) + 'Tekrar bas</span>';
    else if (a.status === 'printed') pill = '<span class="pill warn">' + ic('printer', 15, 2.2) + 'Basıldı</span>';
    tiles += '<button type="button" class="tile busy' + (a.printFailed ? ' failed' : '') + '" data-act="open-table" data-t="' + t + '" data-fk="t-' + t + '"><span class="tile-row"><span class="tile-name">Masa ' + t + '</span>' + pill + '</span><span class="tile-amount">' + TL(a.total) + '</span><span class="tile-sub">' + esc(a.waiterName) + ' · ' + sinceText(a.openedAt) + '</span></button>';
  }
  const bk = S.backup || {};
  const un = S.unclosedDays || [];
  const warn = (S.notice ? '<div class="note info" role="status">' + ic('info') + '<p><strong>' + esc(S.notice) + '</strong></p></div>' : '') +
    (un.length ? '<div class="note warn" role="status">' + ic('moon') + '<p><strong>Gün sonu alınmamış ' + (un.length === 1 ? 'gün' : un.length + ' gün') + ':</strong> ' + un.slice(0, 3).map(d => esc(dateLong(d))).join(', ') + (un.length > 3 ? ' …' : '') + '. Yönetici gün sonunu alıp günü kapatmalı.</p><button type="button" class="btn btn-md" data-act="goto-day" data-d="' + esc(un[un.length - 1]) + '">' + ic('moon', 20) + 'Gün Sonu’na git</button></div>' : '') +
    (bk.lastError ? '<div class="note warn" role="status">' + ic('alert') + '<p><strong>Yedek alınamadı.</strong> ' + esc(bk.lastError) + ' Siparişler kayıtlı; yöneticiye haber verin.</p></div>' : '') +
    (bk.secondError ? '<div class="note warn" role="status">' + ic('alert') + '<p><strong>USB yedeği:</strong> ' + esc(bk.secondError) + '</p></div>' : '') +
    (bk.driveError ? '<div class="note warn" role="status">' + ic('cloud') + '<p><strong>Google Drive yedeği:</strong> ' + esc(bk.driveError) + '</p></div>' : '');
  return warn + '<section class="card strip" aria-label="Salon durumu"><p class="lead">Bir masaya tıklayın, sipariş ekranı açılır.</p><div class="facts"><span><strong>' + S.open.length + '</strong> masa dolu</span><span><strong>' + printed + '</strong> hesap basıldı</span><span>Açık hesaplar <strong>' + TL(openSum) + '</strong></span></div></section>' +
    '<div class="tiles">' + tiles + '</div>' +
    '<div class="legend"><span><span class="swatch"></span>Boş masa</span><span><span class="swatch busy"></span>Dolu: sipariş var</span><span><span class="swatch printed"></span>Hesap basıldı, ödeme bekleniyor</span><span><span class="swatch failed"></span>Baskı yapılamadı</span></div>';
}

// ---- Sipariş ----
function scOrder(t) {
  const a = activeFor(t); const d = ui.drafts[t];
  if (!a && !d) return '<div class="card pad stack"><p>Masa ' + t + ' şu an boş.</p><div class="row"><button type="button" class="btn btn-primary" data-act="nav" data-to="masalar">' + ic('back') + 'Masalara dön</button></div></div>';
  const lines = a ? a.lines : [];
  const waiter = a ? a.waiterName : waiterName(d.waiterId);
  const opened = a ? a.openedAt : d.openedAt;
  const subtotal = lines.reduce((s, l) => s + l.price * l.qty, 0);
  const pct = a ? (a.discountPercent || 0) : 0;
  const kd = vatBreakdown(lines, pct);
  const discount = kd.reduce((s, k) => s + k.discount, 0);
  const total = subtotal - discount;
  const qtyOf = pid => lines.filter(l => l.productId === pid).reduce((s, l) => s + l.qty, 0);
  // Bu adisyon basılırken ya da kapanırken ürün eklenmesin (basılan ile kapanan tutar aynı kalsın).
  const frozen = !!(a && ui.busy && (ui.busy === 'print:' + a.no || ui.busy === 'pay:' + a.no || ui.busy === 'move:' + a.no || ui.busy === 'paywait:' + t));

  const cats = S.categories.filter(c => S.products.some(p => p.categoryId === c.id && p.active));
  if (ui.catTab !== 'all' && !cats.some(c => c.id === ui.catTab)) ui.catTab = 'all';
  const tabs = '<div class="cat-tabs" role="group" aria-label="Kategori"><button type="button" class="cat-tab" data-act="cat-tab" data-c="all" aria-pressed="' + (ui.catTab === 'all') + '" data-fk="ct-all">Tümü</button>' +
    cats.map(c => '<button type="button" class="cat-tab" data-act="cat-tab" data-c="' + c.id + '" aria-pressed="' + (ui.catTab === c.id) + '" data-fk="ct-' + c.id + '"><span class="dot" style="background:' + colorHex(c.color) + '"></span>' + esc(c.name) + '</button>').join('') + '</div>';
  const groups = cats.filter(c => ui.catTab === 'all' || c.id === ui.catTab).map(c => {
    const ps = S.products.filter(p => p.categoryId === c.id && p.active);
    return '<div class="prod-group">' + (ui.catTab === 'all' ? '<h2 class="eyebrow">' + esc(c.name) + '</h2>' : '') + '<div class="prods">' + ps.map(p => {
      const q = qtyOf(p.id); const noPrice = !(p.price > 0);
      return '<button type="button" class="prod" style="--cat:' + colorHex(c.color) + '" data-act="add" data-t="' + t + '" data-p="' + p.id + '" data-fk="p-' + p.id + '"' + (noPrice || frozen ? ' disabled' : '') + ' aria-label="' + esc(p.name) + (noPrice ? ', fiyat girilmemiş' : ' ekle, ' + N2(p.price) + ' lira' + (q ? ', adisyonda ' + q : '')) + '"><span class="prod-name">' + esc(p.name) + '</span><span class="prod-price">' + (noPrice ? 'Fiyat girilmemiş' : TL(p.price) + ' <span class="unit">/ ' + esc(p.unit) + '</span>') + '</span>' + (q ? '<span class="qty-badge">' + q + '</span>' : '') + '</button>';
    }).join('') + '</div></div>';
  }).join('');

  let chip = '<span class="pill accent">Açılıyor</span>';
  if (a && a.status === 'open') chip = '<span class="pill accent">Açık</span>';
  if (a && a.status === 'printed') chip = '<span class="pill warn">' + ic('printer', 16, 2.2) + 'Basıldı</span>';

  let notice = '';
  if (!a) notice = '<div class="note info">' + ic('info') + '<p>Adisyon numarası ilk ürün eklenince verilir.</p></div>';
  else if (a.printFailed) notice = '<div class="note bad" role="status">' + ic('alert') + '<p>Son baskı yapılamadı. Yazıcıyı kontrol edip <strong>' + (a.status === 'printed' ? 'Tekrar Bas' : 'Adisyonu Bas') + '</strong>’a tekrar basın.</p></div>';
  else if (a.status === 'printed' && a.changed) notice = '<div class="note warn" role="status">' + ic('alert') + '<p>Adisyon değişti. Müşteriye vermeden önce tekrar basın.</p></div>';
  // Sorunsuz basıldıysa ayrı kutu yerine başlıkta kısa bilgi: kalemlere daha çok yer kalır.
  const printedOK = a && a.status === 'printed' && !a.changed && !a.printFailed;
  const ticketSub = '<span class="ticket-sub">' + (printedOK ? '<span class="ok">' + ic('check', 18, 2.6) + (a.printCount > 1 ? a.printCount + '. baskı ' : 'Basıldı ') + hm(a.printedAt) + '</span><span aria-hidden="true">·</span>' : '') + lines.length + ' kalem</span>';

  const lineHTML = lines.length ? '<ul class="lines" data-t="' + t + '">' + lines.map(l =>
    '<li class="line"><div class="line-top"><span class="line-name">' + esc(l.name) + '</span><span class="line-total">' + TL(l.price * l.qty) + '</span></div>' +
    '<div class="line-bottom"><span class="line-sub">' + TL(l.price) + ' × ' + l.qty + ' ' + esc(l.unit) + '</span><div class="stepper"><button type="button" class="qtybtn" data-act="qty" data-no="' + a.no + '" data-l="' + l.id + '" data-d="-1" data-fk="dec-' + l.id + '"' + (frozen ? ' disabled' : '') + ' aria-label="' + esc(l.name) + ' adedini azalt">' + ic('minus', 26, 2.6) + '</button><span class="qty">' + l.qty + '</span><button type="button" class="qtybtn" data-act="qty" data-no="' + a.no + '" data-l="' + l.id + '" data-d="1" data-fk="inc-' + l.id + '"' + (frozen ? ' disabled' : '') + ' aria-label="' + esc(l.name) + ' adedini artır">' + ic('plus', 26, 2.6) + '</button></div></div></li>').join('') + '</ul>'
    : '<p class="empty">Henüz ürün yok. Soldaki ürünlere tıklayarak ekleyin.</p>';

  const busyPrint = a && ui.busy === 'print:' + a.no;
  const busyPay = a && ui.busy === 'pay:' + a.no;
  const undoCount = a ? (S.undoCounts[a.no] || 0) : 0;
  const payBtn = a && !lines.length
    ? '<button type="button" class="btn" data-act="empty-close" data-t="' + t + '" data-fk="empty-close">' + ic('x') + 'Masayı boşalt</button>'
    : '<button type="button" class="btn btn-good' + (busyPay ? ' busy' : '') + '" data-busy="Kapanıyor…" data-act="pay" data-t="' + t + '" data-fk="pay"' + (lines.length && !ui.busy ? '' : ' disabled') + '>Hesap Ödendi</button>';

  return '<div class="order"><div class="order-head"><button type="button" class="btn" data-act="nav" data-to="masalar" data-fk="back">' + ic('back') + 'Masalar</button><h1>Masa ' + t + '</h1>' + chip +
    '<div class="order-meta"><span>No <strong>' + (a ? a.no : '—') + '</strong></span><span>Açılış <strong>' + hm(opened) + '</strong></span><button type="button" class="btn waiter-btn" data-act="change-waiter" data-t="' + t + '" data-fk="cw" aria-label="Garson ' + esc(waiter) + ', değiştir">' + ic('user', 20) + '<strong>' + esc(waiter) + '</strong><span class="muted">değiştir</span></button>' +
    (a ? '<button type="button" class="btn" data-act="move-table" data-t="' + t + '" data-fk="mv"' + (ui.busy ? ' disabled' : '') + '>' + ic('move', 20) + 'Masayı taşı</button>' : '') +
    (a ? '<button type="button" class="btn' + (pct ? ' btn-outline' : '') + '" data-act="discount" data-t="' + t + '" data-fk="disc"' + (ui.busy ? ' disabled' : '') + ' aria-label="' + (pct ? 'İndirim yüzde ' + pct + ', değiştir' : 'İndirim uygula' + (S.discountNoPin ? '' : ', yönetici şifresi gerekir')) + '">' + ic(pct || S.discountNoPin ? 'percent' : 'lock', 20) + (pct ? '<strong>İndirim %' + pct + '</strong>' : 'İndirim') + '</button>' : '') + '</div></div>' +
    '<section class="products" aria-label="Ürünler">' + tabs + groups + '</section>' +
    '<aside class="card ticket" aria-label="Adisyon"><div class="ticket-head"><h2>Adisyon</h2>' + ticketSub + '</div>' + notice + lineHTML +
    '<div class="totals">' + (pct ? '<div class="kdv-row"><span>Ara toplam</span><span>' + TL(subtotal) + '</span></div><div class="kdv-row disc"><span>İndirim (%' + pct + ') <button type="button" class="linkish small" data-act="discount-clear" data-t="' + t + '"' + (frozen ? ' disabled' : '') + '>kaldır</button></span><span>−' + TL(discount) + '</span></div>' : '') +
    '<div class="total-row"><span class="lbl">Toplam (KDV dahil)</span><span class="total-amount">' + TL(total) + '</span></div>' + kd.map(k => '<div class="kdv-row"><span>İçindeki KDV (%' + k.rate + ')</span><span>' + TL(k.vat) + '</span></div>').join('') + '</div>' +
    '<button type="button" class="btn btn-primary btn-xl' + (busyPrint ? ' busy' : '') + '" data-busy="Basılıyor…" data-act="print" data-t="' + t + '" data-fk="print"' + (lines.length && !ui.busy ? '' : ' disabled') + '>' + ic('printer', 28) + (a && a.status === 'printed' ? 'Tekrar Bas' : 'Adisyonu Bas') + '</button>' +
    '<div class="ticket-actions">' + payBtn + '<button type="button" class="btn" data-act="undo" data-t="' + t + '" data-fk="undo"' + (undoCount && !ui.busy ? '' : ' disabled') + '>' + ic('undo') + 'Geri Al</button><button type="button" class="btn btn-danger-quiet" data-act="cancel" data-t="' + t + '" data-fk="cancel"' + (a && !ui.busy ? '' : ' disabled') + ' aria-label="İptal, yönetici şifresi gerekir">' + ic('lock', 20) + 'İptal</button></div>' +
    '</aside></div>';
}

// ---- Gün sonu ----
async function loadDay(date) {
  ui.day.loading = true;
  if (date !== undefined) ui.day.date = date || '';
  const want = ui.day.date;
  const r = await act('daySummary', { date: want });
  ui.day.loading = false;
  if (r && ui.day.date === want) { ui.day.data = r.data; ui.day.date = r.data.date; }
  if (ui.route === 'gunsonu') render();
}
function deltaHTML(c, money, upGood) {
  if (!c || c.change == null) return '<span class="delta flat">önceki dönemde satış yok</span>';
  const ch = c.change; const good = upGood === false ? ch < 0 : ch > 0;
  if (Math.abs(ch) < 0.05) return '<span class="delta flat">' + ic('minus', 16, 2.6) + 'değişmedi</span>';
  return '<span class="delta ' + (good ? 'up' : 'down') + '">' + ic(ch > 0 ? 'up' : 'down', 16, 2.8) + '%' + nf1.format(Math.abs(ch)) + '</span>';
}
function kpi(label, value, sub) {
  return '<div class="card kpi"><span class="k-lbl">' + esc(label) + '</span><span class="k-val">' + value + '</span>' + (sub ? '<span class="k-sub">' + sub + '</span>' : '') + '</div>';
}
function statusPill(r) {
  switch (r.status) {
    case 'paid': return '<span class="pill good">' + ic('check', 15, 2.6) + 'Ödendi</span>';
    case 'cancelled': return r.mergedInto ? '<span class="pill quiet">' + ic('move', 15, 2.2) + 'Birleşti → No ' + r.mergedInto + '</span>' : '<span class="pill bad">' + ic('x', 15, 2.6) + 'İptal</span>';
    case 'printed': return '<span class="pill warn">' + ic('printer', 15, 2.2) + 'Açık · basıldı</span>';
    default: return '<span class="pill accent">Açık</span>';
  }
}
const PAY_CLASS = { nakit: 's1', kart: 's2', havale: 's3' };
function payShareHTML(list, total) {
  const rows = list.filter(p => p.method || p.count);
  const bar = total > 0 ? '<div class="sharebar viz" role="img" aria-label="Ödeme şekillerinin payı">' + rows.filter(p => p.total > 0).map(p => '<span style="flex:' + p.total + ' 1 0;background:var(--' + (PAY_CLASS[p.method] || 'axis') + ')" title="' + esc(p.label) + '"></span>').join('') + '</div>' : '';
  return bar + '<table class="legend-table viz"><tbody>' + rows.map(p => '<tr><td><span class="pm"><span class="swatch-k" style="background:var(--' + (PAY_CLASS[p.method] || 'axis') + ')"></span>' + ic(PAY_ICON[p.method] || 'info', 18, 2) + esc(p.label) + '</span><br><span class="muted small">' + nf0.format(p.count) + ' adisyon</span></td><td class="r"><strong>' + TL(p.total) + '</strong><br><span class="muted small">' + (total > 0 ? '%' + nf1.format(p.total * 100 / total) : '—') + '</span></td></tr>').join('') + '</tbody></table>';
}
function scDay() {
  const D = ui.day.data;
  const today = S.businessDate;
  const date = ui.day.date || today;
  const nav = '<div class="date-nav"><button type="button" class="icon-btn" data-act="day-prev" aria-label="Önceki gün">' + ic('left') + '</button><label class="sr" for="day-date">Tarih</label><input class="input" type="date" id="day-date" value="' + esc(date) + '" max="' + esc(today) + '"><button type="button" class="icon-btn" data-act="day-next" aria-label="Sonraki gün"' + (date >= today ? ' disabled' : '') + '>' + ic('right') + '</button>' + (date !== today ? '<button type="button" class="btn btn-md" data-act="day-today">Bugün</button>' : '') + '</div>';
  const head = '<div class="page-head"><div class="ttl"><h1>Gün sonu</h1><p class="muted">' + esc(dateLong(date)) + (D && D.date === date ? (D.closed ? (D.migrated ? ' · <strong>1.0.0 verisi</strong>' : ' · <strong>kapatıldı ' + esc(stampText(D.closedAt)) + '</strong>') : ' · gün açık') : '') + '</p></div>' + nav + '</div>';
  if (!D || D.date !== date) return head + '<div class="card pad"><p class="muted">' + (ui.day.loading ? 'Yükleniyor…' : 'Gün özeti okunamadı.') + '</p></div>';
  const actions = '<div class="row"><button type="button" class="btn" data-act="day-print">' + ic('printer') + (D.closed ? 'Gün sonu fişi bas' : 'Ara rapor bas') + '</button><button type="button" class="btn" data-act="day-preview">' + ic('eye') + 'Fiş önizlemesi</button><button type="button" class="btn" data-act="nav" data-to="kayit">' + ic('history') + 'İşlem kaydı</button>' +
    (D.closed ? '<span class="pill good">' + ic('lock', 16) + (D.migrated ? '1.0.0 verisi · kapatılmış sayıldı' : 'Gün kapatıldı; değiştirilemez') + '</span>'
      : date > (S.clockDate || today) ? '<span class="pill quiet" style="margin-left:auto">' + ic('info', 16) + 'Bu gün henüz başlamadı; gün sonu gün bitince alınır</span>'
      : '<button type="button" class="btn btn-primary" data-act="day-close" style="margin-left:auto">' + ic('moon') + 'Günü kapat</button>') + '</div>';
  const un = (D.unclosed || []).filter(x => x !== date);
  const unNote = un.length ? '<div class="note warn">' + ic('moon') + '<p>Gün sonu alınmamış ' + (un.length === 1 ? 'gün' : 'günler') + ': ' + un.map(x => '<button type="button" class="linkish" data-act="goto-day" data-d="' + esc(x) + '">' + esc(dateLong(x)) + '</button>').join(' ') + '</p></div>' : '';
  const warns = D.warnings || [];
  const warnHTML = (D.noMissing && !warns.length)
    ? '<div class="big-note good">' + ic('check', 26, 2.6) + 'Eksik adisyon yok' + (D.firstNo ? ' · No ' + D.firstNo + '–' + D.lastNo + ' tam' : '') + '</div>'
    : (D.noMissing ? '<div class="big-note good">' + ic('check', 26, 2.6) + 'Numara atlaması yok' + (D.firstNo ? ' · No ' + D.firstNo + '–' + D.lastNo : '') + '</div>' : '') +
      '<ul class="warn-list">' + warnGroups(warns).map(g => '<li class="' + g.level + '">' + ic('alert', 20) + '<span>' + g.html + '</span>' + (g.no ? '<button type="button" class="linkish" data-act="detail" data-no="' + g.no + '">Aç</button>' : '') + '</li>').join('') + '</ul>';
  const feat = D.featured ? kpi('Satılan ' + D.featured.name, nf0.format(D.featured.qty) + ' <span class="muted" style="font-size:20px">' + esc(D.featured.unit) + '</span>') : '';
  const vatText = (D.vats || []).length > 1 ? D.vats.map(v => 'KDV %' + v.rate + ' ' + TL(v.vat)).join(' · ') : (D.vats || []).length === 1 ? 'KDV %' + D.vats[0].rate + ': ' + TL(D.vat) : 'KDV ' + TL(D.vat);
  const kpis = '<div class="kpis">' + kpi('Ciro (KDV dahil)', TL(D.revenue), vatText + ' · matrah ' + TL(D.revenue - D.vat)) + kpi('Ödenen adisyon', nf0.format(D.paid), D.firstNo ? 'No ' + D.firstNo + '–' + D.lastNo : '') + kpi('Ortalama adisyon', TL(D.avg)) + feat +
    kpi('İptal', nf0.format(D.cancelled), D.cancelled ? TL(D.cancelledTotal) : (D.merged ? D.merged + ' masa birleşti' : '')) + (D.discounts ? kpi('İndirim', '−' + TL(D.discountTotal), D.discounts + ' adisyon') : '') + (D.open ? kpi('Açık masa', '<span style="color:var(--bad)">' + D.open + '</span>', 'Gün kapatılmadan önce kapatılmalı') : '') + '</div>';
  const pay = '<section class="card pad stack"><div class="card-head"><h2>Ödeme şekilleri</h2><span class="muted small">Kasa sayımı için</span></div>' + payShareHTML(D.byPayment || [], D.revenue) + '</section>';
  const waiters = '<section class="card pad stack"><div class="card-head"><h2>Garsonlar</h2></div>' + ((D.byWaiter || []).length ? '<table class="legend-table"><tbody>' + D.byWaiter.map(w => '<tr><td>' + ic('user', 18) + ' ' + esc(w.name) + '</td><td class="r muted">' + w.count + ' adisyon</td><td class="r"><strong>' + TL(w.total) + '</strong></td></tr>').join('') + '</tbody></table>' : '<p class="muted">Ödenen adisyon yok.</p>') + '</section>';
  const prods = '<section class="card pad stack"><div class="card-head"><h2>Satılan ürünler</h2></div>' + ((D.products || []).length ? '<table class="legend-table"><tbody>' + D.products.map(p => '<tr><td>' + esc(p.name) + '</td><td class="r"><strong>' + nf0.format(p.qty) + '</strong> <span class="muted">' + esc(p.unit) + '</span></td><td class="r muted">' + TL(p.total) + '</td></tr>').join('') + '</tbody></table>' : '<p class="muted">Henüz satış yok.</p>') + '</section>';
  const f = ui.day.filter;
  const rows = (D.rows || []).filter(r => f === 'all' || (f === 'paid' && r.status === 'paid') || (f === 'cancelled' && r.status === 'cancelled') || (f === 'open' && (r.status === 'open' || r.status === 'printed')));
  const chips = [['all', 'Tümü', (D.rows || []).length], ['paid', 'Ödenen', D.paid], ['cancelled', 'İptal ve birleşen', D.cancelled + D.merged], ['open', 'Açık', D.open]].map(([k, l, n]) => '<button type="button" class="chip" data-act="day-filter" data-f="' + k + '" aria-pressed="' + (f === k) + '">' + l + ' (' + n + ')</button>').join('');
  const table = '<section class="card stack" style="padding:16px 0 4px;gap:12px"><div class="card-head" style="padding:0 18px"><h2>Adisyonlar</h2><div class="chips">' + chips + '</div></div>' +
    (rows.length ? '<div class="tablebox scrollbox"><table class="data"><thead><tr><th>No</th><th>Açılış</th><th>Kapanış</th><th>Masa</th><th>Garson</th><th>Durum</th><th>Ödeme</th><th class="r">Tutar</th></tr></thead><tbody>' +
      rows.map(r => '<tr class="click' + (r.status === 'cancelled' ? ' muted-row' : '') + '" data-act="detail" data-no="' + r.no + '" tabindex="0"><td><strong>' + r.no + '</strong></td><td>' + hm(r.openedAt) + '</td><td>' + hm(r.closedAt) + '</td><td>Masa ' + r.table + '</td><td>' + esc(r.waiter) + '</td><td>' + statusPill(r) + '</td><td>' + (r.status === 'paid' ? '<span class="pm">' + ic(PAY_ICON[r.payment] || 'info', 18) + esc(r.paymentLabel || 'Belirtilmemiş') + '</span>' : '') + '</td><td class="r"><strong>' + TL(r.total) + '</strong>' + (r.discountPercent ? '<br><span class="muted small">%' + r.discountPercent + ' indirim</span>' : '') + '</td></tr>').join('') +
      dayTotals(rows, f, D) + '</tbody></table></div>' : '<p class="muted" style="padding:0 18px 14px">Bu filtrede adisyon yok.</p>') + '</section>';
  const cancels = (D.rows || []).filter(r => r.status === 'cancelled' && !r.mergedInto);
  const cancelCard = cancels.length ? '<section class="card stack" style="padding:16px 0 4px;gap:10px"><div class="card-head" style="padding:0 18px"><h2>İptaller</h2><span class="muted small">' + cancels.length + ' adisyon · ' + TL(D.cancelledTotal) + '</span></div><div class="tablebox"><table class="data"><thead><tr><th>No</th><th>Saat</th><th>Masa</th><th>Garson</th><th>Sebep</th><th class="r">Tutar</th></tr></thead><tbody>' +
    cancels.map(r => '<tr class="click" data-act="detail" data-no="' + r.no + '" tabindex="0"><td><strong>' + r.no + '</strong></td><td>' + hm(r.closedAt || r.openedAt) + '</td><td>Masa ' + r.table + '</td><td>' + esc(r.waiter) + '</td><td>' + esc(r.cancelReason || '') + (r.printCount ? ' <span class="pill warn" style="padding:1px 8px">' + ic('printer', 14, 2.2) + 'basılmıştı</span>' : '') + '</td><td class="r">' + TL(r.total) + '</td></tr>').join('') + '</tbody></table></div></section>' : '';
  return head + actions + unNote + '<section class="card pad stack"><h2 class="sec-title">Kontrol</h2>' + warnHTML + '</section>' + kpis + '<div class="grid-3">' + pay + waiters + prods + '</div>' + cancelCard + table;
}
// Gün listesinin altındaki toplamlar: ödenenler ödeme şekline göre, diğerleri adet ve tutar.
function dayTotals(rows, f, D) {
  const paid = rows.filter(r => r.status === 'paid');
  const sum = list => list.reduce((a, r) => a + r.total, 0);
  const tr = (label, mid, total) => '<tr class="total"><td colspan="6">' + label + '</td><td>' + mid + '</td><td class="r">' + total + '</td></tr>';
  if (f === 'all' || f === 'paid') {
    let out = tr('Ödenen ' + paid.length + ' adisyon (ciro)', '', TL(sum(paid)));
    (D.byPayment || []).filter(p => p.count).forEach(p => { out += '<tr class="total" style="font-weight:600"><td colspan="6"></td><td><span class="pm">' + ic(PAY_ICON[p.method] || 'info', 18) + esc(p.label) + ' (' + p.count + ')</span></td><td class="r">' + TL(p.total) + '</td></tr>'; });
    if (f === 'all') {
      const c = rows.filter(r => r.status === 'cancelled' && !r.mergedInto);
      if (c.length) out += tr(c.length + ' iptal (ciroya girmez)', '', TL(sum(c)));
    }
    return out;
  }
  return tr(rows.length + ' adisyon', '', TL(sum(rows)));
}
// Aynı türden uyarılar tek satırda toplanır (ör. "7 adisyon basılmadan kapatıldı: No 12, 15…").
function warnGroups(warns) {
  const out = []; const by = {};
  const titles = { unprinted: n => n + ' adisyon basılmadan kapatıldı:', changed: n => n + ' adisyonun son hali basılmadan kapatıldı:' };
  warns.forEach(w => {
    if (titles[w.kind]) { if (!by[w.kind]) { by[w.kind] = { level: 'warn', kind: w.kind, nos: [] }; out.push(by[w.kind]); } by[w.kind].nos.push(w.no); }
    else out.push({ level: w.level === 'bad' ? 'bad' : 'warn', html: esc(w.text), no: w.no });
  });
  out.forEach(g => {
    if (!g.nos) return;
    if (g.nos.length === 1) { g.html = esc(warns.find(w => w.no === g.nos[0] && w.kind === g.kind).text); g.no = g.nos[0]; return; }
    g.html = '<strong>' + titles[g.kind](g.nos.length) + '</strong> ' + g.nos.map(n => '<button type="button" class="linkish" style="min-height:36px;padding:2px 4px" data-act="detail" data-no="' + n + '">No ' + n + '</button>').join(' ');
  });
  return out;
}
async function showDetail(no) {
  const r = await act('detail', { no });
  if (!r) return;
  const d = r.data; const a = d.adisyon;
  const pills = statusPill({ status: a.status, mergedInto: a.mergedInto });
  const disc = a.discountPercent ? '<tr><td class="muted">Ara toplam</td><td class="r muted">' + TL(a.total + a.discountAmount) + '</td></tr><tr><td class="muted">İndirim (%' + a.discountPercent + ')</td><td class="r muted">−' + TL(a.discountAmount) + '</td></tr>' : '';
  const lines = (a.lines || []).length ? '<table class="legend-table"><tbody>' + a.lines.map(l => '<tr><td><strong>' + esc(l.name) + '</strong><br><span class="muted">' + l.qty + ' ' + esc(l.unit) + ' × ' + TL(l.price) + '</span></td><td class="r"><strong>' + TL(l.qty * l.price) + '</strong></td></tr>').join('') + disc + '<tr><td><strong>Toplam</strong>' + (d.vats || []).map(v => '<br><span class="muted">KDV %' + v.rate + ': ' + TL(v.vat) + '</span>').join('') + '</td><td class="r"><strong style="font-size:24px">' + TL(a.total) + '</strong></td></tr></tbody></table>' : '<p class="muted">Kalem yok.</p>';
  const canChange = a.status === 'paid' && !d.dayClosed;
  const kv = '<dl class="kv"><dt>İş günü</dt><dd>' + esc(dateLong(a.businessDate)) + '</dd><dt>Masa · garson</dt><dd>Masa ' + a.table + ' · ' + esc(a.waiterName) + '</dd><dt>Açılış</dt><dd>' + esc(stampText(a.openedAt)) + '</dd>' +
    (a.printCount ? '<dt>Baskı</dt><dd>' + a.printCount + ' kez · son ' + esc(stampText(a.printedAt)) + (a.changed ? ' · <span style="color:var(--warn)">sonra değişti</span>' : '') + '</dd>' : '<dt>Baskı</dt><dd>Basılmadı</dd>') +
    (a.closedAt ? '<dt>Kapanış</dt><dd>' + esc(stampText(a.closedAt)) + '</dd>' : '') +
    (a.status === 'paid' ? '<dt>Ödeme şekli</dt><dd><span class="pm">' + ic(PAY_ICON[a.paymentMethod] || 'info', 18) + esc(payLabel(a.paymentMethod)) + '</span>' + (canChange ? ' <button type="button" class="linkish" data-act="change-pay" data-no="' + a.no + '" data-m="' + esc(a.paymentMethod || '') + '">Değiştir</button>' : '') + '</dd>' : '') +
    (a.cancelReason ? '<dt>' + (a.mergedInto ? 'Not' : 'İptal sebebi') + '</dt><dd>' + esc(a.cancelReason) + '</dd>' : '') + '</dl>';
  const hist = '<h3 class="sec-title" style="font-size:20px">İşlem geçmişi</h3><ul class="hist">' + (d.history || []).map(h => '<li><time>' + hm(h.at) + '</time><span><b>' + esc(h.label) + '</b>' + (h.text ? ' · ' + esc(h.text) : '') + '</span></li>').join('') + '</ul>';
  openModal({
    title: 'Adisyon No ' + a.no,
    wide: true,
    body: '<div class="row">' + pills + (d.dayClosed ? '<span class="pill quiet">' + ic('lock', 15) + 'Gün kapatıldı</span>' : '') + '</div>' + kv +
      (canChange ? '<div class="note info"><p><strong>Yanlış kapatıldıysa:</strong> “Düzeltmek için yeniden aç” adisyonu masaya geri getirir; adet düzeltilir, gerekirse tekrar basılır ve yeniden ödenir. “İptal et” hesabı tümüyle iptal eder. İkisi de yönetici şifresi ister ve işlem kaydına yazılır.</p></div>' : '') + lines + hist,
    actions: [{ label: 'Fiş önizlemesi', icon: 'eye', onClick: async () => { const p = await act('preview', { no: a.no }); if (p) showPreview(p.data.image, 'No ' + a.no + ' · fiş önizlemesi', 'Ödeme şekli fişte yer almaz.', () => showDetail(a.no)); } }]
      .concat(canChange ? [{ label: 'Düzeltmek için yeniden aç', icon: 'undo', onClick: () => reopenFlow(a) }, { label: 'İptal et', kind: 'btn-danger-quiet', icon: 'lock', onClick: () => cancelFlowFor(a, async () => { toast('No ' + a.no + ' iptal edildi.'); await loadDay(); }) }] : [])
      .concat([{ label: 'Kapat', kind: 'btn-primary', onClick: () => closeModal() }])
  });
}
// Ödenmiş adisyonu düzeltmek için masaya geri açar (yönetici şifresi + sebep).
function reopenFlow(a) {
  const busy = new Set(S.open.map(o => o.table));
  const free = []; for (let t = 1; t <= S.tables; t++) if (!busy.has(t)) free.push(t);
  if (!free.length) { toast('Boş masa yok; önce bir masayı kapatın.', 'bad'); return; }
  const def = busy.has(a.table) ? free[0] : a.table;
  openModal({
    title: 'No ' + a.no + ' yeniden açılsın mı?',
    body: '<p>Adisyon ' + TL(a.total) + ' · ' + esc(payLabel(a.paymentMethod)) + ' olarak ödenmiş görünüyor. Yeniden açılınca ödeme kaydı kalkar; adisyon masada düzeltilip <strong>yeniden ödenir</strong>. Numarası ve açılış saati aynı kalır.</p>' +
      '<div class="fields"><div class="field"><label for="reopen-reason">Sebep</label><input class="input" type="text" id="reopen-reason" placeholder="Örnek: 5 yerine 8 cağ yazılmış" maxlength="200" data-autofocus></div>' +
      '<div class="field"><label for="reopen-table">Hangi masada açılsın?</label><select class="input" id="reopen-table">' + free.map(t => '<option value="' + t + '"' + (t === def ? ' selected' : '') + '>Masa ' + t + (t === a.table ? ' (kendi masası)' : '') + '</option>').join('') + '</select>' + (busy.has(a.table) ? '<p class="hint">Masa ' + a.table + ' şu an dolu; boş bir masa seçildi.</p>' : '') + '</div></div>',
    actions: [{ label: 'Vazgeç', onClick: () => closeModal() }, { label: 'Yeniden aç', kind: 'btn-primary', icon: 'undo', onClick: async () => {
      const reason = val('reopen-reason').trim();
      if (!reason) { toast('Sebep yazın.', 'bad'); document.getElementById('reopen-reason').focus(); return; }
      const table = Number(val('reopen-table'));
      closeModal(true);
      const r = await act('reopen', { no: a.no, table, reason });
      if (!r) return;
      toast('No ' + a.no + ' Masa ' + r.data.table + '’' + numSfx(r.data.table)[1].slice(0, -1) + ' yeniden açıldı; düzeltip yeniden ödeyin.', 'good');
      go('siparis', r.data.table);
      render();
    } }]
  });
}
// Yüzde indirim penceresi: hazır yüzdeler ya da elle; uygulamak yönetici şifresi ister.
function discountFlow(t) {
  const a = activeFor(t); if (!a) return;
  const cur = a.discountPercent || 0;
  const presets = (S.discountPresets || [5, 10, 15, 20, 25, 50]).map(p => '<button type="button" class="pay-btn disc-btn" data-act="discount-set" data-t="' + t + '" data-pct="' + p + '"' + (p === cur ? ' aria-pressed="true"' : '') + '><strong>%' + p + '</strong><span class="muted small">−' + TL(discountOf(a.lines.reduce((s, l) => s + l.price * l.qty, 0), p)) + '</span></button>').join('');
  openModal({
    title: 'No ' + a.no + ' · indirim',
    body: '<p class="muted">İndirim adisyonun tamamına uygulanır, fişte “İndirim” satırı olarak çıkar ve gün sonunda ayrıca görünür.' + (S.discountNoPin ? '' : ' Uygulamak yönetici şifresi ister.') + '</p>' +
      '<div class="pay-methods disc-grid">' + presets + '</div>' +
      '<div class="row" style="align-items:flex-end"><div class="field" style="flex:0 1 200px"><label for="disc-pct">Başka yüzde</label><input class="input" id="disc-pct" inputmode="numeric" placeholder="1–100" maxlength="3"></div><button type="button" class="btn btn-primary" data-act="discount-custom" data-t="' + t + '">' + ic('percent', 20) + 'Uygula</button>' + (cur ? '<button type="button" class="btn btn-danger-quiet" data-act="discount-set" data-t="' + t + '" data-pct="0">İndirimi kaldır</button>' : '') + '</div>' +
      '<p class="hint">%100 ikram demektir: toplam sıfır olur.</p>',
    actions: [{ label: 'Vazgeç', onClick: () => closeModal() }]
  });
}
async function setDiscount(t, pct) {
  const a = activeFor(t); if (!a) return;
  if (!(pct >= 0 && pct <= 100) || pct !== Math.floor(pct)) { toast('Yüzde 1 ile 100 arasında tam sayı olmalı.', 'bad'); return; }
  closeModal(true);
  const r = await act('discount', { no: a.no, percent: pct });
  if (!r) return;
  render();
  const b = activeFor(t);
  toast(pct ? 'No ' + a.no + ' · %' + pct + ' indirim · toplam ' + TL(b ? b.total : 0) : 'No ' + a.no + ' · indirim kaldırıldı', 'good');
}
function changePayFlow(no, cur) {
  const opts = (S.paymentMethods || []).map(m => '<button type="button" class="pay-btn" data-act="change-pay-to" data-no="' + no + '" data-m="' + esc(m.id) + '"' + (m.id === cur ? ' disabled' : '') + '>' + ic(PAY_ICON[m.id] || 'cash', 34, 1.8) + '<span>' + esc(m.label) + '</span>' + (m.id === cur ? '<span class="muted small">şu anki</span>' : '') + '</button>').join('');
  openModal({ title: 'No ' + no + ' · ödeme şeklini değiştir', body: '<p class="muted">Yanlış seçilen ödeme şeklini düzeltir. Yönetici şifresi sorulur; değişiklik işlem kaydına yazılır.</p><div class="pay-methods">' + opts + '</div>', actions: [{ label: 'Vazgeç', onClick: () => closeModal() }] });
}
async function changePayTo(no, method) {
  closeModal(true);
  const r = await act('changePayment', { no, method });
  if (!r) return;
  toast('No ' + no + ' ödeme şekli: ' + payLabel(method), 'good');
  await loadDay();
  showDetail(no);
}
function closeDayFlow() {
  const D = ui.day.data; if (!D || D.closed) return;
  if (D.open) {
    openModal({ title: 'Açık masa var', body: '<div class="err-box">' + ic('alert', 30, 2.2) + '<div><strong>' + D.open + ' masa hâlâ açık.</strong><p>Gün kapatılmadan önce bu hesapları kapatın ya da iptal edin: ' + D.rows.filter(r => r.status === 'open' || r.status === 'printed').map(r => 'Masa ' + r.table + ' (No ' + r.no + ')').join(', ') + '</p></div></div>', actions: [{ label: 'Tamam', kind: 'btn-primary', onClick: () => closeModal() }] });
    return;
  }
  const sends = [];
  if (S.whatsapp && S.whatsapp.enabled) sends.push('WhatsApp');
  if (S.sheets && S.sheets.enabled) sends.push('Google Sheets');
  openModal({
    title: dateLong(D.date) + ' kapatılsın mı?',
    body: '<dl class="kv"><dt>Ciro</dt><dd>' + TL(D.revenue) + '</dd><dt>Ödenen adisyon</dt><dd>' + D.paid + '</dd><dt>İptal</dt><dd>' + D.cancelled + (D.cancelled ? ' · ' + TL(D.cancelledTotal) : '') + '</dd>' + (D.discounts ? '<dt>İndirim</dt><dd>' + D.discounts + ' adisyon · −' + TL(D.discountTotal) + '</dd>' : '') + '</dl>' +
      ((D.warnings || []).length ? '<div class="note warn">' + ic('alert') + '<p>' + D.warnings.length + ' uyarı var; kapatmadan önce “Kontrol” bölümüne bakın.</p></div>' : '') +
      '<p>Kapatılan gün değiştirilemez. Kapanışta yedek alınır' + (sends.length ? '; özet ' + sends.join(' ve ') + ' ile gönderilir (internet yoksa sırada bekler)' : '') + '.</p>' +
      '<label class="switch-row" for="close-print"><span>Gün sonu fişini bas</span><input class="switch" type="checkbox" id="close-print" checked></label>',
    actions: [{ label: 'Vazgeç', onClick: () => closeModal() }, { label: 'Günü kapat', kind: 'btn-primary', icon: 'moon', onClick: async () => {
      const pr = checked('close-print');
      closeModal(true);
      const r = await act('closeDay', { date: D.date });
      if (!r) return;
      ui.day.data = r.data;
      toast(dateLong(D.date) + ' kapatıldı.', 'good');
      render();
      if (pr) printDay(D.date);
      loadDay(D.date);
    } }]
  });
}
async function printDay(date) {
  const r = await act('printDay', { date });
  if (!r) return;
  const out = r.data;
  if (out.ok) { if (out.preview) showPreview(out.preview, 'Gün sonu fişi (yazıcı seçili değil)'); else toast('Gün sonu fişi basıldı.', 'good'); return; }
  openModal({ title: out.title || 'Gün sonu fişi basılamadı', body: '<div class="err-box">' + ic('alert', 34, 2.2) + '<div><strong>' + esc(out.message || 'Yazıcıya ulaşılamıyor.') + '</strong><p>' + esc(out.hint || '') + '</p></div></div>', actions: [{ label: 'Kapat', onClick: () => closeModal() }, { label: 'Tekrar Dene', kind: 'btn-primary', icon: 'printer', onClick: () => { closeModal(true); printDay(date); } }] });
}

// ---- İşlem kaydı ----
async function loadAudit(more) {
  const A = ui.audit;
  if (!A.from && !A.to && !A.no) { A.from = addDays(S.businessDate, -6); A.to = dayStr(new Date()); }
  A.loading = true;
  const before = more && A.rows && A.rows.length ? A.rows[A.rows.length - 1].id : 0;
  const r = await act('auditLog', { from: A.from, to: A.to, kind: A.kind, no: Number(A.no) || 0, before });
  A.loading = false;
  if (r) { A.rows = more && A.rows ? A.rows.concat(r.data.rows) : r.data.rows; A.more = r.data.more; }
  if (ui.route === 'kayit') render();
}
const AUDIT_KINDS = [['', 'Tümü'], ['iptal', 'İptaller'], ['odeme', 'Ödemeler'], ['baski', 'Baskılar'], ['masa', 'Masa ve garson'], ['kalem', 'Kalem değişiklikleri'], ['fiyat', 'Fiyat ve ürün'], ['gun', 'Gün sonu ve yedek'], ['ayar', 'Ayarlar ve şifre']];
function scAudit() {
  const A = ui.audit;
  const kinds = AUDIT_KINDS.map(([k, l]) => '<button type="button" class="chip" data-act="audit-kind" data-k="' + k + '" aria-pressed="' + (A.kind === k) + '">' + l + '</button>').join('');
  const rows = A.rows || [];
  let lastDay = '';
  const body = rows.map(r => {
    const d = r.at.slice(0, 10);
    const sep = d !== lastDay ? '<tr><td colspan="4" style="background:var(--surface-2);font-weight:700">' + esc(dateLong(d)) + '</td></tr>' : '';
    lastDay = d;
    return sep + '<tr' + (r.no ? ' class="click" data-act="detail" data-no="' + r.no + '" tabindex="0"' : '') + '><td class="nowrap">' + hm(r.at) + '</td><td class="nowrap"><strong>' + esc(r.label) + '</strong></td><td>' + (r.no ? 'No ' + r.no : '') + '</td><td>' + esc(r.text) + '</td></tr>';
  }).join('');
  return '<div class="page-head"><div class="ttl"><h1>İşlem kaydı</h1><p class="muted">Programda yapılan her işlem: baskı, ödeme, iptal, fiyat değişikliği, ayarlar. Kayıtlar silinemez.</p></div><button type="button" class="btn" data-act="nav" data-to="gunsonu">' + ic('back') + 'Gün sonu</button></div>' +
    '<section class="card pad stack"><div class="filters"><div class="field"><label for="au-from">Başlangıç</label><input class="input" type="date" id="au-from" value="' + esc(A.from) + '"></div><div class="field"><label for="au-to">Bitiş</label><input class="input" type="date" id="au-to" value="' + esc(A.to) + '"></div><div class="field"><label for="au-no">Adisyon no</label><input class="input" id="au-no" inputmode="numeric" value="' + esc(A.no) + '" placeholder="Tümü" style="width:150px"></div><button type="button" class="btn btn-primary btn-md" data-act="audit-load" style="align-self:flex-end">' + ic('refresh', 20) + 'Göster</button></div><div class="chips">' + kinds + '</div></section>' +
    '<section class="card" style="padding:8px 0 4px">' + (A.rows == null ? '<p class="muted pad">' + (A.loading ? 'Yükleniyor…' : '') + '</p>' : rows.length ? '<div class="tablebox"><table class="data"><thead><tr><th>Saat</th><th>İşlem</th><th>Adisyon</th><th>Ayrıntı</th></tr></thead><tbody>' + body + '</tbody></table></div>' + (A.more ? '<div class="row pad"><button type="button" class="btn btn-md" data-act="audit-more">Daha eski kayıtlar</button></div>' : '') : '<p class="muted pad">Bu aralıkta kayıt yok.</p>') + '</section>';
}

// ---- İstatistik ----
const nfc = new Intl.NumberFormat('tr-TR', { notation: 'compact', maximumFractionDigits: 1 });
function statsDefaults() {
  const T = ui.stats;
  if (!T.from) { const d = parseDay(S.businessDate); T.from = dayStr(new Date(d.getFullYear(), d.getMonth(), 1)); }
  if (!T.to) T.to = S.businessDate;
}
async function loadStats() {
  statsDefaults();
  const T = ui.stats;
  T.loading = true;
  if (ui.route === 'istatistik') render();
  const q = { mode: T.mode, from: T.from, to: T.to };
  const r = await act('stats', q);
  T.loading = false;
  if (r && T.mode === q.mode && T.from === q.from && T.to === q.to) {
    T.data = r.data;
    if (!T.data.products.some(p => p.productId === T.product)) T.product = T.data.featuredId && T.data.products.some(p => p.productId === T.data.featuredId) ? T.data.featuredId : (T.data.products[0] || {}).productId || 0;
  }
  if (ui.route === 'istatistik') render();
}
function shiftStats(dir) {
  const T = ui.stats; const d = parseDay(T.from);
  if (T.mode === 'day') T.from = addDays(T.from, dir);
  else if (T.mode === 'month') T.from = dayStr(new Date(d.getFullYear(), d.getMonth() + dir, 1));
  else if (T.mode === 'year') T.from = dayStr(new Date(d.getFullYear() + dir, 0, 1));
  else { const t = parseDay(T.to); const n = Math.round((t - d) / 864e5) + 1; T.from = addDays(T.from, dir * n); T.to = addDays(T.to, dir * n); }
  loadStats();
}
function setStatsMode(mode) {
  const T = ui.stats; const today = parseDay(S.businessDate);
  const cur = parseDay(T.from || S.businessDate);
  T.mode = mode;
  if (mode === 'day') T.from = (T.data && T.data.to) || S.businessDate;
  else if (mode === 'month') T.from = dayStr(new Date(cur.getFullYear(), cur.getMonth(), 1));
  else if (mode === 'year') T.from = dayStr(new Date(cur.getFullYear(), 0, 1));
  else { T.to = S.businessDate; T.from = addDays(S.businessDate, -29); }
  if (mode !== 'range' && parseDay(T.from) > today) T.from = S.businessDate;
  loadStats();
}
function statsControls() {
  const T = ui.stats;
  const seg = '<div class="seg" role="group" aria-label="Dönem">' + [['day', 'Gün'], ['month', 'Ay'], ['year', 'Yıl'], ['range', 'Tarih aralığı']].map(([k, l]) => '<button type="button" data-act="st-mode" data-m="' + k + '" aria-pressed="' + (T.mode === k) + '">' + l + '</button>').join('') + '</div>';
  let ctl = '';
  const prev = '<button type="button" class="icon-btn" data-act="st-shift" data-d="-1" aria-label="Önceki dönem">' + ic('left') + '</button>';
  const next = '<button type="button" class="icon-btn" data-act="st-shift" data-d="1" aria-label="Sonraki dönem">' + ic('right') + '</button>';
  if (T.mode === 'day') ctl = prev + '<label class="sr" for="st-day">Gün</label><input class="input" type="date" id="st-day" value="' + esc(T.from) + '">' + next;
  else if (T.mode === 'month') ctl = prev + '<label class="sr" for="st-month">Ay</label><input class="input" type="month" id="st-month" value="' + esc(T.from.slice(0, 7)) + '">' + next;
  else if (T.mode === 'year') ctl = prev + '<span class="label" style="font-weight:700;font-size:22px;min-width:80px;text-align:center">' + esc(T.from.slice(0, 4)) + '</span>' + next;
  else ctl = '<label class="sr" for="st-from">Başlangıç</label><input class="input" type="date" id="st-from" value="' + esc(T.from) + '"><span class="muted">–</span><label class="sr" for="st-to">Bitiş</label><input class="input" type="date" id="st-to" value="' + esc(T.to) + '">' + next.replace('data-d="1"', 'data-d="1" hidden');
  return '<section class="card pad stack"><div class="filters">' + seg + '<div class="date-nav">' + ctl + '</div><button type="button" class="btn btn-outline btn-md grow-gap" data-act="st-excel">' + ic('download', 20) + 'Excel’e aktar</button></div>' +
    (T.data ? '<p class="muted"><strong style="color:var(--ink)">' + esc(T.data.label) + '</strong>' + (T.data.partial ? (T.mode === 'day' ? ' (gün sürüyor)' : ' (bugüne kadar)') : '') + ' · karşılaştırma: ' + esc(T.data.prevLabel) + (T.data.partial && T.mode === 'day' ? ' · Gün bitmeden karşılaştırma eksik görünür.' : '') + '</p>' : '') + '</section>';
}
function scStats() {
  statsDefaults();
  const T = ui.stats; const D = T.data;
  const head = '<div class="page-head"><div class="ttl"><h1>İstatistik</h1><p class="muted">Ciro yalnız ödenen adisyonlardan hesaplanır. Gün sabah 05:00’te başlar.</p></div></div>';
  if (!D) return head + statsControls() + '<div class="card pad"><p class="muted">' + (T.loading ? 'Yükleniyor…' : 'Veri okunamadı.') + '</p></div>';
  const prevNote = (c, f) => c.prev ? 'önceki: ' + f(c.prev) : '';
  const kpis = '<div class="kpis' + (T.loading ? '" style="opacity:.6' : '') + '">' +
    kpi('Ciro', TL(D.revenue.value), deltaHTML(D.revenue) + '<span>' + prevNote(D.revenue, TL) + '</span>') +
    kpi('Adisyon', nf0.format(D.count.value), deltaHTML(D.count) + '<span>' + prevNote(D.count, nf0.format) + '</span>') +
    kpi('Ortalama adisyon', TL(D.avg.value), deltaHTML(D.avg) + '<span>' + prevNote(D.avg, TL) + '</span>') +
    (D.featuredId ? kpi('Satılan ' + D.featuredName, nf0.format(D.featured.value) + ' <span class="muted" style="font-size:20px">' + esc(D.featuredUnit) + '</span>', deltaHTML(D.featured) + '<span>' + prevNote(D.featured, nf0.format) + '</span>') : '') + '</div>';
  const empty = msg => '<div class="chart-empty">' + esc(msg) + '</div>';
  const has = D.count.value > 0;
  // Ciro grafiği: gün görünümünde saatlik, yılda aylık sütun, diğerlerinde günlük çizgi.
  let trend;
  if (T.mode === 'day') trend = '';
  else if (D.months && D.months.length) {
    const multiYear = D.months[0].month.slice(0, 4) !== D.months[D.months.length - 1].month.slice(0, 4);
    trend = card2('Aylık ciro', 'Ödenen adisyonlar, KDV dahil', has ? chartBox('st-trend', { kind: 'cols', fmt: 'tl', items: D.months.map(m => { const t = parseDay(m.month + '-01'); return { label: AY[t.getMonth()].slice(0, 3) + (multiYear ? ' ' + String(t.getFullYear()).slice(2) : ''), v: m.revenue, tip: [AY[t.getMonth()] + ' ' + t.getFullYear(), [TL(m.revenue), 'ciro'], [nf0.format(m.count), 'adisyon']] }; }) }, 300) : empty('Bu dönemde satış yok.'));
  } else {
    const multiMonth = D.days.length && D.days[0].date.slice(0, 7) !== D.days[D.days.length - 1].date.slice(0, 7);
    trend = card2('Günlük ciro', 'Ödenen adisyonlar, KDV dahil' + (D.partial ? ' · son gün sürüyor' : ''), has ? chartBox('st-trend', { kind: 'line', fmt: 'tl', partialLast: D.partial, items: D.days.map(d => ({ label: multiMonth ? dmShort(d.date) : String(parseDay(d.date).getDate()), long: dmShort(d.date), v: d.revenue, tip: [dateLong(d.date), [TL(d.revenue), 'ciro'], [nf0.format(d.count), 'adisyon'], [d.count ? TL(Math.round(d.revenue / d.count)) : '—', 'ortalama']] })) }, 300) : empty('Bu dönemde satış yok.'));
  }
  const pay = card2('Ödeme şekilleri', 'Ödenen adisyonların tutarı', payShareHTML(D.byPayment || [], D.revenue.value));
  const top = D.products.slice(0, 12);
  const prods = card2('Ürünler', 'Satış adedi' + (D.products.length > 12 ? ' · ilk 12' : ''), D.products.length ? chartBox('st-prod', { kind: 'hbars', items: top.map(p => ({ label: p.name, v: p.qty, valueText: nf0.format(p.qty) + ' ' + p.unit, tip: [p.name, [nf0.format(p.qty) + ' ' + p.unit, 'adet'], [TL(p.total), 'tutar'], ['%' + nf1.format(D.revenue.value ? p.total * 100 / D.revenue.value : 0), 'ciro payı']] })) }, Math.max(120, top.length * 40 + 16)) : empty('Bu dönemde satış yok.'));
  let one = '';
  if (T.mode !== 'day' && D.products.length) {
    const pi = Math.max(0, D.products.findIndex(p => p.productId === T.product));
    const p = D.products[pi];
    const sel = '<label class="sr" for="st-product">Ürün</label><select class="input" id="st-product" style="width:auto;min-height:52px">' + D.products.map(x => '<option value="' + x.productId + '"' + (x.productId === p.productId ? ' selected' : '') + '>' + esc(x.name) + '</option>').join('') + '</select>';
    let items;
    if (D.months && D.months.length) {
      const by = {};
      D.days.forEach((d, i) => { const m = d.date.slice(0, 7); by[m] = (by[m] || 0) + ((D.matrix[i] || [])[pi] || 0); });
      const ys = Object.keys(by).sort(); const multiYear = ys.length && ys[0].slice(0, 4) !== ys[ys.length - 1].slice(0, 4);
      items = ys.map(m => { const t = parseDay(m + '-01'); return { label: AY[t.getMonth()].slice(0, 3) + (multiYear ? ' ' + String(t.getFullYear()).slice(2) : ''), v: by[m], tip: [AY[t.getMonth()] + ' ' + t.getFullYear(), [nf0.format(by[m]) + ' ' + p.unit, p.name]] }; });
    } else {
      const multiMonth = D.days.length && D.days[0].date.slice(0, 7) !== D.days[D.days.length - 1].date.slice(0, 7);
      items = D.days.map((d, i) => { const q = (D.matrix[i] || [])[pi] || 0; return { label: multiMonth ? dmShort(d.date) : String(parseDay(d.date).getDate()), v: q, tip: [dateLong(d.date), [nf0.format(q) + ' ' + p.unit, p.name]] }; });
    }
    one = '<section class="card pad stack"><div class="card-head"><div><h2>Ürün bazında satış</h2><p class="muted small">' + esc(p.name) + ' · ' + (D.months && D.months.length ? 'aylık' : 'günlük') + ' adet</p></div>' + sel + '</div>' + chartBox('st-one', { kind: 'cols', fmt: 'n', items }, 260) + '</section>';
  }
  const hours = hourItems(D.hours);
  const hourChart = card2('Saatlere göre yoğunluk', 'Masanın açıldığı saate göre adisyon sayısı', hours.length ? chartBox('st-hour', { kind: 'cols', fmt: 'n', items: hours }, 260) : empty('Bu dönemde satış yok.'));
  const waiters = card2('Garsonlar', 'Ödenen adisyonlar', (D.byWaiter || []).length ? '<table class="legend-table"><tbody>' + D.byWaiter.map(w => '<tr><td>' + ic('user', 18) + ' ' + esc(w.name) + '</td><td class="r muted">' + nf0.format(w.count) + ' adisyon</td><td class="r"><strong>' + TL(w.total) + '</strong></td><td class="r muted">ort. ' + TL(w.count ? Math.round(w.total / w.count) : 0) + '</td></tr>').join('') + '</tbody></table>' : '<p class="muted">Ödenen adisyon yok.</p>');
  const extra = '<div class="status-line"><span>KDV toplamı <strong>' + TL(D.vat) + '</strong></span><span>Matrah <strong>' + TL(D.revenue.value - D.vat) + '</strong></span><span>İptal <strong>' + nf0.format(D.cancelled) + '</strong>' + (D.cancelled ? ' · ' + TL(D.cancelledTotal) : '') + '</span>' + (D.discounts ? '<span>İndirim <strong>' + nf0.format(D.discounts) + '</strong> · −' + TL(D.discountTotal) + '</span>' : '') + '</div>';
  return head + statsControls() + kpis + extra +
    (trend ? '<div class="grid-2">' + trend + pay + '</div><div class="grid-2">' + prods + (one || hourChart) + '</div>' + (one ? '<div class="grid-2">' + hourChart + waiters + '</div>' : '<div class="grid-2">' + waiters + '</div>')
      : '<div class="grid-2">' + hourChart + pay + '</div><div class="grid-2">' + prods + waiters + '</div>') +
    dayTable(D);
}
function card2(title, sub, inner) {
  return '<section class="card pad stack"><div class="card-head"><div><h2>' + esc(title) + '</h2>' + (sub ? '<p class="muted small">' + esc(sub) + '</p>' : '') + '</div></div>' + inner + '</section>';
}
function hourItems(hours) {
  // İş günü 05:00'te başlar: saatler 05 → 04 sırasıyla, baştaki ve sondaki boş saatler atlanır.
  const order = []; for (let i = 0; i < 24; i++) order.push((i + 5) % 24);
  let a = 0, b = 23;
  while (a <= b && !(hours[order[a]] || {}).count) a++;
  while (b >= a && !(hours[order[b]] || {}).count) b--;
  if (a > b) return [];
  const out = [];
  for (let i = a; i <= b; i++) { const h = order[i]; const x = hours[h] || { count: 0, revenue: 0 }; out.push({ label: pad(h), v: x.count, tip: [pad(h) + ':00 – ' + pad((h + 1) % 24) + ':00', [nf0.format(x.count), 'adisyon'], [TL(x.revenue), 'ciro']] }); }
  return out;
}
function dayTable(D) {
  if (!D.days.length) return '';
  const T = ui.stats; const sort = T.sort;
  // Uzun aralıklarda (ör. birkaç yıl) tablo aylara göre toplanır; binlerce satır ekranı yavaşlatır.
  const byMonth = D.days.length > 400;
  const sums = ['revenue', 'count', 'cash', 'card', 'transfer', 'unknown', 'cancelled', 'discount'];
  let rows;
  if (byMonth) {
    const mi = {}; rows = [];
    D.days.forEach((d, i) => {
      const m = d.date.slice(0, 7);
      let r = mi[m];
      if (!r) { r = mi[m] = { date: m, q: D.products.map(() => 0) }; sums.forEach(k => { r[k] = 0; }); rows.push(r); }
      sums.forEach(k => { r[k] += d[k] || 0; });
      (D.matrix[i] || []).forEach((q, j) => { r.q[j] += q || 0; });
    });
  } else {
    rows = D.days.map((d, i) => Object.assign({ q: D.matrix[i] || [] }, d));
  }
  const cols = [['date', byMonth ? 'Ay' : 'Tarih', 0], ['revenue', 'Ciro', 1], ['count', 'Adisyon', 1], ['avg', 'Ortalama', 1], ['cash', 'Nakit', 1], ['card', 'Kredi Kartı', 1], ['transfer', 'Havale', 1]];
  if (D.days.some(d => d.unknown)) cols.push(['unknown', 'Belirtilmemiş', 1]);
  cols.push(['cancelled', 'İptal', 1]);
  if (D.days.some(d => d.discount)) cols.push(['discount', 'İndirim', 1]);
  D.products.forEach((p, j) => cols.push(['p' + j, p.name, 1]));
  if (D.products.length) cols.push(['qty', 'Toplam adet', 1]);
  const valOf = (r, k) => k === 'date' ? r.date : k === 'avg' ? (r.count ? r.revenue / r.count : 0) : k === 'qty' ? r.q.reduce((a, q) => a + (q || 0), 0) : /^p\d+$/.test(k) ? (r.q[Number(k.slice(1))] || 0) : (r[k] || 0);
  const hideEmpty = byMonth || T.mode === 'year';
  const idx = rows.map((r, i) => i).filter(i => !hideEmpty || rows[i].count || rows[i].cancelled);
  idx.sort((x, y) => { const a = valOf(rows[x], sort.key), b = valOf(rows[y], sort.key); return (a < b ? -1 : a > b ? 1 : 0) * sort.dir || (x - y); });
  const money = new Set(['revenue', 'avg', 'cash', 'card', 'transfer', 'unknown', 'discount']);
  const cell = (k, v) => k === 'date' ? '' : money.has(k) ? TL(Math.round(v)) : nf0.format(v);
  const th = cols.map(([k, l, r]) => '<th' + (r ? ' class="r"' : '') + ' aria-sort="' + (sort.key === k ? (sort.dir > 0 ? 'ascending' : 'descending') : 'none') + '"><button type="button" class="th-sort" data-act="st-sort" data-k="' + k + '">' + esc(l) + (sort.key === k ? ic(sort.dir > 0 ? 'up' : 'down', 16, 2.6) : '') + '</button></th>').join('');
  const first = r => byMonth ? (() => { const t = parseDay(r.date + '-01'); return esc(AY[t.getMonth()] + ' ' + t.getFullYear()); })()
    : esc(dateShort(r.date)) + ' <span class="muted">' + GUN[parseDay(r.date).getDay()].slice(0, 3) + '</span>';
  const body = idx.map(i => { const r = rows[i]; return '<tr><td class="nowrap">' + first(r) + '</td>' + cols.slice(1).map(([k]) => '<td class="r">' + cell(k, valOf(r, k)) + '</td>').join('') + '</tr>'; }).join('');
  const tot = {}; cols.slice(1).forEach(([k]) => { tot[k] = 0; });
  rows.forEach(r => cols.slice(1).forEach(([k]) => { if (k !== 'avg') tot[k] += valOf(r, k); }));
  tot.avg = tot.count ? tot.revenue / tot.count : 0;
  const foot = '<tr class="total"><td>Toplam</td>' + cols.slice(1).map(([k]) => '<td class="r">' + cell(k, tot[k]) + '</td>').join('') + '</tr>';
  const note = 'Sütun başlığına tıklayarak sıralayın. Ürün sütunları satış adedidir.' + (byMonth ? ' Aralık uzun olduğu için aylara göre toplanmıştır; satış olmayan aylar gösterilmez.' : T.mode === 'year' ? ' Satış olmayan günler gösterilmez.' : '');
  return '<section class="card stack" style="padding:16px 0 4px;gap:10px"><div class="card-head" style="padding:0 18px"><div><h2>' + (byMonth ? 'Aylara göre' : 'Günlere göre') + '</h2><p class="muted small">' + note + '</p></div></div><div class="tablebox scrollbox"><table class="data sticky-first"><thead><tr>' + th + '</tr></thead><tbody>' + body + foot + '</tbody></table></div></section>';
}

// ---- Grafikler (elle çizilen SVG) ----
const CHARTS = {};
function chartBox(id, spec, h) {
  CHARTS[id] = Object.assign({ h }, spec);
  return '<div class="chart viz" data-chart="' + id + '" style="height:' + h + 'px"></div>';
}
let measureCtx = null;
function textW(t, size, weight) {
  if (!measureCtx) measureCtx = document.createElement('canvas').getContext('2d');
  measureCtx.font = (weight || 400) + ' ' + size + 'px Plex, "Segoe UI", sans-serif';
  return measureCtx.measureText(t).width;
}
function niceMax(v) {
  if (!(v > 0)) return { max: 1, step: 1 };
  const raw = v / 4; const p = Math.pow(10, Math.floor(Math.log10(raw)));
  const step = Math.max(1, [1, 2, 5, 10].map(m => m * p).find(x => x >= raw));
  return { max: Math.ceil(v / step) * step, step };
}
// Eksen yazısı: 60 bin, 1,5 mn (₺ ya da adet). Küçük sayılar olduğu gibi.
function fmtTick(v, fmt) {
  const x = fmt === 'tl' ? v / 100 : v;
  if (x >= 1e6) return nf1.format(x / 1e6).replace(/,0$/, '') + ' mn';
  if (x >= (fmt === 'tl' ? 1e3 : 1e4)) return nf1.format(x / 1e3).replace(/,0$/, '') + ' bin';
  return nf0.format(x);
}
function fmtVal(v, fmt) { return fmt === 'tl' ? TL0(v) : nf0.format(v); }
function topRounded(x, y, w, h, r) {
  if (h <= 0) return '';
  r = Math.min(r, w / 2, h);
  return 'M' + x + ',' + (y + h) + 'V' + (y + r) + 'Q' + x + ',' + y + ' ' + (x + r) + ',' + y + 'H' + (x + w - r) + 'Q' + (x + w) + ',' + y + ' ' + (x + w) + ',' + (y + r) + 'V' + (y + h) + 'Z';
}
function rightRounded(x, y, w, h, r) {
  if (w <= 0) return '';
  r = Math.min(r, h / 2, w);
  return 'M' + x + ',' + y + 'H' + (x + w - r) + 'Q' + (x + w) + ',' + y + ' ' + (x + w) + ',' + (y + r) + 'V' + (y + h - r) + 'Q' + (x + w) + ',' + (y + h) + ' ' + (x + w - r) + ',' + (y + h) + 'H' + x + 'Z';
}
function yAxis(fmt, max, step, W, padL, padR, top, plotH) {
  let g = '';
  for (let v = 0; v <= max + 1e-9; v += step) {
    const y = top + plotH - v / max * plotH;
    g += '<line x1="' + padL + '" x2="' + (W - padR) + '" y1="' + y + '" y2="' + y + '" stroke="var(' + (v === 0 ? '--axis' : '--grid') + ')" stroke-width="1"/>';
    g += '<text class="tk" x="' + (padL - 10) + '" y="' + (y + 5) + '" text-anchor="end">' + esc(fmtTick(v, fmt)) + '</text>';
  }
  return g;
}
function xLabels(items, xs, y, every) {
  const n = items.length;
  return items.map((it, i) => (i % every === 0 || (i === n - 1 && (n - 1) % every >= Math.ceil(every / 2)))
    ? '<text class="tk" x="' + xs[i] + '" y="' + y + '" text-anchor="middle">' + esc(it.label) + '</text>' : '').join('');
}
function drawChart(el, spec) {
  const W = Math.floor(el.clientWidth), H = spec.h;
  if (!W) return;
  const tip = '<div class="viz-tip" hidden></div>';
  const items = spec.items || [];
  if (spec.kind === 'hbars') {
    const max = Math.max(1, ...items.map(i => i.v));
    const labW = Math.min(W * 0.38, Math.max(...items.map(i => textW(i.label, 17))) + 14);
    // Sığmayan ürün adı kısaltılır (tamamı ipucunda görünür).
    const fitText = (t, w) => { if (textW(t, 17) <= w) return t; let x = t; while (x.length > 1 && textW(x + '…', 17) > w) x = x.slice(0, -1); return x + '…'; };
    items.forEach(it => { it.shown = fitText(it.label, labW - 18); });
    const valW = Math.max(...items.map(i => textW(i.valueText, 16, 600))) + 12;
    const plotW = Math.max(40, W - labW - valW - 4);
    const rowH = 40, bh = 22;
    let g = '';
    items.forEach((it, i) => {
      const y = 8 + i * rowH; const w = Math.max(it.v > 0 ? 3 : 0, it.v / max * plotW);
      g += '<text class="lbl-n" x="' + (labW - 14) + '" y="' + (y + bh / 2 + 6) + '" text-anchor="end">' + esc(it.shown) + '</text>';
      g += '<rect class="hit" data-i="' + i + '" tabindex="0" x="0" y="' + (y - 8) + '" width="' + W + '" height="' + rowH + '" aria-label="' + esc(it.label + ': ' + it.valueText) + '"/>';
      g += '<path class="bar" d="' + rightRounded(labW, y, w, bh, 4) + '" fill="var(--s1)"/>';
      g += '<text class="lbl-v" x="' + (labW + w + 8) + '" y="' + (y + bh / 2 + 6) + '">' + esc(it.valueText) + '</text>';
    });
    g += '<line x1="' + labW + '" x2="' + labW + '" y1="0" y2="' + (items.length * rowH) + '" stroke="var(--axis)" stroke-width="1"/>';
    el.innerHTML = '<svg width="' + W + '" height="' + H + '" role="img" aria-label="Ürünlere göre satış adedi">' + g + '</svg>' + tip;
    el._items = items; el._mode = 'bars';
    el._pos = items.map((it, i) => ({ x: labW + Math.max(3, it.v / max * plotW) / 2, y: 8 + i * rowH }));
    return;
  }
  const { max, step } = niceMax(Math.max(0, ...items.map(i => i.v)));
  const padL = Math.max(...[0, step, max].map(v => textW(fmtTick(v, spec.fmt), 16))) + 18, padR = 14, top = 26, bottom = 34;
  const plotW = W - padL - padR, plotH = H - top - bottom;
  const n = items.length;
  let g = yAxis(spec.fmt, max, step, W, padL, padR, top, plotH);
  const yOf = v => top + plotH - v / max * plotH;
  const maxLabels = Math.max(2, Math.floor(plotW / 52));
  const every = Math.max(1, Math.ceil(n / maxLabels));
  let xs;
  if (spec.kind === 'line') {
    xs = items.map((_, i) => n === 1 ? padL + plotW / 2 : padL + i * plotW / (n - 1));
    const pts = items.map((it, i) => xs[i].toFixed(1) + ',' + yOf(it.v).toFixed(1));
    // Dönem bugünü içeriyorsa son gün henüz bitmedi: o parça soluk çizilir, ucu boş halkadır.
    const solid = spec.partialLast && n > 1 ? pts.slice(0, n - 1) : pts;
    g += '<path d="M' + solid.join('L') + 'L' + xs[solid.length - 1].toFixed(1) + ',' + yOf(0) + 'L' + xs[0].toFixed(1) + ',' + yOf(0) + 'Z" fill="var(--wash)"/>';
    g += '<path d="M' + solid.join('L') + '" fill="none" stroke="var(--s1)" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"/>';
    if (solid.length < n) {
      g += '<path d="M' + pts[n - 2] + 'L' + pts[n - 1] + '" fill="none" stroke="var(--s1)" stroke-opacity=".35" stroke-width="2" stroke-linecap="round"/>';
      g += '<circle cx="' + xs[n - 1] + '" cy="' + yOf(items[n - 1].v) + '" r="4" fill="var(--surface)" stroke="var(--s1)" stroke-width="2"/>';
    }
    // En yüksek gün: nokta ve değer (seçici etiket)
    let mi = 0; items.forEach((it, i) => { if (it.v > items[mi].v) mi = i; });
    if (items[mi].v > 0) {
      const lx = Math.min(Math.max(xs[mi], padL + 40), W - padR - 40);
      g += '<circle cx="' + xs[mi] + '" cy="' + yOf(items[mi].v) + '" r="5" fill="var(--s1)" stroke="var(--surface)" stroke-width="2"/>';
      g += '<text class="lbl-v" x="' + lx + '" y="' + (yOf(items[mi].v) - 12) + '" text-anchor="middle">' + esc(fmtVal(items[mi].v, spec.fmt)) + '</text>';
    }
    g += '<line class="xh" x1="0" x2="0" y1="' + top + '" y2="' + (top + plotH) + '" stroke="var(--tick)" stroke-width="1" visibility="hidden"/><circle class="xh-dot" r="5" fill="var(--s1)" stroke="var(--surface)" stroke-width="2" visibility="hidden"/>';
    g += '<rect class="hit" data-line="1" tabindex="0" x="' + padL + '" y="' + top + '" width="' + plotW + '" height="' + plotH + '" aria-label="Günlük ciro grafiği; sağ-sol oklarla gezin"/>';
    el._mode = 'line';
  } else {
    const band = plotW / Math.max(1, n); const bw = Math.max(2, Math.min(24, band * 0.62));
    xs = items.map((_, i) => padL + band * (i + 0.5));
    let mi = 0; items.forEach((it, i) => { if (it.v > items[mi].v) mi = i; });
    items.forEach((it, i) => {
      const y = yOf(it.v); const h = top + plotH - y;
      g += '<rect class="hit" data-i="' + i + '" tabindex="0" x="' + (xs[i] - band / 2) + '" y="' + top + '" width="' + band + '" height="' + plotH + '" aria-label="' + esc(it.tip[0] + ': ' + it.tip.slice(1).map(t => t[0] + ' ' + t[1]).join(', ')) + '"/>';
      g += '<path class="bar" d="' + topRounded(xs[i] - bw / 2, y, bw, h, 4) + '" fill="var(--s1)"/>';
    });
    if (items[mi].v > 0) g += '<text class="lbl-v" x="' + Math.min(Math.max(xs[mi], padL + 30), W - padR - 30) + '" y="' + (yOf(items[mi].v) - 8) + '" text-anchor="middle">' + esc(fmtVal(items[mi].v, spec.fmt)) + '</text>';
    el._mode = 'bars';
  }
  g += xLabels(items, xs, H - 10, every);
  el.innerHTML = '<svg width="' + W + '" height="' + H + '" role="img">' + g + '</svg>' + tip;
  el._items = items; el._xs = xs; el._yOf = yOf; el._top = top; el._partial = !!spec.partialLast;
  el._pos = items.map((it, i) => ({ x: xs[i], y: yOf(it.v) }));
}
function drawCharts() {
  document.querySelectorAll('[data-chart]').forEach(el => {
    const spec = CHARTS[el.dataset.chart];
    if (spec) drawChart(el, spec);
  });
}
function showTip(el, i) {
  const tipEl = el.querySelector('.viz-tip'); const it = el._items && el._items[i]; if (!tipEl || !it) return;
  tipEl.textContent = '';
  const h = document.createElement('div'); h.className = 't-head'; h.textContent = it.tip[0]; tipEl.appendChild(h);
  it.tip.slice(1).forEach((r, j) => {
    const row = document.createElement('div'); row.className = 't-row';
    if (j === 0) { const k = document.createElement('span'); k.className = 't-key'; k.style.background = 'var(--s1)'; row.appendChild(k); }
    const b = document.createElement('b'); b.textContent = r[0]; row.appendChild(b);
    const sp = document.createElement('span'); sp.className = 'muted'; sp.textContent = r[1]; row.appendChild(sp);
    tipEl.appendChild(row);
  });
  if (el._partial && i === el._items.length - 1) { const nt = document.createElement('div'); nt.className = 't-head'; nt.textContent = 'gün sürüyor'; tipEl.appendChild(nt); }
  const p = el._pos[i];
  tipEl.hidden = false;
  const w = tipEl.offsetWidth;
  tipEl.style.left = Math.min(Math.max(p.x, w / 2 + 4), el.clientWidth - w / 2 - 4) + 'px';
  tipEl.style.top = Math.max(p.y, tipEl.offsetHeight + 18) + 'px';
  if (el._mode === 'line') {
    const xh = el.querySelector('.xh'), dot = el.querySelector('.xh-dot');
    xh.setAttribute('x1', p.x); xh.setAttribute('x2', p.x); xh.setAttribute('visibility', 'visible');
    dot.setAttribute('cx', p.x); dot.setAttribute('cy', p.y); dot.setAttribute('visibility', 'visible');
  }
  el._cur = i;
}
function hideTip(el) {
  const tipEl = el.querySelector('.viz-tip'); if (tipEl) tipEl.hidden = true;
  el.querySelectorAll('.xh, .xh-dot').forEach(x => x.setAttribute('visibility', 'hidden'));
  el._cur = null;
}
document.addEventListener('pointermove', e => {
  const el = e.target.closest && e.target.closest('.chart'); if (!el || !el._items) return;
  if (el._mode === 'line') {
    const hit = e.target.closest('[data-line]'); if (!hit) { hideTip(el); return; }
    const r = el.getBoundingClientRect(); const x = e.clientX - r.left;
    let best = 0; el._xs.forEach((v, i) => { if (Math.abs(v - x) < Math.abs(el._xs[best] - x)) best = i; });
    if (best !== el._cur) showTip(el, best);
  } else {
    const hit = e.target.closest('[data-i]'); if (!hit) { hideTip(el); return; }
    const i = Number(hit.dataset.i); if (i !== el._cur) showTip(el, i);
  }
});
document.addEventListener('pointerout', e => { const el = e.target.closest && e.target.closest('.chart'); if (el && !el.contains(e.relatedTarget)) hideTip(el); });
document.addEventListener('focusin', e => {
  const el = e.target.closest && e.target.closest('.chart'); if (!el || !el._items) return;
  if (e.target.dataset.i != null) showTip(el, Number(e.target.dataset.i));
  else if (e.target.dataset.line) showTip(el, el._items.length - 1);
});
document.addEventListener('focusout', e => { const el = e.target.closest && e.target.closest('.chart'); if (el) hideTip(el); });

// ---- Ürünler ----
function scProducts() {
  const cats = S.categories;
  const filter = ui.prodFilter;
  if (filter !== 'all' && !cats.some(c => c.id === filter)) ui.prodFilter = 'all';
  const tabs = '<div class="cat-tabs" role="group" aria-label="Kategoriye göre süz"><button type="button" class="cat-tab" data-act="prod-filter" data-c="all" aria-pressed="' + (ui.prodFilter === 'all') + '">Tümü</button>' + cats.map(c => '<button type="button" class="cat-tab" data-act="prod-filter" data-c="' + c.id + '" aria-pressed="' + (ui.prodFilter === c.id) + '"><span class="dot" style="background:' + colorHex(c.color) + '"></span>' + esc(c.name) + '</button>').join('') + '</div>';
  const list = [];
  for (const c of cats) {
    if (ui.prodFilter !== 'all' && c.id !== ui.prodFilter) continue;
    const ps = S.products.filter(p => p.categoryId === c.id);
    ps.forEach((p, i) => list.push({ p, c, first: i === 0, last: i === ps.length - 1 }));
  }
  const rows = list.map(({ p, c, first, last }) => '<tr class="' + (ui.editProduct === p.id ? 'sel' : '') + (p.active ? '' : ' off') + '"><td><div class="row" style="gap:6px;flex-wrap:nowrap"><button type="button" class="icon-btn" data-act="p-move" data-id="' + p.id + '" data-d="-1" aria-label="' + esc(p.name) + ' yukarı"' + (first ? ' disabled' : '') + '>' + ic('up') + '</button><button type="button" class="icon-btn" data-act="p-move" data-id="' + p.id + '" data-d="1" aria-label="' + esc(p.name) + ' aşağı"' + (last ? ' disabled' : '') + '>' + ic('down') + '</button></div></td><td><span class="row" style="gap:10px;flex-wrap:nowrap"><span class="dot" style="background:' + colorHex(c.color) + '"></span><span><strong>' + esc(p.name) + '</strong><br><span class="muted small">' + esc(p.unit) + '</span></span></span></td><td>' + esc(c.name) + '</td><td class="r nowrap"><strong>' + (p.price > 0 ? TL(p.price) : '<span class="pill warn">Fiyat yok</span>') + '</strong></td><td class="nowrap">' + (p.vat == null ? '%' + S.defaultVat + ' <span class="muted small">varsayılan</span>' : '%' + p.vat) + '</td><td class="nowrap">' + (p.active ? '<span class="pill good">Satışta</span>' : '<span class="pill quiet">Gizli</span>') + '</td><td class="r"><button type="button" class="btn btn-md' + (ui.editProduct === p.id ? ' btn-outline' : '') + '" data-act="p-edit" data-id="' + p.id + '" data-fk="pe-' + p.id + '">Düzenle</button></td></tr>').join('');

  return '<div class="page-head"><div class="ttl"><h1>Ürünler ve fiyatlar</h1><p class="muted">Fiyatları KDV dahil girin. Değişiklik satış ekranına hemen yansır; geçmiş adisyonların fiyatı değişmez.</p></div><button type="button" class="btn btn-outline" data-act="bulk-price" data-fk="bulk">' + ic('percent') + 'Toplu zam</button><button type="button" class="btn btn-primary" data-act="p-new" data-fk="p-new">' + ic('plus') + 'Yeni ürün</button></div>' +
    '<div class="split"><section class="card grow" style="gap:12px;padding:16px 0 4px;overflow:hidden" aria-label="Ürün listesi"><div style="padding:0 18px">' + tabs + '</div><p class="muted small" style="padding:0 18px">Satış ekranındaki buton sırası bu listeyle aynıdır; okları kullanarak değiştirin.</p><div class="tablebox"><table class="data"><thead><tr><th>Sıra</th><th>Ürün</th><th>Kategori</th><th class="r">Fiyat</th><th>KDV</th><th>Durum</th><th class="r"><span class="sr">İşlem</span></th></tr></thead><tbody>' + rows + '</tbody></table></div></section>' +
    '<div class="side">' + productForm() + categoryCard() + '</div></div>';
}

// ---- Toplu zam ----
function bulkFlow(keep) {
  const priced = S.products.filter(p => p.price > 0);
  if (!priced.length) { toast('Fiyatı girilmiş ürün yok.', 'bad'); return; }
  if (!keep || !ui.bulk) ui.bulk = { percent: '10', round: 500, ids: new Set(priced.map(p => p.id)) };
  const B = ui.bulk;
  const groups = S.categories.map(c => {
    const ps = priced.filter(p => p.categoryId === c.id); if (!ps.length) return '';
    return '<div class="stack" style="gap:6px"><span class="lbl-b"><span class="dot" style="background:' + colorHex(c.color) + '"></span> ' + esc(c.name) + '</span><div class="radios">' + ps.map(p => '<label class="radio"><input type="checkbox" data-bulk-id="' + p.id + '"' + (B.ids.has(p.id) ? ' checked' : '') + '><span>' + esc(p.name) + ' · ' + TL(p.price) + '</span></label>').join('') + '</div></div>';
  }).join('');
  const rounds = [[0, 'Yok (kuruşlu)'], [100, '1 ₺'], [500, '5 ₺'], [1000, '10 ₺']];
  openModal({
    title: 'Toplu zam',
    wide: true,
    body: '<div class="fields"><div class="field"><label for="bulk-pct">Zam oranı (%)</label><input class="input money" id="bulk-pct" inputmode="decimal" value="' + esc(B.percent) + '" data-autofocus><p class="hint">İndirim için eksi yazın (örnek: -5).</p></div>' +
      '<fieldset class="reasons"><legend class="lbl-b">Yeni fiyatı yuvarla</legend><div class="radios">' + rounds.map(([v, l]) => '<label class="radio"><input type="radio" name="bulk-round" value="' + v + '"' + (B.round === v ? ' checked' : '') + '><span>' + l + '</span></label>').join('') + '</div></fieldset></div>' +
      '<div class="row"><span class="lbl-b" style="margin-right:auto">Zam yapılacak ürünler</span><button type="button" class="btn btn-md" data-act="bulk-all" data-v="1">Tümünü seç</button><button type="button" class="btn btn-md" data-act="bulk-all" data-v="0">Hiçbiri</button></div>' + groups +
      '<p class="hint">Fiyatı girilmemiş ürünler listede yok. Ucuz ürünlerde yuvarlama kendiliğinden küçülür (ör. 5 ₺ seçiliyse 50 ₺ altındaki ürünler 1 ₺’ye yuvarlanır); zam hiçbir fiyatı düşürmez. Önce yeni fiyatları görürsünüz; “Uygula”ya basmadan hiçbir fiyat değişmez.</p>',
    actions: [{ label: 'Vazgeç', onClick: () => closeModal() }, { label: 'Yeni fiyatları göster', kind: 'btn-primary', icon: 'eye', onClick: () => bulkPreview() }]
  });
}
function readBulk() {
  const B = ui.bulk;
  B.percent = val('bulk-pct').trim();
  const r = document.querySelector('input[name="bulk-round"]:checked'); B.round = r ? Number(r.value) : 0;
  B.ids = new Set([...document.querySelectorAll('[data-bulk-id]')].filter(x => x.checked).map(x => Number(x.dataset.bulkId)));
  const pct = Number(B.percent.replace(',', '.'));
  return pct;
}
async function bulkPreview() {
  const pct = readBulk(); const B = ui.bulk;
  if (!isFinite(pct) || pct === 0) { toast('Zam oranını yazın (örnek: 10).', 'bad'); return; }
  if (!B.ids.size) { toast('En az bir ürün seçin.', 'bad'); return; }
  const r = await act('bulkPrice', { ids: [...B.ids], percent: pct, round: B.round, apply: false });
  if (!r) return;
  const ch = r.data.changes; const same = r.data.unchanged || [];
  openModal({
    title: 'Toplu ' + (pct > 0 ? 'zam' : 'indirim') + ' · %' + nf1.format(Math.abs(pct)).replace(/,0$/, ''),
    wide: true,
    body: '<div class="tablebox scrollbox" style="max-height:48vh"><table class="data"><thead><tr><th>Ürün</th><th class="r">Şu an</th><th class="r">Yeni</th><th class="r">Fark</th></tr></thead><tbody>' + ch.map(c => '<tr><td><strong>' + esc(c.name) + '</strong></td><td class="r">' + TL(c.old) + '</td><td class="r"><strong>' + TL(c.new) + '</strong></td><td class="r muted">' + (c.new >= c.old ? '+' : '') + TL(c.new - c.old) + '</td></tr>').join('') + '</tbody></table></div>' +
      (same.length ? '<div class="note warn">' + ic('alert') + '<p>Yuvarlama nedeniyle fiyatı değişmeyenler: ' + same.map(esc).join(', ') + '.</p></div>' : '') +
      '<div class="note info">' + ic('info') + '<p>' + ch.length + ' ürünün fiyatı değişecek. Geçmiş adisyonların ve şu an açık masalardaki ürünlerin fiyatı değişmez. Her değişiklik işlem kaydına yazılır.</p></div>',
    actions: [{ label: 'Geri', icon: 'back', onClick: () => bulkFlow(true) }, { label: 'Uygula', kind: 'btn-primary', icon: 'check', onClick: () => once('bulk-apply', async () => {
      const r2 = await act('bulkPrice', { ids: [...B.ids], percent: pct, round: B.round, apply: true });
      if (!r2) return;
      closeModal(true); ui.bulk = null; render();
      toast(r2.data.changes.length + ' ürünün fiyatı güncellendi.', 'good');
    }) }]
  });
}

function productForm() {
  if (ui.editProduct == null) return '<section class="card pad stack"><h2 class="sec-title">Ürün düzenle</h2><p class="muted">Düzenlemek için listeden bir ürün seçin ya da <strong>Yeni ürün</strong>’e basın.</p></section>';
  const isNew = ui.editProduct === 'new';
  const p = isNew ? { id: 0, name: '', categoryId: (S.categories[0] || {}).id, price: 0, vat: null, unit: 'porsiyon', active: true } : productById(ui.editProduct);
  if (!p) return '';
  const units = S.units.includes(p.unit) ? S.units : S.units.concat([p.unit]);
  const vatOpts = [['', 'Varsayılan (%' + S.defaultVat + ')'], ['1', '%1'], ['10', '%10'], ['20', '%20']];
  const cur = p.vat == null ? '' : String(p.vat);
  return '<form class="card pad stack" id="p-form" novalidate><h2 class="sec-title">' + (isNew ? 'Yeni ürün' : 'Ürünü düzenle: ' + esc(p.name)) + '</h2>' +
    '<div class="field"><label for="f-name">Ürün adı</label><input class="input" id="f-name" type="text" value="' + esc(p.name) + '" maxlength="40" autocomplete="off"></div>' +
    '<div class="fields"><div class="field"><label for="f-price">Fiyat (KDV dahil, ₺)</label><input class="input money" id="f-price" type="text" inputmode="decimal" value="' + priceText(p.price) + '" placeholder="0,00"></div>' +
    '<div class="field"><label for="f-unit">Birim</label><select class="input" id="f-unit">' + units.map(u => '<option' + (u === p.unit ? ' selected' : '') + '>' + esc(u) + '</option>').join('') + '</select></div></div>' +
    '<div class="field"><label for="f-cat">Kategori</label><select class="input" id="f-cat">' + S.categories.map(c => '<option value="' + c.id + '"' + (c.id === p.categoryId ? ' selected' : '') + '>' + esc(c.name) + '</option>').join('') + '</select></div>' +
    '<fieldset class="reasons"><legend class="lbl-b">KDV oranı</legend><div class="radios">' + vatOpts.map(([v, l]) => '<label class="radio"><input type="radio" name="f-vat" value="' + v + '"' + (v === cur ? ' checked' : '') + '><span>' + l + '</span></label>').join('') + '</div><p class="hint">Yeme-içmede bugün %10. “Varsayılan” seçilirse Ayarlar’daki oran kullanılır. Yeni oran yalnızca bundan sonraki adisyonlara uygulanır.</p></fieldset>' +
    '<label class="switch-row" for="f-active"><span>Satış ekranında göster</span><input class="switch" type="checkbox" id="f-active"' + (p.active ? ' checked' : '') + '></label>' +
    '<div class="row" style="justify-content:flex-end"><button type="button" class="btn btn-md" data-act="p-cancel">Vazgeç</button><button type="submit" class="btn btn-primary btn-md">' + ic('check') + 'Kaydet</button></div></form>';
}

function categoryCard() {
  const rows = S.categories.map((c, i) => {
    const count = S.products.filter(p => p.categoryId === c.id).length;
    return '<div class="list-row" style="flex-wrap:wrap"><span class="dot" style="background:' + colorHex(c.color) + ';width:18px;height:18px"></span><label class="sr" for="cat-name-' + c.id + '">Kategori adı</label><input class="input" style="flex:1 1 160px;min-height:52px" id="cat-name-' + c.id + '" data-cat-name="' + c.id + '" value="' + esc(c.name) + '" maxlength="30"><div class="row" style="gap:6px;flex-wrap:nowrap"><button type="button" class="icon-btn" data-act="c-move" data-id="' + c.id + '" data-d="-1" aria-label="' + esc(c.name) + ' yukarı"' + (i === 0 ? ' disabled' : '') + '>' + ic('up') + '</button><button type="button" class="icon-btn" data-act="c-move" data-id="' + c.id + '" data-d="1" aria-label="' + esc(c.name) + ' aşağı"' + (i === S.categories.length - 1 ? ' disabled' : '') + '>' + ic('down') + '</button><button type="button" class="icon-btn" data-act="c-delete" data-id="' + c.id + '" aria-label="' + esc(c.name) + ' kategorisini sil"' + (count ? ' disabled title="İçinde ' + count + ' ürün var"' : '') + ' style="color:var(--bad)">' + ic('trash') + '</button></div>' +
      '<div class="color-pick" role="group" aria-label="' + esc(c.name) + ' rengi" style="flex-basis:100%">' + S.colors.map(k => '<button type="button" data-act="c-color" data-id="' + c.id + '" data-color="' + k.id + '" style="background:' + k.hex + '" aria-pressed="' + (k.id === c.color) + '" aria-label="' + esc(k.name) + '" title="' + esc(k.name) + '"></button>').join('') + '</div></div>';
  }).join('');
  return '<section class="card pad stack" aria-labelledby="cat-h"><div class="card-head"><h2 id="cat-h">Kategoriler</h2><span class="muted small">Satış ekranındaki sekmeler</span></div><div>' + rows + '</div>' +
    '<div class="row" style="flex-wrap:nowrap"><label class="sr" for="cat-new">Yeni kategori adı</label><input class="input" id="cat-new" placeholder="Yeni kategori adı" maxlength="30" autocomplete="off"><button type="button" class="btn btn-outline btn-md" data-act="c-add">' + ic('plus', 20) + 'Ekle</button></div></section>';
}

// ---- Yazıcı formu (Ayarlar ve kurulum) ----
function printerForm(cfg, scope) {
  const P = ui.printers;
  const list = P ? P.printers : [];
  const names = list.map(p => p.name);
  const nameOpts = (cfg.name && !names.includes(cfg.name) ? [{ name: cfg.name, port: 'bulunamadı', driver: '' }] : []).concat(list);
  const mode = cfg.mode;
  const winPart = '<div class="field"><label for="' + scope + '-pname">Windows yazıcısı</label><div class="row" style="flex-wrap:nowrap"><select class="input" id="' + scope + '-pname" data-pf="' + scope + '">' + (nameOpts.length ? '' : '<option value="">Yazıcı bulunamadı</option>') + nameOpts.map(p => '<option value="' + esc(p.name) + '"' + (p.name === cfg.name ? ' selected' : '') + '>' + esc(p.name) + (p.port ? ' · ' + esc(p.port) : '') + (p.default ? ' (varsayılan)' : '') + '</option>').join('') + '</select><button type="button" class="icon-btn" data-act="printers-refresh" aria-label="Yazıcı listesini yenile" title="Listeyi yenile">' + ic('refresh') + '</button></div><p class="hint">' + (P ? (P.guess ? 'Fiş yazıcısı olarak “' + esc(P.guess) + '” görünüyor.' : 'Listede yoksa Koddata sürücüsünü kurup listeyi yenileyin. Ethernet kablosu takılıysa “Ağ yazıcısı”nı seçebilirsiniz.') : 'Yazıcılar okunuyor…') + '</p></div>';
  const netPart = '<div class="fields"><div class="field"><label for="' + scope + '-host">Yazıcının IP adresi</label><input class="input" id="' + scope + '-host" data-pf="' + scope + '" value="' + esc(cfg.host) + '" placeholder="192.168.1.87" inputmode="decimal" autocomplete="off"></div><div class="field"><label for="' + scope + '-port">Port</label><input class="input" id="' + scope + '-port" data-pf="' + scope + '" value="' + (cfg.port || 9100) + '" inputmode="numeric"></div></div><p class="hint">IP adresi, yazıcıyı kapatıp FEED tuşuna basılı tutarak açınca çıkan ayar fişinde yazar.</p>';
  return '<fieldset class="reasons"><legend class="lbl-b">Bağlantı</legend><div class="radios col">' +
    [['windows', 'Windows yazıcısı (USB kablo, sürücü kurulu)'], ['network', 'Ağ yazıcısı (Ethernet kablosu, IP adresi)'], ['none', 'Yazıcı yok: fiş yalnızca ekranda önizlenir']].map(([v, l]) => '<label class="radio"><input type="radio" name="' + scope + '-mode" value="' + v + '" data-pf="' + scope + '"' + (mode === v ? ' checked' : '') + '><span>' + l + '</span></label>').join('') + '</div></fieldset>' +
    (mode === 'windows' ? winPart : mode === 'network' ? netPart : '') +
    '<div class="fields"><div class="field"><label for="' + scope + '-width">Kâğıt</label><select class="input" id="' + scope + '-width" data-pf="' + scope + '">' + [[576, '80 mm (önerilen, 576 nokta)'], [512, '80 mm dar baskı (512 nokta)'], [384, '58 mm (384 nokta)']].map(([w, l]) => '<option value="' + w + '"' + (cfg.width === w ? ' selected' : '') + '>' + l + '</option>').join('') + '</select></div>' +
    '<div class="field"><label for="' + scope + '-cutmode">Kesme</label><select class="input" id="' + scope + '-cutmode" data-pf="' + scope + '">' + [['partial', 'Kısmi kes (önerilen)'], ['full', 'Tam kes'], ['off', 'Kesme yok']].map(([v, l]) => '<option value="' + v + '"' + ((cfg.cut ? cfg.cutMode : 'off') === v ? ' selected' : '') + '>' + l + '</option>').join('') + '</select></div></div>';
}
function readPrinterForm(scope, base) {
  const mode = (document.querySelector('input[name="' + scope + '-mode"]:checked') || {}).value || base.mode;
  const cut = val(scope + '-cutmode') || (base.cut ? base.cutMode : 'off');
  const cfg = Object.assign({}, base, { mode, width: Number(val(scope + '-width')) || base.width, cut: cut !== 'off', cutMode: cut === 'full' ? 'full' : 'partial' });
  if (document.getElementById(scope + '-pname')) cfg.name = val(scope + '-pname');
  if (document.getElementById(scope + '-host')) { cfg.host = val(scope + '-host').trim(); cfg.port = Number(val(scope + '-port')) || 9100; }
  return cfg;
}
function testResultHTML() {
  const r = ui.testResult;
  if (!r) return '';
  const msg = r.ok
    ? (r.mode === 'none' ? '<div class="note info">' + ic('info') + '<p>Yazıcı seçilmediği için fiş yalnızca ekranda gösterildi.</p></div>' : '<div class="note good">' + ic('check', 22, 2.4) + '<p><strong>Test fişi yazıcıya gönderildi.</strong> Fiş çıktıysa Türkçe harfleri, çerçevenin iki kenarını ve QR kodu kontrol edin.</p></div>')
    : '<div class="note bad">' + ic('alert') + '<p><strong>' + esc(r.message) + '</strong> ' + esc(r.hint || '') + '</p></div>';
  return '<div class="stack" style="gap:10px">' + msg + (r.preview ? '<div class="result"><img class="thumb" src="' + r.preview + '" alt="Test fişi önizlemesi"><p class="hint" style="flex:1 1 200px">Kâğıda basılan fiş bu önizlemeyle aynı olmalı.</p></div>' : '') + '</div>';
}
async function loadPrinters() {
  const r = await act('printers');
  if (r) ui.printers = r.data;
  if (ui.route === 'ayarlar' || !S.setupDone) {
    // Kurulumda henüz yazıcı seçilmediyse tahmini seç.
    if (!S.setupDone && ui.setup && ui.printers && !ui.setup.printer.name) ui.setup.printer.name = ui.printers.guess || ui.printers.default || '';
    render();
  }
}

// ---- Ayarlar ----
function scSettings() {
  const b = S.business;
  const f = (id, label, key, ph, max) => '<div class="field"><label for="' + id + '">' + label + '</label><input class="input" id="' + id + '" data-biz="' + key + '" value="' + esc(b[key]) + '"' + (ph ? ' placeholder="' + esc(ph) + '"' : '') + ' maxlength="' + (max || 80) + '" autocomplete="off"></div>';
  const waiters = S.waiters.map(w => '<div class="list-row"><label class="sr" for="w-' + w.id + '">Garson adı</label><input class="input" id="w-' + w.id + '" value="' + esc(w.name) + '" data-wrename="' + w.id + '" maxlength="30"><button type="button" class="icon-btn" data-act="w-remove" data-id="' + w.id + '" aria-label="' + esc(w.name) + ' adlı garsonu kaldır" style="color:var(--bad);border-color:var(--bad-line)">' + ic('x', 20, 2.4) + '</button></div>').join('');
  const bk = S.backup;
  const pg = S.program;
  const preview = '<div class="preview-pane"><span class="lbl-b">Canlı önizleme</span><div class="receipt-scroll">' + (ui.sample.image ? '<img id="sample-img" class="receipt-img" src="' + ui.sample.image + '" alt="Örnek adisyon fişi önizlemesi">' : '<p class="muted center" id="sample-wait" style="padding:30px 0">Önizleme hazırlanıyor…</p>') + '</div><p class="hint center">Örnek adisyon; yazdıkça güncellenir. Kâğıda basılan fiş bununla aynıdır.</p><button type="button" class="btn btn-md" data-act="sample-print">' + ic('printer', 20) + 'Örnek adisyon bas</button></div>';
  const featOpts = '<option value="0"' + (!S.featuredProductId ? ' selected' : '') + '>Otomatik (adı “Cağ” olan ürün)</option>' + S.products.map(p => '<option value="' + p.id + '"' + (S.featuredProductId === p.id ? ' selected' : '') + '>' + esc(p.name) + '</option>').join('');
  return '<div class="page-head"><div class="ttl"><h1>Ayarlar</h1><p class="muted">Değişiklikler kendiliğinden kaydedilir.</p></div><span class="pill accent">' + ic('unlock', 18) + 'Yönetici girişi açık · ' + S.lockMinutes + ' dk işlem olmazsa kilitlenir</span></div>' +
    '<div class="split"><div class="grow">' +
      '<section class="card pad stack" aria-labelledby="fis-h"><div><h2 id="fis-h" class="sec-title">Fiş bilgileri</h2><p class="muted small">Müşteri adisyonunun üstünde ve altında görünür.</p></div><div class="fis-layout"><div class="stack">' +
        '<div class="fields">' + f('s-name', 'İşletme adı', 'name', '', 60) + f('s-slogan', 'Slogan', 'slogan', 'İsteğe bağlı') + f('s-addr1', 'Adres satırı 1', 'address1') + f('s-addr2', 'Adres satırı 2', 'address2') + f('s-phone', 'Telefon', 'phone', 'Fişte görünecek numara', 30) + f('s-insta', 'Instagram', 'instagram', '@hesap_adiniz', 50) + f('s-fb', 'Facebook', 'facebook', 'Sayfa adı', 50) + f('s-tt', 'TikTok', 'tiktok', '@hesap_adiniz', 50) + '</div>' +
        '<div class="switch-row" style="flex-wrap:wrap;cursor:default"><span class="logo-box">' + (S.hasLogo ? '<img src="/logo.png?r=' + S.logoRev + '" alt="Yüklenen logo">' : 'LOGO') + '</span><div style="flex:1 1 220px;min-width:0"><strong>' + (S.hasLogo ? 'Logo yüklendi' : 'Logo yüklenmedi') + '</strong><p class="hint">' + (S.hasLogo ? 'Fişte siyah-beyaz basılır.' : 'Logo yokken fişte işletme adı büyük harflerle basılır. PNG ya da JPG; siyah-beyaz logo en iyi sonucu verir.') + '</p></div><div class="row"><label class="btn btn-outline btn-md file-btn" for="logo-file">' + ic('upload', 20) + (S.hasLogo ? 'Değiştir' : 'Logo yükle') + '<input type="file" id="logo-file" accept="image/png,image/jpeg,image/svg+xml,image/webp"></label>' + (S.hasLogo ? '<button type="button" class="btn btn-md" data-act="logo-remove">Kaldır</button>' : '') + '</div></div>' +
        '<div class="field"><label for="s-invite">Fişin altındaki davet metni</label><textarea class="input" id="s-invite" rows="2" data-biz="invite" maxlength="300">' + esc(b.invite) + '</textarea></div>' +
        '<div class="field"><label for="s-google">Google Haritalar yorum bağlantısı</label><input class="input" id="s-google" data-biz="googleUrl" value="' + esc(b.googleUrl) + '" placeholder="https://g.page/r/.../review" autocomplete="off"><p class="hint">Google İşletme Profili’nde “Yorum iste” bölümünden kopyalayın. Fişin altındaki QR kod bu bağlantıyı açar.</p></div>' +
      '</div>' + preview + '</div></section>' +
      '<section class="card pad stack" aria-labelledby="yazici-h"><h2 id="yazici-h" class="sec-title">Yazıcı</h2>' + printerForm(S.printer, 'set') +
        '<div class="row"><button type="button" class="btn btn-primary" data-act="test-print" data-scope="set">' + ic('printer') + 'Test fişi bas</button><button type="button" class="btn btn-md" data-act="test-preview">' + ic('eye', 20) + 'Test fişi önizlemesi</button></div>' + testResultHTML() + '</section>' +
      backupSection(bk) + whatsappSection() + sheetsSection() +
    '</div><div class="side">' +
      '<section class="card pad stack" aria-labelledby="masa-h"><h2 id="masa-h" class="sec-title">Masa ve vergi</h2><div class="field"><label for="set-tables">Masa sayısı</label><input class="input" id="set-tables" type="number" min="1" max="200" value="' + S.tables + '" style="max-width:160px"></div>' +
        '<fieldset class="reasons"><legend class="lbl-b">Varsayılan KDV oranı</legend><div class="radios">' + [1, 10, 20].map(r => '<label class="radio"><input type="radio" name="set-vat" value="' + r + '"' + (S.defaultVat === r ? ' checked' : '') + '><span>%' + r + '</span></label>').join('') + '</div><p class="hint">Ürün kartında “Varsayılan” seçili ürünler bu oranı kullanır.</p></fieldset>' +
        '<dl class="kv"><dt>Sıradaki adisyon no</dt><dd>' + S.nextNo + '</dd></dl><p class="hint">Numaralar 1’den başlar; hiçbir numara silinmez ya da tekrar kullanılmaz.</p></section>' +
      '<section class="card pad stack" aria-labelledby="rapor-h"><h2 id="rapor-h" class="sec-title">Raporlar</h2><div class="field"><label for="set-featured">Öne çıkan ürün</label><select class="input" id="set-featured">' + featOpts + '</select><p class="hint">İstatistikte “Satılan …” kartında, gün sonu fişinde ve WhatsApp mesajında bu ürünün adedi gösterilir.</p></div>' +
        '<div class="row"><button type="button" class="btn btn-md" data-act="open-folder" data-w="reports">' + ic('folder', 20) + 'Excel raporları klasörü</button></div></section>' +
      '<section class="card pad stack" aria-labelledby="gars-h"><div class="card-head"><h2 id="gars-h">Garsonlar</h2></div><p class="hint">Masa açılırken garson seçilir. Adı değiştirmek için kutuya yazın.</p><div>' + waiters + '</div><div class="row" style="flex-wrap:nowrap"><label class="sr" for="w-new">Yeni garson adı</label><input class="input" id="w-new" placeholder="Yeni garson adı" maxlength="30" autocomplete="off"><button type="button" class="btn btn-outline btn-md" data-act="w-add">' + ic('plus', 20) + 'Ekle</button></div></section>' +
      '<section class="card pad stack" aria-labelledby="sifre-h"><h2 id="sifre-h" class="sec-title">Yönetici şifresi</h2><p class="hint">Şifre istenen ekranlar: Gün Sonu, İstatistik, Ürünler ve Ayarlar. Adisyon iptali, basılmış adisyondan ürün düşmek, basılmış adisyonu boşaltmak, ödeme şeklini değiştirmek, ödenmiş adisyonu yeniden açmak ve yedekten geri yüklemek her seferinde ayrıca şifre ister.</p>' +
        '<div class="fields"><div class="field"><label for="pin-old">Mevcut şifre</label><input class="input" id="pin-old" type="password" inputmode="numeric" maxlength="4" autocomplete="off"></div><div class="field"><label for="pin-new">Yeni şifre (4 rakam)</label><input class="input" id="pin-new" type="password" inputmode="numeric" maxlength="4" autocomplete="off"></div><div class="field"><label for="pin-new2">Yeni şifre tekrar</label><input class="input" id="pin-new2" type="password" inputmode="numeric" maxlength="4" autocomplete="off"></div></div>' +
        '<div class="row"><button type="button" class="btn btn-outline btn-md" data-act="pin-save">Şifreyi değiştir</button><div class="field" style="flex-direction:row;align-items:center;gap:10px"><label for="set-lock">Kendiliğinden kilit</label><select class="input" id="set-lock" style="width:auto">' + [2, 5, 10, 15].map(m => '<option value="' + m + '"' + (S.lockMinutes === m ? ' selected' : '') + '>' + m + ' dk</option>').join('') + '</select></div></div>' +
        '<label class="switch-row" for="set-disc-pin"><span>İndirim için yönetici şifresi istensin<br><span class="muted small">Kapalıysa garson şifresiz indirim uygulayabilir; her indirim yine işlem kaydına yazılır.</span></span><input class="switch" type="checkbox" id="set-disc-pin"' + (S.discountNoPin ? '' : ' checked') + '></label></section>' +
      '<section class="card pad stack" aria-labelledby="prog-h"><h2 id="prog-h" class="sec-title">Program</h2>' +
        '<label class="switch-row" for="set-fullscreen"><span>Açılışta tam ekran</span><input class="switch" type="checkbox" id="set-fullscreen"' + (pg.fullscreen ? ' checked' : '') + '></label>' +
        (pg.autostartSupported ? '<label class="switch-row" for="set-autostart"><span>Windows açılınca programı başlat</span><input class="switch" type="checkbox" id="set-autostart"' + (pg.autostart ? ' checked' : '') + '></label>' : '') +
        '<dl class="kv"><dt>Sürüm</dt><dd>' + esc(S.version) + '</dd><dt>Veri klasörü</dt><dd>' + esc(pg.dataDir) + '</dd></dl>' +
        '<div class="row"><button type="button" class="btn btn-md" data-act="open-folder" data-w="data">' + ic('folder', 20) + 'Veri klasörünü aç</button><button type="button" class="btn btn-md" data-act="nav" data-to="kayit">' + ic('history', 20) + 'İşlem kaydı</button></div>' +
        '<p class="hint">Şifre unutulursa: veri klasörüne <strong>sifre_sifirla.txt</strong> adlı boş bir dosya koyup programı yeniden açın; şifre 1234 olur.</p></section>' +
    '</div></div>';
}
function backupSection(bk) {
  return '<section class="card pad stack" aria-labelledby="yedek-h"><h2 id="yedek-h" class="sec-title">Yedek</h2>' +
    '<div class="note good">' + ic('check', 22, 2.4) + '<p>Adisyonlar, günler ve istatistikler programın veritabanında <strong>süresiz</strong> saklanır; hiçbir kayıt kendiliğinden silinmez. Yedekler bu verinin kopyalarıdır: açılışta, saatte bir, gün kapanınca ve program kapanırken alınır; son 24 saatin ve son 90 günün kopyaları tutulur.</p></div>' +
    (bk.lastError ? '<div class="note bad">' + ic('alert') + '<p>Son yedek alınamadı: ' + esc(bk.lastError) + '</p></div>' : '') +
    '<dl class="kv"><dt>Yedek klasörü</dt><dd>' + esc(bk.dir) + '</dd><dt>Son yedek</dt><dd>' + esc(bk.lastAt || '—') + '</dd></dl>' +
    '<div class="row"><button type="button" class="btn btn-md" data-act="backup-now">' + ic('save', 20) + 'Şimdi yedekle</button><button type="button" class="btn btn-md" data-act="open-folder" data-w="backup">' + ic('folder', 20) + 'Klasörü aç</button><button type="button" class="btn btn-md btn-danger-quiet" data-act="restore-open">' + ic('history', 20) + 'Yedekten geri yükle</button></div>' +
    '<div class="field"><label for="s-second">İkinci yedek yeri (USB bellek ya da ikinci disk)</label><div class="row" style="flex-wrap:nowrap"><input class="input" id="s-second" value="' + esc(bk.secondDir) + '" placeholder="Örnek: E:\\" autocomplete="off"><button type="button" class="btn btn-outline btn-md" data-act="second-save">Kaydet</button>' + (bk.secondDir ? '<button type="button" class="btn btn-md" data-act="second-clear">Kaldır</button>' : '') + '</div>' +
    '<div class="row" id="drives"><button type="button" class="btn btn-md" data-act="drives">' + ic('refresh', 20) + 'Takılı USB bellekleri göster</button></div>' +
    (bk.secondDir ? '<p class="hint">Son kopya: <strong>' + esc(bk.secondAt || 'henüz yok') + '</strong></p>' : '') +
    (bk.secondError ? '<div class="note warn">' + ic('alert') + '<p>' + esc(bk.secondError) + '</p></div>' : '') + '</div>' +
    '<div class="field"><label for="s-drive">Google Drive klasörü</label><div class="row" style="flex-wrap:nowrap"><input class="input" id="s-drive" value="' + esc(bk.driveDir) + '" placeholder="Örnek: G:\\Drive’ım" autocomplete="off"><button type="button" class="btn btn-outline btn-md" data-act="drive-save">Kaydet</button>' + (bk.driveDir ? '<button type="button" class="btn btn-md" data-act="drive-clear">Kaldır</button>' : '') + '</div>' +
    '<div class="row" id="drive-dirs"><button type="button" class="btn btn-md" data-act="drive-find">' + ic('cloud', 20) + 'Google Drive klasörünü bul</button></div>' +
    (bk.driveDir ? '<p class="hint">Drive klasörüne son kopya: <strong>' + esc(bk.driveAt || 'henüz yok') + '</strong>. Google Drive programı kopyayı internete kendisi yükler.</p>' : '<p class="hint">Bilgisayarda “Google Drive” (masaüstü) programı kurulu ve oturum açık olmalı. Günlük yedeğin kopyası Drive’daki “Halis Adisyon Yedekleri” klasörüne yazılır; son 30 gün saklanır.</p>') +
    (bk.driveError ? '<div class="note warn">' + ic('alert') + '<p>' + esc(bk.driveError) + '</p></div>' : '') + '</div>' +
  '</section>';
}
function outboxStatusHTML(st, kind) {
  if (!st) return '';
  const parts = [];
  if (st.lastSent) parts.push('<span>Son gönderim <strong>' + esc(stampText(st.lastSent)) + '</strong></span>');
  if (st.pending) parts.push('<span>Sırada bekleyen <strong>' + st.pending + '</strong></span>');
  if (st.failed) parts.push('<span style="color:var(--bad)">Gönderilemeyen <strong>' + st.failed + '</strong></span>');
  if (!parts.length) parts.push('<span>Henüz gönderim yok.</span>');
  return '<div class="status-line">' + parts.join('') + '</div>' + (st.lastError ? '<div class="note warn">' + ic('alert') + '<p>Son hata: ' + esc(st.lastError) + '</p>' + (st.pending || st.failed ? '<button type="button" class="btn btn-md" data-act="outbox-retry">' + ic('refresh', 20) + 'Şimdi yeniden dene</button>' : '') + '</div>' : (st.pending ? '<div class="row"><button type="button" class="btn btn-md" data-act="outbox-retry">' + ic('refresh', 20) + 'Şimdi gönder</button></div>' : ''));
}
const WA_TEMPLATE = 'Halis Adisyon gün sonu raporu\nTarih: {{1}}\nCiro: {{2}}\nAdisyon: {{3}}\nCağ: {{4}}\nİptal: {{5}}\nÖdemeler: {{6}}\nGarsonlar: {{7}}\nBu mesaj programdan otomatik gönderilmiştir.';
function whatsappSection() {
  const W = S.whatsapp || {};
  return '<section class="card pad stack" aria-labelledby="wa-h"><div><h2 id="wa-h" class="sec-title">Gün sonu mesajı (WhatsApp)</h2><p class="muted small">Gün kapatılınca ciro, adisyon, cağ, iptal, ödeme şekilleri ve garsonlar onaylı bir WhatsApp şablonuyla gönderilir. İnternet yoksa mesaj sırada bekler.</p></div>' +
    '<label class="switch-row" for="wa-enabled"><span>Gün kapanınca WhatsApp ile gönder</span><input class="switch" type="checkbox" id="wa-enabled"' + (W.enabled ? ' checked' : '') + '></label>' +
    '<div class="fields"><div class="field"><label for="wa-pnid">Telefon numarası kimliği (Phone number ID)</label><input class="input" id="wa-pnid" inputmode="numeric" value="' + esc(W.phoneNumberId || '') + '" autocomplete="off"></div>' +
    '<div class="field"><label for="wa-token">Erişim anahtarı (kalıcı)</label><input class="input" id="wa-token" type="password" autocomplete="off" placeholder="' + (W.hasToken ? 'Kayıtlı (' + esc(W.tokenHint || '') + ') · değiştirmek için yenisini yapıştırın' : 'EAAG… ile başlayan anahtar') + '"></div>' +
    '<div class="field"><label for="wa-template">Şablon adı</label><input class="input" id="wa-template" value="' + esc(W.template || '') + '" placeholder="gun_sonu_ozeti" autocomplete="off"></div>' +
    '<div class="field"><label for="wa-lang">Şablon dili</label><input class="input" id="wa-lang" value="' + esc(W.lang || 'tr') + '" autocomplete="off" style="max-width:140px"></div></div>' +
    '<div class="field"><label for="wa-rcpt">Alıcı numaralar (her satıra bir numara)</label><textarea class="input" id="wa-rcpt" rows="3" placeholder="0532 123 45 67">' + esc((W.recipients || []).map(n => n.startsWith('90') ? '+' + n : n).join('\n')) + '</textarea></div>' +
    '<details class="help"><summary>Gelişmiş</summary><div class="field"><label for="wa-api">Graph API sürümü</label><input class="input" id="wa-api" value="' + esc(W.apiVersion || '') + '" placeholder="v25.0" style="max-width:160px"><p class="hint">Boş bırakılırsa v25.0 kullanılır. Meta eski sürümleri kapatınca buradan güncelleyin.</p></div></details>' +
    '<div class="row"><button type="button" class="btn btn-primary btn-md" data-act="wa-save">' + ic('save', 20) + 'Kaydet</button><button type="button" class="btn btn-md" data-act="wa-test">' + ic('send', 20) + 'Test mesajı gönder</button></div>' +
    outboxStatusHTML(W.status, 'whatsapp') +
    '<details class="help"><summary>Nasıl kurulur?</summary><ol>' +
      '<li>business.facebook.com’da Meta işletme hesabı açın. developers.facebook.com’da “İşletme” türünde bir uygulama oluşturup WhatsApp ürününü ekleyin.</li>' +
      '<li>WhatsApp › API Setup bölümünde işletmenin telefon numarasını ekleyip doğrulayın. Orada yazan <strong>Phone number ID</strong> değerini yukarıya girin.</li>' +
      '<li>İşletme Ayarları › Sistem kullanıcıları bölümünde bir sistem kullanıcısı oluşturun, uygulamayı ona atayın ve <strong>whatsapp_business_messaging</strong> izniyle süresi dolmayan bir erişim anahtarı üretin.</li>' +
      '<li>WhatsApp Yöneticisi › Mesaj şablonları’nda “Yardımcı program (Utility)” kategorisinde, Türkçe dilinde aşağıdaki metinle bir şablon oluşturun; değişkenler için örnek değer girin ve onaylanmasını bekleyin. Şablonun adını yukarıya yazın.</li>' +
      '<li>Alıcı numaraları girip Kaydet’e, ardından “Test mesajı gönder”e basın.</li></ol>' +
      '<textarea class="code-box" id="wa-template-text" readonly rows="10" style="min-height:0">' + esc(WA_TEMPLATE) + '</textarea><div class="row"><button type="button" class="btn btn-md" data-act="copy" data-from="wa-template-text">' + ic('copy', 20) + 'Şablon metnini kopyala</button></div>' +
      '<p class="hint">Meta, işletmenin başlattığı şablon mesajları için ücret alabilir; güncel fiyatlar Meta’nın sayfasındadır.</p></details>' +
  '</section>';
}
const SHEETS_SCRIPT = `// Halis Adisyon → Google Sheets
// Program gün kapanınca özeti bu tabloya yazar. Bu kodu değiştirmeyin.
const GIZLI_ANAHTAR = '__SECRET__';

function doPost(e) {
  const kilit = LockService.getScriptLock();
  kilit.waitLock(30000);
  try {
    const veri = JSON.parse(e.postData.contents);
    if (veri.secret !== GIZLI_ANAHTAR) return yanit({ ok: false, error: 'Gizli anahtar uyuşmuyor.' });
    if (veri.type === 'test') return yanit({ ok: true });
    if (veri.type !== 'gun_sonu') return yanit({ ok: false, error: 'Bilinmeyen veri.' });
    const tablo = SpreadsheetApp.getActiveSpreadsheet();
    const tarih = veri.gun.tarih;
    yaz(tablo, 'Gün Sonu', ['Tarih', 'Kapanış', 'Ciro', 'KDV', 'Adisyon', 'Ortalama', 'İptal', 'İptal tutarı', 'Nakit', 'Kredi Kartı', 'Havale', 'Öne çıkan ürün', 'Adet', 'Garsonlar', 'Adisyon no', 'İndirimli adisyon', 'İndirim tutarı'], [veri.gun.satir], tarih);
    yaz(tablo, 'Adisyonlar', ['Tarih', 'No', 'Masa', 'Garson', 'Açılış', 'Kapanış', 'Durum', 'Ödeme şekli', 'Tutar', 'Baskı', 'Not', 'İndirim %', 'İndirim'], veri.adisyonlar, tarih);
    yaz(tablo, 'Ürünler', ['Tarih', 'Ürün', 'Adet', 'Birim', 'Tutar'], veri.urunler, tarih);
    return yanit({ ok: true });
  } catch (hata) {
    return yanit({ ok: false, error: String(hata) });
  } finally {
    kilit.releaseLock();
  }
}

// Aynı günün eski satırları silinip yenileri yazılır: gün iki kez gönderilse de çift kayıt olmaz.
function yaz(tablo, ad, basliklar, satirlar, tarih) {
  let sayfa = tablo.getSheetByName(ad);
  if (!sayfa) {
    sayfa = tablo.insertSheet(ad);
    sayfa.appendRow(basliklar);
    sayfa.setFrozenRows(1);
    sayfa.getRange(1, 1, 1, basliklar.length).setFontWeight('bold');
    sayfa.getRange('A:A').setNumberFormat('@');
  }
  const son = sayfa.getLastRow();
  if (son > 1) {
    const tarihler = sayfa.getRange(2, 1, son - 1, 1).getDisplayValues();
    let i = tarihler.length - 1;
    while (i >= 0) {
      if (tarihler[i][0] !== tarih) { i--; continue; }
      let j = i;
      while (j > 0 && tarihler[j - 1][0] === tarih) j--;
      sayfa.deleteRows(j + 2, i - j + 1);
      i = j - 1;
    }
  }
  if (satirlar.length) {
    sayfa.getRange(sayfa.getLastRow() + 1, 1, satirlar.length, satirlar[0].length).setValues(satirlar);
  }
}

function yanit(nesne) {
  return ContentService.createTextOutput(JSON.stringify(nesne)).setMimeType(ContentService.MimeType.JSON);
}
`;
function sheetsSection() {
  const G = S.sheets || {};
  const code = SHEETS_SCRIPT.replace('__SECRET__', G.secret || 'ÖNCE KAYDEDİN');
  return '<section class="card pad stack" aria-labelledby="gs-h"><div><h2 id="gs-h" class="sec-title">Google Sheets</h2><p class="muted small">Gün kapatılınca gün özeti, o günün adisyonları (ödeme şekliyle) ve ürün adetleri Google Sheets’teki tablonuza yazılır. İnternet yoksa sırada bekler.</p></div>' +
    '<label class="switch-row" for="gs-enabled"><span>Gün kapanınca Google Sheets’e yaz</span><input class="switch" type="checkbox" id="gs-enabled"' + (G.enabled ? ' checked' : '') + '></label>' +
    '<div class="field"><label for="gs-url">Web uygulaması adresi</label><input class="input" id="gs-url" value="' + esc(G.url || '') + '" placeholder="https://script.google.com/macros/s/…/exec" autocomplete="off"></div>' +
    '<div class="row"><button type="button" class="btn btn-primary btn-md" data-act="gs-save">' + ic('save', 20) + 'Kaydet</button><button type="button" class="btn btn-md" data-act="gs-test">' + ic('send', 20) + 'Bağlantıyı dene</button></div>' +
    outboxStatusHTML(G.status, 'sheets') +
    '<details class="help"><summary>Nasıl kurulur?</summary><ol>' +
      '<li>Google Sheets’te yeni bir tablo açın. Uzantılar › Apps Script’i seçin.</li>' +
      '<li>Açılan sayfadaki kodu silip aşağıdaki kodu yapıştırın ve kaydedin. Kodun içinde size özel gizli anahtar var.</li>' +
      '<li>Dağıt › Yeni dağıtım › Web uygulaması seçin. “Çalıştıran: Ben”, “Erişimi olanlar: Herkes” seçip dağıtın, istenen izinleri verin.</li>' +
      '<li>Verilen web uygulaması adresini (…/exec ile biter) yukarıya yapıştırıp Kaydet’e, sonra “Bağlantıyı dene”ye basın.</li></ol>' +
      '<textarea class="code-box" id="gs-code" readonly rows="16">' + esc(code) + '</textarea>' +
      '<div class="row"><button type="button" class="btn btn-md" data-act="copy" data-from="gs-code">' + ic('copy', 20) + 'Kodu kopyala</button><button type="button" class="btn btn-md" data-act="gs-secret">' + ic('refresh', 20) + 'Yeni gizli anahtar üret</button></div>' +
      '<p class="hint">Yeni anahtar üretirseniz kodu Apps Script’e yeniden yapıştırıp yeni bir dağıtım yapmanız gerekir.</p></details>' +
  '</section>';
}
// Ayarlardaki canlı fiş önizlemesi: yazdıkça (kaydetmeden) örnek adisyon yeniden çizilir.
async function refreshSample() {
  if (ui.route !== 'ayarlar' || !S.setupDone) return;
  const b = Object.assign({}, S.business);
  document.querySelectorAll('[data-biz]').forEach(el => { b[el.dataset.biz] = el.value; });
  const seq = ++ui.sample.seq;
  const r = await api('samplePreview', { business: b }).catch(() => null);
  if (!r || seq !== ui.sample.seq || ui.route !== 'ayarlar') return;
  ui.sample.image = r.data.image;
  const img = document.getElementById('sample-img');
  if (img) img.src = ui.sample.image;
  else { const w = document.getElementById('sample-wait'); if (w) w.outerHTML = '<img id="sample-img" class="receipt-img" src="' + ui.sample.image + '" alt="Örnek adisyon fişi önizlemesi">'; }
}
function scheduleSample() { clearTimeout(ui.sample.timer); ui.sample.timer = setTimeout(refreshSample, 450); }
async function restoreFlow() {
  const r = await act('backups');
  if (!r) return;
  const list = r.data.backups || [];
  if (!list.length) { toast('Geri yüklenecek yedek bulunamadı.', 'bad'); return; }
  const groups = {};
  list.forEach(f => { (groups[f.where] = groups[f.where] || []).push(f); });
  const order = ['Saatlik', 'Günlük', 'Gün sonu', 'Geri yükleme öncesi', 'Güncelleme öncesi', 'USB bellek', 'Google Drive', 'Elle alınan'];
  const keys = Object.keys(groups).sort((a, b) => (order.indexOf(a) + 99) % 99 - (order.indexOf(b) + 99) % 99);
  ui.restoreList = list;
  const body = keys.map(k => '<details class="help"' + (k === 'Saatlik' || keys.length === 1 ? ' open' : '') + '><summary>' + esc(k) + ' (' + groups[k].length + ')</summary><table class="legend-table"><tbody>' + groups[k].map(f => '<tr><td><strong>' + esc(stampText(f.at)) + '</strong><br><span class="muted">' + esc(f.name) + '</span></td><td class="r muted">' + (f.size / 1048576).toFixed(1).replace('.', ',') + ' MB</td><td class="r"><button type="button" class="btn btn-md" data-act="restore-pick" data-i="' + list.indexOf(f) + '">Geri yükle</button></td></tr>').join('') + '</tbody></table></details>').join('');
  openModal({ title: 'Yedekten geri yükle', wide: true, body: '<div class="note warn">' + ic('alert') + '<p>Geri yükleme, programdaki tüm verileri seçilen yedekteki haline döndürür. Şu anki veriler önce ayrıca yedeklenir; yanlış yedek seçerseniz o yedeği geri yükleyerek dönebilirsiniz.</p></div>' + body, actions: [{ label: 'Kapat', onClick: () => closeModal() }] });
}
function restorePick(i) {
  const f = (ui.restoreList || [])[i]; if (!f) return;
  openModal({
    title: 'Bu yedek geri yüklensin mi?',
    body: '<dl class="kv"><dt>Yedek</dt><dd>' + esc(stampText(f.at)) + ' · ' + esc(f.where) + '</dd><dt>Dosya</dt><dd>' + esc(f.name) + '</dd></dl>' +
      '<ul class="warn-list"><li class="warn">' + ic('info', 20) + 'Bu yedekten sonra girilen adisyonlar ve değişiklikler ekrandan kalkar (silinmez, kenara alınır).</li><li class="warn">' + ic('info', 20) + 'Adisyon numaraları geriye düşmez; sıradaki numara No ' + S.nextNo + ' olarak devam eder.</li><li class="warn">' + ic('refresh', 20) + 'Program kapanıp yeniden açılır. Yönetici şifresi sorulur.</li></ul>',
    actions: [{ label: 'Vazgeç', onClick: () => closeModal() }, { label: 'Geri yükle', kind: 'btn-danger', icon: 'history', onClick: () => once('restore', async () => {
      closeModal(true);
      const r = await act('restore', { path: f.path });
      if (!r) return;
      restarting = true;
      const d = document.createElement('div'); d.className = 'restart'; d.innerHTML = ic('refresh', 48, 2) + '<span>Yedek geri yükleniyor, program yeniden başlıyor…</span><span class="muted" style="font-size:19px;font-weight:500">Birkaç saniye içinde açılmazsa masaüstündeki Halis Adisyon simgesinden açın.</span>';
      document.body.appendChild(d);
    }) }]
  });
}
async function copyFrom(id) {
  const el = document.getElementById(id); if (!el) return;
  try { await navigator.clipboard.writeText(el.value || el.textContent); toast('Kopyalandı.', 'good'); }
  catch (e) { el.focus(); el.select(); toast('Seçildi; Ctrl+C ile kopyalayın.'); }
}
async function saveWhatsApp(extra) {
  const rcpt = val('wa-rcpt').split(/\n|,|;/).map(x => x.trim()).filter(Boolean);
  const w = { phoneNumberId: val('wa-pnid').trim(), template: val('wa-template').trim(), lang: val('wa-lang').trim(), apiVersion: val('wa-api').trim(), recipients: rcpt };
  const tok = val('wa-token').trim(); if (tok) w.token = tok;
  Object.assign(w, extra || {});
  const r = await act('saveSettings', { whatsapp: w });
  // Hata olursa yazılanlar silinmesin; yalnız açma anahtarı eski haline döner.
  if (r) render();
  else if (extra && 'enabled' in extra) { const sw = document.getElementById('wa-enabled'); if (sw) sw.checked = !extra.enabled; }
  return r;
}
async function saveSheets(extra) {
  const g = Object.assign({ url: val('gs-url').trim() }, extra || {});
  const r = await act('saveSettings', { sheets: g });
  if (r) render();
  else if (extra && 'enabled' in extra) { const sw = document.getElementById('gs-enabled'); if (sw) sw.checked = !extra.enabled; }
  return r;
}

// ---- İlk kurulum sihirbazı ----
const SETUP_STEPS = ['İşletme', 'Masalar ve garsonlar', 'Yazıcı', 'Fiyatlar', 'Yönetici şifresi'];
function initSetup() {
  if (ui.setup) return;
  ui.setup = {
    step: 0,
    business: Object.assign({}, S.business),
    tables: S.tables,
    waiters: S.waiters.map(w => w.name),
    printer: Object.assign({}, S.printer),
    prices: {},
    pin: '', pin2: ''
  };
  if (!ui.setup.waiters.length) ui.setup.waiters = ['Garson 1'];
  for (const p of S.products) ui.setup.prices[p.id] = priceText(p.price);
  loadPrinters();
}
function scSetup() {
  initSetup();
  const W = ui.setup;
  const steps = '<div class="steps" aria-label="Kurulum adımları">' + SETUP_STEPS.map((s, i) => '<span class="step' + (i === W.step ? ' on' : i < W.step ? ' done' : '') + '"><b>' + (i < W.step ? ic('check', 16, 3) : i + 1) + '</b>' + s + '</span>').join('') + '</div>';
  let body = '';
  if (W.step === 0) {
    const f = (key, label, ph, max) => '<div class="field"><label for="w-' + key + '">' + label + '</label><input class="input" id="w-' + key + '" data-wz="business.' + key + '" value="' + esc(W.business[key]) + '" placeholder="' + esc(ph || '') + '" maxlength="' + (max || 80) + '" autocomplete="off"></div>';
    body = '<h1 style="font-size:30px">Hoş geldiniz</h1><p class="muted">Programı birkaç adımda hazırlayalım. Bu bilgiler fişin üstünde ve altında görünür; sonra Ayarlar’dan değiştirebilirsiniz.</p>' +
      '<div class="fields">' + f('name', 'İşletme adı', '', 60) + f('slogan', 'Slogan (isteğe bağlı)', 'Örnek: Odun ateşinde cağ kebabı') + f('address1', 'Adres satırı 1', '') + f('address2', 'Adres satırı 2', '') + f('phone', 'Telefon (isteğe bağlı)', '0242 ...', 30) + '</div>';
  } else if (W.step === 1) {
    body = '<h1 style="font-size:30px">Masalar ve garsonlar</h1>' +
      '<div class="field"><span class="lbl-b">Kaç masa var?</span><div class="counter"><button type="button" class="icon-btn" data-act="wz-tables" data-d="-1" aria-label="Bir masa azalt" style="width:64px;height:64px">' + ic('minus', 26, 2.6) + '</button><input class="input" id="w-tables" data-wz="tables" type="number" min="1" max="200" value="' + W.tables + '" aria-label="Masa sayısı"><button type="button" class="icon-btn" data-act="wz-tables" data-d="1" aria-label="Bir masa ekle" style="width:64px;height:64px">' + ic('plus', 26, 2.6) + '</button></div><p class="hint">Sonradan Ayarlar’dan değiştirilebilir.</p></div>' +
      '<div class="field"><span class="lbl-b">Garsonlar</span><p class="hint">Masa açılırken garson seçilir; ileride kimin kaç adisyon açtığı görülür.</p>' +
      W.waiters.map((n, i) => '<div class="list-row"><label class="sr" for="w-wt-' + i + '">Garson adı</label><input class="input" id="w-wt-' + i + '" data-wz="waiters.' + i + '" value="' + esc(n) + '" maxlength="30" placeholder="Garson adı"><button type="button" class="icon-btn" data-act="wz-waiter-del" data-i="' + i + '" aria-label="Sil"' + (W.waiters.length <= 1 ? ' disabled' : '') + ' style="color:var(--bad)">' + ic('x', 20, 2.4) + '</button></div>').join('') +
      '<div><button type="button" class="btn btn-outline btn-md" data-act="wz-waiter-add">' + ic('plus', 20) + 'Garson ekle</button></div></div>';
  } else if (W.step === 2) {
    body = '<h1 style="font-size:30px">Yazıcı</h1><p class="muted">Koddata yazıcı USB kabloyla bağlıysa ve sürücüsü kuruluysa listeden seçin, sonra test fişi basın.</p>' +
      printerForm(W.printer, 'wz') + '<div class="row"><button type="button" class="btn btn-primary" data-act="test-print" data-scope="wz">' + ic('printer') + 'Test fişi bas</button></div>' + testResultHTML() +
      '<p class="hint">Yazıcıyı şimdi ayarlayamazsanız “Yazıcı yok” seçip devam edin; sonra Ayarlar › Yazıcı bölümünden ayarlarsınız.</p>';
  } else if (W.step === 3) {
    const rows = S.categories.map(c => S.products.filter(p => p.categoryId === c.id).map(p => '<tr><td><span class="row" style="gap:10px;flex-wrap:nowrap"><span class="dot" style="background:' + colorHex(c.color) + '"></span><strong>' + esc(p.name) + '</strong></span></td><td class="muted">' + esc(p.unit) + '</td><td class="r"><label class="sr" for="w-price-' + p.id + '">' + esc(p.name) + ' fiyatı</label><input class="input money" id="w-price-' + p.id + '" data-wz="prices.' + p.id + '" value="' + esc(W.prices[p.id] || '') + '" inputmode="decimal" placeholder="0,00"></td></tr>').join('')).join('');
    body = '<h1 style="font-size:30px">Fiyatlar</h1><p class="muted">Kâğıt adisyondaki 12 ürün hazır. KDV dahil fiyatları girin (örnek: 340 ya da 340,50). Fiyatı boş kalan ürün satılamaz; sonra Ürünler ekranından girebilirsiniz.</p>' +
      '<div class="tablebox card"><table class="data price-table"><thead><tr><th>Ürün</th><th>Birim</th><th class="r">Fiyat (₺, KDV dahil)</th></tr></thead><tbody>' + rows + '</tbody></table></div>';
  } else {
    body = '<h1 style="font-size:30px">Yönetici şifresi</h1><p class="muted">Ürünler, Ayarlar ve adisyon iptali bu 4 haneli şifreyle açılır. Garsonlara söylemeyin.</p>' +
      '<div class="fields" style="max-width:520px"><div class="field"><label for="w-pin">Şifre (4 rakam)</label><input class="input" id="w-pin" data-wz="pin" type="password" inputmode="numeric" maxlength="4" value="' + esc(W.pin) + '" autocomplete="off"></div><div class="field"><label for="w-pin2">Şifre tekrar</label><input class="input" id="w-pin2" data-wz="pin2" type="password" inputmode="numeric" maxlength="4" value="' + esc(W.pin2) + '" autocomplete="off"></div></div>';
  }
  const last = W.step === SETUP_STEPS.length - 1;
  return '<div class="wizard">' + steps + '<section class="card pad stack">' + body + '</section>' +
    '<div class="wizard-nav"><button type="button" class="btn" data-act="wz-back"' + (W.step === 0 ? ' disabled' : '') + '>' + ic('back') + 'Geri</button><button type="button" class="btn btn-primary" data-act="wz-next" data-fk="wz-next">' + (last ? ic('check') + 'Kurulumu bitir' : 'İleri') + '</button></div></div>';
}
function setupValidate(step) {
  const W = ui.setup;
  if (step === 0 && !String(W.business.name || '').trim()) return 'İşletme adını yazın.';
  if (step === 1) {
    const n = Number(W.tables);
    if (!Number.isInteger(n) || n < 1 || n > 200) return 'Masa sayısı 1 ile 200 arasında olmalı.';
    if (!W.waiters.some(x => String(x).trim())) return 'En az bir garson adı yazın.';
  }
  if (step === 2) {
    const p = W.printer;
    if (p.mode === 'windows' && !p.name) return 'Listeden bir yazıcı seçin ya da “Yazıcı yok”u işaretleyin.';
    if (p.mode === 'network' && !p.host) return 'Yazıcının IP adresini yazın.';
  }
  if (step === 3) {
    for (const p of S.products) {
      const v = String(W.prices[p.id] || '').trim();
      if (v && !(parsePrice(v) >= 0)) return p.name + ' fiyatı anlaşılamadı. Örnek: 340 ya da 340,50';
    }
  }
  if (step === 4) {
    if (!/^\d{4}$/.test(W.pin)) return 'Şifre 4 rakam olmalı.';
    if (W.pin !== W.pin2) return 'İki şifre aynı değil.';
  }
  return '';
}
async function setupNext() {
  const W = ui.setup;
  if (W.step === 2) W.printer = readPrinterForm('wz', W.printer);
  const err = setupValidate(W.step);
  if (err) { toast(err, 'bad'); return; }
  if (W.step < SETUP_STEPS.length - 1) { W.step++; ui.testResult = null; render(); window.scrollTo(0, 0); return; }
  const prices = {};
  for (const p of S.products) { const v = parsePrice(W.prices[p.id]); prices[p.id] = v > 0 ? v : 0; }
  const r = await act('finishSetup', { business: W.business, tables: Number(W.tables), waiters: W.waiters, printer: W.printer, prices, pin: W.pin });
  if (!r) return;
  ui.setup = null; ui.testResult = null; ui.route = 'masalar';
  render();
  toast('Kurulum tamamlandı. İyi çalışmalar!', 'good');
}

// ---------- İşlemler ----------
async function openTable(t) {
  if (activeFor(t) || ui.drafts[t]) { go('siparis', t); return; }
  const ws = S.waiters;
  if (ws.length === 1) { ui.drafts[t] = { waiterId: ws[0].id, openedAt: new Date().toISOString() }; go('siparis', t); return; }
  pickWaiter('Masa ' + t + ' · garson seçin', null, w => { ui.drafts[t] = { waiterId: w, openedAt: new Date().toISOString() }; go('siparis', t); });
}
function pickWaiter(title, current, onPick) {
  openModal({
    title,
    body: '<p class="muted">Masaya bakan garsonu seçin.</p><div class="pick-grid">' + S.waiters.map((w, i) => '<button type="button" class="pick' + (w.id === current ? ' on' : '') + '" data-act="pick-waiter" data-w="' + w.id + '"' + (i === 0 ? ' data-autofocus' : '') + '>' + ic('user', 24) + '<span>' + esc(w.name) + '</span></button>').join('') + '</div>',
    actions: [{ label: 'Vazgeç', onClick: () => closeModal() }]
  });
  modal.onPick = onPick;
}
function revealLine(id) {
  const ul = document.querySelector('.ticket .lines'); if (!ul || ul.scrollHeight <= ul.clientHeight) return;
  const li = [...ul.children].find(x => x.querySelector('[data-l="' + id + '"]')); if (!li) return;
  if (li.offsetTop < ul.scrollTop) ul.scrollTop = li.offsetTop;
  else if (li.offsetTop + li.offsetHeight > ul.scrollTop + ul.clientHeight) ul.scrollTop = li.offsetTop + li.offsetHeight - ul.clientHeight;
}
async function addItem(t, pid) {
  const d = ui.drafts[t];
  const r = await act('addItem', { table: t, productId: pid, waiterId: d ? d.waiterId : 0 });
  if (!r) return;
  delete ui.drafts[t];
  render();
  const a = activeFor(t);
  if (a) { const l = a.lines.filter(x => x.productId === pid).pop(); if (l) revealLine(l.id); }
}
async function doPrint(t, then) {
  const a = activeFor(t); if (!a || !a.lines.length || ui.busy) return;
  ui.busy = 'print:' + a.no; render();
  const r = await act('print', { no: a.no });
  ui.busy = null; render();
  if (!r) return;
  handlePrintResult(r.data, () => doPrint(t, then), then);
}
function handlePrintResult(out, retry, then) {
  if (out.ok) {
    if (out.preview) showPreview(out.preview, 'Fiş önizlemesi (yazıcı seçili değil)', 'Yazıcı ayarlanınca bu fiş kâğıda basılır.', then);
    else { toast('No ' + out.no + ' basıldı' + (out.copy ? ' (kopya)' : '') + '.', 'good'); if (then) then(); }
    return;
  }
  openModal({
    title: out.title || 'Adisyon basılamadı',
    body: '<div class="err-box">' + ic('alert', 34, 2.2) + '<div><strong>' + esc(out.message || 'Yazıcıya ulaşılamıyor.') + '</strong><p>' + esc(out.hint || '') + '</p></div></div><p class="muted">Adisyon kaybolmadı; masa açık duruyor.</p>',
    actions: [
      { label: 'Önizlemeyi göster', icon: 'eye', onClick: async () => { closeModal(true); const p = await act('preview', { no: out.no }); if (p) showPreview(p.data.image, 'Fiş önizlemesi'); } },
      { label: 'Kapat', onClick: () => closeModal() },
      { label: 'Tekrar Dene', kind: 'btn-primary', icon: 'printer', onClick: () => { closeModal(true); retry(); } }
    ]
  });
}
function showPreview(image, title, note, onDone) {
  openModal({ title: title || 'Fiş önizlemesi', wide: false, onClose: onDone, body: (note ? '<p class="muted small">' + esc(note) + '</p>' : '') + '<div class="receipt-scroll"><img class="receipt-img" src="' + image + '" alt="Fiş önizlemesi"></div>', actions: [{ label: 'Tamam', kind: 'btn-primary', onClick: () => closeModal() }] });
}
// Hesap ödendi: ödeme şekli seçilince hesap kapanır. Ödeme şekli fişe yazılmaz;
// adisyon fişi ödemeden önce basılır, şekil yalnız programda adisyona kaydedilir.
async function payFlow(t) {
  // Yoldaki ürün ekleme/azaltma istekleri bitmeden açılmasın: pencerede son toplam görünsün.
  if (ui.busy) return;
  ui.busy = 'paywait:' + t;
  try { await chain.catch(() => {}); } finally { if (ui.busy === 'paywait:' + t) ui.busy = null; }
  if (ui.route !== 'siparis' || ui.table !== t) return;
  const a = activeFor(t); if (!a || !a.lines.length || ui.busy) return;
  let warn = '';
  if (a.printFailed) warn = '<div class="note bad">' + ic('alert') + '<p><strong>Son baskı yapılamadı.</strong> Müşteriye fiş verilecekse önce basın.</p><button type="button" class="btn btn-md" data-act="pay-print" data-t="' + t + '">' + ic('printer', 20) + 'Adisyonu Bas</button></div>';
  else if (a.status === 'open') warn = '<div class="note warn">' + ic('alert') + '<p><strong>Adisyon basılmadı.</strong> Müşteriye fiş verilecekse önce basın.</p><button type="button" class="btn btn-md" data-act="pay-print" data-t="' + t + '">' + ic('printer', 20) + 'Adisyonu Bas</button></div>';
  else if (a.changed) warn = '<div class="note warn">' + ic('alert') + '<p><strong>Adisyon basıldıktan sonra değişti.</strong> Güncel halini vermek için tekrar basın.</p><button type="button" class="btn btn-md" data-act="pay-print" data-t="' + t + '">' + ic('printer', 20) + 'Tekrar Bas</button></div>';
  const methods = (S.paymentMethods || []).map((m, i) => '<button type="button" class="pay-btn" data-act="pay-method" data-t="' + t + '" data-no="' + a.no + '" data-total="' + a.total + '" data-m="' + esc(m.id) + '"><span class="kbd" aria-hidden="true">' + (i + 1) + '</span>' + ic(PAY_ICON[m.id] || 'cash', 40, 1.8) + '<span>' + esc(m.label) + '</span></button>').join('');
  openModal({
    title: 'Hesap Ödendi · Masa ' + t,
    kind: 'pay',
    focusClose: true,
    body: '<div class="pay-head"><span class="who">No ' + a.no + ' · ' + esc(a.waiterName) + '</span><strong class="pay-total">' + TL(a.total) + '</strong></div>' + warn +
      '<fieldset class="reasons"><legend class="lbl-b">Müşteri nasıl ödedi?</legend><div class="pay-methods">' + methods + '</div></fieldset>' +
      '<p class="hint">Seçtiğiniz ödeme şekli adisyona kaydedilir ve masa kapanır. Ödeme şekli fişe basılmaz. Klavyeden 1, 2 ya da 3’e de basabilirsiniz.</p>',
    actions: [{ label: 'Vazgeç', onClick: () => closeModal() }]
  });
  modal.payTable = t;
}
async function doPay(t, no, method, total) {
  const a = activeFor(t);
  if (!a || a.no !== no || ui.busy) { closeModal(true); render(); return; }
  closeModal(true);
  ui.busy = 'pay:' + a.no; render();
  const r = await act('pay', { no: a.no, method, total });
  ui.busy = null;
  if (!r) { render(); return; }
  const out = r.data;
  toast('Masa ' + t + ' kapatıldı · ' + TL(out.total) + ' · ' + out.methodLabel, 'good');
  // Garson bu arada başka bir masaya geçtiyse onu yerinden etme.
  if (ui.route === 'siparis' && ui.table === t) { ui.route = 'masalar'; ui.table = null; }
  render();
}
function moveFlow(t) {
  const a = activeFor(t); if (!a || ui.busy) return;
  const byTable = new Map(S.open.map(x => [x.table, x]));
  let tiles = '';
  for (let n = 1; n <= S.tables; n++) {
    const o = byTable.get(n);
    if (n === t) { tiles += '<button type="button" class="move-t busy" disabled><b>Masa ' + n + '</b><span>bu masa</span></button>'; continue; }
    tiles += o
      ? '<button type="button" class="move-t busy" data-act="move-to" data-t="' + t + '" data-to="' + n + '"><b>Masa ' + n + '</b><span>' + TL(o.total) + ' · birleştir</span></button>'
      : '<button type="button" class="move-t" data-act="move-to" data-t="' + t + '" data-to="' + n + '"><b>Masa ' + n + '</b><span>boş</span></button>';
  }
  openModal({
    title: 'Masa ' + t + ' · taşı ya da birleştir',
    wide: true,
    body: '<p class="muted">Boş bir masa seçerseniz adisyon oraya taşınır. Dolu bir masa seçerseniz iki adisyon birleşir: ürünler o masanın adisyonuna eklenir.</p><div class="move-grid">' + tiles + '</div>',
    actions: [{ label: 'Vazgeç', onClick: () => closeModal() }]
  });
}
async function moveTo(t, target) {
  const a = activeFor(t); if (!a || ui.busy) { closeModal(); return; }
  const o = activeFor(target);
  const run = async () => {
    ui.busy = 'move:' + a.no; render();
    const r = await act('moveTable', { no: a.no, target });
    ui.busy = null;
    if (!r) { render(); return; }
    const res = r.data;
    toast(res.merged ? 'Masa ' + t + ', Masa ' + target + ' ile birleşti (No ' + res.targetNo + ').' : 'Adisyon Masa ' + dat(target) + ' taşındı.', 'good');
    if (ui.route === 'siparis' && ui.table === t) go('siparis', target); else render();
  };
  if (!o) { closeModal(true); await run(); return; }
  openModal({
    title: 'Masalar birleşsin mi?',
    body: '<p>Masa ' + t + ' (No ' + a.no + ', ' + TL(a.total) + ') ile Masa ' + target + ' (No ' + o.no + ', ' + TL(o.total) + ') birleşecek.</p><ul class="warn-list"><li class="warn">' + ic('info', 20) + 'Ürünler Masa ' + loc(target) + 'ki adisyona eklenir; toplam ' + TL(a.total + o.total) + ' olur.</li><li class="warn">' + ic('info', 20) + 'No ' + a.no + ' silinmez, “birleşti” olarak kayıtta kalır.</li>' + (o.status === 'printed' ? '<li class="warn">' + ic('printer', 20) + 'Masa ' + target + ' daha önce basıldı; birleşince tekrar basın.</li>' : '') + '</ul>',
    actions: [{ label: 'Vazgeç', onClick: () => closeModal() }, { label: 'Birleştir', kind: 'btn-primary', icon: 'move', onClick: () => { closeModal(true); run(); } }]
  });
}
function cancelFlow(t) {
  const a = activeFor(t); if (!a) return;
  cancelFlowFor(a, () => { toast('No ' + a.no + ' iptal edildi.'); ui.route = 'masalar'; ui.table = null; render(); });
}
function cancelFlowFor(a, after) {
  openModal({
    title: 'No ' + a.no + ' iptal edilsin mi?',
    body: '<p>İptal edilen adisyon silinmez; numarası “İptal” olarak kayıtta kalır.' + (a.status === 'paid' ? ' Hesap ' + TL(a.total) + ' · ' + esc(payLabel(a.paymentMethod)) + ' olarak ödenmiş görünüyor; iptal edilince ciroya girmez.' : '') + ' Sebebi seçince yönetici şifresi sorulur.</p><fieldset class="reasons"><legend>Sebep</legend>' + REASONS.map((r, i) => '<label class="reason"><input type="radio" name="reason" value="' + esc(r) + '"' + (i === 0 ? ' checked data-autofocus' : '') + '><span>' + esc(r) + '</span></label>').join('') + '<label class="reason"><input type="radio" name="reason" value="__other"><span>Diğer</span></label><input class="input" type="text" id="reason-other" placeholder="Diğer sebebi yazın" aria-label="Diğer sebep" maxlength="200"></fieldset>',
    actions: [{ label: 'Vazgeç', onClick: () => closeModal() }, { label: 'Adisyonu iptal et', kind: 'btn-danger', icon: 'x', onClick: async () => {
      const chosen = document.querySelector('input[name="reason"]:checked');
      const v = chosen ? chosen.value : '';
      const reason = v === '__other' ? val('reason-other').trim() : v;
      if (!reason) { toast('Bir sebep seçin ya da yazın.', 'bad'); return; }
      closeModal(true);
      const r = await act('cancel', { no: a.no, reason });
      if (!r) return;
      after();
    } }]
  });
}
async function saveBusiness(changedEl) {
  // Yalnız değişen alan kaydedilir: ekran bu arada yeniden çizildiyse diğer
  // alanların eski hali yeni kaydın üstüne yazılmasın.
  const b = Object.assign({}, S.business);
  if (changedEl) b[changedEl.dataset.biz] = changedEl.value;
  else document.querySelectorAll('[data-biz]').forEach(el => { b[el.dataset.biz] = el.value; });
  const r = await act('saveSettings', { business: b });
  if (r) toast('Fiş bilgileri kaydedildi.');
  else render();
  scheduleSample();
}
async function savePrinter(scope) {
  const cfg = readPrinterForm(scope, scope === 'wz' ? ui.setup.printer : S.printer);
  if (scope === 'wz') { ui.setup.printer = cfg; ui.testResult = null; render(); return; }
  const r = await act('saveSettings', { printer: cfg });
  ui.testResult = null;
  render();
  if (r) { toast('Yazıcı ayarı kaydedildi.'); refreshSample(); }
}
async function testPrint(scope) {
  const cfg = readPrinterForm(scope, scope === 'wz' ? ui.setup.printer : S.printer);
  if (scope === 'wz') ui.setup.printer = cfg;
  const btn = document.querySelector('[data-act="test-print"]');
  if (btn) { btn.disabled = true; btn.classList.add('busy'); btn.dataset.busy = 'Basılıyor…'; }
  const r = await act('testPrint', { printer: cfg });
  if (r) ui.testResult = Object.assign({ mode: cfg.mode }, r.data);
  render();
}
function readLogo(file) {
  if (!file) return;
  if (!/^image\/(png|jpeg|svg\+xml|webp)$/.test(file.type)) { toast('PNG ya da JPG dosyası seçin.', 'bad'); return; }
  const fr = new FileReader();
  fr.onload = () => {
    const img = new Image();
    img.onload = async () => {
      const w = img.naturalWidth || 576, h = img.naturalHeight || 288;
      const scale = Math.min(1, 576 / w, 400 / h);
      const c = document.createElement('canvas'); c.width = Math.max(1, Math.round(w * scale)); c.height = Math.max(1, Math.round(h * scale));
      const g = c.getContext('2d'); g.fillStyle = '#ffffff'; g.fillRect(0, 0, c.width, c.height); g.drawImage(img, 0, 0, c.width, c.height);
      const r = await act('setLogo', { image: c.toDataURL('image/png') });
      if (r) { render(); refreshSample(); toast('Logo eklendi; fişte siyah-beyaz basılır.'); }
    };
    img.onerror = () => toast('Logo okunamadı.', 'bad');
    img.src = fr.result;
  };
  fr.readAsDataURL(file);
}

// ---------- Tıklamalar ----------
const ACT = {
  'reload': () => location.reload(),
  'nav': el => go(el.dataset.to),
  'lock-now': async () => { await act('lock'); if (LOCKED.has(ui.route)) ui.route = 'masalar'; render(); toast('Yönetici girişi kapatıldı.'); },
  'win': el => act('window', { action: el.dataset.a }),
  'quit': () => openModal({ title: 'Programı kapat', body: '<p>Program kapansın mı? Açık masalar kayıtlı; program yeniden açılınca aynen gelir.</p>', actions: [{ label: 'Vazgeç', onClick: () => closeModal() }, { label: 'Programı kapat', kind: 'btn-primary', icon: 'power', onClick: () => { closeModal(true); act('window', { action: 'quit' }); } }] }),
  'open-table': el => openTable(Number(el.dataset.t)),
  'pick-waiter': el => { const f = modal && modal.onPick; closeModal(true); if (f) f(Number(el.dataset.w)); },
  'cat-tab': el => { ui.catTab = el.dataset.c === 'all' ? 'all' : Number(el.dataset.c); render(); },
  'add': el => addItem(Number(el.dataset.t), Number(el.dataset.p)),
  'qty': async el => { if (await act('changeQty', { no: Number(el.dataset.no), lineId: Number(el.dataset.l), delta: Number(el.dataset.d) })) render(); },
  'undo': async el => { const a = activeFor(Number(el.dataset.t)); if (!a) return; if (await act('undo', { no: a.no })) { render(); toast('Son işlem geri alındı.'); } },
  'print': el => doPrint(Number(el.dataset.t)),
  'pay': el => payFlow(Number(el.dataset.t)),
  'pay-method': el => once('pay', () => doPay(Number(el.dataset.t), Number(el.dataset.no), el.dataset.m, Number(el.dataset.total))),
  'pay-print': el => { const t = Number(el.dataset.t); closeModal(true); doPrint(t, () => { const a = activeFor(t); if (a && ui.route === 'siparis' && ui.table === t) payFlow(t); }); },
  'move-table': el => moveFlow(Number(el.dataset.t)),
  'move-to': el => moveTo(Number(el.dataset.t), Number(el.dataset.to)),
  'goto-day': el => { closeModal(true); go('gunsonu', el.dataset.d); },
  'day-prev': () => loadDay(addDays(ui.day.date || S.businessDate, -1)),
  'day-next': () => { const d = addDays(ui.day.date || S.businessDate, 1); if (d <= S.businessDate) loadDay(d); },
  'day-today': () => loadDay(''),
  'day-filter': el => { ui.day.filter = el.dataset.f; render(); },
  'day-close': () => closeDayFlow(),
  'day-print': () => once('day-print', () => printDay(ui.day.date)),
  'day-preview': async () => { const r = await act('dayPreview', { date: ui.day.date }); if (r) showPreview(r.data.image, 'Gün sonu fişi önizlemesi', 'İşletme içi rapordur; müşteriye verilmez.'); },
  'detail': el => showDetail(Number(el.dataset.no)),
  'change-pay': el => changePayFlow(Number(el.dataset.no), el.dataset.m),
  'change-pay-to': el => once('change-pay', () => changePayTo(Number(el.dataset.no), el.dataset.m)),
  'audit-kind': el => { ui.audit.kind = el.dataset.k; if (document.getElementById('au-from')) { ui.audit.from = val('au-from'); ui.audit.to = val('au-to'); ui.audit.no = val('au-no').trim(); } loadAudit(); },
  'audit-load': () => { ui.audit.from = val('au-from'); ui.audit.to = val('au-to'); ui.audit.no = val('au-no').trim(); loadAudit(); },
  'audit-more': () => loadAudit(true),
  'st-mode': el => setStatsMode(el.dataset.m),
  'st-shift': el => shiftStats(Number(el.dataset.d)),
  'st-sort': el => { const k = el.dataset.k; const so = ui.stats.sort; if (so.key === k) so.dir = -so.dir; else { so.key = k; so.dir = k === 'date' ? 1 : -1; } render(); },
  'st-excel': () => once('st-excel', async () => {
    const T = ui.stats; const r = await act('exportExcel', { mode: T.mode, from: T.from, to: T.to });
    if (r) openModal({ title: 'Excel raporu kaydedildi', body: '<div class="note good">' + ic('check', 22, 2.4) + '<p>Rapor dosyası kaydedildi; klasörü açıldı.</p></div><dl class="kv"><dt>Dosya</dt><dd>' + esc(r.data.path) + '</dd></dl><p class="muted">Sayfalar: Özet, Günlük (gün × ürün adetleriyle), Ürünler, Adisyonlar.</p>', actions: [{ label: 'Klasörü aç', icon: 'folder', onClick: () => act('openFolder', { which: 'reports' }) }, { label: 'Tamam', kind: 'btn-primary', onClick: () => closeModal() }] });
  }),
  'cancel': el => cancelFlow(Number(el.dataset.t)),
  'discount': el => discountFlow(Number(el.dataset.t)),
  'discount-set': el => setDiscount(Number(el.dataset.t), Number(el.dataset.pct)),
  'discount-custom': el => { const v = val('disc-pct').trim().replace('%', ''); if (!/^\d{1,3}$/.test(v)) { toast('Yüzde 1 ile 100 arasında tam sayı olmalı.', 'bad'); return; } setDiscount(Number(el.dataset.t), Number(v)); },
  'discount-clear': el => setDiscount(Number(el.dataset.t), 0),
  'empty-close': el => once('empty-close', async () => { const t = Number(el.dataset.t); const a = activeFor(t); if (!a) return; if (await act('closeEmpty', { no: a.no })) { toast('Masa ' + t + ' boşaltıldı.'); ui.route = 'masalar'; ui.table = null; render(); } }),
  'change-waiter': el => {
    const t = Number(el.dataset.t); const a = activeFor(t); const d = ui.drafts[t];
    pickWaiter('Masa ' + t + ' · garsonu değiştir', a ? a.waiterId : d && d.waiterId, async w => {
      if (a) { if (await act('changeWaiter', { no: a.no, waiterId: w })) { render(); toast('Garson: ' + waiterName(w)); } }
      else if (d) { d.waiterId = w; render(); }
    });
  },
  'preview': async el => { const a = activeFor(Number(el.dataset.t)); if (!a) return; const r = await act('preview', { no: a.no }); if (r) showPreview(r.data.image, 'Fiş önizlemesi'); },
  // ürünler
  'prod-filter': el => { ui.prodFilter = el.dataset.c === 'all' ? 'all' : Number(el.dataset.c); render(); },
  'bulk-price': () => bulkFlow(false),
  'bulk-all': el => document.querySelectorAll('[data-bulk-id]').forEach(x => { x.checked = el.dataset.v === '1'; }),
  'p-new': () => { ui.editProduct = 'new'; render(); const f = document.getElementById('f-name'); if (f) f.focus(); },
  'p-edit': el => { ui.editProduct = Number(el.dataset.id); render(); const f = document.getElementById('f-price'); if (f) f.focus(); },
  'p-cancel': () => { ui.editProduct = null; render(); },
  'p-move': async el => { if (await act('moveProduct', { id: Number(el.dataset.id), dir: Number(el.dataset.d) })) render(); },
  'c-move': async el => { if (await act('moveCategory', { id: Number(el.dataset.id), dir: Number(el.dataset.d) })) render(); },
  'c-color': async el => { const c = catById(Number(el.dataset.id)); if (!c) return; if (await act('saveCategory', { id: c.id, name: c.name, color: el.dataset.color })) render(); },
  'c-delete': el => { const c = catById(Number(el.dataset.id)); if (!c) return; openModal({ title: c.name + ' silinsin mi?', body: '<p>Kategori boş; silinince satış ekranındaki sekmesi kalkar.</p>', actions: [{ label: 'Vazgeç', onClick: () => closeModal() }, { label: 'Sil', kind: 'btn-danger', onClick: async () => { closeModal(true); if (await act('deleteCategory', { id: c.id })) render(); } }] }); },
  'c-add': () => once('c-add', async () => { const name = val('cat-new').trim(); if (!name) { toast('Kategori adını yazın.', 'bad'); return; } const used = new Set(S.categories.map(c => c.color)); const color = (S.colors.find(c => !used.has(c.id)) || S.colors[0]).id; if (await act('saveCategory', { id: 0, name, color })) { render(); toast(name + ' eklendi.'); } }),
  // ayarlar
  'logo-remove': async () => { if (await act('setLogo', { image: '' })) { render(); refreshSample(); } },
  'test-preview': async () => { const r = await act('testPreview', {}); if (r) showPreview(r.data.image, 'Fiş önizlemesi', 'Test fişi; logo, başlık, QR ve alt bilgiler kayıtlı ayarlarla çizilir.'); },
  'test-print': el => testPrint(el.dataset.scope),
  'printers-refresh': async () => { ui.printers = null; render(); await loadPrinters(); },
  'restore-open': () => restoreFlow(),
  'restore-pick': el => restorePick(Number(el.dataset.i)),
  'sample-print': () => once('sample-print', async () => { const r = await act('samplePrint'); if (!r) return; const out = r.data; if (out.ok && out.preview && S.printer.mode === 'none') showPreview(out.preview, 'Örnek adisyon (yazıcı seçili değil)'); else if (out.ok) toast('Örnek adisyon basıldı.', 'good'); else toast((out.title || 'Basılamadı') + ': ' + (out.message || ''), 'bad'); }),
  'drive-save': async () => { const dir = val('s-drive').trim(); if (await act('saveSettings', { driveBackupDir: dir })) { render(); toast(dir ? 'Google Drive klasörü kaydedildi; sonraki yedekte kopyalanır.' : 'Google Drive klasörü kaldırıldı.'); if (dir) act('backupNow').then(r => { if (r) render(); }); } },
  'drive-clear': async () => { if (await act('saveSettings', { driveBackupDir: '' })) { render(); toast('Google Drive klasörü kaldırıldı.'); } },
  'drive-find': async () => {
    const box = document.getElementById('drive-dirs'); if (box) box.innerHTML = '<p class="hint">Aranıyor…</p>';
    const r = await act('driveDirs'); const b2 = document.getElementById('drive-dirs'); if (!b2) return;
    const ds = r ? r.data.dirs || [] : [];
    b2.innerHTML = ds.length ? ds.map(d => '<button type="button" class="btn btn-md" data-act="gdrive-pick" data-path="' + esc(d) + '">' + ic('cloud', 20) + esc(d) + '</button>').join('') : '<p class="hint">Google Drive klasörü bulunamadı. Google Drive (masaüstü) programını kurup oturum açın, sonra tekrar deneyin. Klasörü elle de yazabilirsiniz.</p>';
  },
  'gdrive-pick': el => { const i = document.getElementById('s-drive'); if (i) i.value = el.dataset.path; ACT['drive-save'](); },
  'wa-save': () => once('wa-save', async () => { if (await saveWhatsApp()) toast('WhatsApp ayarı kaydedildi.', 'good'); }),
  'wa-test': () => once('wa-test', async () => { if (!(await saveWhatsApp())) return; const r = await act('testRemote', { kind: 'whatsapp' }); if (r) toast(r.data.message, 'good'); }),
  'gs-save': () => once('gs-save', async () => { if (await saveSheets()) toast('Google Sheets ayarı kaydedildi.', 'good'); }),
  'gs-test': () => once('gs-test', async () => { if (!(await saveSheets())) return; const r = await act('testRemote', { kind: 'sheets' }); if (r) toast(r.data.message, 'good'); }),
  'gs-secret': () => openModal({ title: 'Yeni gizli anahtar üretilsin mi?', body: '<p>Eski anahtarla çalışan Apps Script kodu artık kabul edilmez. Yeni kodu Apps Script’e yapıştırıp yeniden dağıtmanız gerekir.</p>', actions: [{ label: 'Vazgeç', onClick: () => closeModal() }, { label: 'Yeni anahtar üret', kind: 'btn-primary', onClick: async () => { closeModal(true); if (await saveSheets({ newSecret: true })) toast('Yeni anahtar üretildi; kodu yeniden kopyalayın.'); } }] }),
  'outbox-retry': async () => { if (await act('retryOutbox')) { render(); toast('Gönderim yeniden denenecek.'); } },
  'copy': el => copyFrom(el.dataset.from),
  'backup-now': () => once('backup-now', async () => { if (await act('backupNow')) { render(); toast('Yedek alındı.', 'good'); } }),
  'open-folder': el => act('openFolder', { which: el.dataset.w }),
  'second-save': async () => { const dir = val('s-second').trim(); if (await act('saveSettings', { secondBackupDir: dir })) { render(); toast(dir ? 'İkinci yedek yeri kaydedildi.' : 'İkinci yedek yeri kaldırıldı.'); } },
  'second-clear': async () => { if (await act('saveSettings', { secondBackupDir: '' })) { render(); toast('İkinci yedek yeri kaldırıldı.'); } },
  'drives': async () => {
    const r = await act('drives'); if (!r) return;
    const box = document.getElementById('drives'); if (!box) return;
    const ds = r.data.drives || [];
    box.innerHTML = ds.length ? ds.map(d => '<button type="button" class="btn btn-md" data-act="drive-pick" data-path="' + esc(d.path) + '">' + ic('save', 20) + esc(d.path) + (d.label ? ' · ' + esc(d.label) : '') + '</button>').join('') : '<p class="hint">Takılı USB bellek bulunamadı. Belleği takıp tekrar deneyin.</p>';
  },
  'drive-pick': el => { const i = document.getElementById('s-second'); if (i) i.value = el.dataset.path; ACT['second-save'](); },
  'pin-save': () => once('pin-save', async () => {
    const o = val('pin-old'), n1 = val('pin-new'), n2 = val('pin-new2');
    if (!/^\d{4}$/.test(n1)) { toast('Yeni şifre 4 rakam olmalı.', 'bad'); return; }
    if (n1 !== n2) { toast('Yeni şifreler aynı değil.', 'bad'); return; }
    if (await act('changePin', { old: o, new: n1 })) { render(); toast('Yönetici şifresi değişti.', 'good'); }
  }),
  'w-add': () => once('w-add', async () => { const v = val('w-new').trim(); if (!v) { toast('Garson adını yazın.', 'bad'); return; } if (await act('saveWaiter', { id: 0, name: v })) { render(); toast(v + ' eklendi.'); } }),
  'w-remove': el => { const id = Number(el.dataset.id); const n = waiterName(id); openModal({ title: n + ' listeden kaldırılsın mı?', body: '<p>Geçmiş adisyonlarda adı kalır.</p>', actions: [{ label: 'Vazgeç', onClick: () => closeModal() }, { label: 'Kaldır', kind: 'btn-danger', onClick: async () => { closeModal(true); if (await act('removeWaiter', { id })) { render(); toast(n + ' kaldırıldı.'); } } }] }); },
  // kurulum
  'wz-next': () => setupNext(),
  'wz-back': () => { if (ui.setup.step === 2) ui.setup.printer = readPrinterForm('wz', ui.setup.printer); if (ui.setup.step > 0) { ui.setup.step--; ui.testResult = null; render(); } },
  'wz-tables': el => { const n = Math.min(200, Math.max(1, (Number(ui.setup.tables) || 0) + Number(el.dataset.d))); ui.setup.tables = n; render(); },
  'wz-waiter-add': () => { ui.setup.waiters.push(''); render(); const i = document.getElementById('w-wt-' + (ui.setup.waiters.length - 1)); if (i) i.focus(); },
  'wz-waiter-del': el => { ui.setup.waiters.splice(Number(el.dataset.i), 1); render(); },
  // pencereler
  'modal-action': el => { const a = modal && modal.actions[Number(el.dataset.i)]; if (a && a.onClick) a.onClick(); },
  'modal-close': () => closeModal(),
  'pin-key': el => { const p = modal && modal.pin; if (!p) return; const k = el.dataset.k; if (k === 'C') p.set(''); else if (k === 'B') p.set(p.get().slice(0, -1)); else if (p.get().length < 4) p.set(p.get() + k); }
};

document.addEventListener('click', e => {
  // Çift tıklamanın ikinci tıklaması, birincinin açtığı/kapattığı pencerede ya da
  // altındaki ekranda bir şey seçmesin (ör. ödeme şekli, ürün).
  const now = Date.now();
  if (clickGuard && e.detail > 1 && now < clickGuard.until && Math.abs(e.clientX - clickGuard.x) < 16 && Math.abs(e.clientY - clickGuard.y) < 16) { e.preventDefault(); return; }
  lastClick = { x: e.clientX, y: e.clientY, t: now };
  const el = e.target.closest('[data-act]'); if (!el) return;
  const a = el.dataset.act;
  if (a === 'modal-scrim') { if (e.target === el && !(modal && modal.kind === 'pin')) closeModal(); return; }
  if (el.disabled) return;
  const h = ACT[a]; if (h) h(el, e);
  touch();
});
document.addEventListener('submit', async e => {
  if (e.target.id !== 'p-form') return;
  e.preventDefault();
  const name = val('f-name').trim();
  const rawPrice = val('f-price').trim();
  const price = rawPrice === '' ? 0 : parsePrice(rawPrice);
  if (!name) { toast('Ürün adını yazın.', 'bad'); document.getElementById('f-name').focus(); return; }
  if (isNaN(price) || price < 0) { toast('Geçerli bir fiyat yazın. Örnek: 340 ya da 340,50', 'bad'); document.getElementById('f-price').focus(); return; }
  const v = (document.querySelector('input[name="f-vat"]:checked') || {}).value || '';
  const data = { id: ui.editProduct === 'new' ? 0 : ui.editProduct, name, price, unit: val('f-unit'), categoryId: Number(val('f-cat')), vat: v === '' ? null : Number(v), active: checked('f-active') };
  const r = await once('p-save', () => act('saveProduct', data));
  if (!r) return;
  ui.editProduct = r.data.id;
  render();
  toast(name + ' kaydedildi' + (price > 0 ? '' : ' (fiyat girilmediği için satılamaz)') + '.');
});
document.addEventListener('input', e => {
  const el = e.target;
  if (el.dataset && el.dataset.wz && ui.setup) {
    const [k, sub] = el.dataset.wz.split('.');
    if (k === 'business') ui.setup.business[sub] = el.value;
    else if (k === 'waiters') ui.setup.waiters[Number(sub)] = el.value;
    else if (k === 'prices') ui.setup.prices[sub] = el.value;
    else ui.setup[k] = el.value;
  }
  if (el.dataset && el.dataset.biz && ui.route === 'ayarlar') scheduleSample();
  if (el.id === 'reason-other') { const o = document.querySelector('input[name="reason"][value="__other"]'); if (o) o.checked = true; }
  touch();
});
document.addEventListener('change', async e => {
  const el = e.target;
  if (el.dataset && el.dataset.biz) { saveBusiness(el); return; }
  if (el.dataset && el.dataset.pf) { savePrinter(el.dataset.pf); return; }
  if (el.dataset && el.dataset.catName) {
    const c = catById(Number(el.dataset.catName)); const v = el.value.trim();
    if (c && v && v !== c.name) { if (await act('saveCategory', { id: c.id, name: v, color: c.color })) { render(); toast('Kategori adı güncellendi.'); } } else render();
    return;
  }
  if (el.dataset && el.dataset.wrename) {
    const id = Number(el.dataset.wrename); const v = el.value.trim();
    if (v && v !== waiterName(id)) { if (await act('saveWaiter', { id, name: v })) { render(); toast('Garson adı güncellendi.'); } } else render();
    return;
  }
  switch (el.id) {
    case 'day-date': if (el.value) loadDay(el.value > S.businessDate ? S.businessDate : el.value); return;
    case 'st-day': if (el.value) { ui.stats.from = el.value; loadStats(); } return;
    case 'st-month': if (el.value) { ui.stats.from = el.value + '-01'; loadStats(); } return;
    case 'st-from': case 'st-to': { const f = val('st-from'), t = val('st-to'); if (f && t) { ui.stats.from = f <= t ? f : t; ui.stats.to = f <= t ? t : f; loadStats(); } return; }
    case 'st-product': ui.stats.product = Number(el.value); render(); return;
    case 'logo-file': readLogo(el.files && el.files[0]); el.value = ''; break;
    case 'set-tables': { const n = Number(el.value); if (await act('saveSettings', { tables: n })) toast('Masa sayısı ' + n + ' oldu.'); render(); break; }
    case 'set-lock': if (await act('saveSettings', { lockMinutes: Number(el.value) })) render(); break;
    case 'set-featured': if (await act('saveSettings', { featuredProductId: Number(el.value) })) toast('Öne çıkan ürün kaydedildi.'); render(); break;
    case 'wa-enabled': { const r = await saveWhatsApp({ enabled: el.checked }); if (r) toast(el.checked ? 'Gün kapanınca WhatsApp mesajı gönderilecek.' : 'WhatsApp gönderimi kapatıldı.'); break; }
    case 'gs-enabled': { const r = await saveSheets({ enabled: el.checked }); if (r) toast(el.checked ? 'Gün kapanınca Google Sheets’e yazılacak.' : 'Google Sheets gönderimi kapatıldı.'); break; }
    case 'set-fullscreen': if (await act('saveSettings', { fullscreen: el.checked })) toast(el.checked ? 'Program tam ekran açılacak.' : 'Program pencere olarak açılacak.'); render(); break;
    case 'set-disc-pin': if (await act('saveSettings', { discountNoPin: !el.checked })) toast(el.checked ? 'İndirim için yönetici şifresi istenecek.' : 'İndirim şifresiz uygulanabilecek.'); render(); break;
    case 'set-autostart': if (await act('setAutostart', { on: el.checked })) toast(el.checked ? 'Windows açılınca program başlayacak.' : 'Otomatik başlatma kapatıldı.'); render(); break;
  }
  if (el.name === 'set-vat') { if (await act('saveSettings', { defaultVat: Number(el.value) })) toast('Varsayılan KDV %' + el.value + ' oldu.'); render(); }
});
document.addEventListener('keydown', e => {
  if (!modal && e.key === 'Enter' && e.target && e.target.matches && e.target.matches('tr[data-act]')) { e.preventDefault(); e.target.click(); return; }
  if (!modal && (e.key === 'ArrowLeft' || e.key === 'ArrowRight') && e.target && e.target.dataset && e.target.dataset.line) {
    const el = e.target.closest('.chart'); if (el && el._items) { e.preventDefault(); const n = el._items.length; const i = el._cur == null ? n - 1 : Math.min(n - 1, Math.max(0, el._cur + (e.key === 'ArrowLeft' ? -1 : 1))); showTip(el, i); }
    return;
  }
  if (e.key === 'F11') { e.preventDefault(); if (S && S.program.mode === 'window') act('window', { action: 'fullscreen' }); return; }
  if (!modal) {
    if (e.key === 'Enter' && S && !S.setupDone && e.target && e.target.dataset && e.target.dataset.wz) { e.preventDefault(); setupNext(); }
    return;
  }
  if (e.key === 'Escape') { e.preventDefault(); closeModal(); return; }
  if (modal.kind === 'pay' && /^[1-9]$/.test(e.key) && !(e.target && /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName))) {
    const b = document.querySelectorAll('#modal-root .pay-btn')[Number(e.key) - 1];
    if (b && !b.disabled) { e.preventDefault(); b.click(); }
    return;
  }
  if (modal.pin) {
    if (/^\d$/.test(e.key)) { e.preventDefault(); const v = modal.pin.get(); if (v.length < 4) modal.pin.set(v + e.key); return; }
    if (e.key === 'Backspace') { e.preventDefault(); modal.pin.set(modal.pin.get().slice(0, -1)); return; }
  }
  if (e.key === 'Tab') {
    const f = [...document.querySelectorAll('#modal-root button:not([disabled]), #modal-root input:not([disabled]), #modal-root select, #modal-root textarea')];
    if (!f.length) return;
    const first = f[0], last = f[f.length - 1];
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  }
});
document.addEventListener('contextmenu', e => { if (!e.target.closest('input, textarea')) e.preventDefault(); });
let resizeTimer = 0;
window.addEventListener('resize', () => { setLayoutVars(); clearTimeout(resizeTimer); resizeTimer = setTimeout(drawCharts, 120); });

// Yönetici ekranında çalışılırken kilit süresini uzat.
function touch() {
  if (!S || !S.setupDone || !LOCKED.has(ui.route) || !isUnlocked()) return;
  const now = Date.now();
  if (now - ui.lastTouch < 30000) return;
  ui.lastTouch = now;
  unlockDeadline = now + S.lockMinutes * 60000;
  rawApi('touch').catch(() => {});
}

// Saat, kilit ve masa süreleri.
setInterval(async () => {
  const c = document.getElementById('clock'); if (c) c.textContent = clockText();
  if (!S || offline) return;
  if (S.setupDone && LOCKED.has(ui.route) && !isUnlocked() && !modal) {
    ui.route = 'masalar'; ui.editProduct = null; render(); toast('Yönetici ekranları kilitlendi.');
    act('state');
    return;
  }
  if (ui.route === 'masalar' && !modal && S.setupDone) {
    const r = await act('state');
    if (r && ui.route === 'masalar' && !modal) render();
  }
}, 15000);

// Program yaşıyor mu? Tarayıcı modunda program bu sinyal kesilince kendini kapatır.
setInterval(() => { if (!offline) rawApi('ping').catch(() => {}); }, 30000);

// ---------- Başlat ----------
(async () => {
  try {
    await api('state');
  } catch (e) {
    connectionLost();
    return;
  }
  if (S.setupDone && location.hash === '#ayarlar') ui.route = 'ayarlar';
  render();
})();
})();
