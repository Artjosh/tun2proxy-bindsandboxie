package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const ConfigFileName = "config.json"

type ConfigManager struct {
	mu         sync.RWMutex
	ConfigPath string
	Data       map[string]interface{}
}

func NewConfigManager() *ConfigManager {
	exePath, err := os.Executable()
	var rootDir string
	if err == nil {
		rootDir = filepath.Dir(exePath)
	} else {
		rootDir = "."
	}

	// For dev, if we are in manager_go, root might be one up
	if filepath.Base(rootDir) == "manager_go" {
		rootDir = filepath.Dir(rootDir)
	}

	cm := &ConfigManager{
		ConfigPath: filepath.Join(rootDir, ConfigFileName),
		Data:       make(map[string]interface{}),
	}
	cm.loadConfig()
	return cm
}

func (c *ConfigManager) loadConfig() {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.ConfigPath)
	if err != nil {
		c.createDefaultConfig()
		return
	}

	err = json.Unmarshal(data, &c.Data)
	if err != nil {
		c.createDefaultConfig()
	}
}

func (c *ConfigManager) createDefaultConfig() {
	c.Data = map[string]interface{}{
		"paths": map[string]interface{}{
			"tun2socks":     "",
			"wintun":        "",
			"sandboxie_ini": "",
			"sbie_ini_exe":  "",
		},
		"proxies_list":               []interface{}{},
		"proxies_planned":             []interface{}{},
		"active_proxies":              map[string]interface{}{},
		"proxies_meta":                []interface{}{},
		"last_shortcuts_dir":          "",
		"ipqs_api_key":                "",
		"spoof_whitelist_processes":   []interface{}{},
	}

	rootDir := filepath.Dir(c.ConfigPath)
	tun2socksGuess := filepath.Join(rootDir, "tun2socks.exe")
	wintunGuess := filepath.Join(rootDir, "wintun.dll")

	paths := c.Data["paths"].(map[string]interface{})
	if _, err := os.Stat(tun2socksGuess); err == nil {
		paths["tun2socks"] = tun2socksGuess
	}
	if _, err := os.Stat(wintunGuess); err == nil {
		paths["wintun"] = wintunGuess
	}

	c.saveConfigNoLock()
}

func (c *ConfigManager) SaveConfig() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saveConfigNoLock()
}

func (c *ConfigManager) saveConfigNoLock() {
	data, err := json.MarshalIndent(c.Data, "", "    ")
	if err == nil {
		os.WriteFile(c.ConfigPath, data, 0644)
	}
}

func (c *ConfigManager) Get(key string) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Data[key]
}

func (c *ConfigManager) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Data[key] = value
	c.saveConfigNoLock()
}

func (c *ConfigManager) GetPath(name string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	paths, ok := c.Data["paths"].(map[string]interface{})
	if !ok {
		return ""
	}
	
	val, ok := paths[name]
	if !ok {
		return ""
	}
	
	strVal, ok := val.(string)
	if !ok {
		return ""
	}
	
	return strVal
}

func (c *ConfigManager) SetPath(name string, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	paths, ok := c.Data["paths"].(map[string]interface{})
	if !ok {
		paths = make(map[string]interface{})
		c.Data["paths"] = paths
	}
	paths[name] = path
	c.saveConfigNoLock()
}

func (c *ConfigManager) GetMap(key string) map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.Data[key].(map[string]interface{})
	if !ok {
		return make(map[string]interface{})
	}
	return v
}

// GetSpoofWhitelistProcesses returns the global spoof whitelist process list from config.
func (c *ConfigManager) GetSpoofWhitelistProcesses() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	raw, ok := c.Data["spoof_whitelist_processes"].([]interface{})
	if !ok {
		return nil
	}
	var result []string
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			result = append(result, s)
		}
	}
	return result
}

