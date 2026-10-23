@echo off
rem Scansione di un LIBRETTO aperto (driver WIA). Il file NOME.png e' direttamente la sola pagina dati (125x88 mm), gia' dritta e ritagliata.
rem Uso: tools\scan-wia.cmd [NOME] [DPI] [LUNGH_MM] [DA_MM]
rem   NOME       nome della scansione: crea in scans\ i file NOME.png (la pagina dati) e NOME.json
rem   DPI        risoluzione, predefinita 300
rem   LUNGH_MM   lunghezza letta in mm, predefinita 190, per inserire il bordo MRZ per primo: la scansione parte dal bordo che resta in mano (l'informativa,
rem              in alto nell'immagine) e la pagina dati sta tra 90 e 180 mm. Il driver ignora l'inizio dell'area,
rem              quindi non si puo' saltare l'informativa. Con 100 si leggerebbe solo l'informativa
rem   DA_MM      opzionale: inizio della lettura (il driver lo ignora)
rem Al termine la finestra RESTA APERTA e chiede il nome della scansione successiva (Invio vuoto per uscire).
setlocal
cd /d "%~dp0.."
set "NOME=%~1"
set "DPI=%~2"
if "%DPI%"=="" set "DPI=300"
set "LENARG=-len 190"
if not "%~3"=="" set "LENARG=-len %~3"
if not "%~4"=="" set "LENARG=%LENARG% -from %~4"
set "ESITO=0"
if "%NOME%"=="" goto :chiedi
goto :esegui

:chiedi
set "NOME="
set /p "NOME=Nome della scansione (Invio per uscire): "
if "%NOME%"=="" exit /b %ESITO%

:esegui
set "ESITO=0"
if exist "scans\%NOME%.png" (
    echo scans\%NOME%.png esiste gia': scegli un altro nome.
    set "ESITO=1"
    goto :chiedi
)
if exist "scans\%NOME%.json" (
    echo scans\%NOME%.json esiste gia': scegli un altro nome.
    set "ESITO=1"
    goto :chiedi
)
echo.
echo SCANSIONE LIBRETTO "%NOME%" a %DPI% dpi %LENARG%
echo   1. Apri il libretto sulla pagina con la FOTO.
echo   2. Inserisci PER PRIMO il bordo con le due righe di MRZ (^<^<^<^<), faccia verso l'alto, al centro, dritto,
echo      nell'apertura FRONTALE. Un solo libretto alla volta.
echo   3. Spessore della meta' inserita: massimo 5 mm.
echo   4. Se si inceppa: NON tirare con forza. Apri prima il coperchio dello scanner, poi estrai.
echo   5. A fine scansione ritira il libretto dall'apertura.
echo.
pause
.\libretto.exe read -yes -o "scans\%NOME%.png" -dpi %DPI% %LENARG% -page-only > "scans\%NOME%.json" 2> "scans\%NOME%.err"
if errorlevel 1 (
    type "scans\%NOME%.err"
    set "ESITO=1"
    goto :chiedi
)
echo Lettura terminata.
.\libretto.exe summary "scans\%NOME%.json"
echo Pagina dati: scans\%NOME%.png   Dettaglio: scans\%NOME%.json
echo.
goto :chiedi
