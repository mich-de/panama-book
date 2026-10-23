@echo off
rem Dopo aver installato PaperStream IP (TWAIN x64): controlla che il driver a 64 bit si apra e accetti l'alto dell'area.
rem NON scansiona e non serve nessun documento. Scrive scans\twain64-probe.txt.
setlocal
cd /d "%~dp0.."
if not exist "C:\Windows\twain_64" echo Manca C:\Windows\twain_64: il driver TWAIN x64 non risulta installato.
if not exist "twainscan64.exe" echo Manca twainscan64.exe: ricompilalo con  go build -o twainscan64.exe .\cmd\twainscan
if not exist "_out\twainprobe64.exe" echo Manca _out\twainprobe64.exe
_out\twainprobe64.exe -try-frame > "scans\twain64-probe.txt" 2>&1
type "scans\twain64-probe.txt" | findstr /i /c:"aperto" /c:"ESITO" /c:"data source" /c:"manca" /c:"errore"
echo.
echo Dettaglio completo: scans\twain64-probe.txt
pause
