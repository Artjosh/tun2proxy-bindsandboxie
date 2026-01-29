@echo off
cd /d "%~dp0"
echo Solicitando privilegios de Administrador para iniciar o Arcanum Manager...
python manager\main.py
pause
