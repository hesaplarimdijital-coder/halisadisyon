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
const sinceText = iso => { const m = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60000)); return m < 1 ? 'az önce' : m < 60 ? m + ' dk' : Math.floor(m / 60) + ' sa ' + (m % 60) + ' dk'; };
// Sayıya Türkçe ek: 9’a, 12’ye (son okunan kelimeye göre)
const SFX1 = [null, ['e', 'den'], ['ye', 'den'], ['e', 'ten'], ['e', 'ten'], ['e', 'ten'], ['ya', 'dan'], ['ye', 'den'], ['e', 'den'], ['a', 'dan']];
const SFX10 = [null, ['a', 'dan'], ['ye', 'den'], ['a', 'dan'], ['a', 'tan'], ['ye', 'den'], ['a', 'tan'], ['e', 'ten'], ['e', 'den'], ['a', 'dan']];
const numSfx = n => { n = Math.abs(Math.round(Number(n) || 0)); if (n % 10) return SFX1[n % 10]; if (n % 100) return SFX10[(n % 100) / 10]; if (n === 0) return ['a', 'dan']; if (n % 1e6) return ['e', 'den']; return ['a', 'dan']; };
const dat = n => n + '’' + numSfx(n)[0];
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
  save: '<path d="M5 3h11l3 3v15H5z"/><path d="M8 3v6h8V3M8 21v-7h8v7"/>'
};
const ic = (n, s = 22, sw = 2, cls = '') => '<svg class="ic ' + cls + '" width="' + s + '" height="' + s + '" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="' + sw + '" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' + (ICONS[n] || '') + '</svg>';

// ---------- Durum ----------
let S = null;                 // programdan gelen güncel durum
let unlockDeadline = 0;
const LOCKED = new Set(['urunler', 'ayarlar']);
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
  lastTouch: 0
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

function vatBreakdown(lines) {
  const by = {};
  for (const l of lines) by[l.vat] = (by[l.vat] || 0) + l.price * l.qty;
  return Object.keys(by).map(Number).sort((a, b) => a - b).map(r => ({ rate: r, gross: by[r], vat: Math.round(by[r] * r / (100 + r)) }));
}

