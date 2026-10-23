//go:build windows

// Package wia acquisisce da uno scanner Fujitsu/Ricoh fi-800R tramite Windows Image Acquisition (COM).
//
// Dal driver (WIA 2.0, versione 2.0.0.2) risulta: Document Handling Capabilities = 5 (alimentatore + duplex),
// nessun flag "solo fronte". Per leggere una sola facciata si imposta Document Handling Select = 1
// (solo alimentatore, senza il bit duplex 4): una passata, un solo lato.
//
// Note COM: ogni operazione pubblica (Describe, List, Dump, Scan) fissa il goroutine a un thread del
// sistema operativo (runtime.LockOSThread), inizializza un apartment STA su quel thread e rilascia tutti
// i riferimenti COM prima di CoUninitialize. Le funzioni sono quindi usabili da qualunque goroutine.
//
// NON VERIFICATO (serve carta nello scanner): che WIA scelga da solo il percorso fisico corretto
// (alimentatore dall'alto con percorso a U, oppure "Return Scan" frontale per libretti fino a 5 mm)
// in base a dove viene inserito il documento.
package wia

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"

	"scanlibretto/internal/imgproc"
)

const (
	formatPNG = "{B96B3CAF-0728-11D3-9D7B-0000F81EF32E}"

	sFalse           = 1          // S_FALSE: COM gia' inizializzato sul thread in modo compatibile
	rpcEChangedMode  = 0x80010106 // RPC_E_CHANGED_MODE: thread gia' inizializzato con un altro modello
	connectAttempts  = 3
	connectRetryWait = 1500 * time.Millisecond
)

// Info riassume cosa espone il driver, per diagnostica.
type Info struct {
	Name, Manufacturer, DriverVersion, Port string
	HandlingCapabilities                    int
	HandlingSelect                          int
	HandlingStatus                          int
	DPIMin, DPIMax                          int
}

// DeviceInfo e' una voce dell'elenco dei dispositivi WIA visibili.
type DeviceInfo struct {
	ID           string
	Name         string
	Description  string
	Manufacturer string
	Port         string
}

// Prop e' una proprieta' WIA (diagnostica).
type Prop struct {
	Name     string
	ID       int
	Value    string
	ReadOnly bool
	Min, Max string // SubTypeMin/SubTypeMax se presenti
}

// List elenca i dispositivi WIA visibili senza connettersi a nessuno e senza scansionare.
func List() ([]DeviceInfo, error) {
	var out []DeviceInfo
	err := runWithTimeout(defaultTimeout, func() error {
		return withManager(func(mgr *ole.IDispatch) error {
			return eachDeviceInfo(mgr, func(di *ole.IDispatch) (bool, error) {
				d := DeviceInfo{
					Name:         str(getProp(di, "Name")),
					Description:  str(getProp(di, "Description")),
					Manufacturer: str(getProp(di, "Manufacturer")),
					Port:         str(getProp(di, "Port")),
				}
				if v, err := oleutil.GetProperty(di, "DeviceID"); err == nil {
					d.ID = str(v.Value())
					v.Clear()
				}
				out = append(out, d)
				return false, nil
			})
		})
	})
	return out, err
}

// Describe si connette al dispositivo e ne legge le proprieta' chiave, senza scansionare.
func Describe(match string) (Info, error) {
	var info Info
	err := runWithTimeout(defaultTimeout, func() error {
		return withDevice(match, func(dev, item *ole.IDispatch) error {
			info.Name = str(getProp(dev, "Name"))
			info.Manufacturer = str(getProp(dev, "Manufacturer"))
			info.DriverVersion = str(getProp(dev, "Driver Version"))
			info.Port = str(getProp(dev, "Port"))
			info.HandlingCapabilities = num(getProp(dev, "Document Handling Capabilities"))
			info.HandlingSelect = num(getProp(dev, "Document Handling Select"))
			info.HandlingStatus = num(getProp(dev, "Document Handling Status"))
			info.DPIMin, info.DPIMax = subRange(item, "Horizontal Resolution")
			return nil
		})
	})
	return info, err
}

