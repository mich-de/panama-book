// Comando panamabook ("Panama Book"): interfaccia grafica per l'operatore che scansiona un libretto, mostra l'anteprima e
// i dati estratti, permette di correggerli e IMPARA le correzioni (internal/memoria).
//
// E' una piccola app web locale servita da Go (solo net/http) in una finestra applicativa di Microsoft Edge
// (msedge.exe --app=...). Il server ascolta SOLO su 127.0.0.1, su una porta libera casuale, e ogni API richiede un token
// di sessione casuale. Scansione ed estrazione NON sono reimplementate: si lancia libretto.exe (stessa cartella).
//
//	PanamaBook.exe                  apre la finestra
//	PanamaBook.exe -demo FILE.png   prova senza scanner: "Scansiona" usa quell'immagine al posto dello scanner
//
// Si chiude quando si chiude la finestra o con Ctrl+C (mai nel mezzo di una scansione fisica). Ogni evento utile e il MOTIVO
// dell'uscita finiscono in dati\panamabook.log; i panic e gli errori fatali del runtime in dati\panamabook-crash.log.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

func main() {
	demo := flag.String("demo", os.Getenv("PANAMABOOK_DEMO"), "PROVA senza scanner: immagine PNG/JPG da usare al posto della scansione (anche variabile PANAMABOOK_DEMO)")
	dir := flag.String("dir", "", "cartella di lavoro con libretto.exe, scans\\ e dati\\ (predefinita: quella dell'eseguibile)")
	exe := flag.String("libretto", "", "percorso di libretto.exe (predefinito: nella cartella di lavoro)")
	noWindow := flag.Bool("no-window", false, "non aprire la finestra: stampa l'indirizzo e resta in ascolto (prove, screenshot)")
	port := flag.Int("port", 0, "porta (predefinita: una libera a caso)")
	flag.Parse()

	cfg, err := buildConfig(*dir, *exe, *demo, *noWindow)
	if err != nil {
		fmt.Fprintln(os.Stderr, "errore:", err)
		os.Exit(1)
	}
	lg := openLog(filepath.Join(cfg.Base, "dati", "panamabook.log"))
	defer lg.close()
	installCrashOutput(cfg.Base)
	code := 0
	if err := run(cfg, *port, lg); err != nil {
		lg.printf("USCITA CON ERRORE: %v", err)
		fmt.Fprintln(os.Stderr, "errore:", err)
		code = 1
	}
	lg.printf("processo terminato (codice %d)", code)
	os.Exit(code)
}

// buildConfig risolve le cartelle e controlla l'immagine di prova.
func buildConfig(dir, exe, demo string, noWindow bool) (config, error) {
	base := dir
	if base == "" {
		if self, err := os.Executable(); err == nil {
			base = filepath.Dir(self)
		}
		// con "go run" l'eseguibile sta in una cartella temporanea: se li' manca libretto.exe e nella cartella corrente c'e', si usa quella
		if !fileExists(filepath.Join(base, "libretto.exe")) {
			if wd, err := os.Getwd(); err == nil && fileExists(filepath.Join(wd, "libretto.exe")) {
				base = wd
			}
		}
	}
	base, err := filepath.Abs(base)
	if err != nil {
		return config{}, err
	}
	cfg := config{Base: base, Exe: exe, NoWindow: noWindow, PDFFallback: filepath.Join(base, "_out", "libretto_pdf.exe")}
	if cfg.Exe == "" {
		cfg.Exe = filepath.Join(base, "libretto.exe")
	}
	if demo != "" {
		p, err := filepath.Abs(demo)
		if err != nil || !fileExists(p) || !isImageName(p) {
			return config{}, fmt.Errorf("immagine di prova %q non trovata (serve un file .png o .jpg)", demo)
		}
		cfg.Demo = p
	}
	return cfg, nil
}

