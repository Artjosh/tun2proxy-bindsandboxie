package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

var (
	config         *ConfigManager
	proxyEngine    *ProxyEngine
	sandboxManager *SandboxManager
	logFile        *os.File
)

func setupServerLogging() {
	exePath, _ := os.Executable()
	baseDir := filepath.Dir(exePath)
	logPath := filepath.Join(baseDir, "manager_go.log")
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
		logFile = f
		os.Stdout = f
		os.Stderr = f
		log.SetOutput(f)
	}
}

func killProcessOnPort(port int) {
	// Use netstat to find PID on the port and kill it
	log.Printf("[STARTUP] Checking if port %d is in use...", port)

	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf(`$c = netstat -ano | Select-String ':%d\s' | ForEach-Object { ($_ -split '\s+')[-1] } | Where-Object { $_ -ne '0' } | Select-Object -Unique; foreach ($p in $c) { try { taskkill /F /PID $p 2>$null } catch {} }; $c`, port))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[STARTUP] Port cleanup command error (may be fine): %v", err)
	}
	outStr := string(out)
	if outStr != "" {
		log.Printf("[STARTUP] Port cleanup result: %s", outStr)
	}
}

func waitForPort(port int, maxRetries int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for i := 0; i < maxRetries; i++ {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			ln.Close()
			log.Printf("[STARTUP] Port %d is now available (attempt %d/%d)", port, i+1, maxRetries)
			return nil
		}
		log.Printf("[STARTUP] Port %d still busy, retrying in 2s (attempt %d/%d): %v", port, i+1, maxRetries, err)
		if i == 0 {
			killProcessOnPort(port)
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("port %d still busy after %d attempts", port, maxRetries)
}

func runServer(port int, rootDir string) {
	setupServerLogging()

	log.Println("========================================")
	log.Println("[STARTUP] Manager Go starting up...")
	log.Printf("[STARTUP] Executable dir: %s", func() string { p, _ := os.Executable(); return filepath.Dir(p) }())
	log.Printf("[STARTUP] Root dir (resolved): %s", func() string { p, _ := filepath.Abs(rootDir); return p }())

	// Wait for port to be free
	if err := waitForPort(port, 10); err != nil {
		log.Fatalf("[FATAL] %v", err)
	}

	log.Println("[STARTUP] Loading config...")
	config = NewConfigManager()
	log.Printf("[STARTUP] Config loaded from: %s", config.ConfigPath)

	tun2socksBin := config.GetPath("tun2socks")
	log.Printf("[STARTUP] tun2socks from config: %q", tun2socksBin)
	if tun2socksBin == "" || !filepath.IsAbs(tun2socksBin) {
		tun2socksBin = filepath.Join(rootDir, tun2socksBin)
		if tun2socksBin == "" || tun2socksBin == "." {
			tun2socksBin = filepath.Join(rootDir, "tun2socks.exe")
		}
		if filepath.Base(tun2socksBin) == "." || filepath.Base(tun2socksBin) == tun2socksBin {
			tun2socksBin = filepath.Join(rootDir, "tun2socks.exe")
		}
	}
	log.Printf("[STARTUP] tun2socks resolved to: %s", tun2socksBin)

	proxyEngine = NewProxyEngine(tun2socksBin, config)
	sandboxManager = NewSandboxManager(config)
	log.Println("[STARTUP] ProxyEngine and SandboxManager initialized.")

	mux := http.NewServeMux()
	setupRoutes(mux)
	log.Println("[STARTUP] API routes registered.")

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	log.Printf("[STARTUP] Starting HTTP server on %s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[FATAL] Server failed: %v", err)
	}
}

func main() {
	runServer(8000, "..")
}