// Dump legge (sola lettura) tutte le proprieta' del dispositivo e del primo item di scansione.
func Dump(match string) (device, item []Prop, err error) {
	err = runWithTimeout(defaultTimeout, func() error {
		return withDevice(match, func(dev, it *ole.IDispatch) error {
			device = listProps(dev)
			item = listProps(it)
			return nil
		})
	})
	return
}

// Scan esegue una scansione di UNA sola facciata e salva un PNG in opts.OutPath.
// Il file prodotto e' sempre PNG alla risoluzione richiesta (nessuna post-elaborazione).
func Scan(opts Options) error {
	opts, err := opts.normalized()
	if err != nil {
		return err
	}
	// prima di qualunque contatto con lo scanner: niente sovrascritture silenziose
	if err := checkOutPath(opts.OutPath, opts.Force); err != nil {
		return err
	}
	// e cartella scrivibile: meglio scoprirlo ora che dopo aver consumato la carta
	if _, err := newTempName(opts.OutPath, "probe"); err != nil { // il nome riservato non lascia file
		return err
	}
	return runWithTimeout(opts.Timeout, func() error {
		return withDevice(opts.DeviceMatch, func(dev, item *ole.IDispatch) error {
			return scanItem(dev, item, opts)
		})
	})
}

func scanItem(dev, item *ole.IDispatch, opts Options) error {
	// 1. un solo lato: alimentatore senza duplex, verificato in rilettura PRIMA di toccare altro
	if err := setProp(dev, "Document Handling Select", dpsDocumentHandlingFeeder); err != nil {
		return explain("impossibile selezionare 'solo alimentatore' (simplex)", err)
	}
	selV, selErr := getPropErr(dev, "Document Handling Select")
	if err := checkSimplex(num(selV), selErr == nil && isNum(selV)); err != nil {
		return err
	}
	stV, stErr := getPropErr(dev, "Document Handling Status")
	if stErr != nil || !isNum(stV) {
		return errors.New("Document Handling Status non leggibile: scansione annullata")
	}
	if st := num(stV); st&statusFeedReady == 0 {
		return fmt.Errorf("alimentatore non pronto (Document Handling Status=%d)", st)
	}
	// una sola pagina: con Pages=0 il driver potrebbe consumare tutto il caricatore
	if err := setProp(dev, "Pages", 1); err != nil {
		if pv, perr := getPropErr(dev, "Pages"); perr == nil && num(pv) == 1 {
			// gia' a 1 (proprieta' non scrivibile): accettabile
		} else {
			return explain("impossibile limitare la scansione a 1 pagina (Pages)", err)
		}
	}

	// 2. parametri dell'item
	minDPI, maxDPI := subRange(item, "Horizontal Resolution")
	if err := checkDPI(opts.DPI, minDPI, maxDPI); err != nil {
		return err
	}
	for _, n := range []string{"Horizontal Resolution", "Vertical Resolution"} {
		if err := setProp(item, n, opts.DPI); err != nil {
			return explain(n, err)
		}
	}
	dt := dataTypeColor
	if opts.Gray {
		dt = dataTypeGray
	}
	if err := setProp(item, "Data Type", dt); err != nil {
		return explain("data type", err)
	}
	// area massima: dopo il cambio di risoluzione il driver ricalcola i limiti.
	// Gli errori qui non sono fatali (il driver puo' imporre l'area da solo) ma si segnalano in debug.
	_ = setProp(item, "Horizontal Start Position", 0)
	_ = setProp(item, "Vertical Start Position", 0)
	for _, n := range []string{"Horizontal Extent", "Vertical Extent"} {
		if _, hi := subRange(item, n); hi > 0 {
			_ = setProp(item, n, hi)
		}
	}
	// area limitata (solo se richiesta): ogni errore qui e' fatale e si controlla prima del trasferimento
	if opts.WidthMM > 0 || opts.HeightMM > 0 || opts.StartMM > 0 {
		if err := limitArea(item, opts); err != nil {
			return err
		}
	}

	// 3. trasferimento (UNICO punto in cui lo scanner acquisisce)
	imgVar, err := oleutil.CallMethod(item, "Transfer", formatPNG)
	if err != nil {
		return explain("trasferimento WIA", err)
	}
	imgDisp, err := dispatchOf(imgVar)
	if err != nil {
		return fmt.Errorf("trasferimento WIA: risposta inattesa: %w", err)
	}
	defer imgDisp.Release() // Release equivale a imgVar.Clear(): non fare entrambe (doppio rilascio)

	extV, err := oleutil.GetProperty(imgDisp, "FileExtension")
	if err != nil {
		return explain("estensione dell'immagine acquisita", err)
	}
	ext := strings.ToLower(strings.TrimPrefix(str(extV.Value()), "."))
	extV.Clear()
	if ext == "" {
		ext = "bmp"
	}

	// SaveFile non sovrascrive e vuole l'estensione coerente con il formato: file temporaneo casuale
	// nella cartella di destinazione, rimosso sempre in uscita (successo o errore)
	var temps tempFiles
	defer temps.cleanup()
	tmp, err := temps.new(opts.OutPath, ext)
	if err != nil {
		return err
	}
	if _, err := oleutil.CallMethod(imgDisp, "SaveFile", tmp); err != nil {
		return explain("salvataggio immagine", err)
	}
	return finalizeImage(&temps, tmp, ext, opts.OutPath, opts.Force)
}

