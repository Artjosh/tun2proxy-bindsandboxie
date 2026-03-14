package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"image/color"
)

func buildIPCheckTab() fyne.CanvasObject {
	// ==================== Left Panel ====================
	leftTitle := widget.NewLabel("IP Checker")
	leftTitle.TextStyle = fyne.TextStyle{Bold: true}

	ipLabel := widget.NewLabel("IPs (1 por linha):")
	ipInput := widget.NewMultiLineEntry()
	ipInput.SetPlaceHolder("8.8.8.8\n1.1.1.1")
	ipInput.SetMinRowsVisible(8)

	apiKeyLabel := widget.NewLabel("IPQS API Key:")
	apiKeyEntry := widget.NewEntry()
	apiKeyEntry.SetPlaceHolder("Enter API key")

	// Load saved API key
	go func() {
		cfg, err := api.GetConfig()
		if err == nil {
			_ = cfg // ipqs key is stored separately, loaded at runtime
		}
	}()

	// ==================== Right Panel ====================
	rightTitle := widget.NewLabel("Resultados")
	rightTitle.TextStyle = fyne.TextStyle{Bold: true}

	resultsBox := container.NewVBox()
	resultsScroll := container.NewVScroll(resultsBox)
	resultsScroll.SetMinSize(fyne.NewSize(0, 500))

	// ==================== Check Button ====================
	btnCheck := NewHoverButton("Check", nil)
	btnCheck.Importance = widget.HighImportance

	btnSaveKey := NewHoverButton("Save API Key", func() {
		key := strings.TrimSpace(apiKeyEntry.Text)
		if key != "" {
			go api.UpdateConfig(map[string]string{"ipqs_api_key": key})
		}
	})

	btnCheck.OnTapped = func() {
		raw := ipInput.Text
		ips := []string{}
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				ips = append(ips, line)
			}
		}
		if len(ips) == 0 {
			return
		}

		btnCheck.Disable()
		btnCheck.SetText("Checking...")
		resultsBox.RemoveAll()

		apiKey := strings.TrimSpace(apiKeyEntry.Text)

		go func() {
			for _, ip := range ips {
				ip := ip
				resp, err := api.CheckIP(ip, apiKey)
				if err != nil {
					card := buildErrorCard(ip, err.Error())
					fyne.Do(func() {
						resultsBox.Add(card)
					})
					continue
				}
				card := buildIPCard(ip, resp.Results)
				fyne.Do(func() {
					resultsBox.Add(card)
				})
			}
			fyne.Do(func() {
				btnCheck.SetText("Check")
				btnCheck.Enable()
			})
		}()
	}

	btnRow := container.NewHBox(btnCheck, btnSaveKey)

	leftPanel := container.NewVBox(
		leftTitle,
		ipLabel,
		ipInput,
		apiKeyLabel,
		apiKeyEntry,
		btnRow,
	)

	rightPanel := container.NewBorder(rightTitle, nil, nil, nil, resultsScroll)

	// Split layout: left (input) | right (results)
	split := container.NewHSplit(leftPanel, rightPanel)
	split.SetOffset(0.3)

	return split
}

func normalizeBool(val interface{}) string {
	if val == nil {
		return ""
	}
	s := fmt.Sprintf("%v", val)
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "true", "yes", "y", "1":
		return "Yes"
	case "false", "no", "n", "0":
		return "No"
	}
	return s
}

func isDirty(results []IPCheckResult) (bool, []string) {
	var reasons []string
	seen := map[string]bool{}

	for _, r := range results {
		if r.Status != "ok" {
			continue
		}
		d := r.Data
		flags := []string{"VPN", "Proxy", "TOR", "Bot", "Proxy/VPN", "Hosting/DC"}
		for _, f := range flags {
			if v, ok := d[f]; ok {
				if normalizeBool(v) == "Yes" && !seen[f] {
					reasons = append(reasons, f)
					seen[f] = true
				}
			}
		}
	}
	return len(reasons) > 0, reasons
}

