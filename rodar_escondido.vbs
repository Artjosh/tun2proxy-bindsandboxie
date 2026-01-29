Set UAC = CreateObject("Shell.Application")
' "runas" solicita permissao de Administrador (necessario para criar a placa de rede)
' "0" inicia a janela escondida
UAC.ShellExecute "C:\Users\arthu\OneDrive\Desktop\vpn-proxy\iniciar_proxy.cmd", "", "", "runas", 0