// itemProps adatta un item WIA a areaProps (la logica e l'ordine delle scritture sono in applyArea).
type itemProps struct{ item *ole.IDispatch }

func (i itemProps) Get(name string) (int, error) {
	v, err := getPropErr(i.item, name)
	if err != nil {
		return 0, err
	}
	if !isNum(v) {
		return 0, fmt.Errorf("valore non numerico (%v)", v)
	}
	return num(v), nil
}

func (i itemProps) Set(name string, v int) error {
	if err := setProp(i.item, name, v); err != nil {
		return explain(name, err)
	}
	return nil
}

func (i itemProps) Range(name string) (lo, hi int) { return subRange(i.item, name) }

// limitArea restringe l'area di scansione sull'item reale (vedi applyArea per l'ordine delle scritture).
func limitArea(item *ole.IDispatch, opts Options) error {
	return applyArea(itemProps{item}, opts)
}

// finalizeImage porta il file salvato da WIA al PNG di destinazione.
// Se il driver ha restituito BMP (formato preferito) lo converte; altri formati non sono supportati.
// Prima del rename ricontrolla la destinazione (un file potrebbe essere comparso durante la scansione).
// I temporanei restano registrati in temps: li rimuove il chiamante con cleanup.
func finalizeImage(temps *tempFiles, tmp, ext, out string, force bool) error {
	switch ext {
	case "png":
		if err := checkOutPath(out, force); err != nil {
			return err
		}
		return os.Rename(tmp, out) // MoveFileEx con sostituzione: consentita solo se force
	case "bmp":
		data, err := os.ReadFile(tmp)
		if err != nil {
			return err
		}
		img, err := decodeBMP(data)
		if err != nil {
			return err
		}
		tmpPNG, err := temps.new(out, "png")
		if err != nil {
			return err
		}
		if err := imgproc.SavePNG(img, tmpPNG); err != nil {
			return err
		}
		if err := checkOutPath(out, force); err != nil {
			return err
		}
		return os.Rename(tmpPNG, out)
	}
	return fmt.Errorf("formato immagine %q restituito dal driver non supportato (attesi png o bmp)", ext)
}