func buildIPCard(ip string, results []IPCheckResult) fyne.CanvasObject {
	dirty, reasons := isDirty(results)

	// Header
	headerColor := color.NRGBA{R: 46, G: 163, B: 106, A: 255} // green
	headerText := "✓ CLEAN"
	if dirty {
		headerColor = color.NRGBA{R: 178, G: 59, B: 59, A: 255} // red
		headerText = "✗ DIRTY"
	}

	headerBg := canvas.NewRectangle(headerColor)
	headerBg.SetMinSize(fyne.NewSize(0, 36))

	ipLabel := widget.NewLabel(ip)
	ipLabel.TextStyle = fyne.TextStyle{Bold: true}
	statusLabel := widget.NewLabel(headerText)
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	header := container.NewStack(
		headerBg,
		container.NewBorder(nil, nil, ipLabel, statusLabel),
	)

	// Score summary line
	scoreMap := map[string]interface{}{}
	for _, r := range results {
		if r.Status == "ok" {
			if fs, ok := r.Data["Fraud Score"]; ok {
				scoreMap[r.Source] = fs
			}
		}
	}
	scoreLine := fmt.Sprintf("IPQualityScore: %v | Scamalytics: %v",
		scoreMap["IPQualityScore"], scoreMap["Scamalytics"])
	if len(reasons) > 0 {
		scoreLine += " | Flags: " + strings.Join(reasons, ", ")
	}
	scoreLabel := widget.NewLabel(scoreLine)

	// Detail rows per source
	body := container.NewVBox(scoreLabel, widget.NewSeparator())

	for _, r := range results {
		r := r
		sourceLine := fmt.Sprintf("• %s", r.Source)

		if r.Status != "ok" {
			errText := r.Status
			if r.Error != "" {
				errText += " - " + r.Error
			}
			row := widget.NewLabel(sourceLine + "  " + errText)
			body.Add(row)
			continue
		}

		// Build detail text
		d := r.Data
		parts := []string{sourceLine}

		// Flag fields
		flagFields := []string{"VPN", "Proxy", "TOR", "Bot", "Proxy/VPN", "Hosting/DC"}
		for _, f := range flagFields {
			if v, ok := d[f]; ok {
				norm := normalizeBool(v)
				if norm != "" {
					parts = append(parts, fmt.Sprintf("%s:%s", f, norm))
				}
			}
		}

		// Fraud score and risk
		if v, ok := d["Fraud Score"]; ok {
			parts = append(parts, fmt.Sprintf("Fraud:%v", v))
		}
		if v, ok := d["Risk"]; ok {
			parts = append(parts, fmt.Sprintf("Risk:%v", v))
		}

		// Source-specific fields
		if r.Source == "ipinfo.io" {
			if v, ok := d["Privacy"]; ok {
				parts = append(parts, fmt.Sprintf("Privacy:%s", normalizeBool(v)))
			}
			if v, ok := d["Anycast"]; ok {
				parts = append(parts, fmt.Sprintf("Anycast:%s", normalizeBool(v)))
			}
		}
		if r.Source == "IPQualityScore" {
			if v, ok := d["Recent Abuse"]; ok {
				parts = append(parts, fmt.Sprintf("RecentAbuse:%s", normalizeBool(v)))
			}
		}

		// Geo fields
		geoFields := []string{"Country", "Region", "City", "ISP", "Organization", "ASN"}
		for _, f := range geoFields {
			if v, ok := d[f]; ok {
				vs := fmt.Sprintf("%v", v)
				if vs != "" && vs != "N/A" {
					parts = append(parts, fmt.Sprintf("%s:%s", f, vs))
				}
			}
		}

		row := widget.NewLabel(strings.Join(parts, "  "))
		row.Wrapping = fyne.TextWrapWord
		body.Add(row)
	}

	card := container.NewVBox(header, body, widget.NewSeparator())
	return card
}

func buildErrorCard(ip, errMsg string) fyne.CanvasObject {
	headerBg := canvas.NewRectangle(color.NRGBA{R: 178, G: 59, B: 59, A: 255})
	headerBg.SetMinSize(fyne.NewSize(0, 36))

	ipLabel := widget.NewLabel(ip)
	ipLabel.TextStyle = fyne.TextStyle{Bold: true}
	errLabel := widget.NewLabel("ERROR: " + errMsg)

	header := container.NewStack(
		headerBg,
		container.NewBorder(nil, nil, ipLabel, nil),
	)

	return container.NewVBox(header, errLabel, widget.NewSeparator())
}
