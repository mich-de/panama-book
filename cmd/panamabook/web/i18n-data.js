'use strict';
/* Dizionario delle lingue (vedi i18n.js). La chiave e' il testo ITALIANO esatto che compare nella pagina.
   x(it, en, es)  frase esatta.
   p(/modello/, en, es)  frase con numeri o nomi: {1}, {2}... sono i pezzi catturati (tradotti se si sa, altrimenti lasciati come sono).
   Le frasi lunghe si possono scrivere per intero oppure una frase alla volta: i testi con piu' frasi si traducono frase per frase.
   I modelli si provano nell'ordine: i piu' specifici vanno per primi. */
window.PBI18nData = (() => {
  const X = []; const P = [];
  const x = (it, en, es) => X.push([it, en, es]);
  const p = (re, en, es) => P.push([re, en, es]);

  // ---- nomi delle lingue (restano uguali) ---------------------------------------------------------------------
  x('English', 'English', 'English'); x('Español', 'Español', 'Español'); x('Italiano', 'Italiano', 'Italiano');
  x('Lingua', 'Language', 'Idioma');
  x('Lingua dell\'interfaccia', 'Interface language', 'Idioma de la interfaz');
  x('Salvata nel file', 'Saved in the file', 'Se guarda en el archivo');
  x('insieme alle altre impostazioni.', 'together with the other settings.', 'junto con los demás ajustes.');
  x('Panama Book', 'Panama Book', 'Panama Book'); x('Libreta de embarque', 'Libreta de embarque', 'Libreta de embarque'); x('mic.deangelis@gmail.com', 'mic.deangelis@gmail.com', 'mic.deangelis@gmail.com');

  // ---- testata e comandi ---------------------------------------------------------------------------------------
  x('Vai ai dati', 'Skip to data', 'Ir a los datos');
  x('PROVA: nessuno scanner', 'TEST MODE: no scanner', 'PRUEBA: sin escáner');
  x('Come si inserisce il libretto nello scanner', 'How to insert the book into the scanner', 'Cómo se inserta la libreta en el escáner');
  x('Come si inserisce', 'How to insert', 'Cómo se inserta');
  x('Svuota la schermata e prepara una nuova scansione (Alt+N). Non cancella nessun file.', 'Clears the screen and prepares a new scan (Alt+N). No file is deleted.', 'Vacía la pantalla y prepara un nuevo escaneo (Alt+N). No se borra ningún archivo.');
  x('Svuota la schermata e prepara una nuova scansione (Alt+N)', 'Clears the screen and prepares a new scan (Alt+N)', 'Vacía la pantalla y prepara un nuevo escaneo (Alt+N)');
  x('Ricomincia', 'Start over', 'Empezar de nuevo');
  x('Memoria', 'Memory', 'Memoria');
  x('Autore: Michele De Angelis ·', 'Author: Michele De Angelis ·', 'Autor: Michele De Angelis ·');
  x('Panama Book · Autore: Michele De Angelis (', 'Panama Book · Author: Michele De Angelis (', 'Panama Book · Autor: Michele De Angelis (');

  // ---- anteprima -----------------------------------------------------------------------------------------------
  x('Immagine', 'Image', 'Imagen');
  x('Anteprima', 'Preview', 'Vista previa');
  x('Quale immagine mostrare', 'Which image to show', 'Qué imagen mostrar');
  x('Passaporto intero (informativa + pagina dati): si parte sempre da qui', 'Whole passport (information page + data page): always start here', 'Pasaporte entero (hoja informativa + página de datos): se empieza siempre aquí');
  x('Pagina intera', 'Whole page', 'Página entera');
  x('Solo la pagina dati: scegli questa per salvare solo quella', 'Data page only: choose this to save just that page', 'Solo la página de datos: elige esta para guardar solo esa');
  x('Pagina dati', 'Data page', 'Página de datos');
  x('Grezza', 'Raw', 'Sin procesar');
  x('Strumenti dell\'anteprima', 'Preview tools', 'Herramientas de la vista previa');
  x('Riduci', 'Zoom out', 'Reducir');
  x('Adatta alla finestra', 'Fit to window', 'Ajustar a la ventana');
  x('Ingrandisci (fino a 5x). Doppio clic o rotellina sull\'immagine: ingrandisce verso quel punto', 'Zoom in (up to 5x). Double-click or mouse wheel on the image zooms towards that point', 'Ampliar (hasta 5x). Doble clic o rueda sobre la imagen: amplía hacia ese punto');
  x('Ingrandisci', 'Zoom in', 'Ampliar');
  x('Ruota di 180 gradi', 'Rotate 180 degrees', 'Girar 180 grados');
  x('Immagine scansionata, scorri con le frecce se ingrandita', 'Scanned image, scroll with the arrow keys when zoomed in', 'Imagen escaneada; desplázate con las flechas si está ampliada');
  x('Nessuna immagine.', 'No image.', 'Ninguna imagen.');
  x('Scansiona un libretto o apri un\'immagine già salvata.', 'Scan a book or open an image you already saved.', 'Escanea una libreta o abre una imagen ya guardada.');
  x('Immagine scansionata del libretto', 'Scanned image of the book', 'Imagen escaneada de la libreta');
  x('Area da tagliare: trascina per spostare, trascina i punti per ridimensionare, frecce per spostare (Maiusc per ridimensionare)', 'Crop area: drag to move, drag the handles to resize, arrow keys to move (Shift to resize)', 'Área de recorte: arrastra para mover, arrastra los puntos para cambiar el tamaño, flechas para mover (Mayús para cambiar el tamaño)');
  x('Tira i quattro angoli dove vuoi: se il libretto è storto, il taglio lo raddrizza. Doppio clic o rotellina = zoom sul punto.', 'Drag the four corners anywhere: if the book is crooked, the crop straightens it. Double-click or mouse wheel = zoom on the point.', 'Arrastra las cuatro esquinas donde quieras: si la libreta está torcida, el recorte la endereza. Doble clic o rueda = zoom en ese punto.');
  x('Solo rettangolo dritto', 'Straight rectangle only', 'Solo rectángulo recto');
  x('Blocca proporzione 3:4', 'Lock 3:4 ratio', 'Bloquear proporción 3:4');
  x('Applica taglio', 'Apply crop', 'Aplicar recorte');
  x('Tutto il foglio', 'Whole sheet', 'Toda la hoja');
  x('Annulla', 'Cancel', 'Cancelar');
  x('Leggi i dati', 'Read data', 'Leer los datos');
  x('Riscansiona', 'Rescan', 'Volver a escanear');
  x('Scarta', 'Discard', 'Descartar');
  x('Rifà il taglio da solo: trova il passaporto dalle righe MRZ', 'Crops automatically: finds the passport from the MRZ lines', 'Recorta automáticamente: encuentra el pasaporte a partir de las líneas MRZ');
  x('Taglio automatico', 'Auto crop', 'Recorte automático');
  x('Disegna tu il rettangolo da tagliare', 'Draw the crop area yourself', 'Dibuja tú el área de recorte');
  x('Taglio manuale', 'Manual crop', 'Recorte manual');
  x('Ultime anteprime (non salvate)', 'Latest previews (not saved)', 'Últimas vistas previas (sin guardar)');
  x('Dati e comandi', 'Data and controls', 'Datos y controles');

  // ---- barra comandi e impostazioni ---------------------------------------------------------------------------
  x('Apri un\'immagine già salvata', 'Open a saved image', 'Abrir una imagen ya guardada');
  x('Apri', 'Open', 'Abrir');
  x('Rilegge i dati dell\'immagine aperta', 'Re-reads the data of the open image', 'Vuelve a leer los datos de la imagen abierta');
  x('Rileggi', 'Re-read', 'Releer');
  x('Esporta PDF (max 1 MB)', 'Export PDF (max 1 MB)', 'Exportar PDF (máx. 1 MB)');
  x('Nome di riserva, driver, risoluzione', 'Fallback name, driver, resolution, language', 'Nombre alternativo, controlador, resolución, idioma');
  x('Impostazioni', 'Settings', 'Ajustes');
  x('Nome di riserva dei file', 'Fallback file name', 'Nombre alternativo de los archivos');
  x('Si usa solo se mancano nome, cognome o numero libretto. Altrimenti i file si chiameranno', 'Used only if the first name, surname or book number is missing. Otherwise the files will be named', 'Se usa solo si faltan el nombre, el apellido o el número de libreta. En caso contrario los archivos se llamarán');
  x('NOME - COGNOME - NUMERO', 'FIRST NAME - SURNAME - NUMBER', 'NOMBRE - APELLIDO - NÚMERO');
  x('. Finché non confermi non si scrive nulla in', '. Until you confirm, nothing is written in', '. Hasta que confirmes no se escribe nada en');
  x('Driver dello scanner', 'Scanner driver', 'Controlador del escáner');
  x('wia (consigliato per libretti veri)', 'wia (recommended for real books)', 'wia (recomendado para libretas reales)');
  x('twain (solo fogli stampati)', 'twain (printed sheets only)', 'twain (solo hojas impresas)');
  x('Risoluzione (dpi)', 'Resolution (dpi)', 'Resolución (ppp)');
  x('Lunghezza letta (mm)', 'Length read (mm)', 'Longitud leída (mm)');
  x('Margine sopra il titolo (mm)', 'Margin above the title (mm)', 'Margen sobre el título (mm)');
  x('WIA: l\'immagine esce già dritta. TWAIN: consigliato solo per fogli stampati (l\'immagine esce capovolta e il programma la ruota di 180 gradi).', 'WIA: the image comes out already upright. TWAIN: recommended only for printed sheets (the image comes out upside down and the program rotates it 180 degrees).', 'WIA: la imagen sale ya derecha. TWAIN: recomendado solo para hojas impresas (la imagen sale al revés y el programa la gira 180 grados).');
  x('Con 190 mm la lettura copre la pagina dati sia dal bordo inserito per primo sia da quello in mano (il libretto aperto è lungo circa 176 mm). La scansione è sempre ruotata di 180 gradi (dritta) e ritagliata sul documento.', 'With 190 mm the reading covers the data page whether it is the edge inserted first or the one held in your hand (the open book is about 176 mm long). The scan is always rotated 180 degrees (upright) and cropped to the document.', 'Con 190 mm la lectura cubre la página de datos tanto desde el borde que se inserta primero como desde el que se tiene en la mano (la libreta abierta mide unos 176 mm). El escaneo se gira siempre 180 grados (derecho) y se recorta al documento.');
  x('Ripristina valori consigliati', 'Restore recommended values', 'Restaurar valores recomendados');
  x('Impostazioni consigliate ripristinate.', 'Recommended settings restored.', 'Ajustes recomendados restaurados.');
  x('La risoluzione deve essere tra 75 e 600 dpi.', 'The resolution must be between 75 and 600 dpi.', 'La resolución debe estar entre 75 y 600 ppp.');
  x('La lunghezza letta deve essere tra 30 e 356 mm.', 'The length read must be between 30 and 356 mm.', 'La longitud leída debe estar entre 30 y 356 mm.');
  x('Il margine sopra il titolo deve essere tra -1 e 20 mm.', 'The margin above the title must be between -1 and 20 mm.', 'El margen sobre el título debe estar entre -1 y 20 mm.');
  x('- scegli un\'immagine -', '- choose an image -', '- elige una imagen -');
  x('(nessuna immagine in scans)', '(no images in scans)', '(ninguna imagen en scans)');
  p(/^(.+?)\s+\(salvato\)$/, '{1}  (saved)', '{1}  (guardado)');
  x('Scegli prima un\'immagine dall\'elenco.', 'First choose an image from the list.', 'Primero elige una imagen de la lista.');

  // ---- lavori in corso ----------------------------------------------------------------------------------------
  x('Lavoro in corso', 'Work in progress', 'Trabajo en curso');
  x('Lavoro in corso, durata non prevedibile', 'Work in progress, duration unknown', 'Trabajo en curso, duración desconocida');
  x('Anteprima di PROVA', 'TEST preview', 'Vista previa de PRUEBA');
  x('Anteprima (scansione)', 'Preview (scan)', 'Vista previa (escaneo)');
  x('Scansione e lettura', 'Scan and read', 'Escaneo y lectura');
  x('Lettura dei dati', 'Reading the data', 'Lectura de los datos');
  x('Prova: nessuno scanner in uso.', 'Test: no scanner in use.', 'Prueba: no se usa ningún escáner.');
  x('La scansione fisica non si può interrompere: attendi la fine. Non estrarre il libretto.', 'The physical scan cannot be interrupted: wait for it to finish. Do not pull out the book.', 'El escaneo físico no se puede interrumpir: espera a que termine. No saques la libreta.');
  x('Anteprima pronta.', 'Preview ready.', 'Vista previa lista.');
  x('Taglio automatico fatto.', 'Auto crop done.', 'Recorte automático hecho.');
  x('Annullato.', 'Cancelled.', 'Cancelado.');
  x('Operazione non riuscita.', 'The operation failed.', 'La operación no se pudo completar.');
  p(/^Si apre «(.+)»\.$/, 'Opening «{1}».', 'Se abre «{1}».');
  x('L\'immagine è molto bassa: controlla di aver inserito il libretto con il bordo MRZ per primo, centrato e spinto fino in fondo.', 'The image is very short: check that you inserted the book with the MRZ edge first, centered and pushed all the way in.', 'La imagen es muy baja: comprueba que insertaste la libreta con el borde MRZ primero, centrada y empujada hasta el fondo.');
  x('Se vedi solo l\'informativa, premi «Riscansiona».', 'If you only see the information page, press «Rescan».', 'Si solo ves la hoja informativa, pulsa «Volver a escanear».');
  x('Il libretto è inserito?', 'Is the book inserted?', '¿Está insertada la libreta?');
  x('Pagina con la foto aperta, bordo con le righe MRZ inserito per primo.', 'Page with the photo open, edge with the MRZ lines inserted first.', 'Página con la foto abierta, borde con las líneas MRZ insertado primero.');
  x('Centrato, dritto e spinto fino in fondo.', 'Centered, straight and pushed all the way in.', 'Centrada, derecha y empujada hasta el fondo.');
  x('Altri programmi che usano lo scanner chiusi.', 'Other programs that use the scanner are closed.', 'Los demás programas que usan el escáner están cerrados.');
  x('La scansione dura circa 10 secondi e non si può interrompere.', 'The scan takes about 10 seconds and cannot be interrupted.', 'El escaneo dura unos 10 segundos y no se puede interrumpir.');
  x('L\'immagine resta un\'anteprima temporanea: in scans non viene scritto nulla finché non confermi.', 'The image stays a temporary preview: nothing is written to scans until you confirm.', 'La imagen sigue siendo una vista previa temporal: no se escribe nada en scans hasta que confirmes.');
  x('Sì, avvia la scansione', 'Yes, start the scan', 'Sí, iniciar el escaneo');
  x('No, annulla', 'No, cancel', 'No, cancelar');

  // ---- schermata vuota e didascalia ---------------------------------------------------------------------------
  x('Nessun libretto letto', 'No book read', 'Ninguna libreta leída');
  x('Premi «Anteprima» con il libretto inserito, oppure apri un\'immagine già salvata. I dati letti compariranno qui e potrai correggerli: il programma', 'Press «Preview» with the book inserted, or open an image you already saved. The data read will appear here and you can correct it: the program', 'Pulsa «Vista previa» con la libreta insertada, o abre una imagen ya guardada. Los datos leídos aparecerán aquí y podrás corregirlos: el programa');
  x('imparerà', 'will learn', 'aprenderá');
  x('le tue correzioni.', 'from your corrections.', 'de tus correcciones.');
  x('Anteprima non ancora letta', 'Preview not read yet', 'Vista previa aún sin leer');
  x('Guarda l\'immagine a sinistra: si vede la pagina dati con le due righe MRZ in fondo? Allora premi «Leggi i dati». Se no, «Riscansiona» (o «Scarta»). Finché non confermi non viene creato nessun file in scans.', 'Look at the image on the left: can you see the data page with the two MRZ lines at the bottom? Then press «Read data». If not, «Rescan» (or «Discard»). No file is created in scans until you confirm.', 'Mira la imagen de la izquierda: ¿se ve la página de datos con las dos líneas MRZ al final? Entonces pulsa «Leer los datos». Si no, «Volver a escanear» (o «Descartar»). No se crea ningún archivo en scans hasta que confirmes.');
  x('Premi «Anteprima» con il libretto inserito: vedrai l\'immagine prima di leggerla e salvarla. Oppure apri un\'immagine già salvata.', 'Press «Preview» with the book inserted: you will see the image before reading and saving it. Or open an image you already saved.', 'Pulsa «Vista previa» con la libreta insertada: verás la imagen antes de leerla y guardarla. O abre una imagen ya guardada.');
  x('anteprima NON salvata', 'preview NOT saved', 'vista previa SIN guardar');
  x('non salvata', 'not saved', 'sin guardar'); x('letta', 'read', 'leída'); x('bassa', 'low', 'baja');
  x('immagine molto bassa', 'very low image', 'imagen muy baja');
  p(/^letta alle (.+)$/, 'read at {1}', 'leída a las {1}');
  p(/^vista: (.+)$/, 'view: {1}', 'vista: {1}');
  p(/^verrà salvata: (.+)$/, 'will be saved: {1}', 'se guardará: {1}');
  p(/^anteprima NON salvata \((\d+) x (\d+) px\)$/, 'preview NOT saved ({1} x {2} px)', 'vista previa SIN guardar ({1} x {2} px)');
  x('solo la pagina dati', 'data page only', 'solo la página de datos');
  x('pagina intera (passaporto con informativa)', 'whole page (passport with information page)', 'página entera (pasaporte con hoja informativa)');
  x('scansione GREZZA (come esce dallo scanner)', 'RAW scan (as it comes out of the scanner)', 'escaneo SIN PROCESAR (tal como sale del escáner)');
  x('la pagina intera', 'the whole page', 'la página entera');
  p(/^Anteprima (.+), non salvata, dati letti$/, 'Preview {1}, not saved, data read', 'Vista previa {1}, sin guardar, datos leídos');
  p(/^Anteprima (.+), non salvata$/, 'Preview {1}, not saved', 'Vista previa {1}, sin guardar');
  x('Nascondi questo avviso', 'Hide this notice', 'Ocultar este aviso');
  x('Nascondi l\'avviso sulla posizione del libretto', 'Hide the notice about the book position', 'Ocultar el aviso sobre la posición de la libreta');
  x('Come è stato inserito il libretto', 'How the book was inserted', 'Cómo se insertó la libreta');
  x('Posizione del libretto: nessun problema trovato', 'Book position: no problem found', 'Posición de la libreta: no se encontró ningún problema');

  // ---- taglio ---------------------------------------------------------------------------------------------------
  p(/^Area: (\d+) x (\d+) pixel, quattro angoli liberi \(raddrizzata\)\.$/, 'Area: {1} x {2} pixels, four free corners (straightened).', 'Área: {1} x {2} píxeles, cuatro esquinas libres (enderezada).');
  p(/^Area: (\d+) x (\d+) pixel( \(3:4\))?\.$/, 'Area: {1} x {2} pixels{3}.', 'Área: {1} x {2} píxeles{3}.');
  x('Tira gli angoli, trascina dentro per spostare tutto, fuori per disegnarne uno nuovo.', 'Drag the corners, drag inside to move everything, outside to draw a new one.', 'Tira de las esquinas, arrastra dentro para mover todo, fuera para dibujar uno nuevo.');
  x('Doppio clic o rotellina = zoom sul punto; con un angolo in mano compare la lente 4x.', 'Double-click or mouse wheel = zoom on the point; the 4x lens appears while you hold a corner.', 'Doble clic o rueda = zoom en ese punto; al sujetar una esquina aparece la lupa 4x.');
  x('Trascina fuori dal rettangolo per disegnarne uno nuovo, dentro per spostarlo, i punti per ridimensionarlo.', 'Drag outside the rectangle to draw a new one, inside to move it, the handles to resize it.', 'Arrastra fuera del rectángulo para dibujar uno nuevo, dentro para moverlo, los puntos para cambiar su tamaño.');
  x('Doppio clic o rotellina = zoom sul punto; trascinando un punto compare la lente 4x.', 'Double-click or mouse wheel = zoom on the point; the 4x lens appears while you drag a handle.', 'Doble clic o rueda = zoom en ese punto; al arrastrar un punto aparece la lupa 4x.');
  x('Apri prima l\'immagine (premi «Apri»).', 'Open the image first (press «Open»).', 'Abre primero la imagen (pulsa «Abrir»).');
  x('Immagine non caricata: riprova.', 'Image not loaded: try again.', 'Imagen no cargada: inténtalo de nuevo.');
  x('Sostituire l\'immagine salvata?', 'Replace the saved image?', '¿Sustituir la imagen guardada?');
  p(/^Il taglio sostituisce «(.+)» in scans\.$/, 'The crop replaces «{1}» in scans.', 'El recorte sustituye «{1}» en scans.');
  p(/^Il taglio automatico sostituisce «(.+)» in scans\.$/, 'The auto crop replaces «{1}» in scans.', 'El recorte automático sustituye «{1}» en scans.');
  p(/^L'originale resta salvato come «(.+)», così puoi rifare il taglio quando vuoi\.$/, 'The original stays saved as «{1}», so you can redo the crop whenever you like.', 'El original se conserva como «{1}», así puedes rehacer el recorte cuando quieras.');
  p(/^L'originale resta salvato come «(.+)»\.$/, 'The original stays saved as «{1}».', 'El original se conserva como «{1}».');
  x('JSON e PDF già salvati non cambiano.', 'JSON and PDF files already saved do not change.', 'Los JSON y PDF ya guardados no cambian.');
  x('Sì, taglia', 'Yes, crop', 'Sí, recortar');
  p(/^Taglio manuale applicato: il risultato è in «(.+)»; l'originale non è cambiato\.$/, 'Manual crop applied: the result is in «{1}»; the original has not changed.', 'Recorte manual aplicado: el resultado está en «{1}»; el original no ha cambiado.');
  p(/^Taglio manuale applicato\. L'originale è in «(.+)»\.$/, 'Manual crop applied. The original is in «{1}».', 'Recorte manual aplicado. El original está en «{1}».');
  x('Taglio manuale applicato: è la nuova pagina intera.', 'Manual crop applied: it is the new whole page.', 'Recorte manual aplicado: es la nueva página entera.');
  x('Premi «Leggi i dati» per rifare anche la pagina dati.', 'Press «Read data» to redo the data page as well.', 'Pulsa «Leer los datos» para rehacer también la página de datos.');
  x('Scartare questa anteprima?', 'Discard this preview?', '¿Descartar esta vista previa?');
  p(/^Si cancella solo il file temporaneo «(.+)», creato dal programma in dati\\anteprima\.$/, 'Only the temporary file «{1}», created by the program in dati\\anteprima, is deleted.', 'Solo se borra el archivo temporal «{1}», creado por el programa en dati\\anteprima.');
  x('I libretti già salvati in scans non vengono toccati.', 'Books already saved in scans are not touched.', 'Las libretas ya guardadas en scans no se tocan.');
  x('Anteprima scartata.', 'Preview discarded.', 'Vista previa descartada.');

  // ---- avvisi e contatori -------------------------------------------------------------------------------------
  x('Da sapere', 'Good to know', 'Conviene saber');
  x('1 avviso da sapere', '1 notice to know about', '1 aviso que conviene saber');
  p(/^(\d+) avvisi da sapere$/, '{1} notices to know about', '{1} avisos que conviene saber');
  x('Tutti i campi controllati', 'All fields checked', 'Todos los campos revisados');
  x('Nessun campo da controllare', 'No fields to check', 'Ningún campo por revisar');
  x('1 campo da controllare', '1 field to check', '1 campo por revisar');
  p(/^(\d+) campi da controllare$/, '{1} fields to check', '{1} campos por revisar');
  x('Vai al primo campo da controllare', 'Go to the first field to check', 'Ir al primer campo por revisar');
  x('Campi vuoti: compilali se sul libretto ci sono, altrimenti vanno bene vuoti.', 'Empty fields: fill them in if they are on the book, otherwise empty is fine.', 'Campos vacíos: rellénalos si están en la libreta; si no, pueden quedar vacíos.');

  // ---- dati del libretto --------------------------------------------------------------------------------------
  x('Dati del libretto', 'Book data', 'Datos de la libreta');
  x('Copia i campi negli appunti', 'Copy the fields to the clipboard', 'Copiar los campos al portapapeles');
  x('Formato', 'Format', 'Formato');
  x('Formato della copia', 'Copy format', 'Formato de copia');
  x('Date', 'Dates', 'Fechas');
  x('Formato delle date', 'Date format', 'Formato de las fechas');
  x('Copia tutti i campi nel formato scelto (Alt+Maiusc+A). Alt+Maiusc+C copia il campo su cui ti trovi: si copia il valore che vedi, con le tue correzioni.', 'Copy all fields in the chosen format (Alt+Shift+A). Alt+Shift+C copies the field you are on: the value you see is copied, with your corrections.', 'Copia todos los campos en el formato elegido (Alt+Mayús+A). Alt+Mayús+C copia el campo en el que estás: se copia el valor que ves, con tus correcciones.');
  x('Copia tutto', 'Copy all', 'Copiar todo');
  x('Copia il nome base dei file (NOME - COGNOME - NUMERO)', 'Copy the base file name (FIRST NAME - SURNAME - NUMBER)', 'Copiar el nombre base de los archivos (NOMBRE - APELLIDO - NÚMERO)');
  x('Copia nome', 'Copy name', 'Copiar nombre');
  x('Si salva sempre la sola pagina dati ritagliata (se trovata). Le altre servono a capire problemi di inserimento.', 'Only the cropped data page is always saved (if found). The others help to understand insertion problems.', 'Siempre se guarda solo la página de datos recortada (si se encuentra). Las otras sirven para entender problemas de inserción.');
  x('Conserva anche la scansione intera', 'Also keep the full scan', 'Conservar también el escaneo completo');
  x('Conserva anche la grezza', 'Also keep the raw scan', 'Conservar también el escaneo sin procesar');
  x('Campi letti dal libretto: modifica il valore se è sbagliato', 'Fields read from the book: edit the value if it is wrong', 'Campos leídos de la libreta: modifica el valor si es incorrecto');
  x('Campo stampato', 'Printed field', 'Campo impreso');
  x('Valore', 'Value', 'Valor');
  x('Stato', 'Status', 'Estado');
  x('Modificato da te', 'Edited by you', 'Modificado por ti');
  x('Controllato (da memoria)', 'Checked (from memory)', 'Revisado (de la memoria)');
  x('Controllato', 'Checked', 'Revisado');
  x('Da controllare', 'To check', 'Por revisar');
  x('Ho guardato questo campo e va bene (Alt+C)', 'I looked at this field and it is fine (Alt+C)', 'He mirado este campo y está bien (Alt+C)');
  p(/^Segna controllato: (.+)$/, 'Mark as checked: {1}', 'Marcar como revisado: {1}');
  x('Rimetti da controllare', 'Put back to check', 'Volver a poner por revisar');
  p(/^Rimetti da controllare: (.+)$/, 'Put back to check: {1}', 'Volver a poner por revisar: {1}');
  x('Letto:', 'Read:', 'Leído:');
  x('(vuoto)', '(empty)', '(vacío)');
  p(/^, corretto da te il (.+)$/, ', corrected by you on {1}', ', corregido por ti el {1}');
  x('Era giusto, dimentica', 'It was right, forget it', 'Estaba bien, olvidar');
  x('Annulla modifica', 'Undo edit', 'Deshacer modificación');
  x('Rimetti il valore letto', 'Restore the value read', 'Restaurar el valor leído');
  p(/^Motivo del controllo: (.+)$/, 'Reason to check: {1}', 'Motivo de la revisión: {1}');
  p(/^Copia il valore di «(.+)» \(Alt\+Maiusc\+C\)$/, 'Copy the value of «{1}» (Alt+Shift+C)', 'Copiar el valor de «{1}» (Alt+Mayús+C)');

  // ---- nome dei file, memoria ---------------------------------------------------------------------------------
  p(/^Mancano nome, cognome o numero libretto: dopo la conferma i file si chiameranno «(.+)» \(il nome di riserva\)\.$/, 'First name, surname or book number is missing: after confirming, the files will be named «{1}» (the fallback name).', 'Faltan el nombre, el apellido o el número de libreta: tras confirmar, los archivos se llamarán «{1}» (el nombre alternativo).');
  x('Per ora non c\'è nulla in scans.', 'For now there is nothing in scans.', 'Por ahora no hay nada en scans.');
  p(/^Mancano nome, cognome o numero libretto: dopo la conferma i file restano «(.+)»\.$/, 'First name, surname or book number is missing: after confirming, the files stay «{1}».', 'Faltan el nombre, el apellido o el número de libreta: tras confirmar, los archivos siguen llamándose «{1}».');
  x('I file si chiamano già', 'The files are already named', 'Los archivos ya se llaman');
  x('Dopo la conferma i file si chiameranno:', 'After confirming, the files will be named:', 'Tras confirmar, los archivos se llamarán:');
  x('(solo con «Conferma e salva»; per ora è un\'anteprima temporanea e in scans non c\'è nulla).', '(only with «Confirm and save»; for now it is a temporary preview and there is nothing in scans).', '(solo con «Confirmar y guardar»; por ahora es una vista previa temporal y no hay nada en scans).');
  p(/^\(solo con «Conferma e salva»; ora si chiamano «(.+)»\)\.$/, '(only with «Confirm and save»; they are now named «{1}»).', '(solo con «Confirmar y guardar»; ahora se llaman «{1}»).');
  p(/^Memoria: ho corretto da solo (\d+) campo di questo libretto con le tue correzioni precedenti\.$/, 'Memory: I corrected {1} field of this book on my own using your previous corrections.', 'Memoria: he corregido por mi cuenta {1} campo de esta libreta con tus correcciones anteriores.');
  p(/^Memoria: ho corretto da solo (\d+) campi di questo libretto con le tue correzioni precedenti\.$/, 'Memory: I corrected {1} fields of this book on my own using your previous corrections.', 'Memoria: he corregido por mi cuenta {1} campos de esta libreta con tus correcciones anteriores.');
  x('Controlla e conferma.', 'Check and confirm.', 'Revisa y confirma.');
  p(/^Questo libretto è nella memoria \((\d+) correzione\), ma questa volta la lettura era già giusta\.$/, 'This book is in memory ({1} correction), but this time the reading was already right.', 'Esta libreta está en la memoria ({1} corrección), pero esta vez la lectura ya era correcta.');
  p(/^Questo libretto è nella memoria \((\d+) correzioni\), ma questa volta la lettura era già giusta\.$/, 'This book is in memory ({1} corrections), but this time the reading was already right.', 'Esta libreta está en la memoria ({1} correcciones), pero esta vez la lectura ya era correcta.');
  x('Libretto nuovo per la memoria: se correggi dei campi li ricorderò per la prossima scansione.', 'New book for the memory: if you correct fields I will remember them for the next scan.', 'Libreta nueva para la memoria: si corriges campos los recordaré en el próximo escaneo.');

  // ---- MRZ -------------------------------------------------------------------------------------------------------
  x('MRZ non trovata.', 'MRZ not found.', 'MRZ no encontrada.');
  x('Righe MRZ come lette', 'MRZ lines as read', 'Líneas MRZ tal como se leyeron');
  x('Copia le righe MRZ', 'Copy the MRZ lines', 'Copiar las líneas MRZ');
  x('Riga 1', 'Line 1', 'Línea 1'); x('Riga 2', 'Line 2', 'Línea 2'); x('Entrambe', 'Both', 'Ambas');
  x('Correzioni applicate dal lettore', 'Corrections applied by the reader', 'Correcciones aplicadas por el lector');
  x('Controlli MRZ: tutti corretti', 'MRZ checks: all correct', 'Controles MRZ: todos correctos');
  p(/^(.+): corretto$/, '{1}: correct', '{1}: correcto');
  p(/^(.+): ERRATO$/, '{1}: WRONG', '{1}: INCORRECTO');

  // ---- barra di salvataggio ------------------------------------------------------------------------------------
  x('Nessuna modifica.', 'No changes.', 'Sin cambios.');
  p(/^(.+ (?:modificat[oi]|controllo cambiato))\.$/, '{1}.', '{1}.');
  p(/^(\d+) campo modificato$/, '{1} field edited', '{1} campo modificado');
  p(/^(\d+) campi modificati$/, '{1} fields edited', '{1} campos modificados');
  p(/^(\d+) campo con il controllo cambiato$/, '{1} field with a changed check', '{1} campo con la revisión cambiada');
  p(/^(\d+) campi con il controllo cambiato$/, '{1} fields with a changed check', '{1} campos con la revisión cambiada');
  p(/^(\d+ campi? modificat[oi]) e (\d+ campi? con il controllo cambiato)$/, '{1} and {2}', '{1} y {2}');
  x('Resta 1 campo da controllare.', '1 field left to check.', 'Queda 1 campo por revisar.');
  p(/^Restano (\d+) campi da controllare\.$/, '{1} fields left to check.', 'Quedan {1} campos por revisar.');
  x('Se i dati sono giusti puoi confermare.', 'If the data is right you can confirm.', 'Si los datos son correctos puedes confirmar.');
  x('La memoria non è utilizzabile (MRZ non valida): non verrà ricordato nulla.', 'The memory cannot be used (invalid MRZ): nothing will be remembered.', 'La memoria no se puede usar (MRZ no válida): no se recordará nada.');
  x('Salvataggio in corso…', 'Saving…', 'Guardando…');
  x('Salvataggio…', 'Saving…', 'Guardando…');
  x('Salvato', 'Saved', 'Guardado');
  p(/^✓ Salvato alle (\S+)\. (.+)$/, '✓ Saved at {1}. {2}', '✓ Guardado a las {1}. {2}');
  p(/^in scans\\ come «(.+)»$/, 'in scans\\ as «{1}»', 'en scans\\ como «{1}»');
  p(/^Salvato: (.+)$/, 'Saved: {1}', 'Guardado: {1}');
  x('Nuovo libretto', 'New book', 'Nueva libreta');
  x('Salva correzioni', 'Save corrections', 'Guardar correcciones');
  x('Conferma e salva', 'Confirm and save', 'Confirmar y guardar');
  x('Ci sono campi ancora da controllare', 'Some fields still need checking', 'Hay campos que aún hay que revisar');
  x('Questi campi non sono stati controllati:', 'These fields have not been checked:', 'Estos campos no se han revisado:');
  x('Se confermi comunque, nella verità per i test questi campi restano vuoti (non vengono dati per giusti).', 'If you confirm anyway, these fields stay empty in the ground truth used for tests (they are not assumed to be right).', 'Si confirmas de todos modos, en la verdad de referencia para las pruebas estos campos quedan vacíos (no se dan por correctos).');
  x('Conferma comunque', 'Confirm anyway', 'Confirmar de todos modos');
  x('Torna a controllare', 'Go back to check', 'Volver a revisar');
  x('Sostituire i file esistenti?', 'Replace the existing files?', '¿Sustituir los archivos existentes?');
  x('Questi file esistono già:', 'These files already exist:', 'Estos archivos ya existen:');
  x('La verità precedente viene conservata con il suffisso .prec-DATA.', 'The previous ground truth is kept with the suffix .prec-DATE.', 'La verdad de referencia anterior se conserva con el sufijo .prec-FECHA.');
  x('Sostituisci', 'Replace', 'Sustituir');
  x('Dimenticato: il campo torna al valore letto.', 'Forgotten: the field goes back to the value read.', 'Olvidado: el campo vuelve al valor leído.');

  // ---- ricomincia, PDF ----------------------------------------------------------------------------------------
  x('Ricominciare da capo?', 'Start over?', '¿Empezar de nuevo?');
  p(/^Hai (.+) non salvati\.$/, 'You have {1} not saved.', 'Tienes {1} sin guardar.');
  p(/^Hai (\d+) anteprima non salvata \(restano in dati\\anteprima e si eliminano dopo 7 giorni\)\.$/, 'You have {1} unsaved preview (it stays in dati\\anteprima and is deleted after 7 days).', 'Tienes {1} vista previa sin guardar (queda en dati\\anteprima y se elimina a los 7 días).');
  p(/^Hai (\d+) anteprime non salvate \(restano in dati\\anteprima e si eliminano dopo 7 giorni\)\.$/, 'You have {1} unsaved previews (they stay in dati\\anteprima and are deleted after 7 days).', 'Tienes {1} vistas previas sin guardar (quedan en dati\\anteprima y se eliminan a los 7 días).');
  x('Non viene cancellato nessun file e la memoria resta com\'è.', 'No file is deleted and the memory stays as it is.', 'No se borra ningún archivo y la memoria queda como está.');
  x('Salva quella selezionata e ricomincia', 'Save the selected one and start over', 'Guardar la seleccionada y empezar de nuevo');
  x('Salva e ricomincia', 'Save and start over', 'Guardar y empezar de nuevo');
  x('Ricomincia senza salvare', 'Start over without saving', 'Empezar de nuevo sin guardar');
  x('La lettura non si è fermata: riprova tra un istante.', 'The reading did not stop: try again in a moment.', 'La lectura no se detuvo: inténtalo de nuevo en un momento.');
  x('Modifiche non salvate', 'Unsaved changes', 'Cambios sin guardar');
  p(/^Hai (.+) senza salvare\. Se continui vanno perse\.$/, 'You have unsaved changes: {1}. If you continue they are lost.', 'Tienes cambios sin guardar: {1}. Si continúas, se perderán.');
  x('Continua senza salvare', 'Continue without saving', 'Continuar sin guardar');
  x('Resta qui', 'Stay here', 'Quedarse aquí');
  x('Continua', 'Continue', 'Continuar');
  x('Creo il PDF...', 'Creating the PDF...', 'Creando el PDF...');
  p(/^PDF creato: (.+), (\d+) KB\.$/, 'PDF created: {1}, {2} KB.', 'PDF creado: {1}, {2} KB.');
  p(/^PDF creato: (.+), (\d+) KB \((.+)\)\.$/, 'PDF created: {1}, {2} KB ({3}).', 'PDF creado: {1}, {2} KB ({3}).');
  x('È un PDF dell\'anteprima: con «Conferma e salva» passa in scans con lo stesso nome.', 'It is a PDF of the preview: with «Confirm and save» it moves to scans with the same name.', 'Es un PDF de la vista previa: con «Confirmar y guardar» pasa a scans con el mismo nombre.');
  x('ATTENZIONE: supera 1 MB.', 'WARNING: it is over 1 MB.', 'ATENCIÓN: supera 1 MB.');
  x('Sostituire il PDF?', 'Replace the PDF?', '¿Sustituir el PDF?');

  // ---- memoria (finestra) -------------------------------------------------------------------------------------
  x('Libretti ricordati', 'Remembered books', 'Libretas recordadas');
  x('Qui ci sono le correzioni che hai fatto: quando lo stesso libretto viene scansionato di nuovo, i campi corretti si sistemano da soli. Il file', 'These are the corrections you made: when the same book is scanned again, the corrected fields fix themselves. The file', 'Aquí están las correcciones que hiciste: cuando se vuelve a escanear la misma libreta, los campos corregidos se arreglan solos. El archivo');
  x('contiene dati personali.', 'contains personal data.', 'contiene datos personales.');
  x('Carico...', 'Loading...', 'Cargando...');
  x('Nessun libretto ricordato. Correggi un campo e premi "Salva correzioni": il programma ricorderà la correzione per quel libretto.', 'No book remembered. Correct a field and press "Save corrections": the program will remember the correction for that book.', 'Ninguna libreta recordada. Corrige un campo y pulsa "Guardar correcciones": el programa recordará la corrección de esa libreta.');
  x('(senza nome)', '(no name)', '(sin nombre)');
  p(/^(.+) - aggiornato (.+)$/, '{1} - updated {2}', '{1} - actualizado {2}');
  x('Dimentica libretto', 'Forget book', 'Olvidar libreta');
  x('Dimentica', 'Forget', 'Olvidar');
  x('controllato:', 'checked:', 'revisado:');
  x('letto:', 'read:', 'leído:');
  x('- corretto:', '- corrected:', '- corregido:');
  x('Dimenticare questa correzione?', 'Forget this correction?', '¿Olvidar esta corrección?');
  x('Dimenticare tutto il libretto?', 'Forget the whole book?', '¿Olvidar toda la libreta?');
  x('La prossima volta questo campo non verrà più corretto da solo.', 'Next time this field will no longer be corrected automatically.', 'La próxima vez este campo ya no se corregirá solo.');
  x('Tutte le correzioni ricordate per questo libretto vengono tolte.', 'All the corrections remembered for this book are removed.', 'Se quitan todas las correcciones recordadas de esta libreta.');

  // ---- copia negli appunti ------------------------------------------------------------------------------------
  x('Etichetta: valore (una riga per campo)', 'Label: value (one line per field)', 'Etiqueta: valor (una línea por campo)');
  x('Solo i valori, uno per riga', 'Values only, one per line', 'Solo los valores, uno por línea');
  x('Una riga con tabulazioni (per Excel e gestionale), nell\'ordine della tabella', 'One line with tabs (for Excel and management software), in table order', 'Una línea con tabulaciones (para Excel y el sistema de gestión), en el orden de la tabla');
  x('JSON dei campi (chiavi inglesi, come libretto)', 'Fields as JSON (English keys, like libretto)', 'Campos en JSON (claves en inglés, como libretto)');
  x('GG-MM-AAAA (come stampato)', 'DD-MM-YYYY (as printed)', 'DD-MM-AAAA (como impreso)');
  x('AAAA-MM-GG (ISO)', 'YYYY-MM-DD (ISO)', 'AAAA-MM-DD (ISO)');
  x('Copiato', 'Copied', 'Copiado'); x('Vuoto', 'Empty', 'Vacío'); x('Errore', 'Error', 'Error');
  p(/^Copiato: tutti i campi, formato «(.+)»$/, 'Copied: all fields, format «{1}»', 'Copiado: todos los campos, formato «{1}»');
  p(/^Copiato: (.+)$/, 'Copied: {1}', 'Copiado: {1}');
  x('Errore: non sono riuscito a copiare negli appunti.', 'Error: I could not copy to the clipboard.', 'Error: no pude copiar al portapapeles.');
  p(/^Non riesco a scrivere negli appunti: (.+)$/, 'I cannot write to the clipboard: {1}', 'No puedo escribir en el portapapeles: {1}');
  x('Seleziona il testo a mano e premi Ctrl+C.', 'Select the text by hand and press Ctrl+C.', 'Selecciona el texto a mano y pulsa Ctrl+C.');
  x('il browser non ha dato il permesso di scrivere negli appunti.', 'the browser did not give permission to write to the clipboard.', 'el navegador no dio permiso para escribir en el portapapeles.');
  x('errore degli appunti.', 'clipboard error.', 'error del portapapeles.');
  x('questo browser non permette l\'accesso agli appunti.', 'this browser does not allow access to the clipboard.', 'este navegador no permite acceder al portapapeles.');
  p(/^(.+): il campo è vuoto, niente da copiare\.$/, '{1}: the field is empty, nothing to copy.', '{1}: el campo está vacío, nada que copiar.');
  x('Tutti i campi sono vuoti: niente da copiare.', 'All fields are empty: nothing to copy.', 'Todos los campos están vacíos: nada que copiar.');
  x('le due righe MRZ', 'the two MRZ lines', 'las dos líneas MRZ');
  p(/^riga (\d) dell'MRZ$/, 'line {1} of the MRZ', 'línea {1} de la MRZ');
  x('La riga MRZ non c\'è: niente da copiare.', 'The MRZ line is missing: nothing to copy.', 'Falta la línea MRZ: nada que copiar.');
  p(/^nome file «(.+)»$/, 'file name «{1}»', 'nombre de archivo «{1}»');
  x('Il nome del file non è ancora disponibile.', 'The file name is not available yet.', 'El nombre del archivo aún no está disponible.');
  x('Per copiare un campo spostati prima su una riga della tabella.', 'To copy a field, first move to a row of the table.', 'Para copiar un campo, colócate primero en una fila de la tabla.');
  x('Non ci sono dati da copiare.', 'There is no data to copy.', 'No hay datos que copiar.');

  // ---- avvio ed errori della pagina ----------------------------------------------------------------------------
  p(/^Non riesco a parlare con il programma \((.+)\)\.$/, 'I cannot talk to the program ({1}).', 'No puedo comunicarme con el programa ({1}).');
  x('Riavvia PanamaBook.exe.', 'Restart PanamaBook.exe.', 'Reinicia PanamaBook.exe.');
  p(/^PROVA: nessuno scanner viene usato\. "Anteprima" usa l'immagine (.+) come se fosse appena scansionata\.$/, 'TEST MODE: no scanner is used. "Preview" uses the image {1} as if it had just been scanned.', 'PRUEBA: no se usa ningún escáner. "Vista previa" usa la imagen {1} como si acabara de escanearse.');
  p(/^Manca (.+) accanto a PanamaBook\.exe: senza non si può scansionare né leggere\.$/, '{1} is missing next to PanamaBook.exe: without it you cannot scan or read.', 'Falta {1} junto a PanamaBook.exe: sin él no se puede escanear ni leer.');
  x('Il programma Panama Book non risponde più: è stato chiuso. Riavvia PanamaBook.exe.', 'The Panama Book program no longer responds: it was closed. Restart PanamaBook.exe.', 'El programa Panama Book ya no responde: se cerró. Reinicia PanamaBook.exe.');
  p(/^Errore nella pagina: (.+) \(annotato in dati\\panamabook\.log\)\.$/, 'Page error: {1} (noted in dati\\panamabook.log).', 'Error en la página: {1} (anotado en dati\\panamabook.log).');
  x('Chiudi il messaggio', 'Close the message', 'Cerrar el mensaje');

  // ---- aiuto «Come inserire il libretto» ----------------------------------------------------------------------
  x('Come inserire il libretto', 'How to insert the book', 'Cómo insertar la libreta');
  x('Chiudi', 'Close', 'Cerrar');
  x('Chiudi gli altri programmi che usano lo scanner (per esempio ScanTest).', 'Close the other programs that use the scanner (for example ScanTest).', 'Cierra los demás programas que usan el escáner (por ejemplo ScanTest).');
  x('Apri il libretto sulla pagina con la', 'Open the book on the page with the', 'Abre la libreta en la página con la');
  x('foto', 'photo', 'foto');
  x('Inserisci', 'Insert', 'Inserta');
  x('per primo il bordo con le due righe MRZ', 'first the edge with the two MRZ lines', 'primero el borde con las dos líneas MRZ');
  x('), faccia verso l\'alto.', '), face up.', '), con la cara hacia arriba.');
  x('Libretto', 'Book', 'Libreta');
  x('centrato, dritto e spinto fino in fondo', 'centered, straight and pushed all the way in', 'centrada, derecha y empujada hasta el fondo');
  x(', nell\'apertura frontale. Uno alla volta.', ', in the front slot. One at a time.', ', en la ranura frontal. De una en una.');
  x('Se si inceppa', 'If it jams', 'Si se atasca');
  x('non tirare con forza', 'do not pull hard', 'no tires con fuerza');
  x(': apri prima il coperchio dello scanner, poi estrai.', ': first open the scanner cover, then take it out.', ': abre primero la tapa del escáner y luego sácala.');
  x('Con la Epson DS-1630 puoi anche appoggiare il foglio A4 con il libretto aperto sul vetro: se il caricatore è vuoto il programma usa il vetro da solo.', 'With the Epson DS-1630 you can also put the A4 sheet with the open book on the glass: if the feeder is empty the program uses the glass by itself.', 'Con la Epson DS-1630 también puedes poner la hoja A4 con la libreta abierta sobre el cristal: si el alimentador está vacío el programa usa el cristal por sí solo.');
  x('A fine scansione ritira il libretto.', 'When the scan ends, take the book out.', 'Al terminar el escaneo, retira la libreta.');


  // ================================================================================================================
  // Messaggi del programma (cmd/panamabook/*.go): errori, avvisi, esiti dei lavori, motivi dei campi.
  // ================================================================================================================

  // ---- errori e lavori ------------------------------------------------------------------------------------------
  x('Immagine non trovata.', 'Image not found.', 'Imagen no encontrada.');
  x('immagine non trovata', 'image not found', 'imagen no encontrada');
  x('Anteprima non trovata.', 'Preview not found.', 'Vista previa no encontrada.');
  x('L\'immagine di partenza non c\'è più.', 'The source image is gone.', 'La imagen de origen ya no existe.');
  x('Immagine di partenza non leggibile.', 'The source image cannot be read.', 'La imagen de origen no se puede leer.');
  x('Un altro lavoro è ancora in corso: attendi la fine.', 'Another job is still running: wait for it to finish.', 'Otro trabajo sigue en curso: espera a que termine.');
  x('un altro lavoro è ancora in corso', 'another job is still running', 'otro trabajo sigue en curso');
  p(/^Manca (.+) accanto a PanamaBook\.exe\.$/, '{1} is missing next to PanamaBook.exe.', 'Falta {1} junto a PanamaBook.exe.');
  x('Modo di taglio sconosciuto.', 'Unknown crop mode.', 'Modo de recorte desconocido.');
  x('Taglio automatico in corso (pochi secondi).', 'Auto crop in progress (a few seconds).', 'Recorte automático en curso (unos segundos).');
  x('Puoi annullare.', 'You can cancel.', 'Puedes cancelar.');
  x('Taglio annullato.', 'Crop cancelled.', 'Recorte cancelado.');
  x('Taglio automatico: il passaporto non è stato riconosciuto (MRZ non letta), ritaglio generico al documento.', 'Auto crop: the passport was not recognized (MRZ not read), generic crop to the document.', 'Recorte automático: no se reconoció el pasaporte (MRZ no leída), recorte genérico al documento.');
  x('Taglio automatico: il passaporto non è stato riconosciuto (MRZ non letta), ritaglio generico.', 'Auto crop: the passport was not recognized (MRZ not read), generic crop.', 'Recorte automático: no se reconoció el pasaporte (MRZ no leída), recorte genérico.');
  x('Prova il taglio manuale.', 'Try the manual crop.', 'Prueba el recorte manual.');
  x('Taglio automatico fatto: si vede la pagina intera.', 'Auto crop done: the whole page is shown.', 'Recorte automático hecho: se ve la página entera.');
  p(/^Taglio automatico fatto: l'originale è in «(.+)»\.$/, 'Auto crop done: the original is in «{1}».', 'Recorte automático hecho: el original está en «{1}».');
  p(/^L'originale è in «(.+)»\.$/, 'The original is in «{1}».', 'El original está en «{1}».');
  x('Immagine di partenza sconosciuta.', 'Unknown source image.', 'Imagen de origen desconocida.');
  p(/^Scansione grezza non conservata: (.+)$/, 'Raw scan not kept: {1}', 'Escaneo sin procesar no conservado: {1}');
  p(/^Copia originale non conservata: (.+)$/, 'Original copy not kept: {1}', 'Copia original no conservada: {1}');
  x('Si può tagliare solo un\'immagine PNG o JPEG.', 'Only a PNG or JPEG image can be cropped.', 'Solo se puede recortar una imagen PNG o JPEG.');
  x('Il taglio automatico non è riuscito: l\'immagine non è stata cambiata.', 'The auto crop failed: the image was not changed.', 'El recorte automático falló: la imagen no se ha cambiado.');
  p(/^Il taglio automatico non è riuscito: (.+)$/, 'The auto crop failed: {1}', 'El recorte automático falló: {1}');
  x('L\'immagine non è stata cambiata.', 'The image was not changed.', 'La imagen no se ha cambiado.');
  p(/^Taglio automatico: (.+)$/, 'Auto crop: {1}', 'Recorte automático: {1}');
  x('servono quattro angoli', 'four corners are needed', 'hacen falta cuatro esquinas');
  x('un angolo esce dall\'immagine', 'a corner is outside the image', 'una esquina queda fuera de la imagen');
  p(/^il taglio è troppo piccolo \(almeno (\d+) x (\d+) pixel\)$/, 'the crop is too small (at least {1} x {2} pixels)', 'el recorte es demasiado pequeño (al menos {1} x {2} píxeles)');
  p(/^il taglio esce dall'immagine \((\d+) x (\d+) pixel\)$/, 'the crop goes outside the image ({1} x {2} pixels)', 'el recorte sale de la imagen ({1} x {2} píxeles)');
  x('formato immagine non ritagliabile', 'image format cannot be cropped', 'formato de imagen no recortable');
  p(/^immagine di partenza non leggibile: (.+)$/, 'source image cannot be read: {1}', 'la imagen de origen no se puede leer: {1}');
  p(/^immagine non leggibile: (.+)$/, 'image cannot be read: {1}', 'la imagen no se puede leer: {1}');
  x('non riconosciuto', 'not recognized', 'no reconocido');
  x('Ultime scansioni', 'Latest scans', 'Últimos escaneos');

  x('Scansione in corso: lo scanner sta lavorando.', 'Scan in progress: the scanner is working.', 'Escaneo en curso: el escáner está trabajando.');
  x('Non toccare il libretto e attendi (circa 10 secondi).', 'Do not touch the book and wait (about 10 seconds).', 'No toques la libreta y espera (unos 10 segundos).');
  x('PROVA (demo): nessuno scanner viene usato, si copia l\'immagine di prova.', 'TEST (demo): no scanner is used, the test image is copied.', 'PRUEBA (demo): no se usa ningún escáner, se copia la imagen de prueba.');
  p(/^Immagine di prova non copiata: (.+)$/, 'Test image not copied: {1}', 'Imagen de prueba no copiada: {1}');
  x('La scansione è finita ma il file non c\'è: controlla lo scanner e riprova.', 'The scan has finished but the file is missing: check the scanner and try again.', 'El escaneo terminó pero falta el archivo: revisa el escáner e inténtalo de nuevo.');
  x('Nessun libretto nello scanner.', 'No book in the scanner.', 'No hay ninguna libreta en el escáner.');
  x('Inseriscilo dal bordo con le righe MRZ (<<<), faccia verso l\'alto, centrato e dritto, spinto fino in fondo, e riprova.', 'Insert it by the edge with the MRZ lines (<<<), face up, centered and straight, pushed all the way in, and try again.', 'Insértala por el borde con las líneas MRZ (<<<), con la cara hacia arriba, centrada y derecha, empujada hasta el fondo, e inténtalo de nuevo.');
  x('Inceppamento.', 'Paper jam.', 'Atasco.');
  x('NON tirare con forza: apri prima il coperchio dello scanner, poi estrai il libretto.', 'Do NOT pull hard: first open the scanner cover, then take the book out.', 'NO tires con fuerza: abre primero la tapa del escáner y luego saca la libreta.');
  p(/^La scansione non è riuscita \((.+)\)\.$/, 'The scan failed ({1}).', 'El escaneo falló ({1}).');
  x('La scansione non è riuscita.', 'The scan failed.', 'El escaneo falló.');
  x('Chiudi gli altri programmi che usano lo scanner e riprova.', 'Close the other programs that use the scanner and try again.', 'Cierra los demás programas que usan el escáner e inténtalo de nuevo.');
  x('Lettura dei dati in corso (da 10 a 25 secondi).', 'Reading the data (10 to 25 seconds).', 'Leyendo los datos (de 10 a 25 segundos).');
  x('Lettura annullata.', 'Reading cancelled.', 'Lectura cancelada.');
  p(/^Lettura finita in (\d+) secondi\.$/, 'Reading finished in {1} seconds.', 'Lectura terminada en {1} segundos.');
  p(/^La lettura dei dati non è riuscita \((.+)\)\.?$/, 'Reading the data failed ({1}).', 'La lectura de los datos falló ({1}).');
  x('La lettura dei dati non è riuscita.', 'Reading the data failed.', 'La lectura de los datos falló.');
  x('nome non valido: usa lettere, cifre, punto, trattino o sottolineato (massimo 60 caratteri)', 'invalid name: use letters, digits, dot, hyphen or underscore (60 characters at most)', 'nombre no válido: usa letras, cifras, punto, guion o guion bajo (60 caracteres como máximo)');
  x('driver non valido: scegli twain o wia', 'invalid driver: choose twain or wia', 'controlador no válido: elige twain o wia');
  x('risoluzione non valida: tra 75 e 600 dpi', 'invalid resolution: between 75 and 600 dpi', 'resolución no válida: entre 75 y 600 ppp');
  x('lunghezza non valida: tra 30 e 356 mm', 'invalid length: between 30 and 356 mm', 'longitud no válida: entre 30 y 356 mm');
  x('margine non valido: tra -1 e 20 mm', 'invalid margin: between -1 and 20 mm', 'margen no válido: entre -1 y 20 mm');
  x('il file esiste già', 'the file already exists', 'el archivo ya existe');
  x('Esiste già una scansione con questo nome: scegline un altro.', 'A scan with this name already exists: choose another.', 'Ya existe un escaneo con este nombre: elige otro.');
  x('Questo lavoro non si può annullare adesso (la scansione fisica va attesa fino in fondo).', 'This job cannot be cancelled now (the physical scan has to run to the end).', 'Este trabajo no se puede cancelar ahora (el escaneo físico hay que esperarlo hasta el final).');
  x('La scansione è in corso: attendi la fine prima di ricominciare.', 'A scan is in progress: wait for it to finish before starting over.', 'Hay un escaneo en curso: espera a que termine antes de empezar de nuevo.');
  x('lavoro sconosciuto', 'unknown job', 'trabajo desconocido');
  x('documento sconosciuto', 'unknown document', 'documento desconocido');
  x('Errore interno del programma: i dettagli sono in dati\\panamabook.log.', 'Internal program error: details are in dati\\panamabook.log.', 'Error interno del programa: los detalles están en dati\\panamabook.log.');
  x('richiesta non valida', 'invalid request', 'solicitud no válida');
  x('token mancante o errato', 'missing or wrong token', 'token ausente o incorrecto');
  x('cartella scans non leggibile', 'scans folder cannot be read', 'la carpeta scans no se puede leer');

  // ---- anteprima ------------------------------------------------------------------------------------------------
  p(/^Cartella delle anteprime non creata: (.+)$/, 'Preview folder not created: {1}', 'Carpeta de vistas previas no creada: {1}');
  x('Anteprima: ritaglio della pagina dati in corso (pochi secondi).', 'Preview: cropping the data page (a few seconds).', 'Vista previa: recortando la página de datos (unos segundos).');
  x('Anteprima pronta: si vede la pagina intera; con «Pagina dati» puoi scegliere di salvare solo quella.', 'Preview ready: the whole page is shown; with «Data page» you can choose to save only that.', 'Vista previa lista: se ve la página entera; con «Página de datos» puedes elegir guardar solo esa.');
  x('Per i campi premi «Leggi i dati» (istantaneo).', 'For the fields press «Read data» (instant).', 'Para los campos pulsa «Leer los datos» (instantáneo).');
  x('La pagina dati non è stata trovata (MRZ non letta): resta la pagina intera.', 'The data page was not found (MRZ not read): the whole page stays.', 'No se encontró la página de datos (MRZ no leída): queda la página entera.');
  x('Controlla l\'inserimento (vedi i suggerimenti) e riscansiona.', 'Check how it was inserted (see the suggestions) and rescan.', 'Revisa cómo se insertó (mira las sugerencias) y vuelve a escanear.');
  x('ATTENZIONE: l\'immagine è molto bassa.', 'WARNING: the image is very short.', 'ATENCIÓN: la imagen es muy baja.');
  p(/^Anteprima non scartata: (.+)$/, 'Preview not discarded: {1}', 'Vista previa no descartada: {1}');

  // ---- posizione del libretto (diagnosi della scansione grezza) -----------------------------------------------
  x('Nella scansione grezza non si vede nessun documento: il libretto non è entrato nello scanner (o non è stato rilevato).', 'No document can be seen in the raw scan: the book did not go into the scanner (or was not detected).', 'En el escaneo sin procesar no se ve ningún documento: la libreta no entró en el escáner (o no se detectó).');
  x('Controlla l\'inserimento e riscansiona.', 'Check how it was inserted and rescan.', 'Revisa cómo se insertó y vuelve a escanear.');
  x('Nessun documento rilevato nella scansione grezza.', 'No document detected in the raw scan.', 'Ningún documento detectado en el escaneo sin procesar.');
  x('SINISTRA', 'the LEFT', 'la IZQUIERDA'); x('DESTRA', 'the RIGHT', 'la DERECHA');
  x('destro', 'right', 'derecho'); x('sinistro', 'left', 'izquierdo');
  p(/^Il libretto è decentrato verso (.+) \(come nell'anteprima\): spostalo di circa ([\d.,]+) cm verso (.+)\.$/, 'The book is off-center towards {1} (as in the preview): move it about {2} cm towards {3}.', 'La libreta está descentrada hacia {1} (como en la vista previa): muévela unos {2} cm hacia {3}.');
  p(/^Il libretto è decentrato verso (.+) e il bordo (.+) esce dal campo di lettura \(tagliato\): spostalo di circa ([\d.,]+) cm verso (.+)\.$/, 'The book is off-center towards {1} and the {2} edge goes outside the reading area (cut off): move it about {3} cm towards {4}.', 'La libreta está descentrada hacia {1} y el borde {2} sale del campo de lectura (cortado): muévela unos {3} cm hacia {4}.');
  p(/^Il libretto è entrato troppo poco: il documento rilevato è alto solo ([\d.,]+) mm invece di circa ([\d.,]+)\.$/, 'The book went in too little: the detected document is only {1} mm tall instead of about {2}.', 'La libreta entró muy poco: el documento detectado mide solo {1} mm de alto en lugar de unos {2}.');
  x('Spingilo fino in fondo, con il bordo MRZ per primo.', 'Push it all the way in, with the MRZ edge first.', 'Empújala hasta el fondo, con el borde MRZ primero.');
  p(/^Il bordo del libretto è inclinato di circa ([\d.,]+) gradi: raddrizzalo e spingilo contro la guida\.$/, 'The edge of the book is tilted by about {1} degrees: straighten it and push it against the guide.', 'El borde de la libreta está inclinado unos {1} grados: enderézala y empújala contra la guía.');
  p(/^Documento rilevato nella grezza: ([\d.,]+) x ([\d.,]+) mm; centro spostato di ([\d.,]+) mm (.+); inclinazione ([\d.,]+) gradi\.$/, 'Document detected in the raw scan: {1} x {2} mm; center shifted by {3} mm {4}; tilt {5} degrees.', 'Documento detectado en el escaneo sin procesar: {1} x {2} mm; centro desplazado {3} mm {4}; inclinación {5} grados.');
  x('a destra', 'to the right', 'a la derecha'); x('a sinistra', 'to the left', 'a la izquierda');

  // ---- campi, motivi, stati -------------------------------------------------------------------------------------
  x('Corretto da memoria', 'Corrected from memory', 'Corregido de la memoria');
  x('Non letto', 'Not read', 'No leído');
  x('OK', 'OK', 'OK');
  x('letto dall\'MRZ', 'read from the MRZ', 'leído de la MRZ');
  x('da controllare', 'to check', 'por revisar');
  x('lettura incerta', 'uncertain reading', 'lectura incierta');
  x('diverso da quello dell\'MRZ', 'different from the MRZ value', 'distinto del valor de la MRZ');
  x('lettura MRZ incerta', 'uncertain MRZ reading', 'lectura MRZ incierta');
  x('non è dopo l\'emissione', 'is not after the issue date', 'no es posterior a la emisión');
  x('emissione non prima della scadenza', 'issue date not before the expiry', 'emisión no anterior al vencimiento');
  x('non è prima dell\'emissione', 'is not before the issue date', 'no es anterior a la emisión');
  x('emissione non dopo la nascita', 'issue date not after the birth date', 'emisión no posterior al nacimiento');
  x('formato data non valido (GG-MM-AAAA)', 'invalid date format (DD-MM-YYYY)', 'formato de fecha no válido (DD-MM-AAAA)');
  x('data inesistente', 'date does not exist', 'fecha inexistente');
  x('formato insolito (P o PA e 7 cifre)', 'unusual format (P or PA and 7 digits)', 'formato inusual (P o PA y 7 cifras)');
  x('valore insolito (M o F)', 'unusual value (M or F)', 'valor inusual (M o F)');
  x('sul libretto è stampato SI', 'SI is printed on the book', 'en la libreta está impreso SI');
  p(/^l'ultima volta era stato confermato «(.+)»$/, 'last time «{1}» was confirmed', 'la última vez se confirmó «{1}»');
  x('non letto: se sul libretto c\'è scrivilo, se non c\'è va bene così', 'not read: if it is on the book type it in, if it is not there that is fine', 'no leído: si está en la libreta escríbelo, si no está no pasa nada');
  x('campo assente o vuoto', 'field missing or empty', 'campo ausente o vacío');
  x('Numero libretto', 'Book number', 'Número de libreta');
  x('Data di nascita', 'Date of birth', 'Fecha de nacimiento');
  x('Data di scadenza', 'Expiry date', 'Fecha de vencimiento');
  x('Dati opzionali', 'Optional data', 'Datos opcionales');
  x('Controllo finale', 'Final check', 'Control final');
  x('Numero libretto, Data di nascita, Data di scadenza, Dati opzionali, Controllo finale', 'Book number, Date of birth, Expiry date, Optional data, Final check', 'Número de libreta, Fecha de nacimiento, Fecha de vencimiento, Datos opcionales, Control final');
  p(/^(.+); (.+)$/, '{1}; {2}', '{1}; {2}');

  // ---- avvisi del lettore ---------------------------------------------------------------------------------------
  x('Le due righe MRZ (quelle con i simboli <<<) non sono state trovate.', 'The two MRZ lines (the ones with the <<< symbols) were not found.', 'No se encontraron las dos líneas MRZ (las de los símbolos <<<).');
  x('I dati stampati non si possono verificare e la memoria non può essere usata.', 'The printed data cannot be verified and the memory cannot be used.', 'Los datos impresos no se pueden verificar y la memoria no se puede usar.');
  x('Controlla come è inserito il libretto (bordo MRZ per primo, centrato, dritto, spinto fino in fondo) e rifai la scansione.', 'Check how the book is inserted (MRZ edge first, centered, straight, pushed all the way in) and scan again.', 'Revisa cómo está insertada la libreta (borde MRZ primero, centrada, derecha, empujada hasta el fondo) y repite el escaneo.');
  x('Le righe MRZ sono state lette, ma uno o più codici di controllo non tornano: qualche carattere è probabilmente sbagliato.', 'The MRZ lines were read, but one or more check digits do not match: some character is probably wrong.', 'Se leyeron las líneas MRZ, pero uno o más dígitos de control no cuadran: probablemente algún carácter es incorrecto.');
  x('Controlla bene tutti i dati.', 'Check all the data carefully.', 'Revisa bien todos los datos.');
  x('La memoria non viene usata.', 'The memory is not used.', 'La memoria no se usa.');
  p(/^Campi stampati non letti: (.+)\.$/, 'Printed fields not read: {1}.', 'Campos impresos no leídos: {1}.');
  x('Se sul libretto ci sono, scrivili a mano; se non ci sono (normale per alcuni modelli) va bene così.', 'If they are on the book, type them in; if they are not there (normal for some models) that is fine.', 'Si están en la libreta, escríbelos a mano; si no están (normal en algunos modelos) no pasa nada.');
  x('Cognome e nomi (riga 1 dell\'MRZ) sono incerti: due letture diverse.', 'Surname and given names (MRZ line 1) are uncertain: two different readings.', 'El apellido y los nombres (línea 1 de la MRZ) son inciertos: dos lecturas distintas.');
  x('Confrontali con il libretto.', 'Compare them with the book.', 'Compáralos con la libreta.');
  p(/^Il campo (.+) dell'MRZ è incerto: due letture diverse\.$/, 'The field {1} of the MRZ is uncertain: two different readings.', 'El campo {1} de la MRZ es incierto: dos lecturas distintas.');
  x('Confrontalo con il libretto.', 'Compare it with the book.', 'Compáralo con la libreta.');
  p(/^Lettura incerta per il campo (.+): controllalo con l'immagine\.$/, 'Uncertain reading for the field {1}: check it against the image.', 'Lectura incierta del campo {1}: revísalo con la imagen.');
  p(/^Il campo (.+) stampato è diverso da quello dell'MRZ: uno dei due è letto male\.$/, 'The printed field {1} differs from the MRZ one: one of the two was read wrongly.', 'El campo impreso {1} es distinto del de la MRZ: uno de los dos se leyó mal.');
  x('L\'MRZ di solito è più affidabile.', 'The MRZ is usually more reliable.', 'La MRZ suele ser más fiable.');
  x('La data di scadenza non è dopo la data di emissione: una delle due è letta male.', 'The expiry date is not after the issue date: one of the two was read wrongly.', 'La fecha de vencimiento no es posterior a la de emisión: una de las dos se leyó mal.');
  x('La data di nascita non è prima della data di emissione: una delle due è letta male.', 'The birth date is not before the issue date: one of the two was read wrongly.', 'La fecha de nacimiento no es anterior a la de emisión: una de las dos se leyó mal.');
  x('Il lettore ha segnalato un\'anomalia che il programma non sa descrivere: controlla i dati a mano.', 'The reader reported an anomaly the program cannot describe: check the data by hand.', 'El lector señaló una anomalía que el programa no sabe describir: revisa los datos a mano.');
  p(/^Avviso del lettore non riconosciuto \((.+)\): controlla i dati a mano\.$/, 'Unrecognized reader notice ({1}): check the data by hand.', 'Aviso del lector no reconocido ({1}): revisa los datos a mano.');
  x('MRZ non trovata: senza MRZ valida non si può riconoscere il libretto, quindi la memoria non viene né applicata né salvata.', 'MRZ not found: without a valid MRZ the book cannot be recognized, so the memory is neither applied nor saved.', 'MRZ no encontrada: sin una MRZ válida no se puede reconocer la libreta, así que la memoria ni se aplica ni se guarda.');
  x('MRZ con codici di controllo non validi: il libretto non si può riconoscere con sicurezza, quindi la memoria non viene né applicata né salvata.', 'MRZ with invalid check digits: the book cannot be recognized reliably, so the memory is neither applied nor saved.', 'MRZ con dígitos de control no válidos: la libreta no se puede reconocer con seguridad, así que la memoria ni se aplica ni se guarda.');

  // ---- salvataggio ----------------------------------------------------------------------------------------------
  x('modo di salvataggio sconosciuto', 'unknown save mode', 'modo de guardado desconocido');
  x('documento sconosciuto: rileggi l\'immagine', 'unknown document: re-read the image', 'documento desconocido: vuelve a leer la imagen');
  x('Restano dei campi da controllare.', 'Some fields still need checking.', 'Quedan campos por revisar.');
  p(/^Mancano nome, cognome o numero libretto: i file si chiamano «(.+)»\.$/, 'First name, surname or book number is missing: the files are named «{1}».', 'Faltan el nombre, el apellido o el número de libreta: los archivos se llaman «{1}».');
  p(/^Mancano nome, cognome o numero libretto: i file restano col nome «(.+)»\.$/, 'First name, surname or book number is missing: the files keep the name «{1}».', 'Faltan el nombre, el apellido o el número de libreta: los archivos conservan el nombre «{1}».');
  x('Esistono già dei file con questo nome: confermi di sostituirli? (i vecchi file della verità si conservano con il suffisso .prec)', 'Files with this name already exist: do you confirm replacing them? (old ground-truth files are kept with the suffix .prec)', 'Ya existen archivos con este nombre: ¿confirmas sustituirlos? (los archivos antiguos de verdad de referencia se conservan con el sufijo .prec)');
  x('Correzioni NON memorizzate: l\'MRZ non è valida, il libretto non si può riconoscere con sicurezza.', 'Corrections NOT remembered: the MRZ is not valid, the book cannot be recognized reliably.', 'Correcciones NO memorizadas: la MRZ no es válida, la libreta no se puede reconocer con seguridad.');
  p(/^Memoria non aggiornata: (.+)$/, 'Memory not updated: {1}', 'Memoria no actualizada: {1}');
  p(/^Memoria non salvata: (.+)$/, 'Memory not saved: {1}', 'Memoria no guardada: {1}');
  p(/^Conferme non salvate: (.+)$/, 'Confirmations not saved: {1}', 'Confirmaciones no guardadas: {1}');
  p(/^Conferme non aggiornate: (.+)$/, 'Confirmations not updated: {1}', 'Confirmaciones no actualizadas: {1}');
  x('Le conferme NON sono state ricordate: l\'MRZ non è valida, il libretto non si può riconoscere con sicurezza.', 'The confirmations were NOT remembered: the MRZ is not valid, the book cannot be recognized reliably.', 'Las confirmaciones NO se recordaron: la MRZ no es válida, la libreta no se puede reconocer con seguridad.');
  p(/^Ricordati (\d+) campi controllati: la prossima volta, se la lettura è la stessa, saranno già controllati\.$/, 'Remembered {1} checked fields: next time, if the reading is the same, they will already be checked.', 'Recordados {1} campos revisados: la próxima vez, si la lectura es la misma, ya estarán revisados.');
  p(/^Memorizzati (\d+) campi: alla prossima scansione di questo libretto saranno corretti da soli\.$/, 'Remembered {1} fields: at the next scan of this book they will be corrected automatically.', 'Memorizados {1} campos: en el próximo escaneo de esta libreta se corregirán solos.');
  p(/^Dimenticati (\d+) campi che avevi rimesso al valore letto\.$/, 'Forgot {1} fields you had put back to the value read.', 'Olvidados {1} campos que habías devuelto al valor leído.');
  x('Nessuna correzione nuova da memorizzare.', 'No new corrections to remember.', 'Ninguna corrección nueva que memorizar.');
  p(/^Immagine non salvata in scans: (.+)$/, 'Image not saved in scans: {1}', 'Imagen no guardada en scans: {1}');
  p(/^Salvata in scans la sola pagina dati ritagliata come «(.+)»\.$/, 'Only the cropped data page was saved in scans as «{1}».', 'Se guardó en scans solo la página de datos recortada como «{1}».');
  p(/^Salvata in scans la pagina intera come «(.+)»\.$/, 'The whole page was saved in scans as «{1}».', 'Se guardó en scans la página entera como «{1}».');
  p(/^Rinomina dei file non riuscita \(non è stato cambiato nulla\): (.+)$/, 'Renaming the files failed (nothing was changed): {1}', 'No se pudieron renombrar los archivos (no se cambió nada): {1}');
  p(/^File rinominati in «(.+)» \(nome - cognome - numero libretto\)\.$/, 'Files renamed to «{1}» (first name - surname - book number).', 'Archivos renombrados a «{1}» (nombre - apellido - número de libreta).');
  p(/^Non riesco a conservare la verità precedente: (.+)$/, 'I cannot keep the previous ground truth: {1}', 'No puedo conservar la verdad de referencia anterior: {1}');
  p(/^Verità non scritta: (.+)$/, 'Ground truth not written: {1}', 'Verdad de referencia no escrita: {1}');
  p(/^JSON non costruito: (.+)$/, 'JSON not built: {1}', 'JSON no construido: {1}');
  p(/^JSON non scritto: (.+)$/, 'JSON not written: {1}', 'JSON no escrito: {1}');
  p(/^Salvati scans\\(.+)\.json e la verità per la regressione \(scans\\verita\\(.+)\.json\)\.$/, 'Saved scans\\{1}.json and the ground truth for regression tests (scans\\verita\\{2}.json).', 'Guardados scans\\{1}.json y la verdad de referencia para las pruebas de regresión (scans\\verita\\{2}.json).');
  x('Questo libretto non ha una MRZ valida: non c\'è nessuna memoria da togliere.', 'This book has no valid MRZ: there is no memory to remove.', 'Esta libreta no tiene una MRZ válida: no hay memoria que quitar.');
  x('campo sconosciuto', 'unknown field', 'campo desconocido');
  x('manca il libretto', 'the book is missing', 'falta la libreta');
  p(/^Il PDF (.+) esiste già: vuoi sostituirlo\?$/, 'The PDF {1} already exists: do you want to replace it?', 'El PDF {1} ya existe: ¿quieres sustituirlo?');
  x('Questa versione di libretto.exe non sa ancora creare il PDF (comando \'libretto pdf\'), e _out\\libretto_pdf.exe non c\'è.', 'This version of libretto.exe cannot create the PDF yet (command \'libretto pdf\'), and _out\\libretto_pdf.exe is missing.', 'Esta versión de libretto.exe aún no sabe crear el PDF (comando \'libretto pdf\'), y falta _out\\libretto_pdf.exe.');
  p(/^Il PDF non è stato creato \((.+)\)\.$/, 'The PDF was not created ({1}).', 'No se creó el PDF ({1}).');
  x('Il PDF non è stato creato.', 'The PDF was not created.', 'No se creó el PDF.');

  // ---- modelli generici (sempre per ultimi) ---------------------------------------------------------------------
  p(/^Copia (.+)$/, 'Copy {1}', 'Copiar {1}');

  return { exact: X, patterns: P };
})();
