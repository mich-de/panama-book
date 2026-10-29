@echo off
rem CREA IL PDF PER IL GESTIONALE (massimo 1 MB) da una scansione gia' fatta. Non usa lo scanner.
rem Uso: tools\make-pdf.cmd NOME      legge scans\NOME.png e crea scans\NOME.pdf (una pagina, JPEG alla qualita' piu' alta che sta in 1.000.000 byte)
rem La finestra resta aperta e chiede il nome successivo (Invio vuoto = esci).
setlocal
cd /d "%~dp0.."
set "EXE=.\libretto.exe"
.\libretto.exe 2>&1 | findstr /c:"libretto pdf" >nul || set "EXE=.\_out\libretto_pdf.exe"
set "NOME=%~1"
set "ESITO=0"
if "%NOME%"=="" goto :chiedi
goto :esegui

:chiedi
set "NOME="
set /p "NOME=Nome della scansione (Invio per uscire): "
if "%NOME%"=="" exit /b %ESITO%

:esegui
set "ESITO=0"
if not exist "scans\%NOME%.png" (
    echo scans\%NOME%.png non esiste.
    set "ESITO=1"
    goto :chiedi
)
if exist "scans\%NOME%.pdf" (
    echo scans\%NOME%.pdf esiste gia': scegli un altro nome o cancellalo tu.
    set "ESITO=1"
    goto :chiedi
)
%EXE% pdf -i "scans\%NOME%.png" -o "scans\%NOME%.pdf"
if errorlevel 1 (
    set "ESITO=1"
    goto :chiedi
)
echo.
goto :chiedi
