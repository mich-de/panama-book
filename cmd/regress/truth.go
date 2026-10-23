package main

// Confronto con la verita' (flag -truth). Per l'immagine NOME.png si cerca DIR\NOME.json, scritto a occhio
// dall'agente verificatore (scans\verita\): contiene dati personali, quindi si usa solo in memoria e nella tabella
// restano simboli (G giusto, S sbagliato, M mancante, X letto ma vuoto in verita', - niente da confrontare), mai valori.
// Lettura del file e confronto dei valori sono quelli di internal/verify (verify.LoadTruth e verify.Compare): qui si
// trovano solo il file giusto e si traducono gli esiti in simboli.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"scanlibretto/internal/outsum"
	"scanlibretto/internal/verify"
)

// truthPrint e' l'impronta della cartella delle verita' (SHA-256 abbreviato di nomi e contenuto dei .json) e il
// numero di file. Va nell'intestazione della tabella: se la verita' cambia, i "giusti" di una baseline vecchia non
// sono piu' confrontabili con quelli di una prova nuova e il confronto lo segnala.
func truthPrint(dir string) (print string, n int, err error) {
	entries, err := os.ReadDir(dir) // ordinato per nome
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return "", 0, err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", e.Name(), len(b))
		h.Write(b)
		n++
	}
	return hex.EncodeToString(h.Sum(nil))[:12], n, nil
}

// loadTruth legge la verita' dell'immagine rel (DIR\NOME.json, NOME = nome dell'immagine senza estensione).
// ok=false se il file non esiste.
func loadTruth(dir, rel string) (truth verify.Truth, ok bool, err error) {
	base := path.Base(rel)
	truth, err = verify.LoadTruth(filepath.Join(dir, strings.TrimSuffix(base, path.Ext(base))+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return verify.Truth{}, false, nil
	}
	return truth, err == nil, err
}

// scoreTruth confronta i 14 campi letti con la verita' e restituisce un simbolo per campo, nell'ordine di
// outsum.FieldKeys, piu' il simbolo dei nomi: cognome e nomi della riga 1 dell'MRZ (che non ha check digit) contro la
// riga del nome scritta in verita' (G giusti, S sbagliati, M senza MRZ, 0 se la verita' non ha la riga del nome).
func scoreTruth(sum outsum.Summary, truth verify.Truth) (fields string, name byte) {
	res := verify.Verify("", sum, &truth)
	out := make([]byte, 0, outsum.NFields)
	for _, r := range res.Fields {
		switch r.Status {
		case verify.StatusOK:
			out = append(out, symRight)
		case verify.StatusWrong:
			out = append(out, symWrong)
		case verify.StatusMissing:
			out = append(out, symMissing)
		case verify.StatusExtra:
			out = append(out, symSpurious)
		default: // StatusNotPrinted
			out = append(out, symNA)
		}
	}
	switch res.Names {
	case verify.NameOK:
		name = symRight
	case verify.NameWrong:
		name = symWrong
	case verify.NameMissing:
		name = symMissing
	}
	return string(out), name
}
