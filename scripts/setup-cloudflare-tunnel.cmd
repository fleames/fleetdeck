@echo off
setlocal
cd /d "%~dp0.."
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0setup-cloudflare-tunnel.ps1" %*
exit /b %ERRORLEVEL%
