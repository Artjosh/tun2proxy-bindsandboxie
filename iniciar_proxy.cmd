@echo off
cd /d "%~dp0"

echo ===================================================
echo LENDO CONFIGURACOES DO CONFIG.JSON...
echo ===================================================

:: Usa PowerShell para ler o JSON e gerar comandos de set variavel
for /f "usebackq tokens=1,* delims==" %%A in (`powershell -NoProfile -Command "$json = Get-Content 'config.json' -Raw | ConvertFrom-Json; $tun = $json.paths.tun2socks; Write-Host \"TUN2SOCKS=$tun\"; $i=1; foreach($p in $json.proxies_list){ Write-Host \"P[$i]=$p\"; $i++ }"`) do set %%A=%%B

if "%TUN2SOCKS%"=="" (
    echo ERRO: Caminho do tun2socks nao encontrado no config.json
    pause
    exit /b
)

echo Usando tun2socks em: %TUN2SOCKS%

echo ===================================================
echo LIMPANDO PROCESSOS ANTIGOS...
echo ===================================================
taskkill /F /IM tun2socks.exe
timeout /t 2 /nobreak > nul

:: Loop para iniciar proxies (assume até 10, ou ajustável)
:: O PowerShell exportou P[1], P[2]... P[N]
:: Vamos iterar até não encontrar mais P[i]

set I=1
:LOOP_START
call set VAL=%%P[%I%]%%
if "%VAL%"=="" goto LOOP_END

echo.
call :SETUP_PROXY %I% "%VAL%"
set /a I+=1
goto LOOP_START

:LOOP_END
echo.
echo TODOS OS PROXIES INICIADOS!
pause
exit /b

:SETUP_PROXY
set ID=%1
set VAL=%~2
set "NAME=Proxy_%ID%"
set "IP=10.0.%ID%.1"
set "GW=10.0.%ID%.254"

:: Parse IP:PORT:USER:PASS
for /f "tokens=1,2,3,4 delims=:" %%a in ("%VAL%") do (
    set REMOTE_IP=%%a
    set REMOTE_PORT=%%b
    set REMOTE_USER=%%c
    set REMOTE_PASS=%%d
)

echo [%ID%] Iniciando %NAME% -> %REMOTE_IP%:%REMOTE_PORT%
start /b "" "%TUN2SOCKS%" -device %NAME% -proxy socks5://%REMOTE_USER%:%REMOTE_PASS%@%REMOTE_IP%:%REMOTE_PORT% -loglevel warning

echo Aguardando interface %NAME%...
:WAIT_LOOP
netsh interface show interface name="%NAME%" >nul 2>&1
if errorlevel 1 (
    timeout /t 1 /nobreak >nul
    goto WAIT_LOOP
)

netsh interface ip set address name="%NAME%" source=static addr=%IP% mask=255.255.255.0 gateway=%GW%
netsh interface ip set interface "%NAME%" metric=500
exit /b