// withManager inizializza COM sul thread corrente, crea il WIA.DeviceManager e passa il dispatch a fn.
func withManager(fn func(mgr *ole.IDispatch) error) error {
	// STA: tutte le chiamate devono restare sullo stesso thread OS per tutta la durata
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		code, _ := hresultOf(err)
		switch code {
		case sFalse:
			defer ole.CoUninitialize() // S_FALSE e' un successo e va bilanciato
		case rpcEChangedMode:
			// il thread ha gia' COM in un'altra modalita': si usa cosi' com'e', senza CoUninitialize
		default:
			return explain("COM", err)
		}
	} else {
		defer ole.CoUninitialize()
	}

	unk, err := oleutil.CreateObject("WIA.DeviceManager")
	if err != nil {
		return explain("WIA non disponibile (servizio Acquisizione di immagini di Windows)", err)
	}
	defer unk.Release()
	mgr, err := unk.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return explain("WIA.DeviceManager", err)
	}
	defer mgr.Release()
	return fn(mgr)
}

// eachDeviceInfo scorre DeviceInfos (indici da 1); fn riceve ogni voce (rilasciata dopo la chiamata)
// e restituisce true per fermarsi.
func eachDeviceInfo(mgr *ole.IDispatch, fn func(di *ole.IDispatch) (stop bool, err error)) error {
	infos, err := getDisp(mgr, "DeviceInfos")
	if err != nil {
		return explain("elenco dispositivi", err)
	}
	defer infos.Release()
	cv, err := oleutil.GetProperty(infos, "Count")
	if err != nil {
		return explain("elenco dispositivi", err)
	}
	count := num(cv.Value())
	cv.Clear()
	for i := 1; i <= count; i++ { // le collezioni WIA partono da 1; Item e' una proprieta' (propget), non un metodo
		di, err := getDisp(infos, "Item", i)
		if err != nil {
			return explain(fmt.Sprintf("dispositivo %d", i), err)
		}
		stop, ferr := fn(di)
		di.Release()
		if ferr != nil || stop {
			return ferr
		}
	}
	return nil
}

