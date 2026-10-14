@echo off
chcp 65001 >nul
powershell.exe -NoProfile -ExecutionPolicy RemoteSigned -File "%~dp0启动挂服机.ps1"
if errorlevel 1 pause
