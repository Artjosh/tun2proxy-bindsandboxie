@echo off
echo Compilando Joshboxie Manager...
echo Certifique-se de que pyinstaller esta instalado (pip install pyinstaller auto-py-to-exe)

:: Clean previous builds
rmdir /s /q build
rmdir /s /q dist
del /q *.spec

:: Run PyInstaller
:: --noconsole: Esconde a janela do terminal
:: --onefile: Cria um unico arquivo .exe
:: --collect-all customtkinter: Copia os temas do customtkinter
:: --name: Nome do executavel
:: manager/main.py: Script principal

pyinstaller --noconsole --onefile --name "JoshboxieManager" --collect-all customtkinter manager/main.py

echo.
echo Compilacao finalizada!
echo O executavel esta na pasta 'dist'.
echo Mova 'JoshboxieManager.exe' para a pasta raiz (junto com config.json e tun2socks.exe) para funcionar.
pause
