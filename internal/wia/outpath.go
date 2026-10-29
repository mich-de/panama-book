package wia

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// checkOutPath verifica la destinazione PRIMA di toccare lo scanner (funzione pura, nessun hardware).
// Se il file esiste gia' e force e' falso rifiuta, citando il file e il flag -force.
// Una cartella al posto del file e' sempre rifiutata. La cartella di destinazione deve esistere.
func checkOutPath(path string, force bool) error {
	if path == "" {
		return errors.New("wia: OutPath obbligatorio")
	}
	dir := filepath.Dir(path)
	if st, err := os.Stat(dir); err != nil {
		return fmt.Errorf("cartella di destinazione %q non utilizzabile: %w", dir, err)
	} else if !st.IsDir() {
		return fmt.Errorf("%q non e' una cartella: impossibile salvare %q", dir, path)
	}
	st, err := os.Lstat(path)
	switch {
	case err == nil:
		if st.IsDir() {
			return fmt.Errorf("%q e' una cartella: indicare un file di destinazione", path)
		}
		if !force {
			return fmt.Errorf("il file %q esiste gia': per sostituirlo usare il flag -force", path)
		}
		return nil
	case errors.Is(err, os.ErrNotExist):
		return nil
	default:
		return fmt.Errorf("destinazione %q non verificabile: %w", path, err)
	}
}

// newTempName riserva un nome di file temporaneo casuale nella stessa cartella di out (stesso volume:
// il rename finale e' atomico) e lo restituisce SENZA il file, perche' WIA SaveFile non sovrascrive.
// Il nome e' casuale (os.CreateTemp), quindi due scansioni parallele non collidono.
func newTempName(out, ext string) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(out), "."+filepath.Base(out)+".*.wia."+ext)
	if err != nil {
		return "", fmt.Errorf("impossibile creare il file temporaneo accanto a %q: %w", out, err)
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

// tempFiles tiene traccia dei temporanei creati da una scansione per rimuoverli sempre in uscita.
type tempFiles []string

// new crea un nuovo nome temporaneo accanto a out e lo registra per la pulizia.
func (t *tempFiles) new(out, ext string) (string, error) {
	name, err := newTempName(out, ext)
	if err != nil {
		return "", err
	}
	*t = append(*t, name)
	return name, nil
}

// cleanup rimuove i temporanei ancora presenti (quelli gia' spostati con rename non esistono piu').
func (t *tempFiles) cleanup() {
	for _, n := range *t {
		_ = os.Remove(n)
	}
	*t = nil
}