// withDevice trova il dispositivo, lo connette e passa device e primo item a fn.
func withDevice(match string, fn func(dev, item *ole.IDispatch) error) error {
	if match == "" {
		match = defaultDevice
	}
	return withManager(func(mgr *ole.IDispatch) error {
		var names []string
		var chosen *ole.IDispatch
		err := eachDeviceInfo(mgr, func(di *ole.IDispatch) (bool, error) {
			name := strings.TrimSpace(str(getProp(di, "Name")) + " " + str(getProp(di, "Description")))
			names = append(names, name)
			if strings.Contains(strings.ToLower(name), strings.ToLower(match)) {
				di.AddRef() // lo teniamo oltre la callback
				chosen = di
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			return err
		}
		if chosen == nil {
			return fmt.Errorf("nessuno scanner WIA contiene %q (trovati: %q)", match, names)
		}
		defer chosen.Release()

		var dev *ole.IDispatch
		for attempt := 1; ; attempt++ {
			devVar, cerr := oleutil.CallMethod(chosen, "Connect")
			if cerr == nil {
				if dev, cerr = dispatchOf(devVar); cerr == nil {
					break
				}
			}
			if attempt >= connectAttempts || !isRetryable(cerr) {
				return explain("connessione allo scanner", cerr)
			}
			time.Sleep(connectRetryWait) // occupato o in riscaldamento: attesa breve, solo per Connect
		}
		defer dev.Release()

		items, err := getDisp(dev, "Items")
		if err != nil {
			return explain("item di scansione", err)
		}
		defer items.Release()
		cv, err := oleutil.GetProperty(items, "Count")
		if err != nil {
			return explain("item di scansione", err)
		}
		n := num(cv.Value())
		cv.Clear()
		if n < 1 {
			return errors.New("il driver non espone nessun item di scansione")
		}
		item, err := getDisp(items, "Item", 1)
		if err != nil {
			return explain("item di scansione", err)
		}
		defer item.Release()

		return fn(dev, item)
	})
}

// dispatchOf prende possesso del riferimento IDispatch contenuto nel VARIANT (da rilasciare con Release,
// senza Clear sul VARIANT). Se il VARIANT non e' un dispatch lo libera e restituisce errore.
func dispatchOf(v *ole.VARIANT) (*ole.IDispatch, error) {
	if v == nil {
		return nil, errors.New("risultato COM nullo")
	}
	d := v.ToIDispatch()
	if d == nil {
		vt := v.VT
		v.Clear()
		return nil, fmt.Errorf("risultato COM non e' un oggetto (VT=%d)", vt)
	}
	return d, nil
}

// getDisp legge una proprieta' che restituisce un oggetto (da rilasciare con Release).
func getDisp(obj *ole.IDispatch, name string, params ...interface{}) (*ole.IDispatch, error) {
	v, err := oleutil.GetProperty(obj, name, params...)
	if err != nil {
		return nil, err
	}
	return dispatchOf(v)
}

// prop restituisce l'oggetto Property con quel nome (da rilasciare con Release).
func prop(obj *ole.IDispatch, name string) (*ole.IDispatch, error) {
	props, err := getDisp(obj, "Properties")
	if err != nil {
		return nil, err
	}
	defer props.Release()
	p, err := getDisp(props, "Item", name)
	if err != nil {
		return nil, fmt.Errorf("proprieta' %q: %w", name, err)
	}
	return p, nil
}

// getPropErr legge il valore di una proprieta' WIA.
func getPropErr(obj *ole.IDispatch, name string) (interface{}, error) {
	p, err := prop(obj, name)
	if err != nil {
		return nil, err
	}
	defer p.Release()
	v, err := oleutil.GetProperty(p, "Value")
	if err != nil {
		return nil, err
	}
	defer v.Clear()
	return v.Value(), nil
}

// getProp come getPropErr ma restituisce nil in caso di errore (uso diagnostico).
func getProp(obj *ole.IDispatch, name string) interface{} {
	v, _ := getPropErr(obj, name)
	return v
}

func setProp(obj *ole.IDispatch, name string, value interface{}) error {
	p, err := prop(obj, name)
	if err != nil {
		return err
	}
	defer p.Release()
	v, err := oleutil.PutProperty(p, "Value", value)
	if v != nil {
		v.Clear()
	}
	return err
}

// subRange legge SubTypeMin/Max di una proprieta' con intervallo; (0,0) se non disponibile.
func subRange(obj *ole.IDispatch, name string) (lo, hi int) {
	p, err := prop(obj, name)
	if err != nil {
		return 0, 0
	}
	defer p.Release()
	lo = num(variantValue(oleutil.GetProperty(p, "SubTypeMin")))
	hi = num(variantValue(oleutil.GetProperty(p, "SubTypeMax")))
	return
}

func variantValue(v *ole.VARIANT, err error) interface{} {
	if err != nil || v == nil {
		return nil
	}
	defer v.Clear()
	if v.VT == ole.VT_DISPATCH {
		return nil
	}
	return v.Value()
}

// listProps elenca le proprieta' di un oggetto WIA (solo lettura).
func listProps(obj *ole.IDispatch) []Prop {
	props, err := getDisp(obj, "Properties")
	if err != nil {
		return nil
	}
	defer props.Release()
	cv, err := oleutil.GetProperty(props, "Count")
	if err != nil {
		return nil
	}
	n := num(cv.Value())
	cv.Clear()
	var out []Prop
	for i := 1; i <= n; i++ {
		p, err := getDisp(props, "Item", i)
		if err != nil {
			continue
		}
		pr := Prop{
			Name:  str(variantValue(oleutil.GetProperty(p, "Name"))),
			ID:    num(variantValue(oleutil.GetProperty(p, "PropertyID"))),
			Value: str(variantValue(oleutil.GetProperty(p, "Value"))),
		}
		if ro, ok := variantValue(oleutil.GetProperty(p, "IsReadOnly")).(bool); ok {
			pr.ReadOnly = ro
		}
		if lo := variantValue(oleutil.GetProperty(p, "SubTypeMin")); lo != nil {
			pr.Min = str(lo)
			pr.Max = str(variantValue(oleutil.GetProperty(p, "SubTypeMax")))
		}
		p.Release()
		out = append(out, pr)
	}
	return out
}
