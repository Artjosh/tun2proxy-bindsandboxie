package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// APIClient wraps HTTP calls to the Python FastAPI backend.
type APIClient struct {
	BaseURL    string
	httpClient *http.Client
}

func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		BaseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// --------------- helpers ---------------

func (c *APIClient) getJSON(path string, out interface{}) error {
	resp, err := c.httpClient.Get(c.BaseURL + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return json.Unmarshal(body, out)
}

func (c *APIClient) postJSON(path string, payload interface{}, out interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Post(c.BaseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// --------------- Admin ---------------

type AdminResp struct {
	IsAdmin bool `json:"is_admin"`
}

func (c *APIClient) CheckAdmin() (bool, error) {
	var r AdminResp
	err := c.getJSON("/api/admin", &r)
	return r.IsAdmin, err
}

// --------------- Config ---------------

type ConfigResp struct {
	LastShortcutsDir string `json:"last_shortcuts_dir"`
	Tun2socks        string `json:"tun2socks"`
	Wintun           string `json:"wintun"`
	SandboxieINI     string `json:"sandboxie_ini"`
	SbieINIExe       string `json:"sbie_ini_exe"`
}

func (c *APIClient) GetConfig() (*ConfigResp, error) {
	var r ConfigResp
	err := c.getJSON("/api/config", &r)
	return &r, err
}

func (c *APIClient) UpdateConfig(data map[string]string) error {
	return c.postJSON("/api/config", data, nil)
}

type SelectFolderResp struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

func (c *APIClient) SelectFolder() (*SelectFolderResp, error) {
	var r SelectFolderResp
	err := c.postJSON("/api/config/select_folder", map[string]string{}, &r)
	return &r, err
}

// --------------- Legacy Proxy Engine ---------------

type LegacyStatusResp struct {
	SavedProxies []string `json:"saved_proxies"`
}

func (c *APIClient) GetLegacyStatus() (*LegacyStatusResp, error) {
	var r LegacyStatusResp
	err := c.getJSON("/api/legacy/status", &r)
	return &r, err
}

func (c *APIClient) StartProxies(lines []string) error {
	payload := map[string]interface{}{"proxies": lines}
	var r map[string]interface{}
	return c.postJSON("/api/legacy/start", payload, &r)
}

func (c *APIClient) StopProxies() (map[string]interface{}, error) {
	var r map[string]interface{}
	err := c.postJSON("/api/legacy/stop", map[string]string{}, &r)
	return r, err
}

func (c *APIClient) AbortProxies() error {
	var r map[string]interface{}
	return c.postJSON("/api/legacy/abort", map[string]string{}, &r)
}

// --------------- Active Proxies ---------------

type ActiveProxiesResp struct {
	Proxies [][]interface{} `json:"proxies"` // [[dev_name, {info}], ...]
}

func (c *APIClient) GetActiveProxies() (*ActiveProxiesResp, error) {
	var r ActiveProxiesResp
	err := c.getJSON("/api/proxies/active", &r)
	return &r, err
}

func (c *APIClient) KillProxy(devName string, pid int) error {
	payload := map[string]interface{}{"dev_name": devName, "pid": pid}
	return c.postJSON("/api/proxies/kill", payload, nil)
}

func (c *APIClient) GetEgressIP(devName string) (map[string]interface{}, error) {
	var r map[string]interface{}
	err := c.getJSON("/api/proxies/egress?dev_name="+url.QueryEscape(devName), &r)
	return r, err
}

// --------------- Sandboxes ---------------

type SandboxInfo struct {
	ID        string   `json:"id"`
	BoxName   string   `json:"box_name"`
	Apps      []string `json:"apps"`
	Bind      string   `json:"bind"`
	IsSpoofed bool     `json:"is_spoofed"`
}

type SandboxesResp struct {
	Sandboxes         []SandboxInfo `json:"sandboxes"`
	AvailableAdapters []string      `json:"available_adapters"`
	Folder            string        `json:"folder"`
	Error             string        `json:"error"`
}

func (c *APIClient) GetSandboxes() (*SandboxesResp, error) {
	var r SandboxesResp
	err := c.getJSON("/api/sandboxes", &r)
	return &r, err
}

func (c *APIClient) BindSandbox(boxName, adapter string) error {
	payload := map[string]string{"box_name": boxName, "adapter": adapter}
	return c.postJSON("/api/sandboxes/bind", payload, nil)
}

func (c *APIClient) ToggleSpoof(boxName string) error {
	payload := map[string]string{"box_name": boxName}
	return c.postJSON("/api/sandboxes/spoof", payload, nil)
}

func (c *APIClient) LaunchShortcut(path string) error {
	payload := map[string]string{"path": path}
	return c.postJSON("/api/sandboxes/launch", payload, nil)
}

func (c *APIClient) LaunchByName(boxName, appName string) error {
	payload := map[string]string{"box_name": boxName, "app_name": appName}
	return c.postJSON("/api/sandboxes/launch_by_name", payload, nil)
}

// --------------- IP Check ---------------

type IPCheckResult struct {
	Source string                 `json:"source"`
	Status string                 `json:"status"`
	Error  string                 `json:"error"`
	Data   map[string]interface{} `json:"data"`
}

type IPCheckResp struct {
	Results []IPCheckResult `json:"results"`
}

func (c *APIClient) CheckIP(ip, apiKey string) (*IPCheckResp, error) {
	payload := map[string]string{"ip": ip, "key": apiKey}
	var r IPCheckResp
	err := c.postJSON("/api/ipcheck/single", payload, &r)
	return &r, err
}

// --------------- Health Check ---------------

func (c *APIClient) IsBackendAlive() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(c.BaseURL + "/api/admin")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func (c *APIClient) WaitForBackend(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.IsBackendAlive() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("backend did not start within %v", timeout)
}
