package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ProxyConfig struct {
	IP       string
	Port     string
	Username string
	Password string
	ID       int
}

type ProxyEngine struct {
	tun2socksPath string
	config        *ConfigManager
	abortStart    bool
	mu            sync.Mutex
}

func NewProxyEngine(tun2socksPath string, config *ConfigManager) *ProxyEngine {
	return &ProxyEngine{
		tun2socksPath: tun2socksPath,
		config:        config,
	}
}

func (pe *ProxyEngine) AbortStart() {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.abortStart = true
}

func (pe *ProxyEngine) isAbortStart() bool {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	return pe.abortStart
}

func (pe *ProxyEngine) logPath() string {
	rootDir := filepath.Dir(pe.config.ConfigPath)
	return filepath.Join(rootDir, "joshboxie.log")
}

func (pe *ProxyEngine) Log(msg string) {
	line := fmt.Sprintf("[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
	f, err := os.OpenFile(pe.logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		f.WriteString(line)
	}
}

func (pe *ProxyEngine) getInterfaceIPv4(name string) string {
	psCmd := fmt.Sprintf(`(Get-NetIPAddress -InterfaceAlias "%s" -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -ExpandProperty IPAddress) -join ','`, name)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (pe *ProxyEngine) isProcessAlive(pid int) bool {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "tun2socks.exe")
}

func (pe *ProxyEngine) ParseProxies(text string) []ProxyConfig {
	var proxies []ProxyConfig
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i, line := range lines {
		parts := strings.Split(strings.TrimSpace(line), ":")
		if len(parts) >= 4 {
			proxies = append(proxies, ProxyConfig{
				IP:       parts[0],
				Port:     parts[1],
				Username: parts[2],
				Password: parts[3],
				ID:       i + 1,
			})
		}
	}
	return proxies
}

func (pe *ProxyEngine) StopAll() map[string]interface{} {
	pe.Log("STOP_ALL begin")
	summary := make(map[string]interface{})

	// 1. Taskkill
	cmd := exec.Command("taskkill", "/F", "/T", "/IM", "tun2socks.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, _ := cmd.CombinedOutput()
	summary["taskkill"] = strings.TrimSpace(string(out))
	pe.Log(fmt.Sprintf("STOP_ALL taskkill=%s", summary["taskkill"]))

	// 2. Powershell kill
	cmd2 := exec.Command("powershell", "-NoProfile", "-Command", "Get-Process -Name tun2socks -ErrorAction SilentlyContinue | Stop-Process -Force")
	cmd2.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out2, _ := cmd2.CombinedOutput()
	summary["powershell"] = strings.TrimSpace(string(out2))
	pe.Log(fmt.Sprintf("STOP_ALL powershell=%s", summary["powershell"]))

	// 3. Clean up adapters
	cmd3 := exec.Command("netsh", "interface", "show", "interface")
	cmd3.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out3, err := cmd3.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out3)), "\n")
		for _, line := range lines {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				devName := strings.Join(parts[3:], " ")
				if strings.HasPrefix(devName, "Proxy_") {
					pe.Log(fmt.Sprintf("STOP_ALL removing adapter=%s", devName))
					rmCmd := exec.Command("powershell", "-NoProfile", "-Command", fmt.Sprintf("Remove-NetAdapter -Name '%s' -Confirm:$false -ErrorAction SilentlyContinue", devName))
					rmCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
					rmCmd.Run()
				}
			}
		}
	}

	pe.config.Set("active_proxies", make(map[string]interface{}))
	pe.Log("STOP_ALL end")
	return summary
}

func (pe *ProxyEngine) updateProxiesMeta(active map[string]interface{}) {
	var meta []interface{}
	for devName, infoRaw := range active {
		info, ok := infoRaw.(map[string]interface{})
		if !ok {
			continue
		}
		
		pid := 0
		if p, ok := info["pid"].(float64); ok {
			pid = int(p)
		} else if p, ok := info["pid"].(int); ok {
			pid = p
		}

		ip, _ := info["ip"].(string)
		port, _ := info["port"].(string)
		user, _ := info["user"].(string)

		meta = append(meta, map[string]interface{}{
			"dev_name": devName,
			"ip":       ip,
			"port":     port,
			"user":     user,
			"pid":      pid,
		})
	}
	pe.config.Set("proxies_meta", meta)
}

