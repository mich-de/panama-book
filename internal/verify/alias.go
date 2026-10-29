package verify

import "scanlibretto/internal/outsum"

// Il campo `Nº Título / Certificate` si chiamava `endorsement` nelle prime versioni dell'uscita di libretto e delle
// verita' (da "Refrendo / Endorsement", l'etichetta che compare su un altro modello). Il nome del JSON segue l'etichetta
// stampata dell'Anexo 8: `certificate`. Durante il passaggio le due chiavi sono la stessa cosa: si legge l'una o
// l'altra, cosi' un libretto.exe vecchio resta verificabile e una verita' vecchia resta valida.
const (
	// FieldCertificate e' il nome JSON del campo `Nº Título / Certificate`.
	FieldCertificate = "certificate"
	// fieldEndorsement e' il vecchio nome, ancora accettato in lettura.
	fieldEndorsement = "endorsement"
)

// aliasesOf dice con quali altri nomi puo' comparire un campo nei JSON.
func aliasesOf(field string) []string {
	switch field {
	case FieldCertificate:
		return []string{fieldEndorsement}
	case fieldEndorsement:
		return []string{FieldCertificate}
	}
	return nil
}

// Canonical restituisce il nome attuale di un campo: `certificate` per il vecchio `endorsement`, gli altri invariati.
// I report stampano sempre il nome attuale.
func Canonical(field string) string {
	if field == fieldEndorsement {
		return FieldCertificate
	}
	return field
}

// lookup restituisce il valore di key in m; se manca o e' vuoto prova i nomi alternativi (vedi aliasesOf).
func lookup(m map[string]string, key string) string {
	if v := m[key]; v != "" {
		return v
	}
	for _, a := range aliasesOf(key) {
		if v := m[a]; v != "" {
			return v
		}
	}
	return ""
}

// normalizeKeys fa trovare i valori sotto i nomi di outsum.FieldKeys: se un campo e' scritto con il nome alternativo,
// il valore viene copiato anche sotto il nome in uso. Cosi' i metodi di outsum.Summary (Found, Present, Missing)
// contano giusto anche con un JSON vecchio o con l'elenco di outsum gia' rinominato.
func normalizeKeys(m map[string]string) {
	for _, k := range outsum.FieldKeys {
		if m[k] == "" {
			if v := lookup(m, k); v != "" {
				m[k] = v
			}
		}
	}
}

// Field restituisce il valore di un campo della verita' (accetta anche il vecchio nome `endorsement`).
func (t Truth) Field(name string) string { return lookup(t.Fields, name) }