func run(cfg config, port int, lg *logger) error {
	app, err := newApp(cfg)
	if err != nil {
		return err
	}
	app.lg = lg
	app.cleanOldPreviews(previewMaxAge) // anteprime temporanee non salvate piu' vecchie di 7 giorni (solo file dell'app)
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return err
	}
	_, p, _ := net.SplitHostPort(ln.Addr().String())
	app.setAddr(p)
	srv := &http.Server{Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second, ErrorLog: nil}
	go func() {
		defer app.recoverTo("server HTTP", nil)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			app.logf("server HTTP terminato: %v", err)
		}
	}()
	url := fmt.Sprintf("http://127.0.0.1:%s/?t=%s", p, app.token)

	app.logf("=== AVVIO Panama Book %s (pid %d, %s %s/%s) cartella %s, porta %s, libretto.exe presente=%v, prova=%v ===",
		version, os.Getpid(), runtime.Version(), runtime.GOOS, runtime.GOARCH, cfg.Base, p, fileExists(cfg.Exe), cfg.Demo != "")
	fmt.Println("Panama Book", version)
	fmt.Println("cartella di lavoro:", cfg.Base)
	if lg != nil {
		fmt.Println("registro degli eventi:", filepath.Join("dati", filepath.Base(lg.path)))
	} else {
		fmt.Println("ATTENZIONE: registro degli eventi non disponibile")
	}
	if cfg.Demo != "" {
		fmt.Println("MODO PROVA: lo scanner NON viene usato; \"Scansiona\" usa", filepath.Base(cfg.Demo))
	}
	if !fileExists(cfg.Exe) {
		fmt.Println("attenzione: manca", filepath.Base(cfg.Exe), "(serve per scansionare e leggere)")
		app.logf("ATTENZIONE: manca %s", cfg.Exe)
	}

	edgeGone := make(chan string, 1)
	if cfg.NoWindow {
		fmt.Println("indirizzo:", url)
	} else {
		if err := openWindow(app, url, cfg.Base, edgeGone); err != nil {
			fmt.Println("non riesco ad aprire la finestra:", err)
			fmt.Println("apri a mano:", url)
			app.logf("apertura della finestra fallita: %v", err)
		}
		fmt.Println("Chiudi la finestra per uscire (oppure Ctrl+C).")
	}

	sig := make(chan os.Signal, 4)
	// Windows: Ctrl+C e Ctrl+Break = os.Interrupt; chiusura della finestra di console, logoff e spegnimento = SIGTERM.
	// Un TerminateProcess (taskkill /F, Stop-Process) NON si puo' intercettare: in quel caso nel registro manca la riga di uscita.
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	started := time.Now()
	lastAlive := started
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	reason := ""
loop:
	for {
		select {
		case <-app.exitCh:
			reason = "uscita richiesta"
			break loop
		case sg := <-sig:
			reason = fmt.Sprintf("ricevuto segnale %v (Ctrl+C, chiusura della console, logoff o spegnimento)", sg)
			app.logf("ricevuto segnale %v", sg)
			break loop
		case r := <-edgeGone:
			reason = r
			break loop
		case now := <-tick.C:
			if now.Sub(lastAlive) >= time.Minute { // un segno di vita ogni minuto: l'ultimo dice quando il processo e' stato fermato dall'esterno
				lastAlive = now
				app.logf("vivo (lavoro in corso: %v)", app.busyNow() != nil)
			}
			if !cfg.NoWindow {
				if exit, why := safeShouldExit(app, now, started); exit {
					reason = why
					break loop
				}
			}
		}
	}
	app.logf("USCITA: %s", reason)
	// mai uscire mentre lo scanner lavora: si attende la fine del trasferimento
	if app.scanningNow() {
		fmt.Println("Scansione in corso: attendo la fine prima di chiudere (non si interrompe lo scanner).")
		app.logf("scansione in corso: attendo la fine prima di chiudere")
		for app.scanningNow() {
			select {
			case sg := <-sig:
				fmt.Println("Attendo la fine della scansione...")
				app.logf("segnale %v durante la scansione: ignorato, attendo la fine", sg)
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	app.cancelActive()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	app.logf("server arrestato")
	return nil
}

// safeShouldExit protegge il ciclo principale da un panic nella logica di uscita (che non deve mai chiudere il programma).
func safeShouldExit(app *App, now, started time.Time) (exit bool, why string) {
	defer app.recoverTo("controllo di uscita", func(string) { exit = false })
	return app.shouldExit(now, started)
}

// findEdge cerca msedge.exe nelle cartelle di installazione abituali e nel PATH.
func findEdge() string {
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
		if root := os.Getenv(env); root != "" {
			if p := filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"); fileExists(p) {
				return p
			}
		}
	}
	if p, err := exec.LookPath("msedge.exe"); err == nil {
		return p
	}
	return ""
}

// openWindow apre la finestra applicativa di Edge con un profilo proprio (dati\edge); se Edge manca usa il browser predefinito.
// Con Edge lanciato da noi la sua FINE e' il segnale di chiusura (molto piu' affidabile degli heartbeat: una finestra coperta o in
// secondo piano rallenta i timer della pagina): arriva su gone. Se Edge termina entro 10 s si pensa a un inoltro a un'istanza gia'
// aperta e si ignora (resta la rete di sicurezza degli heartbeat).
func openWindow(app *App, url, base string, gone chan<- string) error {
	if edge := findEdge(); edge != "" {
		cmd := exec.Command(edge, "--app="+url, "--user-data-dir="+filepath.Join(base, "dati", "edge"),
			"--no-first-run", "--no-default-browser-check", "--disable-background-mode", "--window-size=1360,900")
		if err := cmd.Start(); err == nil {
			t0 := time.Now()
			app.haveEdge.Store(true)
			app.logf("Edge avviato (pid %d)", cmd.Process.Pid)
			go func() {
				defer app.recoverTo("attesa di Edge", nil)
				err := cmd.Wait()
				life := time.Since(t0)
				app.logf("Edge terminato dopo %d s (%v)", int(life.Seconds()), err)
				if life < 10*time.Second {
					app.haveEdge.Store(false)
					app.logf("Edge terminato troppo presto: si usano gli heartbeat della pagina")
					return
				}
				select {
				case gone <- fmt.Sprintf("finestra chiusa (Edge terminato dopo %d s)", int(life.Seconds())):
				default:
				}
			}()
			return nil
		} else {
			app.logf("avvio di Edge fallito: %v", err)
		}
	}
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	if err := cmd.Start(); err != nil {
		return err
	}
	app.logf("finestra aperta con il browser predefinito")
	go func() {
		defer app.recoverTo("rundll32", nil)
		_ = cmd.Wait()
	}()
	return nil
}
