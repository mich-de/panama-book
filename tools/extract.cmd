@echo off
rem ESTRAE I DATI JSON da una scansione gia' fatta (non usa lo scanner).
rem Uso: tools\extract.cmd NOME      legge scans\NOME.png e crea scans\NOME.json (MRZ e campi stampati)
rem La finestra resta aperta e chiede il nome successivo (Invio vuoto = esci).
setlocal
cd /d "%~dp0.."
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
if exist "scans\%NOME%.json" (
    echo scans\%NOME%.json esiste gia': scegli un altro nome o cancellalo tu.
    set "ESITO=1"
    goto :chiedi
)
echo Lettura in corso, circa 15 secondi...
.\libretto.exe extract -i "scans\%NOME%.png" > "scans\%NOME%.json" 2> "scans\%NOME%.err"
if errorlevel 1 (
    type "scans\%NOME%.err"
    set "ESITO=1"
    goto :chiedi
)
.\libretto.exe summary "scans\%NOME%.json"
rem Se libretto.exe sa rinominare i file, li chiama "NOME - COGNOME - NUMERO LIBRETTO" (solo con MRZ valida)
.\libretto.exe 2>&1 | findstr /c:"libretto rename" >nul && .\libretto.exe rename -json "scans\%NOME%.json"
echo Dati: scans\%NOME%.json
echo.
goto :chiedi
