package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

func buildConfigTab(win fyne.Window) fyne.CanvasObject {
	title := widget.NewLabel("Application Paths")
	title.TextStyle = fyne.TextStyle{Bold: true}

	// Load current config
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

	// Save button
	saveBtn := NewHoverButton("Save All Paths", func() {
		go func() {
			api.UpdateConfig(map[string]string{
				"tun2socks":     tun2socksEntry.Text,
				"wintun":        wintunEntry.Text,
				"sandboxie_ini": sboxIniEntry.Text,
				"sbie_ini_exe":  sbieExeEntry.Text,
			})
		}()
	})
	saveBtn.Importance = widget.HighImportance

	// Path selector rows with working browse button
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

	rows := container.NewVBox(
		buildRow("tun2socks.exe Path:", tun2socksEntry, "tun2socks"),
		buildRow("wintun.dll Path:", wintunEntry, "wintun"),
		buildRow("Sandboxie.ini Path:", sboxIniEntry, "sandboxie_ini"),
		buildRow("SbieIni.exe Path:", sbieExeEntry, "sbie_ini_exe"),
	)

	return container.NewVBox(title, rows, saveBtn)
}
