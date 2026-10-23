@echo off
rem AVVIA PANAMA BOOK con il registro degli errori: se il programma si chiude o va in crash, qui resta scritto perche'.
rem File (in dati\, contengono dati personali):
rem   dati\panamabook.log          eventi del programma con data e ora, MOTIVO dell'uscita (scritto da PanamaBook.exe stesso)
rem   dati\panamabook-crash.log    panic ed errori fatali del runtime Go (vuoto se non e' successo niente)
rem   dati\panamabook-console.log  quello che il programma stampa nella finestra nera (scritto da questo file)
rem Se nel registro manca la riga "USCITA: ..." il processo e' stato terminato dall'esterno (taskkill /F, Stop-Process, console chiusa a forza).
rem A fine esecuzione mostra le ultime righe e aspetta un tasto.
setlocal
cd /d "%~dp0.."
if not exist dati mkdir dati
echo Avvio di Panama Book. Non chiudere questa finestra finche' usi il programma.
echo %DATE% %TIME% avvio da start-panamabook.cmd >> dati\panamabook-console.log
.\PanamaBook.exe >> dati\panamabook-console.log 2>&1
set "CODICE=%errorlevel%"
echo %DATE% %TIME% terminato, codice %CODICE% >> dati\panamabook-console.log
echo.
echo Panama Book e' terminato (codice %CODICE%). Ultime righe dei registri:
echo ------------------------------------------------------------
echo [dati\panamabook.log]
call :coda dati\panamabook.log 30
echo [dati\panamabook-crash.log]
call :coda dati\panamabook-crash.log 30
echo [dati\panamabook-console.log]
call :coda dati\panamabook-console.log 10
echo ------------------------------------------------------------
pause
goto :eof

:coda
rem mostra le ultime N righe di un file senza PowerShell (un controllo di sicurezza puo' fermare i processi PowerShell)
setlocal enabledelayedexpansion
if not exist "%~1" goto :eof
set "SK=0"
for /f %%c in ('find /c /v "" ^< "%~1"') do set /a SK=%%c-%~2
if !SK! lss 0 set "SK=0"
for /f "usebackq skip=%SK% delims=" %%l in ("%~1") do (set "L=%%l" & echo(!L!)
endlocal
goto :eof
