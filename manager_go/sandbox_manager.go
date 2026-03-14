package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

type SandboxShortcut struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	BoxName string `json:"box_name"`
	GroupID int    `json:"group_id"`
	AppName string `json:"app_name"`
}

type SandboxManager struct {
	config     *ConfigManager
	sbieIniExe string
}

func NewSandboxManager(config *ConfigManager) *SandboxManager {
	sm := &SandboxManager{config: config}

	if config != nil {
		sm.sbieIniExe = config.GetPath("sbie_ini_exe")
	}
	if sm.sbieIniExe == "" {
		sm.sbieIniExe = `C:\Program Files\Sandboxie-Plus\SbieIni.exe`
	}

	return sm
}

func (sm *SandboxManager) ScanShortcuts(folderPath string) map[string][]SandboxShortcut {
	results := make(map[string][]SandboxShortcut)
	if _, err := os.Stat(folderPath); err != nil {
		return results
	}

	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return results
	}

	pattern := regexp.MustCompile(`(?i)^(\d+)-\[(.+?)\](?: (.+))?\.lnk$`)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		matches := pattern.FindStringSubmatch(filename)
		if len(matches) > 0 {
			groupID := matches[1]
			boxName := matches[2]
			appName := "Shortcut"
			if len(matches) > 3 && matches[3] != "" {
				appName = matches[3]
			}

			fullPath := filepath.Join(folderPath, filename)
			groupIDInt, _ := strconv.Atoi(groupID)

			shortcut := SandboxShortcut{
				Name:    filename,
				Path:    fullPath,
				BoxName: boxName,
				GroupID: groupIDInt,
				AppName: appName,
			}

			results[groupID] = append(results[groupID], shortcut)
		}
	}

	return results
}

func (sm *SandboxManager) GetBindAdapterForBox(boxName string) string {
	iniPath := `C:\Windows\Sandboxie.ini`
	if sm.config != nil {
		p := sm.config.GetPath("sandboxie_ini")
		if p != "" {
			iniPath = p
		}
	}

	contentBytes, err := os.ReadFile(iniPath)
	if err != nil {
		return "None"
	}

	// Assuming utf-16/utf-8 text, just extract strings
	// Go strings functions work well even on utf-16 string conversion if we're careful.
	// Actually typical Sandboxie.ini is UTF-16 LE. Let's just remove null bytes for regex matching
	content := strings.ReplaceAll(string(contentBytes), "\x00", "")

	sections := regexp.MustCompile(`(?m)^\[([^\]]+)\]`).Split(content, -1)
	matches := regexp.MustCompile(`(?m)^\[([^\]]+)\]`).FindAllStringSubmatch(content, -1)

	if len(sections)-1 == len(matches) {
		for i, match := range matches {
			if strings.EqualFold(match[1], boxName) {
				sectionContent := sections[i+1]
				bindMatch := regexp.MustCompile(`(?m)^\s*BindAdapter=(.+)$`).FindStringSubmatch(sectionContent)
				if len(bindMatch) > 1 {
					return strings.TrimSpace(bindMatch[1])
				}
				break
			}
		}
	}

	return "None"
}

func (sm *SandboxManager) SetBindAdapter(boxName string, adapterName string) {
	var cmd *exec.Cmd
	if adapterName == "None" || adapterName == "clean" {
		cmd = exec.Command(sm.sbieIniExe, "set", boxName, "BindAdapter")
	} else {
		cmd = exec.Command(sm.sbieIniExe, "set", boxName, "BindAdapter", adapterName)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Run()
}

func (sm *SandboxManager) LaunchShortcut(path string) error {
	normPath := filepath.Clean(path)
	if _, err := os.Stat(normPath); err == nil {
		// Like os.startfile via explorer
		cmd := exec.Command("explorer", normPath)
		err := cmd.Start()
		if err != nil {
			cmd2 := exec.Command("cmd", "/c", "start", `""`, fmt.Sprintf(`"%s"`, normPath))
			return cmd2.Start()
		}
		return nil
	}

	absPath, _ := filepath.Abs(normPath)
	if _, err := os.Stat(absPath); err == nil {
		cmd := exec.Command("cmd", "/c", "start", `""`, fmt.Sprintf(`"%s"`, absPath))
		return cmd.Start()
	}

	return fmt.Errorf("Shortcut truly missing: %s", normPath)
}

func (sm *SandboxManager) GetAvailableAdapters() []string {
	adapters := []string{"clean"}

	cmd := exec.Command("netsh", "interface", "show", "interface")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			parts := strings.Fields(strings.TrimSpace(line))
			if len(parts) >= 4 {
				name := strings.Join(parts[3:], " ")
				if name != "Interface Name" && name != "---------..." { // Skip headers somewhat safely
					adapters = append(adapters, name)
				}
			}
		}
	}

	return adapters
}

var spoofTemplates = []string{"BlockAccessWMI", "HideInstalledPrograms"}
var spoofKeys = map[string]string{
	"SandboxieAllGroup":     "n",
	"HideFirmwareInfo":      "y",
	"RandomRegUID":          "y",
	"HideDiskSerialNumber":  "y",
	"HideNetworkAdapterMAC": "y",
}

func (sm *SandboxManager) IsBoxSpoofed(boxName string) bool {
	iniPath := `C:\Windows\Sandboxie.ini`
	if sm.config != nil {
		if p := sm.config.GetPath("sandboxie_ini"); p != "" {
			iniPath = p
		}
	}

	contentBytes, err := os.ReadFile(iniPath)
	if err != nil {
		return false
	}
	content := strings.ReplaceAll(string(contentBytes), "\x00", "")

	sections := regexp.MustCompile(`(?m)^\[([^\]]+)\]`).Split(content, -1)
	matches := regexp.MustCompile(`(?m)^\[([^\]]+)\]`).FindAllStringSubmatch(content, -1)

	if len(sections)-1 == len(matches) {
		for i, match := range matches {
			if strings.EqualFold(match[1], boxName) {
				sectionContent := strings.ToLower(sections[i+1])
				if strings.Contains(sectionContent, "hidenetworkadaptermac=y") || strings.Contains(sectionContent, "blockaccesswmi") {
					return true
				}
				break
			}
		}
	}
	return false
}

func (sm *SandboxManager) ToggleSpoof(boxName string, enable bool) {
	runCmd := func(args ...string) {
		cmd := exec.Command(sm.sbieIniExe, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		cmd.Run()
	}

	if enable {
		for _, tpl := range spoofTemplates {
			runCmd("append", boxName, "Template", tpl)
		}
		for k, v := range spoofKeys {
			runCmd("set", boxName, k, v)
		}
	} else {
		for _, tpl := range spoofTemplates {
			runCmd("delete", boxName, "Template", tpl)
		}
		for k := range spoofKeys {
			runCmd("set", boxName, k, "")
		}
	}
}
