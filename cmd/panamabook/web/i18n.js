'use strict';
/* Panama Book: lingue dell'interfaccia (inglese predefinita, spagnolo, italiano).
   Il testo della pagina e' scritto in italiano (index.html, app.js, messaggi del programma); qui lo si traduce DAL VIVO guardando
   la pagina: ogni testo (e title, aria-label, placeholder, alt) viene cercato in i18n-data.js (frasi esatte, poi modelli con
   numeri e nomi, poi frase per frase, poi pezzo per pezzo separati da " - "). Cio' che non si trova resta in italiano e, con
   la lingua diversa dall'italiano, finisce nell'elenco window.PBI18n.missing() (serve a chi sviluppa).
   I valori letti dal libretto non si toccano mai. */
(() => {
  const LANGS = ['en', 'es', 'it'];
  const LOCALES = { en: 'en-GB', es: 'es-PA', it: 'it-IT' };
  const DATA = window.PBI18nData || { exact: [], patterns: [] };
  const ATTRS = ['title', 'aria-label', 'placeholder', 'alt'];
  const SKIP_TAGS = new Set(['SCRIPT', 'STYLE', 'TEXTAREA', 'PRE', 'SVG']);

  let lang = 'en';
  try { const l = localStorage.getItem('pblang'); if (LANGS.includes(l)) lang = l; } catch (_) { /* senza storage: inglese */ }

  const EX = { en: new Map(), es: new Map() };
  for (const [it, en, es] of DATA.exact) { EX.en.set(it, en); EX.es.set(it, es); }
  const PATS = DATA.patterns.map(([re, en, es]) => ({ re, en, es }));

  const missing = new Set();
  // serve solo a chi sviluppa: nomi di file, parole sole e simili non contano come "testo italiano non tradotto"
  const looksItalian = (t) => /[a-zà-ù]{3,}/.test(t) && !/\.(png|jpg|jpeg|json|pdf)/i.test(t) && !/^[\w-]+( \([^)]*\))?$/.test(t);

  // frase esatta o modello; null se non c'e'
  function tx(t, depth) {
    const e = EX[lang].get(t);
    if (e !== undefined) return e;
    for (const p of PATS) {
      const m = p.re.exec(t);
      if (m) return p[lang].replace(/\{(\d)\}/g, (_, i) => { const g = m[+i]; return g === undefined ? '' : inner(g, depth + 1); });
    }
    return null;
  }
  // un pezzo catturato da un modello: si traduce se si sa, altrimenti resta com'e' (nomi, numeri, valori letti)
  function inner(g, depth) {
    if (depth > 3 || !g.trim()) return g;
    const lead = /^\s*/.exec(g)[0]; const trail = /\s*$/.exec(g)[0];
    const r = piece(g.trim(), depth);
    return r === null ? g : lead + r + trail;
  }
  // pezzo senza segnalare i mancanti: frase esatta/modello, poi pezzi separati da " - "
  function piece(t, depth) {
    const r = tx(t, depth);
    if (r !== null) return r;
    const segs = t.split(' - ');
    if (segs.length < 2) return null;
    let any = false;
    const out = segs.map((s) => { const x = tx(s.trim(), depth); if (x === null) return s; any = true; return x; });
    return any ? out.join(' - ') : null;
  }
  function translate(src) {
    if (lang === 'it') return src;
    const t = src.trim();
    if (!t) return src;
    const lead = /^\s*/.exec(src)[0]; const trail = /\s*$/.exec(src)[0];
    let out = piece(t, 0);
    if (out === null) { // frase per frase
      const parts = t.split(/(?<=[.!?…])\s+(?=[A-ZÀ-ÖØ-Þ«"(¿¡])/);
      if (parts.length > 1) {
        let any = false;
        const o = parts.map((s) => { const x = piece(s, 0); if (x === null) { if (looksItalian(s)) missing.add(s); return s; } any = true; return x; });
        if (any) out = o.join(' ');
      }
    }
    if (out === null) { if (looksItalian(t)) missing.add(t); return src; }
    return lead + out + trail;
  }

  // ---- la pagina ----------------------------------------------------------------------------------------------
  const textRec = new WeakMap(); // nodo di testo -> { src, out }
  const attrRec = new WeakMap(); // elemento -> { attr: { src, out } }

  function skipEl(el) {
    for (let e = el; e && e.nodeType === 1; e = e.parentElement) {
      if (SKIP_TAGS.has(e.tagName.toUpperCase())) return true;
      if (e.classList && (e.classList.contains('flabel') || e.classList.contains('no-i18n'))) return true;
    }
    return false;
  }
  function doText(node) {
    const v = node.nodeValue;
    const rec = textRec.get(node);
    if (rec && v === rec.out) return;
    if (!node.parentElement || skipEl(node.parentElement)) return;
    const out = translate(v);
    textRec.set(node, { src: v, out });
    if (out !== v) node.nodeValue = out;
  }
  function doAttr(el, name) {
    const v = el.getAttribute(name);
    if (v === null) return;
    let recs = attrRec.get(el);
    if (!recs) { recs = {}; attrRec.set(el, recs); }
    const rec = recs[name];
    if (rec && v === rec.out) return;
    if (skipEl(el)) return;
    const out = translate(v);
    recs[name] = { src: v, out };
    if (out !== v) el.setAttribute(name, out);
  }
  function walk(root) {
    if (root.nodeType === 3) { doText(root); return; }
    if (root.nodeType !== 1 && root.nodeType !== 9 && root.nodeType !== 11) return;
    if (root.nodeType === 1) for (const a of ATTRS) doAttr(root, a);
    const tw = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT);
    for (let n = tw.nextNode(); n; n = tw.nextNode()) {
      if (n.nodeType === 3) doText(n);
      else for (const a of ATTRS) doAttr(n, a);
    }
  }
  // cambio di lingua: si riparte dai testi originali (italiano) di ogni nodo gia' visto
  function retranslateAll() {
    const tw = document.createTreeWalker(document.documentElement, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT);
    for (let n = tw.nextNode(); n; n = tw.nextNode()) {
      if (n.nodeType === 3) {
        const rec = textRec.get(n);
        if (rec && n.nodeValue === rec.out) { const out = translate(rec.src); rec.out = out; if (n.nodeValue !== out) n.nodeValue = out; } else doText(n);
      } else {
        const recs = attrRec.get(n);
        for (const a of ATTRS) {
          const rec = recs && recs[a];
          if (rec && n.getAttribute(a) === rec.out) { const out = translate(rec.src); rec.out = out; if (n.getAttribute(a) !== out) n.setAttribute(a, out); } else doAttr(n, a);
        }
      }
    }
  }

  const mo = new MutationObserver((list) => {
    for (const m of list) {
      if (m.type === 'characterData') doText(m.target);
      else if (m.type === 'attributes') doAttr(m.target, m.attributeName);
      else for (const n of m.addedNodes) walk(n);
    }
  });
  mo.observe(document.documentElement, { subtree: true, childList: true, characterData: true, attributes: true, attributeFilter: ATTRS });

  document.documentElement.lang = lang;
  if (lang !== 'it') document.documentElement.classList.add('i18n-wait'); // la pagina e' scritta in italiano: non si mostra prima di tradurla
  const ready = () => { walk(document.documentElement); document.documentElement.classList.remove('i18n-wait'); };
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', ready); else ready();
  setTimeout(() => document.documentElement.classList.remove('i18n-wait'), 1500); // in ogni caso la pagina si vede

  window.PBI18n = {
    LANGS,
    get lang() { return lang; },
    locale: () => LOCALES[lang],
    setLang(l) {
      if (!LANGS.includes(l) || l === lang) return;
      lang = l;
      try { localStorage.setItem('pblang', l); } catch (_) { /* ignora */ }
      document.documentElement.lang = l;
      retranslateAll();
    },
    tr: (s) => translate(s),
    missing: () => Array.from(missing),
  };
})();
