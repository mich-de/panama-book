'use strict';
/* Panama Book: logica PURA (nessun DOM, nessun appunti) per portare i campi nel gestionale con copia e incolla.
   La usa app.js (window.PBCopy) ed e' provata con "node --test" (cmd/panamabook/webtest/copy.test.mjs) e con il banco DevTools.
   Un "campo" e' { key, es, en, kind } come li manda il server (key = chiave inglese del JSON di libretto, kind "data" per le date);
   i "valori" sono una mappa key -> testo (quello CORRENTE della tabella, non quello letto). */
(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.PBCopy = api;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  // formati di "Copia tutto" (id + testo del menu)
  const FORMATS = [
    { id: 'label', text: 'Etichetta: valore (una riga per campo)' },
    { id: 'values', text: 'Solo i valori, uno per riga' },
    { id: 'tsv', text: 'Una riga con tabulazioni (per Excel e gestionale), nell\'ordine della tabella' },
    { id: 'json', text: 'JSON dei campi (chiavi inglesi, come libretto)' },
  ];
  // formato delle date: come stampate sul libretto o ISO
  const DATE_MODES = [
    { id: 'print', text: 'GG-MM-AAAA (come stampato)' },
    { id: 'iso', text: 'AAAA-MM-GG (ISO)' },
  ];
  const DEFAULTS = { format: 'label', dates: 'print' };
  const DATE_RE = /^(\d{2})-(\d{2})-(\d{4})$/;

  const has = (list, id) => list.some((x) => x.id === id);

  // normalizePrefs rende valide le preferenze lette da localStorage (valori sconosciuti -> predefiniti)
  function normalizePrefs(p) {
    const o = p && typeof p === 'object' ? p : {};
    return { format: has(FORMATS, o.format) ? o.format : DEFAULTS.format, dates: has(DATE_MODES, o.dates) ? o.dates : DEFAULTS.dates };
  }

  // cleanValue: testo su una riga sola, spazi singoli (tabulazioni e a capo dentro un valore romperebbero righe e colonne)
  function cleanValue(v) {
    return String(v === null || v === undefined ? '' : v).replace(/\s+/g, ' ').trim();
  }

  // formatDate converte GG-MM-AAAA in AAAA-MM-GG con mode "iso"; ogni altro testo resta com'e'
  function formatDate(v, mode) {
    if (mode === 'iso') {
      const m = DATE_RE.exec(v);
      if (m) return m[3] + '-' + m[2] + '-' + m[1];
    }
    return v;
  }

  // outValue e' il valore da copiare per un campo: ripulito e, per le date, nel formato scelto
  function outValue(field, raw, opts) {
    const v = cleanValue(raw);
    return field && field.kind === 'data' ? formatDate(v, opts && opts.dates) : v;
  }

  const labelOf = (field) => field.es + ' / ' + field.en;

  // fieldText: il testo che copia il pulsante "Copia" di una riga (solo il valore). '' = campo vuoto, niente da copiare
  function fieldText(field, raw, opts) {
    return outValue(field, raw, opts);
  }

  // formatFields: il testo di "Copia tutto". '' se tutti i campi sono vuoti (niente da copiare).
  //   label  -> "Etichetta: valore", una riga per campo (anche i vuoti, cosi' si vede cosa manca)
  //   values -> solo i valori, uno per riga (i vuoti restano righe vuote: le posizioni non cambiano)
  //   tsv    -> UNA riga, valori separati da tabulazione nell'ordine della tabella (i vuoti restano celle vuote)
  //   json   -> oggetto con le chiavi inglesi di libretto; i campi vuoti non ci sono (come nell'uscita di libretto)
  function formatFields(fields, values, format, opts) {
    const vals = fields.map((f) => outValue(f, values && values[f.key], opts));
    if (vals.every((v) => v === '')) return '';
    switch (format) {
      case 'values':
        return vals.join('\n');
      case 'tsv':
        return vals.join('\t');
      case 'json': {
        const o = {};
        fields.forEach((f, i) => { if (vals[i] !== '') o[f.key] = vals[i]; });
        return JSON.stringify(o, null, 2);
      }
      default: // 'label'
        return fields.map((f, i) => (labelOf(f) + ': ' + vals[i]).trimEnd()).join('\n');
    }
  }

  // mrzText: le righe MRZ come lette (44 caratteri ciascuna). which: 1, 2 oppure 'both'. '' se manca la riga.
  function mrzText(mrz, which) {
    const l1 = mrz && mrz.line1 ? String(mrz.line1) : '';
    const l2 = mrz && mrz.line2 ? String(mrz.line2) : '';
    if (which === 1 || which === '1') return l1;
    if (which === 2 || which === '2') return l2;
    return l1 && l2 ? l1 + '\n' + l2 : '';
  }

  // fileNameText: il nome base dei file che servira' dopo la conferma ("NOME - COGNOME - NUMERO"), da /api/preview-name.
  // Se i dati non bastano (ok falso) e' il nome di riserva o quello attuale. '' se non si sa ancora.
  function fileNameText(info) {
    if (!info) return '';
    return cleanValue(info.ok ? info.name : (info.fallback || info.current || ''));
  }

  return { FORMATS, DATE_MODES, DEFAULTS, normalizePrefs, cleanValue, formatDate, outValue, labelOf, fieldText, formatFields, mrzText, fileNameText };
});
