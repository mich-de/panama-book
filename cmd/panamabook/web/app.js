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
  const S = { doc: null, cur: null, previews: [], edits: {}, rev: {}, scanning: false, busy: false, demo: false, defaults: { driver: 'wia', dpi: 300, len_mm: 190, top_margin_mm: 2 }, info: null, view: 'full',
              zoom: 1, rot: false, scans: [], pdfBusy: false, saving: false };
  const ZOOMS = [1, 1.5, 2, 3, 4];

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
    if (kind === 'ok' && !opts.sticky) setTimeout(() => el.remove(), 15000);
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
  function loadSettings() {
    let s = null;
    try { s = JSON.parse(localStorage.getItem('pbset3') || 'null'); } catch (_) { /* ignora */ }
    applySettings(Object.assign({}, S.defaults, s || {}));
  }
  function saveSettings() {
    try {
      localStorage.setItem('pbset3', JSON.stringify({ driver: $('setDriver').value, dpi: +$('setDpi').value, len_mm: +$('setLen').value, top_margin_mm: +$('setTop').value }));
    } catch (_) { /* ignora */ }
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
    const hist = $('history');
    if (!S.scans.length) { hist.replaceChildren(h('li', { class: 'muted' }, 'Nessuna scansione salvata.')); return; }
    hist.replaceChildren(...S.scans.slice(0, 8).map((it) => h('li', {},
      h('button', { type: 'button', class: 'btn link hname', title: 'Apri ' + it.file, onclick: () => openFile(it.file) }, it.name),
      it.saved ? h('span', { class: 'pill' }, 'salvato') : null,
      h('time', { datetime: it.time }, fmtTime(it.time)))));
  }

  // ---- anteprima ---------------------------------------------------------------------------------------------------
  function applyView() {
    const img = $('preview');
    img.style.width = (S.zoom * 100) + '%';
    img.style.transform = S.rot ? 'rotate(180deg)' : '';
    $('viewer').classList.toggle('zoomed', S.zoom > 1);
    $('zRot').setAttribute('aria-pressed', S.rot ? 'true' : 'false');
  }
  function zoomBy(dir) {
    const i = Math.max(0, ZOOMS.indexOf(S.zoom));
    S.zoom = ZOOMS[Math.min(ZOOMS.length - 1, Math.max(0, i + dir))];
    applyView();
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
  const VIEW_LABEL = { page: 'pagina dati ritagliata (è quella che verrà salvata)', full: 'scansione intera ritagliata', raw: 'scansione GREZZA (come esce dallo scanner)' };
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
    setImg(url, i.file + '-' + v + '-' + (i.time || '') + (i.page_url ? 'p' : ''));
    $('viewSwitch').hidden = false;
    $('vPage').hidden = !i.page_url; $('vRaw').hidden = !i.raw_url;
    for (const [id, name] of [['vPage', 'page'], ['vFull', 'full'], ['vRaw', 'raw']]) $(id).setAttribute('aria-pressed', v === name ? 'true' : 'false');
    const base = $('caption').dataset.base || '';
    $('caption').textContent = base + ' - vista: ' + VIEW_LABEL[v];
  }
  function setCaption(text) { $('caption').dataset.base = text; $('caption').textContent = text; }
  // diagnosi della posizione del libretto, dalla scansione grezza (senza OCR): solo suggerimenti
  function renderDiag() {
    const box = $('diagBox'); const i = S.info;
    box.replaceChildren();
    if (!i || !i.diag || !i.diag.done) { box.hidden = true; return; }
    box.hidden = false;
    const hints = i.diag.hints || [];
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
      S.zoom = 1; S.rot = false; applyView();
      setView(doc.page_url ? 'page' : 'full'); // dopo la lettura si vede quello che verrà salvato
    } else {
      S.info = null; $('viewSwitch').hidden = true;
      setImg(doc.image_url, doc.doc_id); S.zoom = 1; S.rot = false; applyView();
    }
    renderDiag(); renderKeep(); updateActions(); renderStrip();
  }
  // anteprima senza dati letti
  function showFile(p) {
    S.cur = { dir: 'preview', file: p.file };
    S.info = p;
    setCaption(p.file + ' - anteprima NON salvata (' + p.width + ' x ' + p.height + ' px)');
    S.zoom = 1; S.rot = false; applyView();
    setView(p.page_url ? 'page' : 'full'); // la sola pagina dati ritagliata, se trovata
    renderDiag(); renderKeep(); updateActions(); renderStrip();
  }
  function clearViewer() {
    S.cur = null; S.info = null; $('viewSwitch').hidden = true; $('diagBox').hidden = true; $('keepBox').hidden = true;
    const img = $('preview'); img.removeAttribute('src'); img.hidden = true; $('viewerEmpty').hidden = false; setCaption('');
    S.zoom = 1; S.rot = false; applyView(); updateActions(); renderStrip();
  }
  // dati letti: via, resta l'immagine
  function clearDoc() {
    S.doc = null; S.edits = {}; S.rev = {};
    $('results').hidden = true; $('savebar').hidden = true; $('emptyState').hidden = false;
    $('nameLine').hidden = true; $('fieldsBody').replaceChildren(); $('warns').replaceChildren(); $('mrzLines').textContent = '';
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
    $('btnRead').disabled = S.busy || !prev;
    $('btnRescan').disabled = S.busy;
    $('btnDiscard').disabled = S.busy || !prev;
    $('btnReread').disabled = S.busy || !S.cur || prev; // per le anteprime c'è "Leggi i dati"
    $('btnPdf').disabled = S.busy || S.pdfBusy || !S.cur;
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
    const ws = S.doc.warnings || [];
    $('warnCard').hidden = ws.length === 0;
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
  function refreshNameLine(now) {
    clearTimeout(nameTimer);
    if (!S.doc) return;
    nameTimer = setTimeout(async () => {
      const seq = ++nameSeq; const el = $('nameLine');
      try {
        const r = await api('/api/preview-name', { method: 'POST', body: { doc_id: S.doc.doc_id, values: currentValues(), fallback_name: $('scanName').value.trim() } });
        if (seq !== nameSeq || !S.doc) return;
        el.hidden = false; el.replaceChildren();
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
        if (e.altKey && !e.ctrlKey && (e.key === 'c' || e.key === 'C')) {
          e.preventDefault();
          if (f.flag && !isDirty(f)) setReviewed(f, !effReviewed(f), true);
        } else if (e.key === 'Enter' && !e.ctrlKey) {
          e.preventDefault();
          if (f.flag && !isDirty(f) && !effReviewed(f)) setReviewed(f, true, false); // Invio su un campo non modificato = controllato
          const i = inputs.indexOf(input); (inputs[i + 1] || $('btnConfirm')).focus();
        } else if (e.key === 'Enter' && e.ctrlKey) { e.preventDefault(); save('confirm'); }
      });
      inputs.push(input);
      const tr = h('tr', { dataset: { key: f.key } },
        h('th', { scope: 'row' },
          h('label', { for: id, class: 'flabel' }, f.es + ' / ', h('span', { class: 'en' }, f.en)),
          f.source ? h('span', { class: 'fsrc' }, f.source) : null),
        h('td', {}, input, h('div', { class: 'fnote', id: id + '-note' })),
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
    $('mrzChecks').replaceChildren(...m.checks.map((c) => h('li', { class: c.ok ? 'ok' : 'bad' },
      icon(c.ok ? 'check' : 'x'), c.label + ': ' + (c.ok ? 'corretto' : 'ERRATO'))));
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
    $('saveInfo').textContent = t;
    $('btnSaveCorr').disabled = S.saving || n === 0 || !usable;
    $('btnConfirm').disabled = S.saving;
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
      const resp = await api('/api/save', { method: 'POST', body: { doc_id: S.doc.doc_id, mode, values, reviewed, overwrite: !!opts.overwrite, accept_open: !!opts.acceptOpen, fallback_name: $('scanName').value.trim(), keep_full: $('keepFull').checked, keep_raw: $('keepRaw').checked } });
      const prevFile = S.doc.file; const prevDir = S.doc.dir;
      setDoc(resp.doc, false);
      if (resp.doc.file !== prevFile || resp.doc.dir !== prevDir) refreshImage(resp.doc);
      refreshPreviews();
      for (const m of resp.messages || []) notify(resp.memory_skipped ? 'warn' : 'ok', m);
      if (!(resp.messages || []).length) notify('ok', 'Salvato.');
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
    $('jobTitle').textContent = j.kind === 'preview' ? (j.demo ? 'Anteprima di PROVA' : 'Anteprima (scansione)') : j.kind === 'scan' ? 'Scansione e lettura' : 'Lettura dei dati';
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
      } else if (j.phase === 'done' && j.doc) {
        showImage(j.doc); setDoc(j.doc, false); $('fieldsCard').scrollIntoView({ block: 'nearest' }); refreshScans(); refreshPreviews();
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
      const r = await api('/api/pdf', { method: 'POST', body: { file: S.cur.file, dir: S.cur.dir, overwrite: !!overwrite } });
      const msg = 'PDF creato: ' + r.file + ', ' + r.kb + ' KB' + (r.info ? ' (' + r.info + ')' : '') + '.' + (r.preview ? ' (è un PDF dell\'anteprima: con «Conferma e salva» passa in scans con lo stesso nome.)' : '');
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
    S.defaults = st.defaults; S.demo = st.demo;
    if (st.demo) {
      $('demoBadge').hidden = false; $('btnScanText').textContent = 'Anteprima (PROVA)';
      $('demoNote').hidden = false;
      $('demoNote').textContent = 'PROVA: nessuno scanner viene usato. "Anteprima" usa l\'immagine ' + st.demo_name + ' come se fosse appena scansionata.';
    }
    if (!st.libretto_ok) notify('error', 'Manca ' + st.libretto_name + ' accanto a PanamaBook.exe: senza non si può scansionare ne\' leggere.', { sticky: true });
    $('scanName').value = st.suggested || suggestedName();
    loadSettings();
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
  for (const [id, name] of [['vPage', 'page'], ['vFull', 'full'], ['vRaw', 'raw']]) $(id).addEventListener('click', () => setView(name));
  $('btnDiscard').addEventListener('click', doDiscard);
  $('scanName').addEventListener('input', () => { $('scanName').removeAttribute('aria-invalid'); refreshNameLine(false); });
  $('btnOpen').addEventListener('click', () => { const v = $('openSelect').value; if (v) openFile(v); else { $('openSelect').focus(); notify('info', 'Scegli prima un\'immagine dall\'elenco.'); } });
  $('btnReread').addEventListener('click', readCurrent);
  $('btnPdf').addEventListener('click', () => exportPdf(false));
  $('btnReset').addEventListener('click', doReset);
  $('reviewPill').addEventListener('click', gotoNextOpen);
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
  $('setReset').addEventListener('click', () => { applySettings(S.defaults); saveSettings(); notify('info', 'Impostazioni consigliate ripristinate.'); });
  for (const id of ['setDriver', 'setDpi', 'setLen', 'setTop']) $(id).addEventListener('change', () => { if (typeof readSettings() !== 'string') saveSettings(); });
  $('zIn').addEventListener('click', () => zoomBy(1));
  $('zOut').addEventListener('click', () => zoomBy(-1));
  $('zFit').addEventListener('click', () => { S.zoom = 1; applyView(); });
  $('zRot').addEventListener('click', () => { S.rot = !S.rot; applyView(); });
  $('preview').addEventListener('click', () => { S.zoom = S.zoom > 1 ? 1 : 2; applyView(); });

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

  init();
})();
