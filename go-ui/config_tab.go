package main

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

func buildConfigTab(win fyne.Window) fyne.CanvasObject {
	// ==================== Section 1: Application Paths ====================
	pathsTitle := widget.NewLabel("Application Paths")
	pathsTitle.TextStyle = fyne.TextStyle{Bold: true}

	var cfg *ConfigResp
	cfg, _ = api.GetConfig()
	if cfg == nil {
		cfg = &ConfigResp{}
	}

	tun2socksEntry := widget.NewEntry()
	tun2socksEntry.SetText(cfg.Tun2socks)
	tun2socksEntry.SetPlaceHolder("Path to tun2socks.exe")

	wintunEntry := widget.NewEntry()
	wintunEntry.SetText(cfg.Wintun)
	wintunEntry.SetPlaceHolder("Path to wintun.dll")

	sboxIniEntry := widget.NewEntry()
	sboxIniEntry.SetText(cfg.SandboxieINI)
	sboxIniEntry.SetPlaceHolder("Path to Sandboxie.ini")

	sbieExeEntry := widget.NewEntry()
	sbieExeEntry.SetText(cfg.SbieINIExe)
	sbieExeEntry.SetPlaceHolder("Path to SbieIni.exe")

	savePathsBtn := NewHoverButton("Save All Paths", func() {
		go func() {
			api.UpdateConfig(map[string]string{
				"tun2socks":     tun2socksEntry.Text,
				"wintun":        wintunEntry.Text,
				"sandboxie_ini": sboxIniEntry.Text,
				"sbie_ini_exe":  sbieExeEntry.Text,
			})
		}()
	})
	savePathsBtn.Importance = widget.HighImportance

	buildRow := func(label string, entry *widget.Entry, configKey string) fyne.CanvasObject {
		lbl := widget.NewLabel(label)
		lbl.TextStyle = fyne.TextStyle{Bold: true}
		browseBtn := NewHoverButton("Browse", func() {
			fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
				if err != nil || reader == nil {
					return
				}
				path := reader.URI().Path()
				entry.SetText(path)
				go api.UpdateConfig(map[string]string{configKey: path})
				reader.Close()
			}, win)
			fd.SetFilter(storage.NewExtensionFileFilter([]string{".exe", ".dll", ".ini"}))
			fd.Show()
		})
		return container.NewBorder(nil, nil, lbl, browseBtn, entry)
	}

	pathRows := container.NewVBox(
		buildRow("tun2socks.exe Path:", tun2socksEntry, "tun2socks"),
		buildRow("wintun.dll Path:", wintunEntry, "wintun"),
		buildRow("Sandboxie.ini Path:", sboxIniEntry, "sandboxie_ini"),
		buildRow("SbieIni.exe Path:", sbieExeEntry, "sbie_ini_exe"),
	)

	// ==================== Section 2: Spoof Whitelist ====================
	spoofTitle := widget.NewLabel("Spoof Process Whitelist")
	spoofTitle.TextStyle = fyne.TextStyle{Bold: true}

	spoofDesc := widget.NewLabel(
		"Processes below are excluded from HWID spoofing (whitelisted).\n" +
			"These are applied automatically to every sandbox when you click Spoofar.",
	)
	spoofDesc.Wrapping = fyne.TextWrapWord

	whitelistEntry := widget.NewMultiLineEntry()
	whitelistEntry.SetPlaceHolder("spotify.exe\ndiscord.exe\n...")
	whitelistEntry.SetMinRowsVisible(5)
	// Load current list from config
	whitelistEntry.SetText(strings.Join(cfg.SpoofWhitelistProcesses, "\n"))

	saveWhitelistBtn := NewHoverButton("Save Whitelist", func() {
		lines := strings.Split(whitelistEntry.Text, "\n")
		var procs []string
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" {
				procs = append(procs, l)
			}
		}
		go api.SetSpoofWhitelistProcesses(procs)
	})
	saveWhitelistBtn.Importance = widget.WarningImportance

	spoofSection := container.NewVBox(
		spoofTitle,
		spoofDesc,
		whitelistEntry,
		saveWhitelistBtn,
	)

	// ==================== Full Layout ====================
	return container.NewVBox(
		pathsTitle,
		pathRows,
		savePathsBtn,
		widget.NewSeparator(),
		spoofSection,
	)
}
