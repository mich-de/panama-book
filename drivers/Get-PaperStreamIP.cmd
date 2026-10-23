@echo off
rem Wrapper: downloads (and verifies) the official PaperStream IP installers. Arguments are passed on, e.g.:  Get-PaperStreamIP.cmd -Arch 64 -Install
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0Get-PaperStreamIP.ps1" %*
pause