// ---------- Program ile konuşma ----------
class ApiError extends Error { constructor(r) { super(r.error || 'Bir sorun oldu.'); this.code = r.code || ''; } }
// Sipariş işlemleri tıklama sırasıyla gider (zincir). Baskı ve hesap kapatma kendinden
// önceki tıklamaları bekler ama sonrakileri bekletmez; yazıcı takılsa da diğer masalar
// çalışır. Durum, bağlantı ve pencere istekleri hiç beklemez.
let chain = Promise.resolve();
const AFTER_PRIOR = new Set(['print', 'pay', 'testPrint', 'backupNow']);
const FREE = new Set(['ping', 'touch', 'window', 'state']);
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
        askPin(r => single ? resolve(r) : api(name, args, Object.assign({}, opts, { noPin: true })).then(resolve, reject),
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
  if (offline) return;
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
function openModal(o) {
  const root = document.getElementById('modal-root');
  const prevFocus = modal ? modal.returnFocus : document.activeElement;
  // Açık pencerenin yerine yenisi geliyorsa eskisi “vazgeçildi” sayılır (bekleyen işlem takılı kalmasın).
  if (modal && modal.onClose) { const f = modal.onClose; modal.onClose = null; setTimeout(f, 0); }
  modal = { actions: o.actions || [], onClose: o.onClose, returnFocus: prevFocus, kind: o.kind || '' };
  const acts = modal.actions.map((a, i) => '<button type="button" class="btn ' + (a.kind || '') + '" data-act="modal-action" data-i="' + i + '"' + (a.disabled ? ' disabled' : '') + (a.id ? ' id="' + a.id + '"' : '') + '>' + (a.icon ? ic(a.icon) : '') + esc(a.label) + '</button>').join('');
  root.innerHTML = '<div class="scrim" data-act="modal-scrim"><div class="modal' + (o.wide ? ' wide' : '') + '" role="dialog" aria-modal="true" aria-labelledby="modal-title"><div class="modal-head"><h2 id="modal-title">' + esc(o.title) + '</h2><button type="button" class="icon-btn" data-act="modal-close" aria-label="Kapat">' + ic('x') + '</button></div><div class="modal-body">' + o.body + '</div>' + (acts ? '<div class="modal-actions">' + acts + '</div>' : '') + '</div></div>';
  root.hidden = false;
  const first = root.querySelector('[data-autofocus]') || root.querySelector('.modal-actions .btn-primary') || root.querySelector('.modal-body button, .modal-body input') || root.querySelector('button');
  if (first) first.focus();
}
function closeModal(silent) {
  if (!modal) return;
  const m = modal; modal = null;
  const root = document.getElementById('modal-root'); root.innerHTML = ''; root.hidden = true;
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
        closeModal(true);
        render();
        onOk(res);
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
  ui.route = name; ui.table = table == null ? null : table;
  if (name === 'ayarlar' && !ui.printers) loadPrinters();
  render();
  window.scrollTo(0, 0);
}
function dropEmptyDraft(t) { if (t != null && ui.drafts[t] && !activeFor(t)) delete ui.drafts[t]; }

// ---------- Ekranlar ----------
function header() {
  if (!S.setupDone) {
    return '<header class="top" id="top"><div class="top-in"><div class="brand"><span class="brand-name">' + esc(shortName()) + '</span><span class="brand-sub">Adisyon · İlk kurulum</span></div>' + winButtons() + '</div></header>';
  }
  const items = [['masalar', 'grid', 'Masalar'], ['urunler', 'list', 'Ürünler'], ['ayarlar', 'sliders', 'Ayarlar']];
  const nav = items.map(([k, i, l]) => {
    const on = ui.route === k || (k === 'masalar' && ui.route === 'siparis');
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
  const oldLines = app.querySelector('.ticket .lines');
  const linesKey = oldLines ? oldLines.dataset.t : null; const linesTop = oldLines ? oldLines.scrollTop : 0;
  app.innerHTML = header() + '<main class="main" id="main">' + screen() + '</main>';
  const newLines = app.querySelector('.ticket .lines');
  if (newLines && newLines.dataset.t === linesKey) newLines.scrollTop = linesTop;
  if (fk && !modal) { const el = app.querySelector('[data-fk="' + CSS.escape(fk) + '"]'); if (el) el.focus({ preventScroll: true }); }
  setLayoutVars();
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
  const warn = (bk.lastError ? '<div class="note warn" role="status">' + ic('alert') + '<p><strong>Yedek alınamadı.</strong> ' + esc(bk.lastError) + ' Siparişler kayıtlı; yöneticiye haber verin.</p></div>' : '') +
    (bk.secondError ? '<div class="note warn" role="status">' + ic('alert') + '<p><strong>İkinci yere (USB) yedek alınamadı.</strong> ' + esc(bk.secondError) + '</p></div>' : '');
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
  const total = lines.reduce((s, l) => s + l.price * l.qty, 0);
  const kd = vatBreakdown(lines);
  const qtyOf = pid => lines.filter(l => l.productId === pid).reduce((s, l) => s + l.qty, 0);
  // Bu adisyon basılırken ya da kapanırken ürün eklenmesin (basılan ile kapanan tutar aynı kalsın).
  const frozen = !!(a && ui.busy && (ui.busy === 'print:' + a.no || ui.busy === 'pay:' + a.no));

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
    '<div class="order-meta"><span>No <strong>' + (a ? a.no : '—') + '</strong></span><span>Açılış <strong>' + hm(opened) + '</strong></span><button type="button" class="btn waiter-btn" data-act="change-waiter" data-t="' + t + '" data-fk="cw" aria-label="Garson ' + esc(waiter) + ', değiştir">' + ic('user', 20) + '<strong>' + esc(waiter) + '</strong><span class="muted">değiştir</span></button></div></div>' +
    '<section class="products" aria-label="Ürünler">' + tabs + groups + '</section>' +
    '<aside class="card ticket" aria-label="Adisyon"><div class="ticket-head"><h2>Adisyon</h2>' + ticketSub + '</div>' + notice + lineHTML +
    '<div class="totals"><div class="total-row"><span class="lbl">Toplam (KDV dahil)</span><span class="total-amount">' + TL(total) + '</span></div>' + kd.map(k => '<div class="kdv-row"><span>İçindeki KDV (%' + k.rate + ')</span><span>' + TL(k.vat) + '</span></div>').join('') + '</div>' +
    '<button type="button" class="btn btn-primary btn-xl' + (busyPrint ? ' busy' : '') + '" data-busy="Basılıyor…" data-act="print" data-t="' + t + '" data-fk="print"' + (lines.length && !ui.busy ? '' : ' disabled') + '>' + ic('printer', 28) + (a && a.status === 'printed' ? 'Tekrar Bas' : 'Adisyonu Bas') + '</button>' +
    '<div class="ticket-actions">' + payBtn + '<button type="button" class="btn" data-act="undo" data-t="' + t + '" data-fk="undo"' + (undoCount && !ui.busy ? '' : ' disabled') + '>' + ic('undo') + 'Geri Al</button><button type="button" class="btn btn-danger-quiet" data-act="cancel" data-t="' + t + '" data-fk="cancel"' + (a && !ui.busy ? '' : ' disabled') + ' aria-label="İptal, yönetici şifresi gerekir">' + ic('lock', 20) + 'İptal</button></div>' +
    '</aside></div>';
}

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

  return '<div class="page-head"><div class="ttl"><h1>Ürünler ve fiyatlar</h1><p class="muted">Fiyatları KDV dahil girin. Değişiklik satış ekranına hemen yansır; geçmiş adisyonların fiyatı değişmez.</p></div><button type="button" class="btn btn-primary" data-act="p-new" data-fk="p-new">' + ic('plus') + 'Yeni ürün</button></div>' +
    '<div class="split"><section class="card grow" style="gap:12px;padding:16px 0 4px;overflow:hidden" aria-label="Ürün listesi"><div style="padding:0 18px">' + tabs + '</div><p class="muted small" style="padding:0 18px">Satış ekranındaki buton sırası bu listeyle aynıdır; okları kullanarak değiştirin.</p><div class="tablebox"><table class="data"><thead><tr><th>Sıra</th><th>Ürün</th><th>Kategori</th><th class="r">Fiyat</th><th>KDV</th><th>Durum</th><th class="r"><span class="sr">İşlem</span></th></tr></thead><tbody>' + rows + '</tbody></table></div></section>' +
    '<div class="side">' + productForm() + categoryCard() + '</div></div>';
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
  return '<div class="page-head"><div class="ttl"><h1>Ayarlar</h1><p class="muted">Değişiklikler kendiliğinden kaydedilir.</p></div><span class="pill accent">' + ic('unlock', 18) + 'Yönetici girişi açık · ' + S.lockMinutes + ' dk işlem olmazsa kilitlenir</span></div>' +
    '<div class="split"><div class="grow">' +
      '<section class="card pad stack" aria-labelledby="fis-h"><div><h2 id="fis-h" class="sec-title">Fiş bilgileri</h2><p class="muted small">Müşteri adisyonunun üstünde ve altında görünür.</p></div>' +
        '<div class="fields">' + f('s-name', 'İşletme adı', 'name', '', 60) + f('s-slogan', 'Slogan', 'slogan', 'İsteğe bağlı') + f('s-addr1', 'Adres satırı 1', 'address1') + f('s-addr2', 'Adres satırı 2', 'address2') + f('s-phone', 'Telefon', 'phone', 'Fişte görünecek numara', 30) + f('s-insta', 'Instagram', 'instagram', '@hesap_adiniz', 50) + f('s-fb', 'Facebook', 'facebook', 'Sayfa adı', 50) + f('s-tt', 'TikTok', 'tiktok', '@hesap_adiniz', 50) + '</div>' +
        '<div class="switch-row" style="flex-wrap:wrap;cursor:default"><span class="logo-box">' + (S.hasLogo ? '<img src="/logo.png?r=' + S.logoRev + '" alt="Yüklenen logo">' : 'LOGO') + '</span><div style="flex:1 1 220px;min-width:0"><strong>' + (S.hasLogo ? 'Logo yüklendi' : 'Logo yüklenmedi') + '</strong><p class="hint">' + (S.hasLogo ? 'Fişte siyah-beyaz basılır.' : 'Logo yokken fişte işletme adı büyük harflerle basılır. PNG ya da JPG; siyah-beyaz logo en iyi sonucu verir.') + '</p></div><div class="row"><label class="btn btn-outline btn-md file-btn" for="logo-file">' + ic('upload', 20) + (S.hasLogo ? 'Değiştir' : 'Logo yükle') + '<input type="file" id="logo-file" accept="image/png,image/jpeg,image/svg+xml,image/webp"></label>' + (S.hasLogo ? '<button type="button" class="btn btn-md" data-act="logo-remove">Kaldır</button>' : '') + '</div></div>' +
        '<div class="field"><label for="s-invite">Fişin altındaki davet metni</label><textarea class="input" id="s-invite" rows="2" data-biz="invite" maxlength="300">' + esc(b.invite) + '</textarea></div>' +
        '<div class="field"><label for="s-google">Google Haritalar yorum bağlantısı</label><input class="input" id="s-google" data-biz="googleUrl" value="' + esc(b.googleUrl) + '" placeholder="https://g.page/r/.../review" autocomplete="off"><p class="hint">Google İşletme Profili’nde “Yorum iste” bölümünden kopyalayın. Fişin altındaki QR kod bu bağlantıyı açar.</p></div>' +
        '<div class="row"><button type="button" class="btn btn-md" data-act="test-preview">' + ic('eye', 20) + 'Fiş önizlemesi</button></div>' +
      '</section>' +
      '<section class="card pad stack" aria-labelledby="yazici-h"><h2 id="yazici-h" class="sec-title">Yazıcı</h2>' + printerForm(S.printer, 'set') +
        '<div class="row"><button type="button" class="btn btn-primary" data-act="test-print" data-scope="set">' + ic('printer') + 'Test fişi bas</button></div>' + testResultHTML() + '</section>' +
      '<section class="card pad stack" aria-labelledby="yedek-h"><h2 id="yedek-h" class="sec-title">Yedek</h2>' +
        '<div class="note good">' + ic('check', 22, 2.4) + '<p>Her işlem anında kaydedilir. Ayrıca açılışta, saatte bir ve kapanışta otomatik yedek alınır; son 60 gün saklanır.</p></div>' +
        (bk.lastError ? '<div class="note bad">' + ic('alert') + '<p>Son yedek alınamadı: ' + esc(bk.lastError) + '</p></div>' : '') +
        '<dl class="kv"><dt>Yedek klasörü</dt><dd>' + esc(bk.dir) + '</dd><dt>Son yedek</dt><dd>' + esc(bk.lastAt || '—') + '</dd></dl>' +
        '<div class="row"><button type="button" class="btn btn-md" data-act="backup-now">' + ic('save', 20) + 'Şimdi yedekle</button><button type="button" class="btn btn-md" data-act="open-folder" data-w="backup">' + ic('folder', 20) + 'Klasörü aç</button></div>' +
        '<div class="field"><label for="s-second">İkinci yedek yeri (USB bellek ya da ikinci disk)</label><div class="row" style="flex-wrap:nowrap"><input class="input" id="s-second" value="' + esc(bk.secondDir) + '" placeholder="Örnek: E:\\" autocomplete="off"><button type="button" class="btn btn-outline btn-md" data-act="second-save">Kaydet</button>' + (bk.secondDir ? '<button type="button" class="btn btn-md" data-act="second-clear">Kaldır</button>' : '') + '</div>' +
        '<div class="row" id="drives"><button type="button" class="btn btn-md" data-act="drives">' + ic('refresh', 20) + 'Takılı USB bellekleri göster</button></div>' +
        (bk.secondError ? '<div class="note warn">' + ic('alert') + '<p>' + esc(bk.secondError) + '</p></div>' : '') + '</div>' +
      '</section>' +
    '</div><div class="side">' +
      '<section class="card pad stack" aria-labelledby="masa-h"><h2 id="masa-h" class="sec-title">Masa ve vergi</h2><div class="field"><label for="set-tables">Masa sayısı</label><input class="input" id="set-tables" type="number" min="1" max="200" value="' + S.tables + '" style="max-width:160px"></div>' +
        '<fieldset class="reasons"><legend class="lbl-b">Varsayılan KDV oranı</legend><div class="radios">' + [1, 10, 20].map(r => '<label class="radio"><input type="radio" name="set-vat" value="' + r + '"' + (S.defaultVat === r ? ' checked' : '') + '><span>%' + r + '</span></label>').join('') + '</div><p class="hint">Ürün kartında “Varsayılan” seçili ürünler bu oranı kullanır.</p></fieldset>' +
        '<dl class="kv"><dt>Sıradaki adisyon no</dt><dd>' + S.nextNo + '</dd></dl><p class="hint">Numaralar 1’den başlar; hiçbir numara silinmez ya da tekrar kullanılmaz.</p></section>' +
      '<section class="card pad stack" aria-labelledby="gars-h"><div class="card-head"><h2 id="gars-h">Garsonlar</h2></div><p class="hint">Masa açılırken garson seçilir. Adı değiştirmek için kutuya yazın.</p><div>' + waiters + '</div><div class="row" style="flex-wrap:nowrap"><label class="sr" for="w-new">Yeni garson adı</label><input class="input" id="w-new" placeholder="Yeni garson adı" maxlength="30" autocomplete="off"><button type="button" class="btn btn-outline btn-md" data-act="w-add">' + ic('plus', 20) + 'Ekle</button></div></section>' +
      '<section class="card pad stack" aria-labelledby="sifre-h"><h2 id="sifre-h" class="sec-title">Yönetici şifresi</h2><p class="hint">Şifre istenen yerler: Ürünler ve Ayarlar. Adisyon iptali, basılmış adisyondan ürün düşmek ve basılmış adisyonu boşaltmak her seferinde ayrıca şifre ister.</p>' +
        '<div class="fields"><div class="field"><label for="pin-old">Mevcut şifre</label><input class="input" id="pin-old" type="password" inputmode="numeric" maxlength="4" autocomplete="off"></div><div class="field"><label for="pin-new">Yeni şifre (4 rakam)</label><input class="input" id="pin-new" type="password" inputmode="numeric" maxlength="4" autocomplete="off"></div><div class="field"><label for="pin-new2">Yeni şifre tekrar</label><input class="input" id="pin-new2" type="password" inputmode="numeric" maxlength="4" autocomplete="off"></div></div>' +
        '<div class="row"><button type="button" class="btn btn-outline btn-md" data-act="pin-save">Şifreyi değiştir</button><div class="field" style="flex-direction:row;align-items:center;gap:10px"><label for="set-lock">Kendiliğinden kilit</label><select class="input" id="set-lock" style="width:auto">' + [2, 5, 10, 15].map(m => '<option value="' + m + '"' + (S.lockMinutes === m ? ' selected' : '') + '>' + m + ' dk</option>').join('') + '</select></div></div></section>' +
      '<section class="card pad stack" aria-labelledby="prog-h"><h2 id="prog-h" class="sec-title">Program</h2>' +
        '<label class="switch-row" for="set-fullscreen"><span>Açılışta tam ekran</span><input class="switch" type="checkbox" id="set-fullscreen"' + (pg.fullscreen ? ' checked' : '') + '></label>' +
        (pg.autostartSupported ? '<label class="switch-row" for="set-autostart"><span>Windows açılınca programı başlat</span><input class="switch" type="checkbox" id="set-autostart"' + (pg.autostart ? ' checked' : '') + '></label>' : '') +
        '<dl class="kv"><dt>Sürüm</dt><dd>' + esc(S.version) + ' · MVP 1</dd><dt>Veri klasörü</dt><dd>' + esc(pg.dataDir) + '</dd></dl>' +
        '<div class="row"><button type="button" class="btn btn-md" data-act="open-folder" data-w="data">' + ic('folder', 20) + 'Veri klasörünü aç</button></div>' +
        '<p class="hint">Şifre unutulursa: veri klasörüne <strong>sifre_sifirla.txt</strong> adlı boş bir dosya koyup programı yeniden açın; şifre 1234 olur.</p></section>' +
    '</div></div>';
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
async function doPrint(t) {
  const a = activeFor(t); if (!a || !a.lines.length || ui.busy) return;
  ui.busy = 'print:' + a.no; render();
  const r = await act('print', { no: a.no });
  ui.busy = null; render();
  if (!r) return;
  handlePrintResult(r.data, () => doPrint(t));
}
function handlePrintResult(out, retry) {
  if (out.ok) {
    if (out.preview) showPreview(out.preview, 'Fiş önizlemesi (yazıcı seçili değil)', 'Yazıcı ayarlanınca bu fiş kâğıda basılır.');
    else toast('No ' + out.no + ' basıldı' + (out.copy ? ' (kopya)' : '') + '.', 'good');
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
function showPreview(image, title, note) {
  openModal({ title: title || 'Fiş önizlemesi', wide: false, body: (note ? '<p class="muted small">' + esc(note) + '</p>' : '') + '<div class="receipt-scroll"><img class="receipt-img" src="' + image + '" alt="Fiş önizlemesi"></div>', actions: [{ label: 'Tamam', kind: 'btn-primary', onClick: () => closeModal() }] });
}
async function doPay(t, withPrint) {
  const a = activeFor(t); if (!a || ui.busy) return;
  ui.busy = 'pay:' + a.no; render();
  const r = await act('pay', { no: a.no, print: !!withPrint });
  ui.busy = null;
  if (!r) { render(); return; }
  const out = r.data;
  if (!out.paid) { render(); handlePrintResult(out.print, () => doPay(t, true)); return; }
  if (out.print && out.print.preview) showPreview(out.print.preview, 'Fiş önizlemesi (yazıcı seçili değil)');
  toast('Masa ' + t + ' kapatıldı · ' + TL(out.total != null ? out.total : a.total), 'good');
  ui.route = 'masalar'; ui.table = null; render();
}
function payFlow(t) {
  const a = activeFor(t); if (!a || !a.lines.length) return;
  if (a.status === 'open') {
    openModal({ title: 'Adisyon basılmadı', body: '<p>Müşteriye adisyon verilmeden hesap kapatılıyor. Önce basmak ister misiniz?</p>', actions: [{ label: 'Vazgeç', onClick: () => closeModal() }, { label: 'Basmadan kapat', onClick: () => { closeModal(true); doPay(t, false); } }, { label: 'Bas ve kapat', kind: 'btn-primary', icon: 'printer', onClick: () => { closeModal(true); doPay(t, true); } }] });
  } else if (a.changed) {
    openModal({ title: 'Son değişiklik basılmadı', body: '<p>Adisyon basıldıktan sonra değişti. Müşteriye güncel halini vermek için tekrar basın.</p>', actions: [{ label: 'Vazgeç', onClick: () => closeModal() }, { label: 'Yine de kapat', onClick: () => { closeModal(true); doPay(t, false); } }, { label: 'Tekrar bas ve kapat', kind: 'btn-primary', icon: 'printer', onClick: () => { closeModal(true); doPay(t, true); } }] });
  } else doPay(t, false);
}
function cancelFlow(t) {
  const a = activeFor(t); if (!a) return;
  openModal({
    title: 'No ' + a.no + ' iptal edilsin mi?',
    body: '<p>İptal edilen adisyon silinmez; numarası “İptal” olarak kayıtta kalır. Sebebi seçince yönetici şifresi sorulur.</p><fieldset class="reasons"><legend>Sebep</legend>' + REASONS.map((r, i) => '<label class="reason"><input type="radio" name="reason" value="' + esc(r) + '"' + (i === 0 ? ' checked data-autofocus' : '') + '><span>' + esc(r) + '</span></label>').join('') + '<label class="reason"><input type="radio" name="reason" value="__other"><span>Diğer</span></label><input class="input" type="text" id="reason-other" placeholder="Diğer sebebi yazın" aria-label="Diğer sebep" maxlength="200"></fieldset>',
    actions: [{ label: 'Vazgeç', onClick: () => closeModal() }, { label: 'Adisyonu iptal et', kind: 'btn-danger', icon: 'x', onClick: async () => {
      const chosen = document.querySelector('input[name="reason"]:checked');
      const v = chosen ? chosen.value : '';
      const reason = v === '__other' ? val('reason-other').trim() : v;
      if (!reason) { toast('Bir sebep seçin ya da yazın.', 'bad'); return; }
      closeModal(true);
      const r = await act('cancel', { no: a.no, reason });
      if (!r) return;
      toast('No ' + a.no + ' iptal edildi.');
      ui.route = 'masalar'; ui.table = null; render();
    } }]
  });
}
async function saveBusiness() {
  const b = Object.assign({}, S.business);
  document.querySelectorAll('[data-biz]').forEach(el => { b[el.dataset.biz] = el.value; });
  const r = await act('saveSettings', { business: b });
  if (r) toast('Fiş bilgileri kaydedildi.');
  else render();
}
async function savePrinter(scope) {
  const cfg = readPrinterForm(scope, scope === 'wz' ? ui.setup.printer : S.printer);
  if (scope === 'wz') { ui.setup.printer = cfg; ui.testResult = null; render(); return; }
  const r = await act('saveSettings', { printer: cfg });
  ui.testResult = null;
  render();
  if (r) toast('Yazıcı ayarı kaydedildi.');
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
      if (r) { render(); toast('Logo eklendi; fişte siyah-beyaz basılır.'); }
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
  'cancel': el => cancelFlow(Number(el.dataset.t)),
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
  'p-new': () => { ui.editProduct = 'new'; render(); const f = document.getElementById('f-name'); if (f) f.focus(); },
  'p-edit': el => { ui.editProduct = Number(el.dataset.id); render(); const f = document.getElementById('f-price'); if (f) f.focus(); },
  'p-cancel': () => { ui.editProduct = null; render(); },
  'p-move': async el => { if (await act('moveProduct', { id: Number(el.dataset.id), dir: Number(el.dataset.d) })) render(); },
  'c-move': async el => { if (await act('moveCategory', { id: Number(el.dataset.id), dir: Number(el.dataset.d) })) render(); },
  'c-color': async el => { const c = catById(Number(el.dataset.id)); if (!c) return; if (await act('saveCategory', { id: c.id, name: c.name, color: el.dataset.color })) render(); },
  'c-delete': el => { const c = catById(Number(el.dataset.id)); if (!c) return; openModal({ title: c.name + ' silinsin mi?', body: '<p>Kategori boş; silinince satış ekranındaki sekmesi kalkar.</p>', actions: [{ label: 'Vazgeç', onClick: () => closeModal() }, { label: 'Sil', kind: 'btn-danger', onClick: async () => { closeModal(true); if (await act('deleteCategory', { id: c.id })) render(); } }] }); },
  'c-add': () => once('c-add', async () => { const name = val('cat-new').trim(); if (!name) { toast('Kategori adını yazın.', 'bad'); return; } const used = new Set(S.categories.map(c => c.color)); const color = (S.colors.find(c => !used.has(c.id)) || S.colors[0]).id; if (await act('saveCategory', { id: 0, name, color })) { render(); toast(name + ' eklendi.'); } }),
  // ayarlar
  'logo-remove': async () => { if (await act('setLogo', { image: '' })) render(); },
  'test-preview': async () => { const r = await act('testPreview', {}); if (r) showPreview(r.data.image, 'Fiş önizlemesi', 'Test fişi; logo, başlık, QR ve alt bilgiler kayıtlı ayarlarla çizilir.'); },
  'test-print': el => testPrint(el.dataset.scope),
  'printers-refresh': async () => { ui.printers = null; render(); await loadPrinters(); },
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
  if (el.id === 'reason-other') { const o = document.querySelector('input[name="reason"][value="__other"]'); if (o) o.checked = true; }
  touch();
});
document.addEventListener('change', async e => {
  const el = e.target;
  if (el.dataset && el.dataset.biz) { saveBusiness(); return; }
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
    case 'logo-file': readLogo(el.files && el.files[0]); el.value = ''; break;
    case 'set-tables': { const n = Number(el.value); if (await act('saveSettings', { tables: n })) toast('Masa sayısı ' + n + ' oldu.'); render(); break; }
    case 'set-lock': if (await act('saveSettings', { lockMinutes: Number(el.value) })) render(); break;
    case 'set-fullscreen': if (await act('saveSettings', { fullscreen: el.checked })) toast(el.checked ? 'Program tam ekran açılacak.' : 'Program pencere olarak açılacak.'); render(); break;
    case 'set-autostart': if (await act('setAutostart', { on: el.checked })) toast(el.checked ? 'Windows açılınca program başlayacak.' : 'Otomatik başlatma kapatıldı.'); render(); break;
  }
  if (el.name === 'set-vat') { if (await act('saveSettings', { defaultVat: Number(el.value) })) toast('Varsayılan KDV %' + el.value + ' oldu.'); render(); }
});
document.addEventListener('keydown', e => {
  if (e.key === 'F11') { e.preventDefault(); if (S && S.program.mode === 'window') act('window', { action: 'fullscreen' }); return; }
  if (!modal) {
    if (e.key === 'Enter' && S && !S.setupDone && e.target && e.target.dataset && e.target.dataset.wz) { e.preventDefault(); setupNext(); }
    return;
  }
  if (e.key === 'Escape') { e.preventDefault(); closeModal(); return; }
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
window.addEventListener('resize', setLayoutVars);

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