func (pe *ProxyEngine) GetEgressIP(devName string, url string) map[string]interface{} {
	if url == "" {
		url = "https://api.ipify.org"
	}
	
	localIPs := strings.Split(pe.getInterfaceIPv4(devName), ",")
	localIP := strings.TrimSpace(localIPs[0])
	
	res := map[string]interface{}{
		"dev_name":  devName,
		"local_ip":  localIP,
		"egress_ip": "",
	}
	
	if localIP == "" {
		res["status"] = "no_local_ip"
		return res
	}

	cmd := exec.Command("curl.exe", "-s", "--max-time", "6", "--interface", localIP, url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	outStr := strings.TrimSpace(stdout.String())
	if err == nil && outStr != "" {
		res["status"] = "ok"
		res["egress_ip"] = outStr
	} else {
		res["status"] = "error"
		res["error"] = err.Error()
		res["stderr"] = strings.TrimSpace(stderr.String())
	}
	return res
}

func (pe *ProxyEngine) KillProxy(pid int, devName string) {
	if pid > 0 {
		cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		cmd.Run()
	}

	rmCmd := exec.Command("powershell", "-NoProfile", "-Command", fmt.Sprintf("Remove-NetAdapter -Name '%s' -ErrorAction SilentlyContinue -Confirm:$false", devName))
	rmCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	rmCmd.Run()

	active := pe.config.GetMap("active_proxies")
	if _, exists := active[devName]; exists {
		delete(active, devName)
		pe.config.Set("active_proxies", active)
	}
}

func (pe *ProxyEngine) KillByDevName(devName string) map[string]interface{} {
	summary := map[string]interface{}{
		"dev_name":  devName,
		"killed_by": nil,
		"pid":       nil,
		"errors":    []string{},
	}

	active := pe.config.GetMap("active_proxies")
	infoRaw, exists := active[devName]
	
	pid := 0
	if exists {
		info, ok := infoRaw.(map[string]interface{})
		if ok {
			if p, ok := info["pid"].(float64); ok {
				pid = int(p)
			} else if p, ok := info["pid"].(int); ok {
				pid = p
			}
		}
	}

	killed := false
	if pid > 0 {
		cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		err := cmd.Run()
		if err == nil {
			summary["killed_by"] = "pid"
			summary["pid"] = pid
			killed = true
		}
	}

	if !killed {
		// Use powershell fallback
		psCmd := fmt.Sprintf(`Get-CimInstance Win32_Process -Filter "Name='tun2socks.exe'" | Where-Object { $_.CommandLine -like '*-device %s*' } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }`, devName)
		cmd := exec.Command("powershell", "-NoProfile", "-Command", psCmd)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		err := cmd.Run()
		if err == nil {
			summary["killed_by"] = "powershell_cim"
		}
	}

	// Remove adapter
	rmCmd := exec.Command("powershell", "-NoProfile", "-Command", fmt.Sprintf("Remove-NetAdapter -Name '%s' -ErrorAction SilentlyContinue -Confirm:$false", devName))
	rmCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	rmCmd.Run()

	if exists {
		delete(active, devName)
		pe.config.Set("active_proxies", active)
	}

	return summary
}

func (pe *ProxyEngine) GetActiveProxies() map[string]interface{} {
	active := pe.config.GetMap("active_proxies")
	alive := make(map[string]interface{})
	changed := false

	// Discover un-cached tun2socks processes via WMI (Powershell equivalent)
	psCmd := `Get-CimInstance Win32_Process -Filter "Name='tun2socks.exe'" | Select-Object ProcessId, CommandLine | ConvertTo-Json -Compress`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		// Parse JSON list
		// Go JSON parsing...
		outStr := strings.TrimSpace(string(out))
		// If it's a single object, wrap it in array to easily parse
		if strings.HasPrefix(outStr, "{") {
			outStr = "[" + outStr + "]"
		}
		
		// In Go we really don't need to overcomplicate, we can just use regex on the raw string too, but actually let's just use string operations on commandline
		// Even simpler: just run powershell script that formats it nicely:
	}

	// Let's use a simpler powershell script to output raw lines
	psCmd2 := `Get-CimInstance Win32_Process -Filter "Name='tun2socks.exe'" | ForEach-Object { "$($_.ProcessId) ::: $($_.CommandLine)" }`
	cmd2 := exec.Command("powershell", "-NoProfile", "-Command", psCmd2)
	cmd2.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out2, err := cmd2.Output()
	if err == nil {
		lines := strings.Split(string(out2), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, " ::: ", 2)
			if len(parts) == 2 {
				pidStr, cmdline := parts[0], parts[1]
				pid, _ := strconv.Atoi(pidStr)

				devMatch := regexp.MustCompile(`-device\s+(Proxy_\d+)`).FindStringSubmatch(cmdline)
				proxyMatch := regexp.MustCompile(`-proxy\s+socks5://(?:[^:]+:[^@]+@)?([0-9\.]+):(\d+)`).FindStringSubmatch(cmdline)
				
				if len(devMatch) > 1 && len(proxyMatch) > 2 {
					devName := devMatch[1]
					if _, exists := active[devName]; !exists {
						userMatch := regexp.MustCompile(`-proxy\s+socks5://([^:]+):[^@]+@`).FindStringSubmatch(cmdline)
						user := ""
						if len(userMatch) > 1 {
							user = userMatch[1]
						}
						
						active[devName] = map[string]interface{}{
							"ip":   proxyMatch[1],
							"port": proxyMatch[2],
							"pid":  pid,
							"user": user,
						}
						changed = true
					}
				}
			}
		}
	}

	// Discover all physical adapters from Windows
	windowsAdapters := make(map[string]bool)
	cmd3 := exec.Command("netsh", "interface", "show", "interface")
	cmd3.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out3, err := cmd3.Output()
	if err == nil {
		lines := strings.Split(string(out3), "\n")
		for _, line := range lines {
			parts := strings.Fields(strings.TrimSpace(line))
			if len(parts) >= 4 {
				devName := strings.Join(parts[3:], " ")
				if strings.HasPrefix(devName, "Proxy_") {
					windowsAdapters[devName] = true
				}
			}
		}
	}

	// Reconcile cache
	for dev, infoRaw := range active {
		info, ok := infoRaw.(map[string]interface{})
		if !ok {
			changed = true
			continue
		}
		
		pid := 0
		if p, ok := info["pid"].(float64); ok {
			pid = int(p)
		} else if p, ok := info["pid"].(int); ok {
			pid = p
		}

		if pid > 0 && pe.isProcessAlive(pid) {
			newInfo := make(map[string]interface{})
			for k, v := range info {
				newInfo[k] = v
			}
			newInfo["adapter_ip"] = pe.getInterfaceIPv4(dev)
			alive[dev] = newInfo
		} else if windowsAdapters[dev] {
			// Zombie adapter
			alive[dev] = map[string]interface{}{
				"ip":         "Zombie Adapter",
				"port":       "N/A",
				"pid":        0,
				"user":       "Offline",
				"adapter_ip": pe.getInterfaceIPv4(dev),
			}
			changed = true
		} else {
			changed = true
		}
	}

	// Add undocumented zombies
	for dev := range windowsAdapters {
		if _, exists := alive[dev]; !exists {
			alive[dev] = map[string]interface{}{
				"ip":         "Zombie Adapter",
				"port":       "N/A",
				"pid":        0,
				"user":       "Offline",
				"adapter_ip": pe.getInterfaceIPv4(dev),
			}
			changed = true
		}
	}

	if changed {
		pe.config.Set("active_proxies", alive)
		pe.updateProxiesMeta(alive)
	}

	return alive
}

