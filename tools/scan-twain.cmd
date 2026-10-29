@echo off
rem SCANSIONA UN LIBRETTO: scrivi solo il nome. Crea scans\NOME.png: la parte del libretto dal bordo MRZ fino a circa 3 mm
rem sopra la scritta "REPUBLICA DE PANAMA", ruotata di 180 gradi (dritta) e senza i bordi bianchi. Non legge l'MRZ.
rem Driver TWAIN, una sola facciata, 300 dpi.
rem Uso: tools\scan-twain.cmd [NOME] [LUNGH_MM]
rem   LUNGH_MM  lunghezza letta dal bordo inserito per primo, predefinita 97. Misurato: la scritta parte a circa 94 mm.
rem             Per avere piu' margine sopra la scritta aumenta (es. 99), per meno diminuisci (es. 95).
rem La finestra resta aperta e chiede il nome della scansione successiva (Invio vuoto = esci).
setlocal
cd /d "%~dp0.."
set "NOME=%~1"
set "LUNGH=%~2"
if "%LUNGH%"=="" set "LUNGH=97"
set "ESITO=0"
if "%NOME%"=="" goto :chiedi
goto :esegui

:chiedi
set "NOME="
set /p "NOME=Nome del libretto (Invio per uscire): "
if "%NOME%"=="" exit /b %ESITO%

:esegui
set "ESITO=0"
if exist "scans\%NOME%.png" (
    echo scans\%NOME%.png esiste gia': scegli un altro nome.
    set "ESITO=1"
    goto :chiedi
)
echo.
echo LIBRETTO "%NOME%"  (lettura di %LUNGH% mm)
echo   1. Chiudi ScanTest o altri programmi che usano lo scanner.
echo   2. Apri il libretto sulla pagina con la FOTO.
echo   3. Inserisci PER PRIMO il bordo con le due righe di MRZ (^<^<^<^<), faccia verso l'alto, CENTRATO e dritto,
echo      spinto fino in fondo, nell'apertura FRONTALE. Un solo libretto alla volta.
echo   4. Se si inceppa: NON tirare con forza. Apri prima il coperchio dello scanner, poi estrai.
echo   5. A fine scansione ritira il libretto dall'apertura.
echo.
pause
.\libretto.exe scan -yes -o "scans\%NOME%.png" -driver twain -dpi 300 -len %LUNGH% -from 0 -rotate 180 2> "scans\%NOME%.err"
if errorlevel 1 (
    type "scans\%NOME%.err"
    set "ESITO=1"
    goto :chiedi
)
echo Fatto: scans\%NOME%.png
echo.
goto :chiedi
