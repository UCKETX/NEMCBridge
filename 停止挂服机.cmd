@echo off
chcp 65001 >nul
powershell.exe -NoProfile -ExecutionPolicy RemoteSigned -File "%~dp0scripts\stop.ps1"
if errorlevel 1 pause
