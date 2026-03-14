package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"error": msg})
}

func parseJSONBody(r *http.Request) map[string]interface{} {
	var body map[string]interface{}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	json.Unmarshal(b, &body)
	if body == nil {
		body = make(map[string]interface{})
	}
	return body
}

func setupRoutes(mux *http.ServeMux) {
	// LEGACY
	mux.HandleFunc("/api/legacy/status", func(w http.ResponseWriter, r *http.Request) {
		proxiesList := config.Get("proxies_list")
		if proxiesList == nil {
			proxiesList = []string{}
		}
		jsonResponse(w, map[string]interface{}{"saved_proxies": proxiesList})
	})

	mux.HandleFunc("/api/legacy/start", func(w http.ResponseWriter, r *http.Request) {
		data := parseJSONBody(r)
		linesRaw, ok := data["proxies"].([]interface{})
		if !ok || len(linesRaw) == 0 {
			jsonError(w, "No proxies provided")
			return
		}

		var lines []string
		for _, l := range linesRaw {
			if s, ok := l.(string); ok {
				lines = append(lines, s)
			}
		}

		config.Set("proxies_list", lines)
		rawText := strings.Join(lines, "\n")
		proxies := proxyEngine.ParseProxies(rawText)

		var planned []map[string]interface{}
		for _, p := range proxies {
			planned = append(planned, map[string]interface{}{
				"dev_name": fmt.Sprintf("Proxy_%d", p.ID),
				"ip":       p.IP,
				"port":     p.Port,
				"user":     p.Username,
				"pass":     p.Password,
			})
		}
		config.Set("proxies_planned", planned)

		proxyEngine.StartProxies(proxies, nil)
		jsonResponse(w, map[string]interface{}{"status": "starting"})
	})

	mux.HandleFunc("/api/legacy/stop", func(w http.ResponseWriter, r *http.Request) {
		summary := proxyEngine.StopAll()
		jsonResponse(w, map[string]interface{}{"status": "ok", "summary": summary})
	})

	mux.HandleFunc("/api/legacy/abort", func(w http.ResponseWriter, r *http.Request) {
		proxyEngine.AbortStart()
		jsonResponse(w, map[string]interface{}{"status": "aborted"})
	})

	// PROXIES
	mux.HandleFunc("/api/proxies/active", func(w http.ResponseWriter, r *http.Request) {
		alive := proxyEngine.GetActiveProxies()
		
		type proxyItem struct {
			devName string
			info    interface{}
		}
		var items []proxyItem
		for k, v := range alive {
			items = append(items, proxyItem{k, v})
		}
		
		sort.Slice(items, func(i, j int) bool {
			id1 := 0
			id2 := 0
			fmt.Sscanf(items[i].devName, "Proxy_%d", &id1)
			fmt.Sscanf(items[j].devName, "Proxy_%d", &id2)
			return id1 < id2
		})

		var sorted [][]interface{}
		for _, item := range items {
			sorted = append(sorted, []interface{}{item.devName, item.info})
		}
		
		jsonResponse(w, map[string]interface{}{"proxies": sorted})
	})

	mux.HandleFunc("/api/proxies/kill", func(w http.ResponseWriter, r *http.Request) {
		data := parseJSONBody(r)
		devName, _ := data["dev_name"].(string)
		if devName == "" {
			jsonError(w, "Missing dev_name")
			return
		}

		pidStr := fmt.Sprintf("%v", data["pid"])
		pidInt, _ := strconv.Atoi(pidStr)

		if pidInt > 0 {
			proxyEngine.KillProxy(pidInt, devName)
			jsonResponse(w, map[string]interface{}{"status": "ok", "killed_by": "pid", "pid": pidInt, "dev_name": devName})
			return
		}

		summary := proxyEngine.KillByDevName(devName)
		jsonResponse(w, map[string]interface{}{"status": "ok", "killed_by": summary["killed_by"], "summary": summary})
	})

	mux.HandleFunc("/api/proxies/egress", func(w http.ResponseWriter, r *http.Request) {
		devName := r.URL.Query().Get("dev_name")
		if devName == "" {
			jsonError(w, "Missing dev_name")
			return
		}
		jsonResponse(w, proxyEngine.GetEgressIP(devName, ""))
	})

	// SANDBOXES
	mux.HandleFunc("/api/sandboxes", func(w http.ResponseWriter, r *http.Request) {
		folder, _ := config.Get("last_shortcuts_dir").(string)
		if folder == "" {
			jsonError(w, "Shortcut folder not configured")
			return
		}

		groups := sandboxManager.ScanShortcuts(folder)
		adapters := sandboxManager.GetAvailableAdapters()

		var result []map[string]interface{}
		for gid, shortcuts := range groups {
			if len(shortcuts) == 0 {
				continue
			}
			boxName := shortcuts[0].BoxName
			currentBind := sandboxManager.GetBindAdapterForBox(boxName)
			isSpoofed := sandboxManager.IsBoxSpoofed(boxName)

			var apps []string
			for _, s := range shortcuts {
				apps = append(apps, s.Name)
			}

			result = append(result, map[string]interface{}{
				"id":         gid,
				"box_name":   boxName,
				"apps":       apps,
				"bind":       currentBind,
				"is_spoofed": isSpoofed,
			})
		}

		jsonResponse(w, map[string]interface{}{
			"sandboxes":          result,
			"available_adapters": adapters,
			"folder":             folder,
		})
	})

	mux.HandleFunc("/api/sandboxes/bind", func(w http.ResponseWriter, r *http.Request) {
		data := parseJSONBody(r)
		boxName, _ := data["box_name"].(string)
		adapter, _ := data["adapter"].(string)
		if boxName == "" || adapter == "" {
			jsonError(w, "Missing params")
			return
		}

		sandboxManager.SetBindAdapter(boxName, adapter)
		jsonResponse(w, map[string]interface{}{"status": "ok"})
	})

	mux.HandleFunc("/api/sandboxes/spoof", func(w http.ResponseWriter, r *http.Request) {
		data := parseJSONBody(r)
		boxName, _ := data["box_name"].(string)
		if boxName == "" {
			jsonError(w, "Missing box_name")
			return
		}

		isSpoofed := sandboxManager.IsBoxSpoofed(boxName)
		sandboxManager.ToggleSpoof(boxName, !isSpoofed)
		jsonResponse(w, map[string]interface{}{"status": "ok", "is_spoofed": !isSpoofed})
	})

	mux.HandleFunc("/api/sandboxes/launch", func(w http.ResponseWriter, r *http.Request) {
		data := parseJSONBody(r)
		path, _ := data["path"].(string)
		if path == "" {
			jsonError(w, "Missing absolute shortcut path")
			return
		}
		err := sandboxManager.LaunchShortcut(path)
		if err != nil {
			jsonError(w, err.Error())
		} else {
			jsonResponse(w, map[string]interface{}{"status": "ok"})
		}
	})

	mux.HandleFunc("/api/sandboxes/launch_by_name", func(w http.ResponseWriter, r *http.Request) {
		data := parseJSONBody(r)
		boxName, _ := data["box_name"].(string)
		appName, _ := data["app_name"].(string)

		folder, _ := config.Get("last_shortcuts_dir").(string)
		if folder == "" {
			jsonError(w, "Shortcuts directory not configured")
			return
		}

		groups := sandboxManager.ScanShortcuts(folder)
		targetPath := ""

		for _, shortcuts := range groups {
			for _, s := range shortcuts {
				if s.BoxName == boxName && s.Name == appName {
					targetPath = s.Path
					break
				}
			}
			if targetPath != "" {
				break
			}
		}

		if targetPath == "" {
			jsonError(w, fmt.Sprintf("Shortcut for %s in %s not found", appName, boxName))
			return
		}

		err := sandboxManager.LaunchShortcut(targetPath)
		if err != nil {
			jsonError(w, err.Error())
		} else {
			jsonResponse(w, map[string]interface{}{"status": "ok"})
		}
	})

	// IPCHECK
	mux.HandleFunc("/api/ipcheck/single", func(w http.ResponseWriter, r *http.Request) {
		data := parseJSONBody(r)
		ip, _ := data["ip"].(string)
		key, _ := data["key"].(string)

		if ip == "" {
			jsonError(w, "Missing IP")
			return
		}

		if key != "" {
			config.Set("ipqs_api_key", key)
		} else {
			if k, ok := config.Get("ipqs_api_key").(string); ok {
				key = k
			}
		}

		results := checkAllIP(ip, key)
		jsonResponse(w, map[string]interface{}{"results": results})
	})

	// CONFIG
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			jsonResponse(w, map[string]interface{}{
				"last_shortcuts_dir": config.Get("last_shortcuts_dir"),
				"tun2socks":          config.GetPath("tun2socks"),
				"wintun":             config.GetPath("wintun"),
				"sandboxie_ini":      config.GetPath("sandboxie_ini"),
				"sbie_ini_exe":       config.GetPath("sbie_ini_exe"),
			})
		} else if r.Method == "POST" {
			data := parseJSONBody(r)
			if val, ok := data["last_shortcuts_dir"]; ok {
				config.Set("last_shortcuts_dir", val)
			}
			if val, ok := data["tun2socks"].(string); ok {
				config.SetPath("tun2socks", val)
			}
			if val, ok := data["wintun"].(string); ok {
				config.SetPath("wintun", val)
			}
			if val, ok := data["sandboxie_ini"].(string); ok {
				config.SetPath("sandboxie_ini", val)
			}
			if val, ok := data["sbie_ini_exe"].(string); ok {
				config.SetPath("sbie_ini_exe", val)
			}
			jsonResponse(w, map[string]interface{}{"status": "saved"})
		}
	})

	mux.HandleFunc("/api/config/select_folder", func(w http.ResponseWriter, r *http.Request) {
		psCmd := `Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.FolderBrowserDialog; $f.ShowNewFolderButton = $true; if ($f.ShowDialog() -eq 'OK') { $f.SelectedPath }`
		cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", psCmd)
		out, err := cmd.Output()
		
		path := strings.TrimSpace(string(out))
		if err == nil && path != "" {
			config.Set("last_shortcuts_dir", path)
			jsonResponse(w, map[string]interface{}{"status": "ok", "path": path})
		} else {
			jsonResponse(w, map[string]interface{}{"status": "cancelled"})
		}
	})

	mux.HandleFunc("/api/admin", func(w http.ResponseWriter, r *http.Request) {
		shell32 := syscall.NewLazyDLL("shell32.dll")
		proc := shell32.NewProc("IsUserAnAdmin")
		ret, _, _ := proc.Call()
		isAdmin := ret != 0

		jsonResponse(w, map[string]interface{}{"is_admin": isAdmin})
	})
}
