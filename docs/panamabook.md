# Panama Book: manuale breve

`PanamaBook.exe` e' l'interfaccia grafica per l'operatore: scansiona il libretto, mostra l'anteprima, mostra i dati letti,
permette di correggerli e **impara** le correzioni: se lo stesso libretto viene scansionato di nuovo, i campi corretti
si sistemano da soli.

## Avvio

1. Tieni `PanamaBook.exe` nella stessa cartella di `libretto.exe` (e di `twainscan32.exe`, per lo scanner TWAIN).
2. Doppio clic su `PanamaBook.exe`: si apre una finestra (Microsoft Edge in modalita' applicazione; senza Edge si usa il
   browser predefinito). Accanto compare una finestra nera con l'indirizzo: serve solo da registro, non chiuderla a mano.
3. Per uscire chiudi la finestra (il programma si chiude da solo dopo pochi secondi) oppure premi Ctrl+C nella finestra nera.
   Se in quel momento lo scanner sta lavorando il programma **aspetta la fine della scansione** prima di chiudersi.

Cartelle usate (accanto all'eseguibile): `scans\` (SOLO immagini e risultati salvati con "Conferma e salva"), `scans\verita\` (verita' per la
regressione), `dati\anteprima\` (anteprime temporanee non salvate), `dati\memoria.json` (memoria delle correzioni) e `dati\edge\` (profilo di Edge). **`dati\` contiene dati personali** ed e'
ignorata da git (`.gitignore`).

## Come si usa

1. **Anteprima.** Controlla il promemoria a sinistra (bordo MRZ per primo, libretto centrato, dritto e spinto fino in fondo; chiudi gli altri
   programmi che usano lo scanner) e premi **Anteprima**. Il programma chiede conferma che il libretto e' inserito; la scansione parte solo
   dopo il tuo "Si'" e dura circa 10 secondi: in quel tempo **non c'e' il pulsante Annulla** (non si interrompe lo scanner). L'immagine
   viene scritta in `dati\anteprima\` (per esempio `anteprima-001.png`): **in `scans\` non finisce niente e non si legge niente**. La vedi
   subito nel riquadro (zoom, rotazione di 180 gradi) con l'etichetta *non salvata*: controlla in pochi secondi se la pagina dati c'e'.
   Se l'immagine e' molto bassa (altezza meno della meta' della larghezza: di solito solo l'informativa) il programma lo suggerisce.
   **Leggi i dati** ricava anche la **sola pagina dati ritagliata** (2 mm sopra la scritta REPUBLICA DE PANAMA, dritta, senza bianchi:
   `libretto.exe extract -i ... -page-out ... -top-margin 2`); e' quella che "Conferma e salva" salva e da cui si fa il PDF (se la pagina non si
   trova, per esempio MRZ non valida, si salva l'anteprima cosi' com'e' e il programma lo dice).
   Sotto l'immagine: **Leggi i dati** (esegue `libretto extract`, 10-25 secondi, qui *Annulla* funziona), **Riscansiona** (nuova anteprima:
   la precedente resta) e **Scarta** (cancella solo quel file temporaneo creato dal programma). Una **striscia** mostra le ultime 5
   anteprime non salvate: cliccane una per confrontarla (come si faceva con `provamrz` e `provamrz2`). Le anteprime piu' vecchie di 7
   giorni si eliminano all'avvio (solo i file `anteprima-NNN` in `dati\anteprima\`).
2. **Controlla i dati.** Ogni campo ha l'etichetta stampata (spagnolo / inglese), il valore modificabile e uno stato:
   - **OK**: nessun dubbio;
   - **Da controllare**: lettura incerta, formato strano o diverso dall'MRZ (il motivo e' scritto sotto il valore);
   - **Controllato** (spunta verde): l'hai guardato tu (o la memoria, "Controllato (da memoria)"); il motivo resta scritto sotto;
   - **Non letto**: campo vuoto. Non e' un errore se sul libretto non c'e' (il campo *Nº Título / Certificate* c'e' solo su alcuni);
   - **Corretto da memoria**: valore sistemato con una tua correzione precedente (si vede cosa aveva letto il programma);
   - **Modificato da te**: hai cambiato il valore.
   Sotto ci sono la zona MRZ (le due righe, i 5 codici di controllo, le correzioni del lettore) e gli avvisi in italiano.
   Il pulsante dell'anteprima ruota l'immagine di 180 gradi e ingrandisce.
3. **Segna i campi controllati e correggi.** Sui campi *Da controllare* c'e' il pulsante **Controllato** (oppure **Invio** nella casella
   senza modifiche, oppure **Alt+C**; Alt+C o *Rimetti da controllare* annullano). Se **modifichi** il valore il campo e' *Modificato
   da te* e conta come controllato. In alto la pillola dice **"N campi da controllare"** e scende man mano (un clic porta al primo
   campo aperto); a zero dice **"Tutti i campi controllati"**. Scrivi nella casella per correggere (Invio passa al campo dopo;
   Ctrl+Invio conferma).
4. **Salva correzioni** memorizza le correzioni per quel libretto. **Conferma e salva** fa in piu' tre cose: **salva davvero** (la sola pagina dati ritagliata passa da `dati\anteprima\` a `scans\` col nome definitivo; il PDF,
   se c'e', con lei), scrive
   `scans\NOME.json` (l'estrazione con i campi confermati e un blocco `corrections`: campo, fonte `user` o `memory`, valore letto)
   e la **verita'** `scans\verita\NOME.json` per il verificatore (`cmd/verifyjson`, `regress -truth`). Se uno di questi file esiste
   gia' il programma chiede il permesso; la verita' precedente si conserva come `NOME.json.prec-DATA`.
   **Conferma solo dopo aver guardato i dati: diventano la verita' per i test.** Se restano campi *Da controllare* il programma non
   conferma in silenzio: mostra l'elenco e chiede **Torna a controllare** oppure **Conferma comunque**; in quel caso nella verita'
   i campi non controllati restano **vuoti** (con una nota `_note`: vuoto = non si confronta). In `scans\NOME.json` il blocco
   `review` elenca i campi `reviewed` (nome e fonte `user` o `memory`, senza valori) e gli `unchecked`.
5. **Apri immagine gia' salvata** legge un file di `scans\` come prima (senza spostare nulla); **Rileggi** rilegge l'immagine salvata corrente (le modifiche non salvate
   si perdono, il programma avvisa). **Esporta PDF (max 1 MB)** crea `scans\NOME.pdf` (vedi sotto). **Memoria** elenca i
   libretti ricordati e permette di dimenticare un campo o tutto il libretto.

## Nome dei file e "Ricomincia da capo"

- **Nome dei file.** Dopo l'anteprima e la lettura **non esiste nessun file in `scans\`** (l'immagine e' in `dati\anteprima\`), e nemmeno
  *Salva correzioni* ne crea. Sopra la tabella una riga dice *"Dopo la conferma i file si chiameranno: MARIO - ROSSI - PA0123456"* e cambia
  se correggi nome, cognome o numero libretto. Solo **Conferma e salva** sposta l'immagine (con l'eventuale PDF) in `scans\` col nome
  **`NOME - COGNOME - NUMERO LIBRETTO`** calcolato con i valori che hai confermato nel form (`internal/filename`: solo lettere, cifre e
  spazi, senza accenti ne' simboli), poi scrive `scans\NOME-FINALE.json` e la verita' `scans\verita\NOME-FINALE.json`. Se mancano nome,
  cognome o numero libretto si usa il **nome di riserva** del campo in alto a destra (o `libretto-NNN`) e il programma lo dice. Tutto
  o niente e mai sovrascrivere: se il nome esiste gia' diventa `... (2)`. L'MRZ valida non serve: l'hai controllata tu. Per le immagini
  gia' salvate (aperte da `scans\`) la conferma le rinomina come prima. Correggendo ancora e riconfermando i file e la verita' seguono il
  nuovo nome (senza aggiungere "(2)" a ogni conferma). `fields.given_names` sono i nomi e `fields.surname` i cognomi (l'ordine per i
  libretti di Panama e' gia' giusto nell'uscita di `libretto.exe`; il programma non li inverte).
- **Ricomincia da capo** (pulsante sempre in alto, scorciatoia **Alt+N**): svuota anteprima, campi, MRZ, avvisi, contatore e
  stato *controllato* della sessione e propone il prossimo nome libero `libretto-NNN`; lascia impostazioni e cronologia e **non
  cancella nulla** (ne' immagini, ne' JSON, ne' verita', ne' memoria). Con modifiche o anteprime non salvate chiede (es. *"Hai 2
  anteprime non salvate"*): *Salva quella selezionata e ricomincia* (equivale a *Conferma e salva*, se i dati sono stati letti; altrimenti
  *Salva e ricomincia* = *Salva correzioni*) / *Ricomincia senza salvare* / *Annulla*. Le anteprime non salvate restano nella striscia. Durante la scansione fisica e' disabilitato; durante la
  lettura la annulla e poi ricomincia.

## Come funziona la memoria

- Il libretto si riconosce dal **numero libretto dell'MRZ piu' la data di nascita dell'MRZ**, ma solo se l'MRZ ha tutti e 5 i
  codici di controllo validi. Senza MRZ valida **non si applica e non si salva nulla** (il programma lo dice).
- Per ogni campo corretto si ricorda il valore giusto, quello che il programma aveva letto e la data.
- Per ogni campo segnato **Controllato** (con *Salva correzioni* o *Conferma e salva*) si ricorda "valore X confermato per questo
  libretto". Alla scansione successiva, se il campo letto e' ancora X, e' subito *Controllato (da memoria)* e non si chiede di nuovo;
  se il valore e' diverso torna *Da controllare* e il motivo dice "l'ultima volta era stato confermato X". Una correzione sostituisce
  la conferma dello stesso campo; *Rimetti da controllare* e salvare toglie la conferma. La finestra *Memoria* le elenca e le dimentica.
- Alla scansione successiva dello stesso libretto, dopo la lettura, i campi che differiscono dalla memoria vengono sostituiti e
  marcati *Corretto da memoria*, con il valore letto accanto. Se il valore ricordato era in realta' giusto da leggere,
  premi *Era giusto, dimentica* sulla riga: la correzione viene tolta.
- Se rimetti a mano il valore letto e salvi, la correzione ricordata per quel campo viene tolta.
- La memoria impara **per libretto**: un errore di lettura simile su un altro libretto non viene corretto (vedi limiti).

## Copia e incolla nel gestionale

Tutto si copia con un clic o un tasto: **il programma non scrive mai negli appunti da solo**. Si copia sempre il valore che vedi nella
tabella, cioe' **con le tue correzioni** (non quello letto).

- **Copia (per campo):** il pulsante con l'icona accanto a ogni valore. Per un momento (1,5 s) diventa **"Copiato"**; un campo *Non letto*
  (vuoto) non copia nulla e diventa **"Vuoto"**. Lo stato viene anche annunciato ai lettori di schermo.
- **Copia tutto** (sopra la tabella) con due menu ricordati dal browser:
  - *Formato*: **Etichetta: valore** (una riga per campo, con l'etichetta bilingue), **Solo i valori** (uno per riga, i vuoti restano righe vuote),
    **Una riga con tabulazioni** (per Excel e gestionale, nell'ordine della tabella, un campo vuoto e' una cella vuota), **JSON dei campi**
    (chiavi inglesi come l'uscita di `libretto.exe`; i campi vuoti non ci sono);
  - *Date*: **GG-MM-AAAA** (come stampato) oppure **AAAA-MM-GG** (ISO). Vale per Copia tutto e per il Copia di un campo data.
- **MRZ:** *Copia riga 1*, *Copia riga 2*, *Copia entrambe* (le due righe da 44 caratteri come lette).
- **Copia nome file:** il nome base previsto `NOME - COGNOME - NUMERO` (segue le correzioni; se mancano dati, il nome di riserva).
- **Scorciatoie:** **Alt+Maiusc+C** copia il campo su cui ti trovi (la riga della tabella), **Alt+Maiusc+A** copia tutto nel formato scelto.
  Non toccano le altre: Alt+C = *Controllato*, Alt+N = *Ricomincia da capo*, Ctrl+C resta quella normale del browser. Il tasto Tab va dal
  valore al suo pulsante Copia.
- Se il browser nega l'accesso agli appunti il programma ripiega su `document.execCommand('copy')`; se fallisce anche quello mostra un
  messaggio chiaro ("seleziona il testo a mano e premi Ctrl+C").

## Schede «Autocorrezioni» e «Novità di Panama»

In alto, accanto al nome, ci sono tre schede: **Libretto** (il lavoro di prima), **Autocorrezioni** e **Novità di Panama**.

### Autocorrezioni

Quando correggi un campo e salvi, il programma impara una regola **valida per tutti i libretti**: «se in questo campo leggo `MAS`, il valore giusto e' `MASTER II/`». La volta dopo, su qualsiasi libretto, se legge lo stesso valore lo corregge da solo e segna il campo **Autocorretto** (con il valore letto accanto e «Rimetti il valore letto»). Le regole stanno in un file leggibile, `dati\autocorrezioni.json` (nessun dato personale), e si gestiscono nella scheda: **Aggiungi** una regola a mano, **Attiva** o spegni, **Cancella** (due clic).

- Valgono solo per campi con pochi valori ricorrenti: tipo, codice paese, nazionalita', luogo di nascita, livello, qualifica (*capacity*) e *regional*. **Mai** per nomi, cognomi, numeri, date o sesso.
- Si confronta il valore **intero** del campo (non una parte), senza distinguere maiuscole e spazi: `MAS` non tocca `MASTER`.
- Se rimetti il valore letto su un campo autocorretto e salvi, la regola **si spegne da sola** (la riattivi dalla scheda). Se correggi ancora lo stesso valore letto con un altro valore, la regola si aggiorna.
- La **memoria del singolo libretto** (scheda *Memoria*) ha la precedenza sulle autocorrezioni. Un valore autocorretto e lasciato com'e' non viene ricordato come correzione tua.
- «Usi» conta le volte in cui la regola e' stata applicata e confermata. Massimo 500 regole.
- **Importa dalla memoria** crea le regole dalle correzioni gia' ricordate per i singoli libretti (finestra *Memoria*): per ogni campo ammesso e valore letto prende la correzione su cui concordano piu' libretti; i pareggi si saltano e le regole esistenti non si toccano.

### Novità di Panama

Mostra le notizie **ufficiali** dell'Autorita' Marittima di Panama: l'**AMP** (`amp.gob.pa`, in **spagnolo**) e il **Panama Ship Registry** (`panamashipregistry.com`, che e' dell'AMP, in **inglese**). Non si traducono. Il programma legge i loro feed RSS pubblici **solo quando apri la scheda o premi Aggiorna** (mai all'avvio e mai in sottofondo; entro 30 minuti usa l'ultima lettura). L'ultima lettura riuscita resta in `dati\novita.json`, cosi' senza Internet vedi comunque le ultime novita'.

- Fonti scritte nel programma (nessun indirizzo scelto dall'utente): solo HTTPS, solo quegli host, risposta massima 2 MB, 12 secondi per fonte; il testo ricevuto si pulisce dai tag e i link si aprono solo se puntano allo stesso sito.
- Non viene inviato nessun dato dei libretti. Usa il proxy di sistema se c'e'.
- Si spegne in *Impostazioni > Leggi le novita' ufficiali di Panama da Internet*: allora il programma non contatta nessun sito (utile se la rete dell'azienda non lo consente).

## Cartella di salvataggio

Di norma immagini, JSON, PDF e la verita' (`verita\`) si salvano in `scans\` accanto al programma. In *Impostazioni > Cartella di salvataggio* si puo' scegliere
un'altra cartella (un altro disco, una cartella condivisa o una cartella **OneDrive**): **Sfoglia...** apre la finestra di Windows, oppure si scrive il percorso
completo e si preme **Applica** (o Invio). Se il PC ha OneDrive compare il pulsante **Usa OneDrive**, che sceglie `OneDrive\Panama Book`. **Predefinita** torna a `scans\`.

- La cartella viene creata se manca e deve essere scrivibile; `dati\` (la cartella del programma) non e' ammessa. Non si cambia durante una scansione o una lettura.
- La scelta e' nel file `dati\impostazioni.json` (chiave `outdir`) e si ritrova al riavvio. Se la cartella non c'e' piu' (disco tolto) il programma riparte
  con `scans\` e lo scrive nel registro.
- Cambiando cartella i file gia' salvati **non si spostano**: «Ultime scansioni» mostra la cartella attuale.
- Anteprime (`dati\anteprima\`), memoria e profilo di Edge restano sempre accanto al programma.
- Sono **dati personali**: in una cartella OneDrive vengono sincronizzati nel cloud (per un uso aziendale scegli una cartella consentita dalla policy).

## Scheda «Certificati»

Legge i certificati di Panama che accompagnano il libretto: **SAT** (Security Awareness Training), **SD** (Security Duties), **SSO** (Ship Security Officer) e **CT** (Certificación de Trámite de Título, la ricevuta della domanda che serve per ottenere il libretto definitivo e che vale a bordo al massimo 3 mesi).

1. **Apri PDF o immagine…** (PDF, PNG o JPEG, massimo 25 MB) oppure **Scansiona un foglio A4** (serve `libretto2.exe`: comando `libretto2 sheet`, foglio intero senza ritaglio, dal caricatore o dal vetro).
2. Il programma legge il testo con l'OCR di Windows e riconosce il tipo. Accanto all'immagine compaiono i campi: quelli richiesti dal tipo hanno l'asterisco e, se mancano, il bordo arancione. Si possono correggere a mano e cambiare il tipo.
3. **Salva** scrive due file in `<cartella di salvataggio>\certificati\`: il **PDF** del certificato (al massimo 1 MB: un PDF originale abbastanza piccolo si copia com'è, un'immagine o scansione diventa un PDF di una pagina) e il **JSON** con i campi. Il nome è `MARINEID - NOME (TIPO)`, per esempio `500123456 - MARIO ROSSI (SD)` (il Marine ID è la chiave univoca del marittimo: i file dello stesso marittimo stanno vicini). Se il nome esiste già si aggiunge « - 2». **Scarta** cancella i file di lavoro.

Campi per tipo (date `GG-MM-AAAA`, numero documento `E-NNNNNNNNNA`):

| Tipo | Campi richiesti |
|---|---|
| SAT | numero documento, scadenza, rilascio, nascita |
| SD | numero documento, scadenza, nome, nascita, nazionalità, rilascio, Marine ID |
| SSO | numero documento, nome, nascita, nazionalità, rilascio, scadenza |
| CT | CT No., numero documento, nome, identificazione, capacity, rilascio, scadenza, consolato (Londra o Napoli, da «CONSUL ...»), Marine ID |

Ogni campo ha il pulsante **Copia** e in basso c'è **Copia tutti i campi**.

**Controllo doppio con il QR.** Il QR del certificato contiene il link al servizio di verifica dell'AMP. Il programma lo legge da solo e lo mostra sotto i campi; **Verifica online** interroga il servizio e confronta Marine ID (la chiave univoca del marittimo per Panama), nome, nascita, nazionalità, identificazione, rilascio e scadenza con quello che c'è nella scheda, e mostra lo stato online (per esempio «Valid»). Il controllo parte solo quando lo chiedi, solo verso l'indirizzo del servizio AMP (qualunque altro link nel QR viene ignorato) e invia solo il numero che sta già nel QR, nessun dato tuo. Il servizio usa HTTP non cifrato: serve internet. L'esito si salva nel JSON (`qr_url`, `online_check`).

Gli altri campi stampati (nazionalità, numero di ricevuta, Marine ID...) si leggono comunque se ci sono. I file di lavoro stanno in `dati\certificati\` (`cert-*`) e si cancellano dopo un giorno. I certificati contengono dati personali: restano sul disco e non vanno condivisi. Un PDF digitale (come quelli dell'AMP) si legge dal suo testo incorporato, senza OCR e senza programmi esterni, e si mostra com'e'; solo un PDF senza testo viene reso in immagine (con `powershell.exe`, a finestra nascosta) e letto con l'OCR; da riga di comando: `certread file.pdf` (JSON) e `certread -rows file.pdf` (righe lette dall'OCR).

**QR del libretto.** Anche la pagina dati del libretto ha un QR con il link al servizio AMP. Sotto «Dati del libretto» il programma lo mostra (se riesce a leggerlo: serve una scansione nitida, a 300 dpi) e **Verifica online** confronta Id Marino, nome, nascita, nazionalità, identificazione, numero del libretto, sesso, cargo, livello, emissione e scadenza con la pagina online. Il numero in fondo al link è il «Doc Number» del documento, non l'Id Marino (che la pagina online mostra come «Seafarers ID»).

## Impostazioni di scansione

Valori consigliati gia' impostati, la ricetta collaudata di `tools\prova-libretto.cmd`: driver **`wia`**, **300 dpi**, lunghezza letta **190 mm**,
margine sopra il titolo **2 mm**. *Ripristina valori consigliati* li rimette. Con 190 mm la lettura copre la pagina dati sia se il punto di
partenza e' il bordo inserito per primo sia se e' il bordo in mano (il libretto aperto e' lungo circa 176 mm). **TWAIN** resta selezionabile
ma e' consigliato solo per fogli stampati (`tools\scansiona.cmd`); per libretti veri usa WIA.

Comando lanciato da *Anteprima*:
- WIA: `libretto.exe scan -yes -o dati\anteprima\anteprima-NNN.png -driver wia -dpi 300 -len 190 -top-margin 2 -raw dati\anteprima\anteprima-NNN-grezza.png`
  (nessun `-from` e nessun `-rotate`: con WIA l'immagine esce gia' dritta);
- TWAIN: lo stesso piu' `-from 0 -rotate 180` (con TWAIN l'immagine esce capovolta).
Subito dopo, **in automatico e senza pulsante**, `libretto.exe extract -i anteprima-NNN.png -page-out anteprima-NNN-pagina.png -top-margin 2 -force`
(equivale a `read -page-only`): la **sola pagina dati ritagliata** (dritta, senza informativa ne' bianchi) e' la vista predefinita dell'anteprima;
il JSON della lettura resta in cache (`anteprima-NNN-lettura.json`) e *Leggi i dati* e' istantaneo. Se la pagina non si trova si vede la scansione
intera, con il messaggio e la diagnosi. Verificato con 6 grezze WIA vere (copie in una cartella temporanea): pagina ritagliata ~1494x1030 px e MRZ 5/5 su tutte.

## Scansione grezza, vista e diagnosi

Per ogni anteprima il programma tiene anche la **scansione grezza** (`anteprima-NNN-grezza.png`: cosi' come esce dal driver, prima di
ritaglio e controllo del vuoto; contiene dati personali, stesse regole delle altre immagini). Sopra l'immagine un interruttore sceglie
cosa guardare: **Pagina dati** (solo dopo *Leggi i dati*: la pagina dati ritagliata, quella che verra' salvata), **Intera** (la scansione
ritagliata al documento) o **Grezza**. Dopo la lettura si vede subito la pagina dati.

**Diagnosi automatica** (senza OCR, solo un suggerimento che non blocca nulla): il programma misura il documento nella grezza e dice:
- *"Il libretto e' decentrato verso DESTRA/SINISTRA: spostalo di circa N cm"*, se tocca un bordo laterale (e' tagliato dal campo di lettura) o se il
  suo centro e' lontano piu' di 20 mm dal centro del campo; destra e sinistra sono quelle dell'ANTEPRIMA (immagine dritta come la vedi sullo schermo);
- *"E' entrato troppo poco"*, se il documento e' alto meno del 60% del previsto (il minore tra la lunghezza letta e 176 mm);
- *"Il bordo del libretto e' inclinato di N gradi"*, oltre 1,5 gradi (non si dice se il documento e' tagliato dal bordo: la stima non e' affidabile);
- *"Nella scansione grezza non si vede nessun documento"*.
Sotto ci sono le misure rilevate (larghezza e altezza in mm, spostamento del centro, inclinazione). In modo `-demo` la grezza e' una copia
dell'immagine di prova e non c'e' diagnosi. Salvando, la grezza e la scansione intera **non** si conservano, a meno di spuntare *Conserva anche
la scansione intera* (`NOME (intera).png`) o *Conserva anche la scansione grezza* (`NOME (grezza).png`).

## Esporta PDF (max 1 MB)

Lancia `libretto.exe pdf -i scans\NOME.png -o scans\NOME.pdf` (JPEG alla massima qualita' che sta in 1.000.000 byte) e mostra il
peso ottenuto. **Prima del salvataggio** lavora sull'anteprima temporanea e scrive il PDF in `dati\anteprima\` (accanto all'immagine); con
"Conferma e salva" passa in `scans\` con lo stesso nome base dell'immagine. Se `libretto.exe` non conosce ancora il comando (la sua guida non contiene `libretto pdf`) ripiega su
`_out\libretto_pdf.exe`. Se `scans\NOME.pdf` esiste chiede conferma prima di sostituirlo.

## Se il programma si chiude da solo: i registri

Per capire PERCHE' il programma si e' chiuso, lancialo con **`tools\panamabook.cmd`** (mostra le ultime righe dei registri a fine
esecuzione). Tutti i file stanno in `dati\` e contengono dati personali (nomi dei file; mai i valori dei campi):

| File | Cosa contiene |
|---|---|
| `dati\panamabook.log` | eventi con data e ora: avvio, porta, richieste API (non gli heartbeat riusciti), avvio e fine di ogni lavoro e di ogni sottoprocesso (`libretto.exe scan/extract/pdf`, con codice di uscita e durata), errori della pagina, un "vivo" al minuto e il **MOTIVO dell'uscita** (`USCITA: ...`: finestra chiusa, `heartbeat assente da N s`, `ricevuto segnale ...`, errore). Se un altro programma ha gia' aperto quel nome in scrittura il registro si chiama `dati\panamabook-eventi.log`. |
| `dati\panamabook-crash.log` | stack dei panic non recuperati e degli errori fatali del runtime Go (vuoto se non e' successo niente). |
| `dati\panamabook-console.log` | quello che il programma stampa nella finestra nera (scritto da `tools\panamabook.cmd`). |

Come leggerli: se in `panamabook.log` manca la riga `USCITA: ...` dopo l'ultimo "vivo", il processo e' stato **terminato dall'esterno**
(`taskkill /F`, `Stop-Process`, console chiusa a forza: un `TerminateProcess` non si puo' intercettare); la chiusura normale della
console, Ctrl+C, il logoff e lo spegnimento lasciano la riga `ricevuto segnale ...`. Un panic dentro un lavoro o un gestore non chiude
il programma: il lavoro finisce in errore visibile, lo stack e' nel registro.

Quando il programma esce da solo: **mai durante un lavoro** (scansione o lettura) e **mai con un heartbeat negli ultimi 60 s**. Con la
finestra Edge lanciata dal programma, l'uscita avviene quando quel processo Edge termina (dopo almeno 10 s di vita); come rete di
sicurezza anche dopo 3 minuti senza heartbeat (una finestra coperta o in secondo piano rallenta i timer della pagina). Link e file
trascinati sulla finestra non la portano fuori dalla pagina. **Non terminare mai PanamaBook.exe o libretto.exe per nome**
(`taskkill /IM`, `Stop-Process -Name`): chiude anche l'istanza che l'operatore sta usando.

## Prova senza scanner (`-demo`)

```
PanamaBook.exe -demo scans\immagine.png        (oppure variabile d'ambiente PANAMABOOK_DEMO)
```

*Anteprima (PROVA)* non avvia nessuno scanner: copia l'immagine in `dati\anteprima\anteprima-NNN.png` come se fosse appena scansionata;
poi *Leggi i dati* la legge con `libretto.exe extract`. L'interfaccia lo dice chiaramente (etichetta "PROVA: nessuno scanner").
Altre opzioni: `-dir CARTELLA` (cartella di lavoro), `-libretto PERCORSO`, `-no-window` (stampa l'indirizzo, niente finestra),
`-port N`.

## Sicurezza

Il server ascolta solo su `127.0.0.1`, su una porta casuale. Ogni richiesta alle API richiede un token casuale di sessione
(nell'indirizzo della finestra) e si controllano `Host`, `Origin` e `Sec-Fetch-Site`, cosi' altre pagine aperte sul computer non
possono comandare il programma ne' leggere le immagini. Le immagini si servono solo da `scans\` (niente percorsi con `..`). Il
programma non apre nessuna connessione verso l'esterno e non carica nulla da Internet.

## Sviluppo

- Codice: `cmd\panamabook` (server Go, interfaccia in `web\`, incorporata con `embed`), `internal\memoria` (archivio).
- `go build -o PanamaBook.exe ./cmd/panamabook`; test con `go test ./cmd/panamabook ./internal/memoria`. I test usano un finto
  `libretto.exe` (`cmd\panamabook\fakelibretto`, dati inventati): non toccano mai scanner ne' dati reali.