func (pe *ProxyEngine) StartProxies(proxies []ProxyConfig, logCallback func(string)) {
	pe.mu.Lock()
	pe.abortStart = false
	pe.mu.Unlock()

	go pe.runSequence(proxies, logCallback)
}

func (pe *ProxyEngine) runSequence(proxies []ProxyConfig, logCallback func(string)) {
	log := func(msg string) {
		if logCallback != nil {
			logCallback(msg)
		} else {
			fmt.Println(msg)
		}
		pe.Log(msg)
	}

	aliveProxies := pe.GetActiveProxies()
	boundIPs := make(map[string]bool)
	for _, infoRaw := range aliveProxies {
		info, ok := infoRaw.(map[string]interface{})
		if ok {
			if ip, ok := info["ip"].(string); ok {
				boundIPs[ip] = true
			}
		}
	}

	var newProxies []ProxyConfig
	for _, p := range proxies {
		if !boundIPs[p.IP] {
			newProxies = append(newProxies, p)
		}
	}

	if len(newProxies) == 0 {
		log("No new IP addresses found. All provided proxies are already active.")
		return
	}

	var existingIDs []int
	for devName := range aliveProxies {
		idStr := strings.TrimPrefix(devName, "Proxy_")
		if id, err := strconv.Atoi(idStr); err == nil {
			existingIDs = append(existingIDs, id)
		}
	}

	nextID := 1
	if len(existingIDs) > 0 {
		max := existingIDs[0]
		for _, id := range existingIDs {
			if id > max {
				max = id
			}
		}
		nextID = max + 1
	}

	for _, p := range newProxies {
		if pe.isAbortStart() {
			log("Proxy creation aborted by user request.")
			break
		}

		devName := fmt.Sprintf("Proxy_%d", nextID)

		// Start tun2socks
		args := []string{
			"-device", devName,
			"-proxy", fmt.Sprintf("socks5://%s:%s@%s:%s", p.Username, p.Password, p.IP, p.Port),
			"-loglevel", "error",
		}
		
		log(fmt.Sprintf("[%d] Starting interface %s connected to %s...", p.ID, devName, p.IP))

		cmd := exec.Command(pe.tun2socksPath, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		err := cmd.Start()
		if err != nil {
			log(fmt.Sprintf("[%d] ERROR: Failed to start tun2socks: %v", p.ID, err))
			continue
		}

		if !pe.waitForInterface(devName, 10) {
			log(fmt.Sprintf("[%d] ERROR: Interface %s failed to appear.", nextID, devName))
			// Kill it since it failed
			cmd.Process.Kill()
			continue
		}

		localIP := fmt.Sprintf("10.0.%d.1", nextID)
		gateway := fmt.Sprintf("10.0.%d.254", nextID)
		log(fmt.Sprintf("[%d] Setting IP %s...", nextID, localIP))

		if pe.setIP(devName, localIP, gateway, log, 3) {
			log(fmt.Sprintf("[%d] Ready.", nextID))
			aliveProxies[devName] = map[string]interface{}{
				"ip":   p.IP,
				"port": p.Port,
				"pid":  cmd.Process.Pid,
				"user": p.Username,
			}
			pe.config.Set("active_proxies", aliveProxies)
			pe.updateProxiesMeta(aliveProxies)
		} else {
			log(fmt.Sprintf("[%d] WARNING: Interface may not work correctly!", nextID))
		}

		nextID++
	}

	log("All new proxies started.")
}

func (pe *ProxyEngine) waitForInterface(name string, timeout int) bool {
	start := time.Now()
	for time.Since(start).Seconds() < float64(timeout) {
		cmd := exec.Command("netsh", "interface", "show", "interface", fmt.Sprintf("name=%s", name))
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := cmd.Run(); err == nil {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func (pe *ProxyEngine) setIP(name string, ip string, gateway string, log func(string), maxRetries int) bool {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		cmd1 := exec.Command("netsh", "interface", "ip", "set", "address", fmt.Sprintf("name=%s", name), "source=static", fmt.Sprintf("addr=%s", ip), "mask=255.255.255.0", fmt.Sprintf("gateway=%s", gateway))
		cmd1.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		cmd1.Run()

		cmd2 := exec.Command("netsh", "interface", "ip", "set", "interface", name, "metric=500")
		cmd2.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		cmd2.Run()

		time.Sleep(500 * time.Millisecond)

		if pe.verifyIP(name, ip) {
			return true
		}

		if log != nil {
			log(fmt.Sprintf("[%s] IP verification failed (attempt %d/%d), retrying...", name, attempt, maxRetries))
		}
		time.Sleep(1 * time.Second)
	}

	if log != nil {
		log(fmt.Sprintf("[%s] ERROR: Failed to set IP %s after %d attempts!", name, ip, maxRetries))
	}
	return false
}

func (pe *ProxyEngine) verifyIP(name string, expectedIP string) bool {
	psCmd := fmt.Sprintf(`(Get-NetIPAddress -InterfaceAlias "%s" -AddressFamily IPv4 -ErrorAction SilentlyContinue).IPAddress`, name)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == expectedIP
}
