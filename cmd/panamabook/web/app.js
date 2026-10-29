'use strict';
/* Panama Book: interfaccia. Nessuna libreria. Tutto il testo che arriva dal server (dati personali compresi)
   entra nella pagina solo con textContent: mai innerHTML. */
(() => {
  const SVG = 'http://www.w3.org/2000/svg';
  const $ = (id) => document.getElementById(id);

  function h(tag, props, ...kids) {
    const el = document.createElement(tag);
    for (const [k, v] of Object.entries(props || {})) {
      if (v === null || v === undefined || v === false) continue;
      if (k === 'class') el.className = v;
      else if (k === 'dataset') Object.assign(el.dataset, v);
      else if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
      else el.setAttribute(k, v === true ? '' : v);
    }
    for (const kid of kids.flat()) {
      if (kid === null || kid === undefined || kid === false) continue;
      el.append(kid.nodeType ? kid : document.createTextNode(String(kid)));
    }
    return el;
  }
  function icon(name) {
    const s = document.createElementNS(SVG, 'svg');
    s.setAttribute('class', 'ic'); s.setAttribute('aria-hidden', 'true');
    const u = document.createElementNS(SVG, 'use'); u.setAttribute('href', '#i-' + name);
    s.append(u); return s;
  }

  // ---- token e API --------------------------------------------------------------------------------------------
  let TOKEN = '';
  try {
    const p = new URLSearchParams(location.search);
    TOKEN = p.get('t') || '';
    if (TOKEN) { sessionStorage.setItem('pbt', TOKEN); history.replaceState(null, '', location.pathname); }
    else TOKEN = sessionStorage.getItem('pbt') || '';
  } catch (_) { /* senza storage il token resta nell'URL */ }

  async function api(path, opts = {}) {
    const headers = { 'X-PB-Token': TOKEN };
    if (opts.body !== undefined) headers['Content-Type'] = 'application/json';
    const r = await fetch(path, { method: opts.method || 'GET', headers, body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined });
    let data = null;
    try { data = await r.json(); } catch (_) { /* corpo non JSON */ }
    if (!r.ok) {
      const e = new Error((data && data.error) || ('Errore ' + r.status));
      e.status = r.status; e.code = data && data.code; e.data = data;
      throw e;
    }
    return data;
  }

  // ---- stato ---------------------------------------------------------------------------------------------------
  const S = { doc: null, cur: null, previews: [], edits: {}, rev: {}, scanning: false, busy: false, demo: false, defaults: { driver: 'wia', dpi: 300, len_mm: 190, top_margin_mm: -1 }, info: null, view: 'full',
              diagOff: new Set(), prefs: {}, usePage: false, zoom: 1, anchor: null, cropping: false, crop: null, bust: '', rot: false, scans: [], pdfBusy: false, saving: false };
  const ZOOMS = [1, 1.5, 2, 3, 4, 5]; // massimo 5x
  const ZMAX = ZOOMS[ZOOMS.length - 1];

  const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9._-]{0,59}$/;
  const pad = (n) => String(n).padStart(2, '0');
  function suggestedName() {
    const d = new Date();
    return 'libretto-' + d.getFullYear() + pad(d.getMonth() + 1) + pad(d.getDate()) + '-' + pad(d.getHours()) + pad(d.getMinutes()) + pad(d.getSeconds());
  }
  function fmtTime(iso) {
    const d = new Date(iso); if (isNaN(d)) return '';
    return pad(d.getDate()) + '/' + pad(d.getMonth() + 1) + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
  }

  // ---- messaggi ------------------------------------------------------------------------------------------------
  function notify(kind, text, opts = {}) {
    const box = $('banners');
    const el = h('div', { class: 'banner ' + kind, role: kind === 'error' ? 'alert' : null },
      icon(kind === 'ok' ? 'check' : kind === 'info' ? 'info' : 'warn'),
      h('p', {}, text),
      h('button', { type: 'button', class: 'btn icon ghost', 'aria-label': 'Chiudi il messaggio', onclick: () => el.remove() }, icon('x')));
    box.append(el);
    while (box.children.length > 4) box.firstChild.remove();
    if ((kind === 'ok' || kind === 'info') && !opts.sticky) setTimeout(() => el.remove(), kind === 'ok' ? 9000 : 7000);
    return el;
  }
  const clearBanners = () => $('banners').replaceChildren();

  // ---- finestra di conferma ---------------------------------------------------------------------------------
  // askChoice mostra la finestra e restituisce 'yes', 'extra' o 'no' (Esc = 'no'). Il pulsante predefinito è sempre quello prudente.
  function askChoice({ title, body, yes = 'Continua', no = 'Annulla', extra = '' }) {
    const dlg = $('dlgAsk');
    $('askTitle').textContent = title;
    $('askBody').replaceChildren(...(Array.isArray(body) ? body : [body]).map((b) => (typeof b === 'string' ? h('p', {}, b) : b)));
    $('askYes').textContent = yes; $('askNo').textContent = no;
    $('askExtra').hidden = !extra; $('askExtra').textContent = extra;
    dlg.returnValue = '';
    return new Promise((resolve) => {
      dlg.addEventListener('close', () => resolve(dlg.returnValue === 'yes' || dlg.returnValue === 'extra' ? dlg.returnValue : 'no'), { once: true });
      dlg.showModal();
      $('askNo').focus();
    });
  }
  async function ask(opts) { return (await askChoice(opts)) === 'yes'; }
  async function confirmDiscard() {
    if (!dirtyCount()) return true;
    return ask({ title: 'Modifiche non salvate', body: 'Hai ' + dirtyText() + ' senza salvare. Se continui vanno perse.', yes: 'Continua senza salvare', no: 'Resta qui' });
  }

  // ---- impostazioni ---------------------------------------------------------------------------------------------
  function applySettings(s) {
    $('setDriver').value = s.driver; $('setDpi').value = s.dpi; $('setLen').value = s.len_mm; $('setTop').value = s.top_margin_mm;
  }
  // Le impostazioni stanno nel file dati\\impostazioni.json (lingua, scansione, copia): arrivano con /api/state e si salvano con /api/prefs.
  // Chi aveva le vecchie impostazioni nel browser (localStorage) le ritrova una volta sola.
  let prefsTimer = null; let prefsPatch = {};
  function savePrefs(patch) {
    Object.assign(S.prefs, patch); Object.assign(prefsPatch, patch);
    clearTimeout(prefsTimer);
    prefsTimer = setTimeout(() => {
      const body = prefsPatch; prefsPatch = {};
      api('/api/prefs', { method: 'POST', body }).catch(() => {});
    }, 300);
  }
  const legacy = (key) => { try { return JSON.parse(localStorage.getItem(key) || 'null'); } catch (_) { return null; } };
  function loadSettings() {
    let s = S.prefs.scan;
    if (!s) { s = legacy('pbset4'); if (s) savePrefs({ scan: s }); }
    applySettings(Object.assign({}, S.defaults, s || {}));
  }
  function saveSettings() {
    savePrefs({ scan: { driver: $('setDriver').value, dpi: +$('setDpi').value, len_mm: +$('setLen').value, top_margin_mm: +$('setTop').value } });
  }
  // cartella di salvataggio: si cambia con /api/outdir (il programma verifica e crea la cartella); non sta in /api/prefs
  function renderOut(o) {
    if (!o) return;
    S.out = o; $('outPath').value = o.path;
    const box = $('outOneDrive'); box.replaceChildren();
    for (const d of o.onedrive || []) {
      const dir = d + '\\Panama Book';
      box.append(h('button', { type: 'button', class: 'btn small', title: dir, onclick: () => applyOut(dir) }, 'Usa OneDrive'));
    }
    box.hidden = !(o.onedrive || []).length;
  }
  async function applyOut(path) {
    if (path === undefined) path = $('outPath').value;
    try {
      const r = await api('/api/outdir', { method: 'POST', body: { path } });
      renderOut(r.outdir); refreshScans();
      notify('ok', 'Cartella di salvataggio: ' + r.outdir.path);
    } catch (e) { notify('error', e.message); if (S.out) $('outPath').value = S.out.path; }
  }
  async function pickOut() {
    try {
      const r = await api('/api/outdir/pick', { method: 'POST', body: {} });
      if (r.path) applyOut(r.path);
    } catch (e) { notify('error', e.message); }
  }
  // lingua dell'interfaccia: inglese se non scelta; la pagina si traduce da sola (i18n.js)
  function loadLang() {
    const l = S.prefs.lang || window.PBI18n.lang;
    $('setLang').value = l; window.PBI18n.setLang(l);
  }
  function readSettings() {
    const s = { driver: $('setDriver').value, dpi: Math.round(+$('setDpi').value), len_mm: +$('setLen').value, top_margin_mm: +$('setTop').value };
    if (!(s.dpi >= 75 && s.dpi <= 600)) return 'La risoluzione deve essere tra 75 e 600 dpi.';
    if (!(s.len_mm >= 30 && s.len_mm <= 356)) return 'La lunghezza letta deve essere tra 30 e 356 mm.';
    if (!(s.top_margin_mm >= -1 && s.top_margin_mm <= 20)) return 'Il margine sopra il titolo deve essere tra -1 e 20 mm.';
    return s;
  }

  // ---- elenco immagini e cronologia -------------------------------------------------------------------------------
  async function refreshScans() {
    try { S.scans = (await api('/api/scans')).items || []; } catch (_) { return; }
    const sel = $('openSelect'); const keep = sel.value;
    sel.replaceChildren(h('option', { value: '' }, S.scans.length ? '- scegli un\'immagine -' : '(nessuna immagine in scans)'),
      ...S.scans.map((it) => h('option', { value: it.file }, it.file + (it.saved ? '  (salvato)' : ''))));
    if (S.scans.some((it) => it.file === keep)) sel.value = keep;
  }

  // ---- anteprima ---------------------------------------------------------------------------------------------------
  // larghezza a 1x: l'immagine intera sta nella finestra sia in larghezza sia in altezza
  function fitWidth() {
    const v = $('viewer'); const img = $('preview');
    const cw = v.offsetWidth - 2; const ch = v.offsetHeight - 2; // con il bordo, senza le barre di scorrimento: la misura non dipende dallo zoom
    if (!img.naturalWidth || !cw || !ch) return cw || 300;
    return Math.max(120, Math.floor(Math.min(cw, ch * img.naturalWidth / img.naturalHeight)));
  }
  function applyView() {
    const img = $('preview');
    $('imgWrap').style.width = fitWidth() * S.zoom + 'px'; // 1x = tutta l'immagine visibile nella finestra, senza scorrere
    img.style.transform = S.rot ? 'rotate(180deg)' : '';
    $('viewer').classList.toggle('zoomed', S.zoom > 1);
    $('zRot').setAttribute('aria-pressed', S.rot ? 'true' : 'false');
  }
  // vista iniziale di un'immagine: intera, non ruotata, nessun punto di zoom
  function resetView() { S.zoom = 1; S.rot = false; S.anchor = null; S.usePage = false; applyView(); }
  // Cambia lo zoom tenendo fermo un punto dell'immagine (frazioni fx, fy del riquadro) alla posizione (ax, ay) dello schermo.
  function setZoom(z, fx, fy, ax, ay) {
    const v = $('viewer'); const wrap = $('imgWrap');
    S.zoom = Math.min(ZOOMS[ZOOMS.length - 1], Math.max(1, z));
    applyView();
    const r2 = wrap.getBoundingClientRect(); // il layout si aggiorna da solo alla lettura
    v.scrollLeft += (r2.left + fx * r2.width) - ax;
    v.scrollTop += (r2.top + fy * r2.height) - ay;
  }
  function viewerPoint() { const vr = $('viewer').getBoundingClientRect(); return [vr.left + vr.width / 2, vr.top + vr.height / 2]; }
  // pulsanti + e -: verso l'ultimo punto del doppio clic (se visibile), altrimenti verso il centro della finestra
  function zoomBy(dir) {
    const eps = 0.01; // dopo la rotellina lo zoom puo' non essere uno dei passi
    const z = dir > 0 ? ZOOMS.find((q) => q > S.zoom + eps) : [...ZOOMS].reverse().find((q) => q < S.zoom - eps);
    if (!z) return;
    const r = $('imgWrap').getBoundingClientRect(); const vr = $('viewer').getBoundingClientRect();
    let [ax, ay] = viewerPoint(); let fx = (ax - r.left) / r.width; let fy = (ay - r.top) / r.height;
    if (S.anchor) {
      const px = r.left + S.anchor.fx * r.width; const py = r.top + S.anchor.fy * r.height;
      fx = S.anchor.fx; fy = S.anchor.fy;
      if (px >= vr.left && px <= vr.right && py >= vr.top && py <= vr.bottom) { ax = px; ay = py; }
    }
    setZoom(z, Math.min(1, Math.max(0, fx)), Math.min(1, Math.max(0, fy)), ax, ay);
  }
  // doppio clic: ingrandisce verso il punto cliccato (1x -> 2x -> 3x), poi torna a 1x
  function zoomAt(ev) {
    if (!$('preview').src || $('preview').hidden) return; // vale anche sopra i punti del rettangolo: si ingrandisce proprio sull'angolo
    const r = $('imgWrap').getBoundingClientRect();
    const fx = Math.min(1, Math.max(0, (ev.clientX - r.left) / r.width)); const fy = Math.min(1, Math.max(0, (ev.clientY - r.top) / r.height));
    S.anchor = { fx, fy };
    const z = S.zoom < 1.99 ? 2 : S.zoom < 2.99 ? 3 : S.zoom < ZMAX - 0.01 ? ZMAX : 1; // 1x -> 2x -> 3x -> 5x -> 1x
    setZoom(z, fx, fy, ev.clientX, ev.clientY);
    if (z === 1) S.anchor = null;
  }
  // rotellina: zoom continuo (1x-5x) verso il puntatore. A 1x la rotellina "indietro" lascia scorrere la pagina.
  function zoomWheel(ev) {
    if (!$('preview').src || $('preview').hidden) return;
    if (ev.deltaY > 0 && S.zoom <= 1) return;
    ev.preventDefault();
    const r = $('imgWrap').getBoundingClientRect();
    const fx = Math.min(1, Math.max(0, (ev.clientX - r.left) / r.width)); const fy = Math.min(1, Math.max(0, (ev.clientY - r.top) / r.height));
    const z = Math.min(ZMAX, Math.max(1, S.zoom * Math.pow(1.0015, -ev.deltaY)));
    if (Math.abs(z - S.zoom) < 0.001) return;
    S.anchor = z > 1 ? { fx, fy } : null;
    setZoom(z, fx, fy, ev.clientX, ev.clientY);
  }
  // immagine mostrata nel riquadro: S.cur = { dir: 'preview' | 'scans', file }
  function setImg(url, key) {
    const img = $('preview');
    img.src = url + '&t=' + encodeURIComponent(TOKEN) + '&v=' + encodeURIComponent(key);
    img.hidden = false; $('viewerEmpty').hidden = true;
  }
  function captionFor(doc) {
    return doc.file + (doc.unsaved ? ' - anteprima NON salvata' : '') + ' - letta alle ' + fmtTime(doc.read_at).slice(6);
  }
  const VIEW_LABEL = { page: 'solo la pagina dati', full: 'pagina intera (passaporto con informativa)', raw: 'scansione GREZZA (come esce dallo scanner)' };
  const saveWhich = () => (S.usePage && S.info && S.info.page_url ? 'solo la pagina dati' : 'la pagina intera');
  // anteprima (solo dati dell'anteprima): per i documenti letti si cerca nella striscia
  function infoFor(file) { return S.previews.find((q) => q.file === file) || null; }
  // sceglie quale immagine mostrare: 'page' | 'full' | 'raw' (solo per le anteprime)
  function setView(v) {
    const i = S.info;
    if (!i || !S.cur || S.cur.dir !== 'preview') { $('viewSwitch').hidden = true; S.view = 'full'; return; }
    if (v === 'page' && !i.page_url) v = 'full';
    if (v === 'raw' && !i.raw_url) v = 'full';
    S.view = v;
    const url = v === 'page' ? i.page_url : v === 'raw' ? i.raw_url : (i.url || i.image_url);
    setImg(url, i.file + '-' + v + '-' + (i.time || '') + (i.page_url ? 'p' : '') + S.bust);
    $('viewSwitch').hidden = false;
    $('vPage').hidden = !i.page_url; $('vRaw').hidden = !i.raw_url;
    for (const [id, name] of [['vPage', 'page'], ['vFull', 'full'], ['vRaw', 'raw']]) $(id).setAttribute('aria-pressed', v === name ? 'true' : 'false');
    const base = $('caption').dataset.base || '';
    $('caption').textContent = base + ' - vista: ' + VIEW_LABEL[v] + ' - verrà salvata: ' + saveWhich();
  }
  function setCaption(text) { $('caption').dataset.base = text; $('caption').textContent = text; }
  // diagnosi della posizione del libretto, dalla scansione grezza (senza OCR): solo suggerimenti
  function renderDiag() {
    const box = $('diagBox'); const i = S.info;
    box.replaceChildren();
    if (!i || !i.diag || !i.diag.done || S.diagOff.has(i.file)) { box.hidden = true; return; }
    box.hidden = false;
    const hints = i.diag.hints || [];
    box.append(h('button', { type: 'button', class: 'btn icon ghost diag-x', title: 'Nascondi questo avviso', 'aria-label': 'Nascondi l\'avviso sulla posizione del libretto',
      onclick: () => { S.diagOff.add(i.file); renderDiag(); } }, icon('x')));
    box.append(h('p', { class: 'diag-title' }, icon(hints.length ? 'warn' : 'check'), hints.length ? 'Come è stato inserito il libretto' : 'Posizione del libretto: nessun problema trovato'));
    if (hints.length) box.append(h('ul', {}, ...hints.map((t) => h('li', {}, t))));
    box.append(h('p', { class: 'diag-sum' }, i.diag.summary));
  }
  // caselle "conserva anche ..." (solo per le anteprime non salvate)
  function renderKeep() {
    const i = S.info; const on = !!S.doc && !!S.doc.unsaved && !!i;
    $('keepBox').hidden = !on;
    if (!on) return;
    $('keepFullWrap').hidden = !i.page_url; $('keepRawWrap').hidden = !i.raw_url;
  }
  // dopo "Conferma e salva" l'immagine ha un altro nome (e passa da dati\\anteprima a scans): si aggiorna senza cambiare zoom e rotazione
  function refreshImage(doc) {
    S.cur = { dir: doc.dir, file: doc.file }; S.info = null; S.view = 'full';
    setImg(doc.image_url, doc.doc_id + '-' + doc.file);
    setCaption(captionFor(doc)); $('viewSwitch').hidden = true;
    renderDiag(); renderKeep(); updateActions(); renderStrip();
  }
  function showImage(doc) {
    S.cur = { dir: doc.dir, file: doc.file };
    setCaption(captionFor(doc));
    if (doc.dir === 'preview') {
      const q = infoFor(doc.file) || {};
      S.info = Object.assign({}, q, { file: doc.file, url: doc.image_url, page_url: doc.page_url || null });
      resetView();
      setView('full'); // si parte sempre dalla pagina intera: la pagina dati la sceglie l'operatore
    } else {
      S.info = null; $('viewSwitch').hidden = true;
      setImg(doc.image_url, doc.doc_id + S.bust); resetView();
    }
    renderDiag(); renderKeep(); updateActions(); renderStrip();
  }
  // anteprima senza dati letti
  function showFile(p) {
    S.cur = { dir: 'preview', file: p.file };
    S.info = p;
    setCaption(p.file + ' - anteprima NON salvata (' + p.width + ' x ' + p.height + ' px)');
    resetView();
    setView('full'); // si parte sempre dalla pagina intera: la pagina dati la sceglie l'operatore
    renderDiag(); renderKeep(); updateActions(); renderStrip();
  }
  function clearViewer() {
    if (S.cropping) exitCrop();
    S.cur = null; S.info = null; $('viewSwitch').hidden = true; $('diagBox').hidden = true; $('keepBox').hidden = true;
    const img = $('preview'); img.removeAttribute('src'); img.hidden = true; $('viewerEmpty').hidden = false; setCaption('');
    resetView(); updateActions(); renderStrip();
  }
  // dati letti: via, resta l'immagine
  function clearDoc() {
    S.doc = null; S.edits = {}; S.rev = {};
    $('results').hidden = true; $('savebar').hidden = true; $('emptyState').hidden = false;
    S.nameInfo = null; $('nameLine').hidden = true; $('fieldsBody').replaceChildren(); $('warns').replaceChildren(); $('mrzLines').textContent = '';
    const prev = S.cur && S.cur.dir === 'preview';
    $('emptyTitle').textContent = prev ? 'Anteprima non ancora letta' : 'Nessun libretto letto';
    $('emptyText').textContent = prev
      ? 'Guarda l\'immagine a sinistra: si vede la pagina dati con le due righe MRZ in fondo? Allora premi «Leggi i dati». Se no, «Riscansiona» (o «Scarta»). Finché non confermi non viene creato nessun file in scans.'
      : 'Premi «Anteprima» con il libretto inserito: vedrai l\'immagine prima di leggerla e salvarla. Oppure apri un\'immagine già salvata.';
    updateCounter(); updateActions();
  }

  function updateActions() {
    const prev = !!S.cur && S.cur.dir === 'preview';
    $('prevActions').hidden = !prev;
    $('cropActions').hidden = !S.cur;
    $('btnRead').disabled = S.busy || !prev || S.cropping;
    $('btnRescan').disabled = S.busy || S.cropping;
    $('btnDiscard').disabled = S.busy || !prev || S.cropping;
    $('btnCropAuto').disabled = S.busy || !S.cur || S.cropping;
    $('btnCropManual').disabled = S.busy || !S.cur || S.cropping;
    $('btnReread').disabled = S.busy || !S.cur || prev; // per le anteprime c'è "Leggi i dati"
    $('btnPdf').disabled = S.busy || S.pdfBusy || !S.cur;
  }

  // ---- taglio manuale ------------------------------------------------------------------------------------------
  // Si disegna sull'immagine di partenza: la scansione grezza (foglio intero) se c'e', altrimenti l'anteprima. Il rettangolo e'
  // in frazioni (0-1) dell'immagine, quindi non dipende dallo zoom; al "Applica" diventa pixel dell'immagine originale.
  const MIN_FRAC = 0.05;
  const origName = (f) => f.replace(/(\.[^.]+)$/, ' (originale)$1'); // come scanOriginal() nel programma
  // ---- quattro angoli liberi (predefinito): quad = [[x,y] x4] in frazioni, ordine alto-sx, alto-dx, basso-dx, basso-sx
  const quadOf = (c) => [[c.x, c.y], [c.x + c.w, c.y], [c.x + c.w, c.y + c.h], [c.x, c.y + c.h]];
  function quadBox(c) {
    const xs = c.quad.map((p) => p[0]); const ys = c.quad.map((p) => p[1]);
    c.x = Math.min(...xs); c.y = Math.min(...ys); c.w = Math.max(...xs) - c.x; c.h = Math.max(...ys) - c.y;
  }
  function drawQuad() {
    const c = S.crop; quadBox(c);
    const P = c.quad.map((p) => [p[0] * 100, p[1] * 100]);
    $('cqPoly').setAttribute('points', P.map((p) => p.join(',')).join(' '));
    $('cqMask').setAttribute('d', 'M0 0H100V100H0Z M' + P.map((p) => p.join(' ')).join('L') + 'Z');
    for (let i = 0; i < 4; i++) { const el = $('cq' + i); el.style.left = P[i][0] + '%'; el.style.top = P[i][1] + '%'; }
    const d = (a, b) => Math.hypot((a[0] - b[0]) * c.nw, (a[1] - b[1]) * c.nh);
    const q = c.quad;
    $('cropInfo').textContent = 'Area: ' + Math.round((d(q[0], q[1]) + d(q[3], q[2])) / 2) + ' x ' + Math.round((d(q[0], q[3]) + d(q[1], q[2])) / 2) + ' pixel, quattro angoli liberi (raddrizzata). Tira gli angoli, trascina dentro per spostare tutto, fuori per disegnarne uno nuovo. Doppio clic o rotellina = zoom sul punto; con un angolo in mano compare la lente 4x.';
  }
  // modalita' del taglio: quattro angoli liberi (predefinita) oppure rettangolo dritto (anche con proporzione 3:4)
  function applyCropMode() {
    const c = S.crop; if (!c) return;
    const quad = !$('cropRect').checked && !$('cropLock').checked;
    if (quad && !c.quad) c.quad = quadOf(c);
    if (!quad && c.quad) { quadBox(c); c.quad = null; clampCrop(); }
    $('cropQuad').toggleAttribute('hidden', !quad); for (let i = 0; i < 4; i++) $('cq' + i).hidden = !quad; // l'svg non ha la proprieta' hidden
    $('cropBox').hidden = quad;
    drawCrop();
  }
  function drawCrop() {
    const c = S.crop;
    if (c.quad) { drawQuad(); return; }
    const b = $('cropBox');
    b.style.left = (c.x * 100) + '%'; b.style.top = (c.y * 100) + '%'; b.style.width = (c.w * 100) + '%'; b.style.height = (c.h * 100) + '%';
    const px = (f, n) => Math.round(f * n);
    $('cropInfo').textContent = 'Area: ' + px(c.w, c.nw) + ' x ' + px(c.h, c.nh) + ' pixel' + (c.lock ? ' (3:4)' : '') + '. Trascina fuori dal rettangolo per disegnarne uno nuovo, dentro per spostarlo, i punti per ridimensionarlo. Doppio clic o rotellina = zoom sul punto; trascinando un punto compare la lente 4x.';
    $('cropBox').classList.toggle('locked', !!c.lock);
  }
  // proporzione 3:4 (larghezza:altezza in pixel): in frazioni, h = w * ratioH
  const RATIO = 0.75;
  function clampCrop() {
    const c = S.crop;
    if (c.lock) { c.w = Math.min(c.w, 1, 1 / c.ratioH); c.h = c.w * c.ratioH; }
    c.w = Math.min(1, Math.max(MIN_FRAC, c.w)); c.h = Math.min(1, Math.max(MIN_FRAC, c.h));
    c.x = Math.min(1 - c.w, Math.max(0, c.x)); c.y = Math.min(1 - c.h, Math.max(0, c.y));
  }
  async function startCrop() {
    if (S.busy || S.cropping || !S.cur) return;
    const saved = S.cur.dir !== 'preview';
    if (saved && !S.doc) { notify('warn', 'Apri prima l\'immagine (premi «Apri»).'); return; }
    const img = $('preview');
    let source = 'full';
    if (saved) { // immagine salvata: si disegna sull'originale, se c'e', altrimenti su quella mostrata
      S.prevSrc = img.src;
      source = 'full';
    } else {
      source = S.info && S.info.raw_url ? 'raw' : 'full';
      S.prevView = S.view;
      setView(source); // il foglio intero (grezza) o l'anteprima intera
      $('viewSwitch').hidden = true;
    }
    try { await img.decode(); } catch (_) { /* si usa naturalWidth se c'e' */ }
    if (!img.naturalWidth) { notify('error', 'Immagine non caricata: riprova.'); setView(S.prevView || 'full'); return; }
    S.rot = false; applyView();
    S.cropping = true;
    S.crop = { x: 0.04, y: 0.04, w: 0.92, h: 0.92, nw: img.naturalWidth, nh: img.naturalHeight, source, lock: $('cropLock').checked };
    S.crop.ratioH = S.crop.nw / (RATIO * S.crop.nh);
    await initCropBox(saved); // parte dal passaporto trovato, non dal foglio intero
    $('previewCard').classList.add('cropmode'); $('viewer').classList.add('cropping'); $('cropBar').hidden = false; $('zRot').disabled = true;
    S.crop.lock = $('cropLock').checked; S.crop.quad = null; applyCropMode(); updateActions(); $('cropBox').focus({ preventScroll: true });
  }
  // rettangolo iniziale: il documento trovato dal programma (come nel taglio automatico) piu' un po' di bordo, in 3:4
  async function initCropBox(saved) {
    const c = S.crop;
    let bx = null;
    try { bx = await api('/api/preview/cropbox?file=' + encodeURIComponent(S.cur.file) + '&source=' + c.source + (saved ? '&dir=scans' : '')); } catch (_) { /* si parte dal foglio intero */ }
    if (saved && bx && bx.image && bx.image !== S.cur.file) { // c'e' l'originale: lo si mostra, le misure sono le sue
      const img = $('preview');
      setImg('/api/image?file=' + encodeURIComponent(bx.image), 'orig-' + bx.image + S.bust);
      try { await img.decode(); } catch (_) { /* ignora */ }
      c.nw = img.naturalWidth; c.nh = img.naturalHeight; c.ratioH = c.nw / (RATIO * c.nh);
    }
    c.saved = !!saved;
    if (!bx || !bx.ok) { if (c.lock) { clampCrop(); } return; }
    const bw = bx.w * c.nw; const bh = bx.h * c.nh;
    let w = Math.max(bw * 1.12, bh * 1.06 * RATIO); // contiene il documento con circa 7 mm di bordo ai lati
    let h = w / RATIO;
    if (!c.lock) { w = bw * 1.04; h = bh * 1.04; }
    if (w > c.nw) { h *= c.nw / w; w = c.nw; }
    if (h > c.nh) { w *= c.nh / h; h = c.nh; }
    c.w = w / c.nw; c.h = h / c.nh;
    c.x = (bx.x + bx.w / 2) - c.w / 2; c.y = (bx.y + bx.h / 2) - c.h / 2;
    clampCrop();
  }
  function exitCrop() {
    S.cropping = false; S.crop = null;
    $('previewCard').classList.remove('cropmode'); $('viewer').classList.remove('cropping'); $('cropBox').hidden = true; $('cropQuad').setAttribute('hidden', ''); for (let i = 0; i < 4; i++) $('cq' + i).hidden = true; $('cropBar').hidden = true; $('zRot').disabled = false;
    updateActions();
  }
  function cancelCrop() {
    const saved = S.crop && S.crop.saved;
    exitCrop();
    if (saved) { if (S.prevSrc) $('preview').src = S.prevSrc; } else setView(S.prevView || 'full');
  }
  async function applyCrop() {
    const c = S.crop; if (!c || S.busy) return;
    const body = { file: S.cur.file, dir: c.saved ? 'scans' : 'preview', doc_id: S.doc ? S.doc.doc_id : '', mode: 'manual', source: c.source, x: Math.round(c.x * c.nw), y: Math.round(c.y * c.nh), w: Math.round(c.w * c.nw), h: Math.round(c.h * c.nh) };
    body.w = Math.min(body.w, c.nw - body.x); body.h = Math.min(body.h, c.nh - body.y);
    if (c.quad) body.quad = c.quad.map((p) => [Math.min(c.nw, Math.max(0, p[0] * c.nw)), Math.min(c.nh, Math.max(0, p[1] * c.nh))]);
    $('btnCropApply').disabled = true;
    try {
      if (c.saved && !(await ask({ title: 'Sostituire l\'immagine salvata?', body: ['Il taglio sostituisce «' + S.cur.file + '» in scans.', 'L\'originale resta salvato come «' + origName(S.cur.file) + '», così puoi rifare il taglio quando vuoi. JSON e PDF già salvati non cambiano.'], yes: 'Sì, taglia', no: 'No, annulla' }))) { $('btnCropApply').disabled = false; return; }
      const r = await api('/api/preview/crop', { method: 'POST', body });
      exitCrop();
      S.bust = '-' + Date.now();
      if (c.saved && r.file && r.file !== S.cur.file) { // era aperta la copia originale: si apre l'immagine principale appena tagliata
        notify('info', 'Taglio manuale applicato: il risultato è in «' + r.file + '»; l\'originale non è cambiato.');
        window.scrollTo({ top: 0, behavior: 'smooth' });
        S.bust = '-' + Date.now(); startJob('/api/open', { file: r.file, dir: 'scans' });
        return;
      }
      if (c.saved) { // immagine salvata: si ricarica il documento (stessa lettura, immagine nuova)
        const doc = await api('/api/doc/' + S.doc.doc_id);
        showImage(doc); setDoc(doc, false); refreshScans();
        notify('info', 'Taglio manuale applicato. L\'originale è in «' + r.original + '».');
        return;
      }
      S.info = Object.assign({}, S.info || {}, r.preview, { page_url: r.preview.page_url || null });
      if (S.info.width) setCaption(S.info.file + ' - anteprima NON salvata (' + S.info.width + ' x ' + S.info.height + ' px)');
      S.usePage = false; setView('full'); await refreshPreviews();
      notify('info', 'Taglio manuale applicato: è la nuova pagina intera. Premi «Leggi i dati» per rifare anche la pagina dati.');
    } catch (e) { notify('error', e.message); } finally { $('btnCropApply').disabled = false; }
  }
  // lente: mentre si trascina un punto (o si disegna) mostra 4x l'angolo preciso, accanto al puntatore
  const LOUPE = 200; const LOUPE_Z = 4;
  function showLoupe(fx, fy, cx, cy) {
    const L = $('loupe'); const r = $('imgWrap').getBoundingClientRect();
    const bw = r.width * LOUPE_Z; const bh = r.height * LOUPE_Z;
    L.style.backgroundImage = 'url("' + $('preview').src + '")';
    L.style.backgroundSize = bw + 'px ' + bh + 'px';
    L.style.backgroundPosition = (LOUPE / 2 - fx * bw) + 'px ' + (LOUPE / 2 - fy * bh) + 'px';
    let x = cx + 36; let y = cy - LOUPE - 36;
    if (x + LOUPE > window.innerWidth - 8) x = cx - LOUPE - 36;
    if (y < 8) y = cy + 36;
    L.style.left = x + 'px'; L.style.top = y + 'px'; L.hidden = false;
  }
  const hideLoupe = () => { $('loupe').hidden = true; };
  // trascinamento: corpo = sposta, punti = ridimensiona
  function onCropDown(ev) {
    if (!S.cropping || ev.button > 0) return;
    const handle = ev.target.dataset && ev.target.dataset.h ? ev.target.dataset.h : '';
    const r = $('imgWrap').getBoundingClientRect();
    const start = { x: ev.clientX, y: ev.clientY, c: Object.assign({}, S.crop) };
    ev.preventDefault();
    $('cropBox').setPointerCapture(ev.pointerId);
    const loupe = (e) => {
      if (!handle) return;
      const c = S.crop;
      showLoupe(handle.includes('w') ? c.x : handle.includes('e') ? c.x + c.w : c.x + c.w / 2, handle.includes('n') ? c.y : handle.includes('s') ? c.y + c.h : c.y + c.h / 2, e.clientX, e.clientY);
    };
    loupe(ev);
    const move = (e) => { moveInner(e); loupe(e); };
    const moveInner = (e) => {
      const dx = (e.clientX - start.x) / r.width; const dy = (e.clientY - start.y) / r.height;
      const s = start.c; const c = S.crop;
      if (!handle) { c.x = s.x + dx; c.y = s.y + dy; clampCrop(); drawCrop(); return; }
      if (c.lock) {
        if (handle.length !== 2) return; // con 3:4 si ridimensiona dagli angoli
        const sx = handle.includes('e') ? 1 : -1; const sy = handle.includes('s') ? 1 : -1;
        const ax = sx > 0 ? s.x : s.x + s.w; const ay = sy > 0 ? s.y : s.y + s.h; // angolo opposto, fermo
        const maxW = Math.min(sx > 0 ? 1 - ax : ax, (sy > 0 ? 1 - ay : ay) / c.ratioH);
        const w = Math.max(MIN_FRAC, Math.min(maxW, s.w + sx * dx));
        c.w = w; c.h = w * c.ratioH; c.x = sx > 0 ? ax : ax - w; c.y = sy > 0 ? ay : ay - c.h;
        drawCrop(); return;
      }
      let x0 = s.x; let y0 = s.y; let x1 = s.x + s.w; let y1 = s.y + s.h;
      if (handle.includes('w')) x0 = Math.min(x1 - MIN_FRAC, Math.max(0, s.x + dx));
      if (handle.includes('e')) x1 = Math.max(x0 + MIN_FRAC, Math.min(1, s.x + s.w + dx));
      if (handle.includes('n')) y0 = Math.min(y1 - MIN_FRAC, Math.max(0, s.y + dy));
      if (handle.includes('s')) y1 = Math.max(y0 + MIN_FRAC, Math.min(1, s.y + s.h + dy));
      c.x = x0; c.y = y0; c.w = x1 - x0; c.h = y1 - y0; drawCrop();
    };
    const up = () => { hideLoupe(); $('cropBox').removeEventListener('pointermove', move); $('cropBox').removeEventListener('pointerup', up); $('cropBox').removeEventListener('pointercancel', up); };
    $('cropBox').addEventListener('pointermove', move); $('cropBox').addEventListener('pointerup', up); $('cropBox').addEventListener('pointercancel', up);
  }
  // trascinamento fuori dal rettangolo: ne disegna uno nuovo (libero, o 3:4 se bloccato)
  function onCropDraw(ev) {
    if (!S.cropping || ev.button > 0 || ev.target.closest('#cropBox')) return;
    const wrap = $('imgWrap'); const r = wrap.getBoundingClientRect();
    const f = (e) => [Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)), Math.min(1, Math.max(0, (e.clientY - r.top) / r.height))];
    const [x0, y0] = f(ev); let moved = false;
    ev.preventDefault(); wrap.setPointerCapture(ev.pointerId);
    const move = (e) => {
      if (!moved && Math.hypot(e.clientX - ev.clientX, e.clientY - ev.clientY) < 6) return; // un clic semplice non cambia nulla
      moved = true;
      const [x, y] = f(e); const c = S.crop; const right = x >= x0; const down = y >= y0;
      if (c.lock) {
        const w = Math.max(MIN_FRAC, Math.min(Math.abs(x - x0), right ? 1 - x0 : x0, (down ? 1 - y0 : y0) / c.ratioH));
        c.w = w; c.h = w * c.ratioH; c.x = right ? x0 : x0 - w; c.y = down ? y0 : y0 - c.h;
      } else {
        c.x = Math.min(x0, x); c.y = Math.min(y0, y); c.w = Math.max(MIN_FRAC, Math.abs(x - x0)); c.h = Math.max(MIN_FRAC, Math.abs(y - y0));
        clampCrop();
      }
      if (c.quad) c.quad = quadOf(c);
      drawCrop();
      showLoupe(x, y, e.clientX, e.clientY);
    };
    const up = () => { hideLoupe(); wrap.removeEventListener('pointermove', move); wrap.removeEventListener('pointerup', up); wrap.removeEventListener('pointercancel', up); };
    wrap.addEventListener('pointermove', move); wrap.addEventListener('pointerup', up); wrap.addEventListener('pointercancel', up);
  }
  // quattro angoli: si tira un angolo, oppure si sposta tutto trascinando dentro
  function onQuadDown(ev) {
    if (!S.cropping || !S.crop || !S.crop.quad || ev.button > 0) return;
    const el = ev.currentTarget; const idx = el.dataset.q === undefined ? -1 : Number(el.dataset.q);
    const r = $('imgWrap').getBoundingClientRect(); const c = S.crop;
    const start = c.quad.map((p) => p.slice());
    const cl = (v) => Math.min(1, Math.max(0, v));
    ev.preventDefault(); ev.stopPropagation(); el.setPointerCapture(ev.pointerId);
    const lens = (e) => { if (idx >= 0) showLoupe(c.quad[idx][0], c.quad[idx][1], e.clientX, e.clientY); };
    lens(ev);
    const move = (e) => {
      const dx = (e.clientX - ev.clientX) / r.width; const dy = (e.clientY - ev.clientY) / r.height;
      if (idx >= 0) c.quad[idx] = [cl(start[idx][0] + dx), cl(start[idx][1] + dy)];
      else {
        const lo = [Math.min(...start.map((p) => p[0])), Math.min(...start.map((p) => p[1]))]; const hi = [Math.max(...start.map((p) => p[0])), Math.max(...start.map((p) => p[1]))];
        const sx = Math.min(1 - hi[0], Math.max(-lo[0], dx)); const sy = Math.min(1 - hi[1], Math.max(-lo[1], dy));
        c.quad = start.map((p) => [p[0] + sx, p[1] + sy]);
      }
      drawCrop(); lens(e);
    };
    const up = () => { hideLoupe(); el.removeEventListener('pointermove', move); el.removeEventListener('pointerup', up); el.removeEventListener('pointercancel', up); };
    el.addEventListener('pointermove', move); el.addEventListener('pointerup', up); el.addEventListener('pointercancel', up);
  }
  function onCropKey(ev) {
    if (!S.cropping) return;
    if (ev.key === 'Escape') { ev.preventDefault(); cancelCrop(); return; }
    const d = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }[ev.key];
    if (!d) return;
    ev.preventDefault();
    const st = 0.01; const c = S.crop;
    if (ev.shiftKey) { if (c.lock) c.w += (d[0] + d[1]) * st; else { c.w += d[0] * st; c.h += d[1] * st; } } else { c.x += d[0] * st; c.y += d[1] * st; }
    clampCrop(); drawCrop();
  }
  // taglio automatico: rifà il ritaglio come alla scansione (lavoro in secondo piano)
  async function cropAuto() {
    if (S.busy || S.cropping || !S.cur) return;
    const saved = S.cur.dir !== 'preview';
    if (saved && !S.doc) { notify('warn', 'Apri prima l\'immagine (premi «Apri»).'); return; }
    if (saved && !(await ask({ title: 'Sostituire l\'immagine salvata?', body: ['Il taglio automatico sostituisce «' + S.cur.file + '» in scans.', 'L\'originale resta salvato come «' + origName(S.cur.file) + '». JSON e PDF già salvati non cambiano.'], yes: 'Sì, taglia', no: 'No, annulla' }))) return;
    S.bust = '-' + Date.now();
    S.afterCrop = saved && S.cur.file.includes(' (originale)') ? S.cur.file.replace(' (originale)', '') : ''; // dalla copia originale: poi si apre l'immagine principale
    startJob('/api/preview/crop', saved ? { file: S.cur.file, dir: 'scans', doc_id: S.doc.doc_id, mode: 'auto' } : { file: S.cur.file, mode: 'auto' });
  }

  // striscia delle ultime 5 anteprime non salvate
  async function refreshPreviews() {
    try { S.previews = (await api('/api/previews')).items || []; } catch (_) { return; }
    if (S.cur && S.cur.dir === 'preview') { // le flag (pagina dati, grezza, diagnosi) possono essere cambiate
      const q = infoFor(S.cur.file);
      if (q) { S.info = Object.assign({}, S.info || {}, q, S.info && S.info.page_url ? { page_url: S.info.page_url } : {}); renderDiag(); renderKeep(); }
    }
    renderStrip();
  }
  function renderStrip() {
    const wrap = $('stripWrap'); const list = $('strip');
    wrap.hidden = S.previews.length === 0;
    const items = S.previews.slice(0, 5);
    list.replaceChildren(...items.map((p) => {
      const sel = !!S.cur && S.cur.dir === 'preview' && S.cur.file === p.file;
      const label = p.file.replace(/\.png$/, '');
      const img = h('img', { alt: '', loading: 'lazy' });
      img.src = p.url + '&t=' + encodeURIComponent(TOKEN) + '&v=' + encodeURIComponent(p.file + '-' + p.time);
      return h('li', {}, h('button', { type: 'button', class: 'thumb' + (sel ? ' sel' : ''), 'aria-pressed': sel ? 'true' : 'false',
        title: p.file + ' (' + fmtTime(p.time) + ')' + (p.low ? ' - immagine molto bassa' : ''), 'aria-label': 'Anteprima ' + label + ', non salvata' + (p.doc_id ? ', dati letti' : ''), onclick: () => selectPreview(p) },
        img, h('span', { class: 'tl' }, label), h('span', { class: 'tb' }, 'non salvata' + (p.doc_id ? ' - letta' : '') + (p.low ? ' - bassa' : ''))));
    }));
  }
  async function selectPreview(p) {
    if (S.busy) return;
    if (S.cur && S.cur.dir === 'preview' && S.cur.file === p.file) return;
    if (!(await confirmDiscard())) return;
    if (p.doc_id) {
      try { const doc = await api('/api/doc/' + p.doc_id); showImage(doc); setDoc(doc, false); return; } catch (_) { /* il documento non c'è più: solo immagine */ }
    }
    showFile(p); clearDoc();
  }
  async function doDiscard() {
    if (!S.cur || S.cur.dir !== 'preview' || S.busy) return;
    const file = S.cur.file;
    if (!(await ask({ title: 'Scartare questa anteprima?', body: ['Si cancella solo il file temporaneo «' + file + '», creato dal programma in dati\\anteprima. I libretti già salvati in scans non vengono toccati.'], yes: 'Scarta', no: 'Annulla' }))) return;
    try {
      await api('/api/preview/discard', { method: 'POST', body: { file } });
      clearViewer(); clearDoc(); await refreshPreviews();
      notify('info', 'Anteprima scartata.');
    } catch (e) { notify('error', e.message); }
  }

  // ---- documento -------------------------------------------------------------------------------------------------
  const dirtyCount = () => Object.keys(S.edits).length + Object.keys(S.rev).length;
  function dirtyText() {
    const e = Object.keys(S.edits).length; const r = Object.keys(S.rev).length;
    const parts = [];
    if (e) parts.push(e + (e === 1 ? ' campo modificato' : ' campi modificati'));
    if (r) parts.push(r + (r === 1 ? ' campo con il controllo cambiato' : ' campi con il controllo cambiato'));
    return parts.join(' e ');
  }
  const isDirty = (f) => Object.prototype.hasOwnProperty.call(S.edits, f.key) && S.edits[f.key] !== f.value;
  // un campo "da controllare" (flag) è controllato se il server lo dice (o la memoria) e l'operatore non l'ha annullato, o se l'ha segnato lui
  const effReviewed = (f) => (Object.prototype.hasOwnProperty.call(S.rev, f.key) ? S.rev[f.key] : !!f.reviewed);
  const openFields = () => (S.doc ? S.doc.fields.filter((f) => f.flag && !isDirty(f) && !effReviewed(f)) : []);

  function setDoc(doc, keepEdits) {
    S.doc = doc;
    if (!keepEdits) { S.edits = {}; S.rev = {}; }
    $('emptyState').hidden = true; $('results').hidden = false; $('savebar').hidden = false;
    updateActions();
    renderWarnings(); renderMemLine(); renderFields(); renderMRZ(); updateSave(); updateCounter(); refreshNameLine(true); renderKeep();
  }

  // contatore "N campi da controllare" nella testata
  function updateCounter() {
    const pill = $('reviewPill');
    if (!S.doc) { pill.hidden = true; return; }
    const n = openFields().length;
    pill.hidden = false;
    pill.classList.toggle('done', n === 0);
    const flagged = S.doc.fields.some((f) => f.flag);
    $('reviewPillText').textContent = n === 0 ? (flagged ? 'Tutti i campi controllati' : 'Nessun campo da controllare') : (n === 1 ? '1 campo da controllare' : n + ' campi da controllare');
    pill.disabled = n === 0;
    pill.title = n === 0 ? '' : 'Vai al primo campo da controllare';
    pill.querySelector('use').setAttribute('href', n === 0 ? '#i-check' : '#i-warn');
  }
  function gotoNextOpen() {
    const f = openFields()[0]; if (!f) return;
    const el = $('f-' + f.key); if (el) { el.scrollIntoView({ block: 'center' }); el.focus(); }
  }
  function setReviewed(f, value, refocus) {
    const baseline = !!f.reviewed;
    if (value === baseline) delete S.rev[f.key]; else S.rev[f.key] = value;
    const tr = document.querySelector('tr[data-key="' + f.key + '"]');
    if (tr) { fillRow(tr, f); if (refocus) tr.querySelector('input').focus(); }
    updateSave(); updateCounter();
  }

  function renderWarnings() {
    // gli avvisi sui singoli campi sono già nella riga del campo e nel contatore in testata: qui solo quelli generali
    const ws = (S.doc.warnings || []).filter((w) => !w.on_field);
    $('warnCard').hidden = ws.length === 0;
    $('warnSum').textContent = ws.length === 1 ? '1 avviso da sapere' : ws.length + ' avvisi da sapere';
    $('warns').replaceChildren(...ws.map((w) => h('li', { class: w.severity },
      icon(w.severity === 'info' ? 'info' : 'warn'), h('span', {}, w.text))));
  }

  // "Dopo la conferma i file si chiameranno: ..." calcolato dal server (internal/filename) con i valori correnti del form
  let nameSeq = 0; let nameTimer = null;
  function currentValues() {
    const v = {};
    for (const f of S.doc.fields) v[f.key] = Object.prototype.hasOwnProperty.call(S.edits, f.key) ? S.edits[f.key] : f.value;
    return v;
  }
  // il nome base che avranno i file dopo la conferma, calcolato dal server con i valori correnti del form
  function fetchNameInfo() {
    return api('/api/preview-name', { method: 'POST', body: { doc_id: S.doc.doc_id, values: currentValues(), fallback_name: $('scanName').value.trim() } });
  }
  function refreshNameLine(now) {
    clearTimeout(nameTimer);
    if (!S.doc) return;
    nameTimer = setTimeout(async () => {
      const seq = ++nameSeq; const el = $('nameText');
      try {
        const r = await fetchNameInfo();
        if (seq !== nameSeq || !S.doc) return;
        S.nameInfo = r;
        $('nameLine').hidden = false; el.replaceChildren();
        if (!r.ok && r.unsaved) el.append('Mancano nome, cognome o numero libretto: dopo la conferma i file si chiameranno «' + r.fallback + '» (il nome di riserva). Per ora non c\'è nulla in scans.');
        else if (!r.ok) el.append('Mancano nome, cognome o numero libretto: dopo la conferma i file restano «' + r.current + '».');
        else if (r.same) el.append('I file si chiamano già ', h('strong', {}, r.name), '.');
        else if (r.unsaved) el.append('Dopo la conferma i file si chiameranno: ', h('strong', {}, r.name), '  (solo con «Conferma e salva»; per ora è un\'anteprima temporanea e in scans non c\'è nulla).');
        else el.append('Dopo la conferma i file si chiameranno: ', h('strong', {}, r.name), '  (solo con «Conferma e salva»; ora si chiamano «' + r.current + '»).');
      } catch (_) { /* l'anteprima del nome è accessoria */ }
    }, now ? 0 : 250);
  }

  function renderMemLine() {
    const m = S.doc.memory; const el = $('memLine'); el.hidden = false; el.classList.remove('off');
    if (!m.usable) { el.classList.add('off'); el.textContent = m.reason; }
    else if (m.applied > 0) el.textContent = 'Memoria: ho corretto da solo ' + m.applied + (m.applied === 1 ? ' campo' : ' campi') + ' di questo libretto con le tue correzioni precedenti. Controlla e conferma.';
    else if (m.known > 0) { el.classList.add('off'); el.textContent = 'Questo libretto è nella memoria (' + m.known + (m.known === 1 ? ' correzione' : ' correzioni') + '), ma questa volta la lettura era già giusta.'; }
    else { el.classList.add('off'); el.textContent = 'Libretto nuovo per la memoria: se correggi dei campi li ricorderò per la prossima scansione.'; }
  }

  function rowState(f) {
    const cur = Object.prototype.hasOwnProperty.call(S.edits, f.key) ? S.edits[f.key] : f.value;
    const dirty = cur !== f.value;
    if (dirty) return { st: 'edited', text: 'Modificato da te', dirty: true };
    if (f.flag) {
      if (effReviewed(f)) {
        const fromMem = f.reviewed === 'memory' && !Object.prototype.hasOwnProperty.call(S.rev, f.key);
        return { st: 'reviewed', text: fromMem ? 'Controllato (da memoria)' : 'Controllato', dirty: false };
      }
      return { st: 'check', text: 'Da controllare', dirty: false };
    }
    return { st: f.status, text: f.status_text, dirty: false };
  }
  const STATUS_ICON = { ok: 'check', check: 'warn', reviewed: 'check', empty: 'dash', memory: 'mem', edited: 'pen' };

  function fillRow(tr, f) {
    const r = rowState(f);
    tr.className = 'st-' + r.st;
    const badge = tr.querySelector('.status');
    badge.className = 'status s-' + r.st;
    badge.replaceChildren(icon(STATUS_ICON[r.st]), r.text);
    const act = tr.querySelector('.sact'); act.replaceChildren();
    if (r.st === 'check') {
      act.append(h('button', { type: 'button', class: 'btn small ok-btn', title: 'Ho guardato questo campo e va bene (Alt+C)', 'aria-label': 'Segna controllato: ' + f.es, onclick: () => setReviewed(f, true, true) }, icon('check'), 'Controllato'));
    } else if (r.st === 'reviewed') {
      act.append(h('button', { type: 'button', class: 'btn link', 'aria-label': 'Rimetti da controllare: ' + f.es, onclick: () => setReviewed(f, false, true) }, 'Rimetti da controllare'));
    }
    const note = tr.querySelector('.fnote'); note.replaceChildren();
    const input = tr.querySelector('input');
    const value = r.dirty ? S.edits[f.key] : f.value;
    if (input.value !== value) input.value = value;
    if (r.st === 'memory') {
      note.append(h('span', {}, 'Letto: ', h('span', { class: 'was' }, f.mem_read || '(vuoto)'), ', corretto da te il ' + f.mem_date),
        h('button', { type: 'button', class: 'btn link', onclick: () => forgetField(f.key) }, 'Era giusto, dimentica'));
    } else if (r.st === 'edited') {
      note.append(h('span', {}, 'Letto: ', h('span', { class: 'was' }, f.read || '(vuoto)')));
      if (r.dirty) note.append(h('button', { type: 'button', class: 'btn link', onclick: () => setEdit(f, f.value, true) }, 'Annulla modifica'));
      else note.append(h('button', { type: 'button', class: 'btn link', onclick: () => setEdit(f, f.read, true) }, 'Rimetti il valore letto'));
    } else if (r.st === 'reviewed') {
      note.append(h('span', { title: f.hint }, 'Motivo del controllo: ' + f.hint));
    } else if (f.hint) {
      note.append(h('span', {}, f.hint));
    }
    if (f.extra) note.append(h('span', {}, f.extra));
  }

  function setEdit(f, value, refocus) {
    if (value === f.value) delete S.edits[f.key]; else S.edits[f.key] = value;
    const tr = document.querySelector('tr[data-key="' + f.key + '"]');
    if (tr) { fillRow(tr, f); if (refocus) tr.querySelector('input').focus(); }
    updateSave(); updateCounter();
    if (f.key === 'given_names' || f.key === 'surname' || f.key === 'seamans_book') refreshNameLine(false);
  }

  function renderFields() {
    const body = $('fieldsBody'); body.replaceChildren();
    const inputs = [];
    for (const f of S.doc.fields) {
      const id = 'f-' + f.key;
      const input = h('input', { type: 'text', id, autocomplete: 'off', spellcheck: 'false', 'aria-describedby': id + '-note', maxlength: 120 });
      input.addEventListener('input', () => setEdit(f, input.value.replace(/\s+/g, ' ').toUpperCase().trimStart(), false));
      input.addEventListener('blur', () => { const v = input.value.trim().toUpperCase(); if (v !== input.value) { input.value = v; setEdit(f, v, false); } });
      input.addEventListener('keydown', (e) => {
        if (e.altKey && !e.ctrlKey && !e.shiftKey && (e.key === 'c' || e.key === 'C')) {
          e.preventDefault();
          if (f.flag && !isDirty(f)) setReviewed(f, !effReviewed(f), true);
        } else if (e.key === 'Enter' && !e.ctrlKey) {
          e.preventDefault();
          if (f.flag && !isDirty(f) && !effReviewed(f)) setReviewed(f, true, false); // Invio su un campo non modificato = controllato
          const i = inputs.indexOf(input); (inputs[i + 1] || $('btnConfirm')).focus();
        } else if (e.key === 'Enter' && e.ctrlKey) { e.preventDefault(); save('confirm'); }
      });
      inputs.push(input);
      const copyBtn = h('button', { type: 'button', class: 'btn icon copy', title: 'Copia il valore di «' + f.es + ' / ' + f.en + '» (Alt+Maiusc+C)',
        'aria-label': 'Copia ' + f.es + ' / ' + f.en, onclick: () => copyField(f, copyBtn) }, icon('copy'));
      const tr = h('tr', { dataset: { key: f.key } },
        h('th', { scope: 'row' },
          h('label', { for: id, class: 'flabel' }, f.es + ' / ', h('span', { class: 'en' }, f.en)),
          f.source ? h('span', { class: 'fsrc' }, f.source) : null),
        h('td', {}, h('div', { class: 'valwrap' }, input, copyBtn), h('div', { class: 'fnote', id: id + '-note' })),
        h('td', {}, h('span', { class: 'status' }), h('div', { class: 'sact' })));
      body.append(tr);
      fillRow(tr, f);
    }
  }

  function renderMRZ() {
    const m = S.doc.mrz;
    $('mrzNone').hidden = m.found; $('mrzBody').hidden = !m.found;
    if (!m.found) return;
    $('mrzLines').textContent = m.line1 + '\n' + m.line2;
    // tutti giusti: un solo segno (prima cinque etichette uguali); se uno sbaglia si vedono tutti, con quello ERRATO in evidenza
    $('mrzChecks').replaceChildren(...(m.checks.every((c) => c.ok)
      ? [h('li', { class: 'ok', title: m.checks.map((c) => c.label).join(', ') }, icon('check'), 'Controlli MRZ: tutti corretti')]
      : m.checks.map((c) => h('li', { class: c.ok ? 'ok' : 'bad' }, icon(c.ok ? 'check' : 'x'), c.label + ': ' + (c.ok ? 'corretto' : 'ERRATO')))));
    const cs = m.corrections || [];
    $('mrzCorrWrap').hidden = cs.length === 0;
    $('mrzCorr').replaceChildren(...cs.map((c) => h('li', {}, c)));
  }

  function updateSave() {
    if (!S.doc) return;
    const n = dirtyCount();
    const usable = S.doc.memory.usable;
    const open = openFields().length;
    let t = n === 0 ? 'Nessuna modifica.' : (dirtyText() + '.');
    t = t.charAt(0).toUpperCase() + t.slice(1);
    if (open > 0) t += ' ' + (open === 1 ? 'Resta 1 campo da controllare.' : 'Restano ' + open + ' campi da controllare.');
    else if (n === 0) t += ' Se i dati sono giusti puoi confermare.';
    if (n > 0 && !usable) t += ' La memoria non è utilizzabile (MRZ non valida): non verrà ricordato nulla.';
    const done = !S.saving && n === 0 && S.saved && S.saved.id === S.doc.doc_id;
    const bar = $('savebar'); bar.classList.toggle('saving', S.saving); bar.classList.toggle('saved', !!done);
    if (S.saving) t = 'Salvataggio in corso…';
    else if (done) t = '✓ Salvato alle ' + S.saved.at + '. ' + S.saved.text;
    $('saveInfo').textContent = t; $('saveInfo').title = done && S.saved.full ? t + ' ' + S.saved.full : t;
    $('btnSaveCorr').disabled = S.saving || n === 0 || !usable;
    $('btnConfirm').disabled = S.saving || !!done;
    $('confirmText').textContent = S.saving ? 'Salvataggio…' : done ? 'Salvato' : 'Conferma e salva';
    $('confirmSpin').hidden = !S.saving; $('confirmIcon').hidden = S.saving;
    $('btnNext').hidden = !done;
    if (done) $('btnNext').focus({ preventScroll: true });
  }

  // ---- salvataggio -------------------------------------------------------------------------------------------------
  // save restituisce true se ha salvato. opts: { overwrite, acceptOpen }
  async function save(mode, opts = {}) {
    if (!S.doc || S.saving) return false;
    const values = {};
    for (const f of S.doc.fields) values[f.key] = Object.prototype.hasOwnProperty.call(S.edits, f.key) ? S.edits[f.key] : f.value;
    const reviewed = S.doc.fields.filter((f) => f.flag && !isDirty(f) && effReviewed(f)).map((f) => f.key);
    S.saving = true; updateSave();
    try {
      const resp = await api('/api/save', { method: 'POST', body: { doc_id: S.doc.doc_id, mode, values, reviewed, overwrite: !!opts.overwrite, accept_open: !!opts.acceptOpen, fallback_name: $('scanName').value.trim(), keep_full: $('keepFull').checked, keep_raw: $('keepRaw').checked, use_page: S.usePage && !!(S.info && S.info.page_url) } });
      const prevFile = S.doc.file; const prevDir = S.doc.dir;
      setDoc(resp.doc, false);
      if (resp.doc.file !== prevFile || resp.doc.dir !== prevDir) refreshImage(resp.doc);
      refreshPreviews();
      const base = resp.doc.file.replace(/\.[^.]+$/, '');
      if (resp.memory_skipped) for (const m of resp.messages || []) notify('warn', m); // gli avvisi restano; il resto lo dice la barra qui sotto
      else notify('ok', 'Salvato: ' + base);
      S.saved = { id: resp.doc.doc_id, at: new Date().toLocaleTimeString(window.PBI18n.locale(), { hour: '2-digit', minute: '2-digit' }), text: 'in scans\\ come «' + base + '»', full: (resp.messages || []).join(' ') };
      updateSave();
      refreshScans(); refreshMemCount();
      return true;
    } catch (e) {
      if (e.code === 'campi_aperti') {
        S.saving = false; updateSave();
        const list = h('ul', {}, ...(e.data.open || []).map((o) => h('li', {}, o.label)));
        const yes = await ask({ title: 'Ci sono campi ancora da controllare', body: ['Questi campi non sono stati controllati:', list,
          'Se confermi comunque, nella verità per i test questi campi restano vuoti (non vengono dati per giusti).'], yes: 'Conferma comunque', no: 'Torna a controllare' });
        if (yes) return save(mode, Object.assign({}, opts, { acceptOpen: true }));
        gotoNextOpen();
        return false;
      }
      if (e.code === 'esistono_file') {
        S.saving = false; updateSave();
        const list = h('ul', {}, ...(e.data.existing || []).map((p) => h('li', {}, h('code', {}, p))));
        const yes = await ask({ title: 'Sostituire i file esistenti?', body: ['Questi file esistono già:', list,
          'La verità precedente viene conservata con il suffisso .prec-DATA.'], yes: 'Sostituisci', no: 'Annulla' });
        if (yes) return save(mode, Object.assign({}, opts, { overwrite: true }));
        return false;
      }
      notify('error', e.message);
      return false;
    } finally { S.saving = false; updateSave(); }
  }

  async function forgetField(key) {
    try {
      const doc = await api('/api/memory/forget-field', { method: 'POST', body: { doc_id: S.doc.doc_id, field: key } });
      delete S.edits[key]; delete S.rev[key];
      setDoc(doc, true); refreshMemCount();
      notify('info', 'Dimenticato: il campo torna al valore letto.');
    } catch (e) { notify('error', e.message); }
  }

  // ---- lavori -----------------------------------------------------------------------------------------------------
  function setBusy(b) {
    S.busy = b;
    for (const id of ['btnScan', 'btnOpen', 'openSelect', 'setDriver', 'setDpi', 'setLen', 'setTop', 'setReset']) $(id).disabled = b;
    updateActions();
  }

  function showJob(j) {
    const p = $('jobPanel'); p.hidden = false; p.classList.remove('finished');
    $('jobTitle').textContent = j.kind === 'preview' ? (j.demo ? 'Anteprima di PROVA' : 'Anteprima (scansione)') : j.kind === 'scan' ? 'Scansione e lettura' : j.kind === 'crop' ? 'Taglio automatico' : 'Lettura dei dati';
    $('jobMsg').textContent = j.message || '';
    $('jobElapsed').textContent = j.elapsed_s + ' s';
    S.scanning = j.phase === 'scanning'; $('btnReset').disabled = S.scanning;
    const c = $('btnCancel'); c.disabled = !j.cancelable;
    $('cancelHelp').textContent = j.phase === 'scanning' ? (j.demo ? 'Prova: nessuno scanner in uso.' : 'La scansione fisica non si può interrompere: attendi la fine. Non estrarre il libretto.') : '';
  }

  let pollTimer = null; let currentJob = '';
  async function watchJob(id) {
    currentJob = id; setBusy(true); clearTimeout(pollTimer);
    const tick = async () => {
      let j;
      try { j = await api('/api/jobs/' + id); } catch (e) { pollTimer = setTimeout(tick, 1500); return; }
      showJob(j);
      if (!j.done) { pollTimer = setTimeout(tick, 700); return; }
      $('jobPanel').hidden = true; currentJob = ''; S.scanning = false; $('btnReset').disabled = false; setBusy(false);
      if (j.phase === 'done' && j.preview) {
        showFile(j.preview); clearDoc(); await refreshPreviews();
        if (j.preview.low) notify('warn', 'L\'immagine è molto bassa: controlla di aver inserito il libretto con il bordo MRZ per primo, centrato e spinto fino in fondo. Se vedi solo l\'informativa, premi «Riscansiona».', { sticky: true });
        else notify(j.preview.has_page ? 'info' : 'warn', j.message || 'Anteprima pronta.');
        $('prevActions').scrollIntoView({ block: 'nearest' });
      } else if (j.phase === 'done' && j.kind === 'crop' && S.afterCrop) {
        const f = S.afterCrop; S.afterCrop = '';
        window.scrollTo({ top: 0, behavior: 'smooth' });
        notify('info', (j.message || 'Taglio automatico fatto.') + ' Si apre «' + f + '».');
        startJob('/api/open', { file: f, dir: 'scans' }); refreshScans();
      } else if (j.phase === 'done' && j.doc) {
        showImage(j.doc); setDoc(j.doc, false); $('fieldsCard').scrollIntoView({ block: 'nearest' }); refreshScans(); refreshPreviews();
        if (j.kind === 'crop') window.scrollTo({ top: 0, behavior: 'smooth' }); // il messaggio sta in cima alla pagina: lo si vede insieme all'immagine tagliata
        if (j.kind === 'crop') notify(/non è stato riconosciuto/.test(j.message || '') ? 'warn' : 'info', j.message || 'Taglio automatico fatto.'); // senza questo il taglio di un'immagine salvata non diceva nulla
      }
      else if (j.phase === 'cancelled') notify('info', j.message || 'Annullato.');
      else notify('error', j.error || 'Operazione non riuscita.');
    };
    tick();
  }

  async function startJob(path, body) {
    clearBanners();
    try {
      const r = await api(path, { method: 'POST', body });
      watchJob(r.job);
    } catch (e) {
      const el = $('scanName');
      if (e.code === 'esiste') { el.setAttribute('aria-invalid', 'true'); el.focus(); }
      notify('error', e.message);
    }
  }

  // "Anteprima": scansione fisica in dati\\anteprima (niente in scans, niente lettura): si guarda l'immagine prima di leggerla
  async function doPreview() {
    if (S.busy) return;
    const s = readSettings();
    if (typeof s === 'string') { $('settings').open = true; notify('error', s); return; }
    if (!(await confirmDiscard())) return;
    if (!S.demo) {
      const ok = await ask({ title: 'Il libretto è inserito?', body: [h('ol', {},
        h('li', {}, 'Pagina con la foto aperta, bordo con le righe MRZ inserito per primo.'),
        h('li', {}, 'Centrato, dritto e spinto fino in fondo.'),
        h('li', {}, 'Altri programmi che usano lo scanner chiusi.')), 'La scansione dura circa 10 secondi e non si può interrompere. L\'immagine resta un\'anteprima temporanea: in scans non viene scritto nulla finché non confermi.'], yes: 'Sì, avvia la scansione', no: 'No, annulla' });
      if (!ok) return;
    }
    saveSettings();
    startJob('/api/preview', s);
  }

  async function openFile(file) {
    if (S.busy || !file) return;
    if (!(await confirmDiscard())) return;
    startJob('/api/open', { file, dir: 'scans' });
  }
  // "Leggi i dati" (anteprima) o "Rileggi" (immagine salvata): extract sull'immagine corrente
  async function readCurrent() {
    if (S.busy || !S.cur) return;
    if (!(await confirmDiscard())) return;
    const st = readSettings();
    startJob('/api/open', { file: S.cur.file, dir: S.cur.dir, top_margin_mm: typeof st === 'string' ? 2 : st.top_margin_mm });
  }

  // ---- ricomincia da capo ---------------------------------------------------------------------------------------
  async function waitIdle(ms) {
    const end = Date.now() + ms;
    while (S.busy && Date.now() < end) await new Promise((r) => setTimeout(r, 150));
  }
  async function doReset() {
    if (S.scanning) return;
    const np = S.previews.length;
    if (dirtyCount() || np) {
      const lines = [];
      if (dirtyCount()) lines.push('Hai ' + dirtyText() + ' non salvati.');
      if (np) lines.push('Hai ' + np + (np === 1 ? ' anteprima non salvata' : ' anteprime non salvate') + ' (restano in dati\\anteprima e si eliminano dopo 7 giorni).');
      lines.push('Non viene cancellato nessun file e la memoria resta com\'è.');
      const unsavedDoc = !!S.doc && !!S.doc.unsaved;
      const usable = !!S.doc && S.doc.memory.usable;
      const extra = unsavedDoc ? 'Salva quella selezionata e ricomincia' : (usable && dirtyCount() ? 'Salva e ricomincia' : '');
      const c = await askChoice({ title: 'Ricominciare da capo?', body: lines, yes: 'Ricomincia senza salvare', no: 'Annulla', extra });
      if (c === 'no') return;
      if (c === 'extra' && !(await save(unsavedDoc ? 'confirm' : 'corrections'))) return;
    }
    if (S.busy) { // lettura in corso: si annulla e poi si ricomincia
      try { await api('/api/jobs/' + currentJob + '/cancel', { method: 'POST', body: {} }); } catch (e) { notify('error', e.message); return; }
      await waitIdle(8000);
      if (S.busy) { notify('error', 'La lettura non si è fermata: riprova tra un istante.'); return; }
    }
    let name = '';
    try { name = (await api('/api/reset', { method: 'POST', body: {} })).name; } catch (e) { notify('error', e.message); return; }
    clearBanners();
    clearViewer(); clearDoc();
    $('scanName').value = name || suggestedName(); $('scanName').removeAttribute('aria-invalid');
    refreshScans(); refreshPreviews();
    $('btnScan').focus();
  }

  // ---- PDF --------------------------------------------------------------------------------------------------------
  async function exportPdf(overwrite) {
    if (!S.cur || S.pdfBusy) return;
    S.pdfBusy = true; $('btnPdf').disabled = true; $('btnPdf').querySelector('span').textContent = 'Creo il PDF...';
    try {
      const r = await api('/api/pdf', { method: 'POST', body: { file: S.cur.file, dir: S.cur.dir, overwrite: !!overwrite, use_page: S.usePage && !!(S.info && S.info.page_url) } });
      const msg = 'PDF creato: ' + r.file + ', ' + r.kb + ' KB' + (r.info ? ' (' + r.info + ')' : '') + '.' + (r.preview ? ' È un PDF dell\'anteprima: con «Conferma e salva» passa in scans con lo stesso nome.' : '');
      notify(r.limit_ok ? 'ok' : 'warn', r.limit_ok ? msg : msg + ' ATTENZIONE: supera 1 MB.', { sticky: true });
    } catch (e) {
      if (e.code === 'esiste') {
        S.pdfBusy = false;
        const yes = await ask({ title: 'Sostituire il PDF?', body: [e.message], yes: 'Sostituisci', no: 'Annulla' });
        if (yes) await exportPdf(true);
        return;
      }
      notify('error', e.message);
    } finally {
      S.pdfBusy = false; $('btnPdf').querySelector('span').textContent = 'Esporta PDF (max 1 MB)'; updateActions();
    }
  }

  // ---- memoria ----------------------------------------------------------------------------------------------------
  async function refreshMemCount() {
    try { $('memCount').textContent = String((await api('/api/memory')).items.length); } catch (_) { /* ignora */ }
  }
  async function renderMemory() {
    const box = $('memList'); box.replaceChildren(h('p', { class: 'muted' }, 'Carico...'));
    let items;
    try { items = (await api('/api/memory')).items; } catch (e) { box.replaceChildren(h('p', {}, e.message)); return; }
    $('memCount').textContent = String(items.length);
    if (!items.length) { box.replaceChildren(h('p', { class: 'muted' }, 'Nessun libretto ricordato. Correggi un campo e premi "Salva correzioni": il programma ricorderà la correzione per quel libretto.')); return; }
    box.replaceChildren(...items.map((it) => h('div', { class: 'mem-item' },
      h('header', {}, h('div', {}, h('strong', {}, it.etichetta || '(senza nome)'), ' ', h('span', { class: 'muted' }, it.numero + ' - aggiornato ' + fmtTime(it.aggiornato))),
        h('button', { type: 'button', class: 'btn small danger', onclick: () => forgetMem(it, '') }, 'Dimentica libretto')),
      h('table', {}, h('tbody', {}, ...it.campi.map((c) => h('tr', {},
        h('td', {}, c.label),
        c.tipo === 'conferma'
          ? h('td', {}, 'controllato: ', h('span', { class: 'was' }, c.corretto || '(vuoto)'))
          : h('td', {}, 'letto: ', h('span', { class: 'was' }, c.letto || '(vuoto)'), ' - corretto: ', h('span', { class: 'was' }, c.corretto || '(vuoto)')),
        h('td', { class: 'muted' }, c.data),
        h('td', {}, h('button', { type: 'button', class: 'btn small', onclick: () => forgetMem(it, c.field, c.tipo) }, 'Dimentica')))))))));
  }
  async function forgetMem(it, field, kind) {
    const yes = await ask({ title: field ? 'Dimenticare questa correzione?' : 'Dimenticare tutto il libretto?',
      body: field ? 'La prossima volta questo campo non verrà più corretto da solo.' : 'Tutte le correzioni ricordate per questo libretto vengono tolte.', yes: 'Dimentica', no: 'Annulla' });
    if (!yes) return;
    try {
      await api('/api/memory/forget', { method: 'POST', body: { key: it.key, field, kind } });
      if (S.doc) { S.rev = {}; const keep = S.edits; const doc = await api('/api/doc/' + S.doc.doc_id); for (const k of Object.keys(keep)) { if (doc.fields.find((f) => f.key === k && f.value === keep[k])) delete keep[k]; } S.edits = keep; setDoc(doc, true); }
    } catch (e) { notify('error', e.message); }
    renderMemory();
  }

  // ---- copia e incolla nel gestionale ---------------------------------------------------------------------------
  // La formattazione e' in copy.js (logica pura, window.PBCopy). Qui ci sono solo appunti e interfaccia. Negli appunti si scrive SOLO
  // da un clic o da un tasto dell'utente: mai da soli.
  const PB = window.PBCopy;
  let copyPrefs = PB.normalizePrefs(null);
  function loadCopyPrefs() {
    let c = S.prefs.copy;
    if (!c) { c = legacy('pbcopy1'); if (c) savePrefs({ copy: c }); }
    copyPrefs = PB.normalizePrefs(c || null);
    $('copyFormat').value = copyPrefs.format; $('copyDates').value = copyPrefs.dates;
  }
  function saveCopyPrefs() { savePrefs({ copy: { format: copyPrefs.format, dates: copyPrefs.dates } }); }

  // annuncio per i lettori di schermo (una regione role=status separata dai banner)
  let liveTimer = null;
  function announce(msg) {
    const el = $('copyLive'); el.textContent = '';
    clearTimeout(liveTimer);
    liveTimer = setTimeout(() => { el.textContent = msg; }, 40); // vuoto e poi pieno: anche lo stesso messaggio viene riletto
  }

  // copyText scrive negli appunti: prima navigator.clipboard (localhost e' un contesto sicuro), poi il ripiego
  // document.execCommand('copy') su una textarea temporanea. Restituisce { ok } oppure { ok: false, reason }.
  async function copyText(text) {
    let reason = '';
    if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
      try { await navigator.clipboard.writeText(text); return { ok: true }; } catch (e) {
        reason = e && e.name === 'NotAllowedError' ? 'il browser non ha dato il permesso di scrivere negli appunti.' : ((e && e.message) || 'errore degli appunti.');
      }
    } else reason = 'questo browser non permette l\'accesso agli appunti.';
    const prev = document.activeElement;
    const ta = h('textarea', { 'aria-hidden': 'true', readonly: true, tabindex: '-1', class: 'clip-ta' });
    ta.value = text;
    document.body.append(ta);
    let ok = false;
    try { ta.focus({ preventScroll: true }); ta.select(); ok = document.execCommand('copy'); } catch (_) { ok = false; }
    ta.remove();
    if (prev && prev.focus) { try { prev.focus({ preventScroll: true }); } catch (_) { /* ignora */ } }
    return ok ? { ok: true, fallback: true } : { ok: false, reason };
  }

  // feedback sul pulsante per 1,5 s: "Copiato" (verde), "Vuoto" (giallo) o "Errore" (rosso)
  const flashTimers = new WeakMap();
  function flash(btn, kind) {
    if (!btn) return;
    if (!btn._orig) btn._orig = Array.from(btn.childNodes);
    clearTimeout(flashTimers.get(btn));
    btn.classList.remove('done', 'empty', 'err');
    btn.classList.add({ ok: 'done', empty: 'empty', err: 'err' }[kind]);
    btn.replaceChildren(icon(kind === 'ok' ? 'check' : kind === 'empty' ? 'dash' : 'x'), h('span', {}, kind === 'ok' ? 'Copiato' : kind === 'empty' ? 'Vuoto' : 'Errore'));
    flashTimers.set(btn, setTimeout(() => { btn.classList.remove('done', 'empty', 'err'); btn.replaceChildren(...btn._orig); }, 1500));
  }

  // doCopy: text vuoto = non si fa niente e lo si dice; altrimenti si copia, si mostra l'esito e lo si annuncia
  async function doCopy(text, btn, what, emptyMsg) {
    if (!text) { flash(btn, 'empty'); announce(emptyMsg); return false; }
    const r = await copyText(text);
    if (r.ok) { flash(btn, 'ok'); announce('Copiato: ' + what); return true; }
    flash(btn, 'err');
    announce('Errore: non sono riuscito a copiare negli appunti.');
    notify('error', 'Non riesco a scrivere negli appunti: ' + r.reason + ' Seleziona il testo a mano e premi Ctrl+C.', { sticky: true });
    return false;
  }

  const fieldByKey = (key) => (S.doc ? S.doc.fields.find((x) => x.key === key) : null);
  // il valore CORRENTE del campo (con le correzioni dell'operatore), non quello letto
  function copyField(f, btn) {
    if (!S.doc) return Promise.resolve(false);
    const text = PB.fieldText(f, currentValues()[f.key], copyPrefs);
    return doCopy(text, btn, PB.labelOf(f) + ': ' + text, PB.labelOf(f) + ': il campo è vuoto, niente da copiare.');
  }
  function copyAll() {
    if (!S.doc) return Promise.resolve(false);
    const text = PB.formatFields(S.doc.fields, currentValues(), copyPrefs.format, copyPrefs);
    const fmt = PB.FORMATS.find((x) => x.id === copyPrefs.format);
    return doCopy(text, $('btnCopyAll'), 'tutti i campi, formato «' + (fmt ? fmt.text : copyPrefs.format) + '»', 'Tutti i campi sono vuoti: niente da copiare.');
  }
  function copyMrz(which, btn) {
    if (!S.doc) return Promise.resolve(false);
    return doCopy(PB.mrzText(S.doc.mrz, which), btn, which === 'both' ? 'le due righe MRZ' : 'riga ' + which + ' dell\'MRZ', 'La riga MRZ non c\'è: niente da copiare.');
  }
  async function copyName() {
    if (!S.doc) return false;
    let info = S.nameInfo;
    try { info = await fetchNameInfo(); S.nameInfo = info; } catch (_) { /* si usa l'ultimo noto */ }
    const text = PB.fileNameText(info);
    return doCopy(text, $('btnCopyName'), 'nome file «' + text + '»', 'Il nome del file non è ancora disponibile.');
  }
  // Alt+Maiusc+C: il campo su cui si trova il focus (la sua riga della tabella)
  function copyActiveField() {
    const a = document.activeElement; const tr = a && a.closest ? a.closest('tr[data-key]') : null;
    const f = tr ? fieldByKey(tr.dataset.key) : null;
    if (!S.doc || !f) { announce('Per copiare un campo spostati prima su una riga della tabella.'); return; }
    copyField(f, tr.querySelector('.copy'));
  }

  function initCopyBar() {
    const fe = $('copyFormat'); const de = $('copyDates');
    fe.replaceChildren(...PB.FORMATS.map((x) => h('option', { value: x.id }, x.text)));
    de.replaceChildren(...PB.DATE_MODES.map((x) => h('option', { value: x.id }, x.text)));
    fe.addEventListener('change', () => { copyPrefs.format = fe.value; saveCopyPrefs(); });
    de.addEventListener('change', () => { copyPrefs.dates = de.value; saveCopyPrefs(); });
    $('btnCopyAll').addEventListener('click', copyAll);
    $('btnMrz1').addEventListener('click', () => copyMrz(1, $('btnMrz1')));
    $('btnMrz2').addEventListener('click', () => copyMrz(2, $('btnMrz2')));
    $('btnMrzBoth').addEventListener('click', () => copyMrz('both', $('btnMrzBoth')));
    $('btnCopyName').addEventListener('click', copyName);
    // Alt+Maiusc+C = copia il campo attivo; Alt+Maiusc+A = copia tutto (Alt+C e Alt+N sono gia' usate, Ctrl+C resta quella del browser)
    window.addEventListener('keydown', (e) => {
      if (!e.altKey || !e.shiftKey || e.ctrlKey || e.metaKey || document.querySelector('dialog[open]')) return;
      if (e.code === 'KeyC') { e.preventDefault(); copyActiveField(); }
      else if (e.code === 'KeyA') { e.preventDefault(); if (S.doc) copyAll(); else announce('Non ci sono dati da copiare.'); }
    });
  }

  // ---- avvio ------------------------------------------------------------------------------------------------------
  let beatFail = 0;
  function heartbeat() {
    api('/api/heartbeat', { method: 'POST', body: {} }).then(() => { beatFail = 0; }).catch(() => {
      if (++beatFail === 3) notify('error', 'Il programma Panama Book non risponde più: è stato chiuso. Riavvia PanamaBook.exe.', { sticky: true });
    });
  }

  async function init() {
    let st;
    try { st = await api('/api/state'); } catch (e) {
      notify('error', 'Non riesco a parlare con il programma (' + e.message + '). Riavvia PanamaBook.exe.', { sticky: true }); return;
    }
    S.defaults = st.defaults; S.demo = st.demo; S.prefs = st.prefs || {};
    loadLang(); loadCopyPrefs();
    if (st.demo) {
      $('demoBadge').hidden = false;
      $('demoBadge').title = 'PROVA: nessuno scanner viene usato. "Anteprima" usa l\'immagine ' + st.demo_name + ' come se fosse appena scansionata.';
    }
    if (!st.libretto_ok) notify('error', 'Manca ' + st.libretto_name + ' accanto a PanamaBook.exe: senza non si può scansionare né leggere.', { sticky: true });
    $('scanName').value = st.suggested || suggestedName();
    loadSettings(); renderOut(st.outdir);
    heartbeat(); setInterval(heartbeat, 5000);
    refreshScans(); refreshMemCount();
    await refreshPreviews();
    if (st.busy_job) watchJob(st.busy_job);
    else if (st.last_doc) { showImage(st.last_doc); setDoc(st.last_doc, false); }
    else if (S.previews.length) { showFile(S.previews[0]); clearDoc(); }
  }

  // ---- collegamenti -----------------------------------------------------------------------------------------------
  $('btnScan').addEventListener('click', doPreview);
  $('btnRescan').addEventListener('click', doPreview);
  $('btnRead').addEventListener('click', readCurrent);
  for (const [id, name] of [['vPage', 'page'], ['vFull', 'full'], ['vRaw', 'raw']]) {
    $(id).addEventListener('click', () => {
      if (name === 'page') S.usePage = true; else if (name === 'full') S.usePage = false; // la grezza non cambia la scelta
      setView(name);
    });
  }
  $('btnDiscard').addEventListener('click', doDiscard);
  $('scanName').addEventListener('input', () => { $('scanName').removeAttribute('aria-invalid'); refreshNameLine(false); });
  $('btnOpen').addEventListener('click', () => { const v = $('openSelect').value; if (v) openFile(v); else { $('openSelect').focus(); notify('info', 'Scegli prima un\'immagine dall\'elenco.'); } });
  $('btnReread').addEventListener('click', readCurrent);
  $('btnPdf').addEventListener('click', () => exportPdf(false));
  $('btnReset').addEventListener('click', doReset);
  $('reviewPill').addEventListener('click', gotoNextOpen);
  $('btnHelp').addEventListener('click', () => $('dlgHelp').showModal());
  $('helpClose').addEventListener('click', () => $('dlgHelp').close());
  $('btnNext').addEventListener('click', () => $('btnReset').click());
  window.addEventListener('keydown', (e) => {
    if (e.altKey && !e.ctrlKey && (e.key === 'n' || e.key === 'N')) { e.preventDefault(); if (!$('btnReset').disabled) doReset(); }
  });
  $('btnSaveCorr').addEventListener('click', () => save('corrections'));
  $('btnConfirm').addEventListener('click', () => save('confirm'));
  $('btnMem').addEventListener('click', () => { renderMemory(); $('dlgMem').showModal(); });
  $('memClose').addEventListener('click', () => $('dlgMem').close());
  $('btnCancel').addEventListener('click', async () => {
    try { await api('/api/jobs/' + currentJob + '/cancel', { method: 'POST', body: {} }); } catch (e) { notify('error', e.message); }
  });
  $('setLang').addEventListener('change', () => { window.PBI18n.setLang($('setLang').value); savePrefs({ lang: $('setLang').value }); });
  $('btnOutPick').addEventListener('click', pickOut);
  $('btnOutApply').addEventListener('click', () => applyOut());
  $('btnOutDefault').addEventListener('click', () => applyOut(''));
  $('outPath').addEventListener('keydown', (ev) => { if (ev.key === 'Enter') { ev.preventDefault(); applyOut(); } });
  $('setReset').addEventListener('click', () => { applySettings(S.defaults); saveSettings(); notify('info', 'Impostazioni consigliate ripristinate.'); });
  for (const id of ['setDriver', 'setDpi', 'setLen', 'setTop']) $(id).addEventListener('change', () => { if (typeof readSettings() !== 'string') saveSettings(); });
  $('zIn').addEventListener('click', () => zoomBy(1));
  $('zOut').addEventListener('click', () => zoomBy(-1));
  $('zFit').addEventListener('click', () => { S.anchor = null; S.zoom = 1; applyView(); });
  $('zRot').addEventListener('click', () => { S.rot = !S.rot; applyView(); });
  $('viewer').addEventListener('wheel', zoomWheel, { passive: false });
  $('btnCropAuto').addEventListener('click', cropAuto);
  $('btnCropManual').addEventListener('click', startCrop);
  $('btnCropApply').addEventListener('click', applyCrop);
  $('btnCropCancel').addEventListener('click', cancelCrop);
  $('btnCropAll').addEventListener('click', () => { if (S.crop) { Object.assign(S.crop, { x: 0, y: 0, w: 1, h: 1 }); if (S.crop.quad) S.crop.quad = quadOf(S.crop); drawCrop(); } });
  $('cropRect').addEventListener('change', () => { if (S.crop) { S.crop.lock = $('cropLock').checked; applyCropMode(); } });
  for (const el of [$('cq0'), $('cq1'), $('cq2'), $('cq3'), $('cqPoly')]) el.addEventListener('pointerdown', onQuadDown);
  $('cropLock').addEventListener('change', () => {
    const c = S.crop; if (!c) return;
    c.lock = $('cropLock').checked;
    if (c.lock && c.quad) { quadBox(c); c.quad = null; }
    if (c.lock) { const cx = c.x + c.w / 2; const cy = c.y + c.h / 2; c.h = c.w * c.ratioH; c.x = cx - c.w / 2; c.y = cy - c.h / 2; }
    clampCrop(); applyCropMode();
  });
  $('cropBox').addEventListener('pointerdown', onCropDown);
  $('cropBox').addEventListener('keydown', onCropKey);
  $('imgWrap').addEventListener('dblclick', zoomAt); // anche sopra il rettangolo di taglio
  $('imgWrap').addEventListener('pointerdown', onCropDraw);
  let fitRaf = 0; // la barra del taglio cambia l'altezza: l'immagine si adatta (una volta per fotogramma, niente cicli)
  new ResizeObserver(() => { cancelAnimationFrame(fitRaf); fitRaf = requestAnimationFrame(applyView); }).observe($('viewer'));
  $('preview').addEventListener('load', applyView);
  window.addEventListener('resize', applyView);

  // mai navigare fuori dalla pagina (un link o un file lasciato cadere sulla finestra la scaricherebbe e il server si spegnerebbe)
  document.addEventListener('click', (e) => {
    const a = e.target.closest && e.target.closest('a');
    if (a && !(a.getAttribute('href') || '').startsWith('#')) e.preventDefault();
  });
  for (const ev of ['dragover', 'drop']) window.addEventListener(ev, (e) => e.preventDefault());
  // una finestra tornata in primo piano dà subito un segno di vita (in secondo piano i timer rallentano)
  document.addEventListener('visibilitychange', () => { if (!document.hidden) heartbeat(); });
  // gli errori della pagina finiscono nel registro del programma (dati\panamabook.log) e si vedono
  let reported = 0;
  function reportError(msg) {
    if (reported++ > 5) return;
    notify('error', 'Errore nella pagina: ' + msg + ' (annotato in dati\\panamabook.log).', { sticky: true });
    api('/api/clientlog', { method: 'POST', body: { msg: String(msg).slice(0, 500) } }).catch(() => {});
  }
  window.addEventListener('error', (e) => reportError((e.message || 'errore') + (e.lineno ? ' (riga ' + e.lineno + ')' : '')));
  window.addEventListener('unhandledrejection', (e) => reportError((e.reason && e.reason.message) || String(e.reason)));

  window.addEventListener('pagehide', () => { try { navigator.sendBeacon('/api/bye?t=' + encodeURIComponent(TOKEN), new Blob(['{}'], { type: 'application/json' })); } catch (_) { /* ignora */ } });
  window.addEventListener('beforeunload', (e) => { if (S.busy || dirtyCount() || S.previews.length) { e.preventDefault(); e.returnValue = ''; } });

  initCopyBar();
  init();
})();
