package main

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"image/color"
)

func buildDashboardTab(win fyne.Window) fyne.CanvasObject {
	// ==================== Proxy Management Section ====================
	proxyTitle := widget.NewLabel("Proxy Management")
	proxyTitle.TextStyle = fyne.TextStyle{Bold: true}

	proxyInput := widget.NewMultiLineEntry()
	proxyInput.SetPlaceHolder("IP:PORT:USER:PASS\n(one per line)")
	proxyInput.SetMinRowsVisible(4)

	// Load saved proxies
	go func() {
		status, err := api.GetLegacyStatus()
		if err == nil && len(status.SavedProxies) > 0 {
			fyne.Do(func() {
				proxyInput.SetText(strings.Join(status.SavedProxies, "\n"))
			})
		}
	}()

	// --- Progress UI State ---
	var aborting bool
	var totalProxies int
	var doneCount int

	btnShowProgress := NewHoverButton("Progresso", nil)
	btnAbort := NewHoverButton("X", nil)
	btnAbort.Importance = widget.DangerImportance
	progressToolbar := container.NewHBox(btnShowProgress, btnAbort)
	progressToolbar.Hide()

	modalContent := container.NewVBox()
	scrollBox := container.NewVScroll(modalContent)

	modalTitle := widget.NewLabel("Iniciando interfaces...")
	modalTitle.TextStyle = fyne.TextStyle{Bold: true}

	closeBtn := NewHoverButton("Minimizar", nil)

	header := container.NewBorder(nil, nil, nil, closeBtn, modalTitle)
	modalBody := container.NewBorder(container.NewVBox(header, widget.NewSeparator()), nil, nil, nil, scrollBox)

	bg := canvas.NewRectangle(color.NRGBA{R: 24, G: 24, B: 24, A: 240}) // Dark gray native looking background
	solidModal := container.NewStack(bg, container.NewPadded(modalBody))
	sizedCard := container.New(layout.NewGridWrapLayout(fyne.NewSize(400, float32(normalH)*0.60)), solidModal)

	progressOverlay := container.NewCenter(sizedCard)
	progressOverlay.Hide()

	btnShowProgress.OnTapped = func() {
		progressToolbar.Hide()
		progressOverlay.Show()
	}

	closeBtn.OnTapped = func() {
		progressOverlay.Hide()
		if doneCount < totalProxies && !aborting {
			progressToolbar.Show()
		}
	}

	btnStart := NewHoverButton("START PROXIES (tun2socks)", nil)
	btnStart.Importance = widget.SuccessImportance

	btnStop := NewHoverButton("STOP ALL", nil)
	btnStop.Importance = widget.DangerImportance

	btnAbort.OnTapped = func() {
		aborting = true
		go func() {
			err := api.AbortProxies()
			if err != nil {
				log.Println("Abort API Error:", err)
			}
		}()
		progressToolbar.Hide()
		btnStart.SetText("START PROXIES (tun2socks)")
		btnStart.Enable()
	}

	btnStart.OnTapped = func() {
		text := proxyInput.Text
		lines := []string{}
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				lines = append(lines, line)
			}
		}
		if len(lines) == 0 {
			dialog.ShowInformation("Warning", "No valid proxies found!", win)
			return
		}
		btnStart.Disable()

		aborting = false
		totalProxies = len(lines)
		doneCount = 0

		statusLabels := make([]*widget.Label, totalProxies)
		statusIcons := make([]*canvas.Text, totalProxies)
		activities := make([]*widget.Activity, totalProxies)

		modalContent.RemoveAll()
		modalTitle.SetText(fmt.Sprintf("Iniciando %d interfaces...", totalProxies))
		closeBtn.SetText("Minimizar")

		for i := 0; i < totalProxies; i++ {
			lbl := widget.NewLabel(fmt.Sprintf("Interface %d/%d", i+1, totalProxies))
			statusLabels[i] = lbl

			iconLbl := canvas.NewText("⏳", color.White)
			iconLbl.TextSize = 14
			iconLbl.Alignment = fyne.TextAlignCenter
			statusIcons[i] = iconLbl

			act := widget.NewActivity()
			activities[i] = act
			act.Start()

			// Spacer to shift the loading icon away from the right-edge scrollbar
			rightPad := canvas.NewRectangle(color.Transparent)
			rightPad.SetMinSize(fyne.NewSize(20, 0))

			row := container.NewHBox(lbl, layout.NewSpacer(), act, iconLbl, rightPad)
			modalContent.Add(row)
		}

		progressOverlay.Show()
		progressToolbar.Hide()
		btnShowProgress.SetText(fmt.Sprintf("Interface 1/%d ⏳", totalProxies))

		go func() {
			log.Printf("Starting %d proxies...\n", totalProxies)
			err := api.StartProxies(lines)
			if err != nil {
				log.Printf("Error starting proxies: %v\n", err)
			}

			maxWait := 60 // seconds max to wait
			for elapsed := 0; elapsed < maxWait && doneCount < totalProxies && !aborting; elapsed++ {
				time.Sleep(1 * time.Second)

				activeResp, err := api.GetActiveProxies()
				if err != nil {
					continue
				}

				activeCount := len(activeResp.Proxies)
				newDone := activeCount
				if newDone > totalProxies {
					newDone = totalProxies
				}

				if newDone > doneCount {
					for i := doneCount; i < newDone; i++ {
						idx := i
						fyne.Do(func() {
							activities[idx].Stop()
							activities[idx].Hide()
							statusIcons[idx].Text = "✔"
							statusIcons[idx].Color = color.NRGBA{R: 0x2C, G: 0xFF, B: 0x05, A: 0xFF}
							statusIcons[idx].Refresh()
						})
					}

					fyne.Do(func() {
						offsetY := float32(newDone) * 38.0
						if offsetY > scrollBox.Content.MinSize().Height-scrollBox.Size().Height {
							offsetY = scrollBox.Content.MinSize().Height - scrollBox.Size().Height
						}
						// Only scroll if we are pushing past the visible window
						if offsetY > 0 {
							scrollBox.Offset.Y = offsetY
							scrollBox.Refresh()
						}
						btnShowProgress.SetText(fmt.Sprintf("Interface %d/%d ⏳", newDone+1, totalProxies))
					})

					doneCount = newDone
				}
			}

			// Mark remaining as done/timeout/aborted
			fyne.Do(func() {
				if aborting {
					for i := doneCount; i < totalProxies; i++ {
						activities[i].Stop()
						activities[i].Hide()
						if statusIcons[i].Text == "⏳" {
							statusIcons[i].Text = "🚫"
							statusIcons[i].Color = color.NRGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}
							statusIcons[i].Refresh()
						}
					}
					modalTitle.SetText(fmt.Sprintf("Abortado — %d/%d interfaces ativas", doneCount, totalProxies))
					closeBtn.SetText("Fechar")
				} else {
					for i := 0; i < totalProxies; i++ {
						activities[i].Stop()
						activities[i].Hide()
						if statusIcons[i].Text == "⏳" {
							if doneCount >= totalProxies {
								statusIcons[i].Text = "✔"
								statusIcons[i].Color = color.NRGBA{R: 0x2C, G: 0xFF, B: 0x05, A: 0xFF}
							} else {
								statusIcons[i].Text = "⚠"
								statusIcons[i].Color = color.NRGBA{R: 0xFF, G: 0xA5, B: 0x00, A: 0xFF} // Orange Warning
							}
							statusIcons[i].Refresh()
						}
					}
					closeBtn.SetText("Fechar")
					modalTitle.SetText(fmt.Sprintf("Concluído — %d/%d interfaces ativas", doneCount, totalProxies))
					progressToolbar.Hide()
					btnStart.SetText("START PROXIES (tun2socks)")
					btnStart.Enable()
				}
			})
		}()
	}

	btnStop.OnTapped = func() {
		btnStop.Disable()
		btnStop.SetText("STOPPING...")
		go func() {
			_, err := api.StopProxies()
			fyne.Do(func() {
				btnStop.Enable()
				btnStop.SetText("STOP ALL")
				if err != nil {
					dialog.ShowError(err, win)
					return
				}
				dialog.ShowInformation("Info", "Stopped existing tun2socks processes.", win)
			})
		}()
	}

	proxyBtns := container.NewHBox(btnStart, btnStop, progressToolbar)
	proxySection := container.NewVBox(proxyTitle, proxyInput, proxyBtns)

	// ==================== Sandbox Shortcuts Section ====================
	shortcutDirLabel := widget.NewLabel("Atalhos: None")

	sandboxList := container.NewVBox()
	sandboxScroll := container.NewVScroll(sandboxList)
	sandboxScroll.SetMinSize(fyne.NewSize(0, 400))

	// Loading indicator — shown until first data arrives
	loadingIndicator := widget.NewActivity()
	loadingIndicator.Start()
	loadingLabel := widget.NewLabel("Carregando sandboxes...")
	loadingContainer := container.NewCenter(
		container.NewVBox(
			container.NewCenter(loadingIndicator),
			loadingLabel,
		),
	)
	sandboxList.Add(loadingContainer)

	firstLoad := true

	// Track current adapters for dropdowns
	var currentAdapters []string

	// Column widths for alignment
	idColWidth := float32(160)
	appsColWidth := float32(400)
	spoofColWidth := float32(100)
	adapterColWidth := float32(180)
	bindColWidth := float32(70)

	// Function to refresh sandbox list
	refreshSandboxes := func() {
		resp, err := api.GetSandboxes()
		if err != nil || resp.Error != "" {
			return
		}

		currentAdapters = resp.AvailableAdapters

		fyne.Do(func() {
			shortcutDirLabel.SetText("Atalhos: " + resp.Folder)
			sandboxList.RemoveAll()

			// Hide loading on first successful load
			if firstLoad {
				loadingIndicator.Stop()
				firstLoad = false
			}

			// Sort sandboxes numerically by ID
			sort.Slice(resp.Sandboxes, func(i, j int) bool {
				a, _ := strconv.Atoi(resp.Sandboxes[i].ID)
				b, _ := strconv.Atoi(resp.Sandboxes[j].ID)
				return a < b
			})

			for _, sb := range resp.Sandboxes {
				sb := sb // capture

				// ID + Box name label (fixed width)
				idLabel := widget.NewLabel(fmt.Sprintf("#%s [%s]", sb.ID, sb.BoxName))
				idLabel.TextStyle = fyne.TextStyle{Bold: true}
				idContainer := container.New(layout.NewGridWrapLayout(fyne.NewSize(idColWidth, 36)), idLabel)

				// App launch buttons (fixed width container)
				appBtns := container.NewHBox()
				for _, appFile := range sb.Apps {
					appFile := appFile
					appName := strings.Replace(appFile, ".lnk", "", 1)
					appName = strings.Replace(appName, ".exe", "", 1)
					// Extract just the app portion after the bracket
					parts := strings.SplitN(appName, "] ", 2)
					if len(parts) == 2 {
						appName = parts[1]
					}
					// Truncate long names
					if len(appName) > 15 {
						appName = appName[:12] + "..."
					}

					btn := NewHoverButton(appName, func() {
						go api.LaunchByName(sb.BoxName, appFile)
					})
					appBtns.Add(btn)
				}
				appsContainer := container.New(layout.NewGridWrapLayout(fyne.NewSize(appsColWidth, 36)), appBtns)

				// Spoof button (fixed width)
				spoofText := "Spoofar"
				spoofImportance := widget.MediumImportance
				if sb.IsSpoofed {
					spoofText = "Spoofado"
					spoofImportance = widget.SuccessImportance
				}
				spoofBtn := NewHoverButton(spoofText, nil)
				spoofBtn.Importance = spoofImportance
				spoofBtn.OnTapped = func() {
					fyne.Do(func() {
						spoofBtn.SetText("Aplicando...")
						spoofBtn.Importance = widget.LowImportance
					})
					go func() {
						api.ToggleSpoof(sb.BoxName)
						time.Sleep(500 * time.Millisecond)
						newResp, err := api.GetSandboxes()
						if err == nil {
							for _, newSb := range newResp.Sandboxes {
								if newSb.BoxName == sb.BoxName {
									fyne.Do(func() {
										if newSb.IsSpoofed {
											spoofBtn.SetText("Spoofado")
											spoofBtn.Importance = widget.SuccessImportance
										} else {
											spoofBtn.SetText("Spoofar")
											spoofBtn.Importance = widget.MediumImportance
										}
									})
									break
								}
							}
						}
					}()
				}
				spoofContainer := container.New(layout.NewGridWrapLayout(fyne.NewSize(spoofColWidth, 36)), spoofBtn)

				// Bind adapter dropdown (fixed width)
				adapterSelect := widget.NewSelect(currentAdapters, nil)
				adapterSelect.SetSelected(sb.Bind)
				adapterContainer := container.New(layout.NewGridWrapLayout(fyne.NewSize(adapterColWidth, 36)), adapterSelect)

				// Bind button (fixed width)
				bindBtn := NewHoverButton("Bind", func() {
					selected := adapterSelect.Selected
					if selected == "" {
						return
					}
					go func() {
						api.BindSandbox(sb.BoxName, selected)
						log.Printf("Set %s to %s\n", sb.BoxName, selected)
					}()
				})
				bindBtn.Importance = widget.HighImportance
				bindContainer := container.New(layout.NewGridWrapLayout(fyne.NewSize(bindColWidth, 36)), bindBtn)

				// Aligned row: ID+Apps on left, Spoof+Adapter+Bind flush right
				rightGroup := container.NewHBox(spoofContainer, adapterContainer, bindContainer)
				row := container.NewBorder(nil, nil, idContainer, rightGroup, appsContainer)
				sandboxList.Add(row)
				sandboxList.Add(widget.NewSeparator())
			}
		})
	}

	btnSelectFolder := NewHoverButton("Select Shortcut Folder", func() {
		go func() {
			resp, err := api.SelectFolder()
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(err, win)
				})
				return
			}
			if resp.Status == "ok" {
				fyne.Do(func() {
					shortcutDirLabel.SetText("Atalhos: " + resp.Path)
				})
				refreshSandboxes()
			}
		}()
	})

	shortcutHeader := container.NewBorder(nil, nil, shortcutDirLabel, btnSelectFolder)

	// Initial load (delayed to allow backend to stabilize)
	go func() {
		time.Sleep(2 * time.Second)
		refreshSandboxes()
	}()

	// Auto-refresh every 6 seconds
	go func() {
		time.Sleep(8 * time.Second) // first auto-refresh after 8s
		for {
			refreshSandboxes()
			time.Sleep(6 * time.Second)
		}
	}()

	// ==================== Full Layout ====================
	mainLayout := container.NewBorder(
		container.NewVBox(proxySection, widget.NewSeparator(), shortcutHeader),
		nil, nil, nil,
		sandboxScroll,
	)

	return container.NewStack(mainLayout, progressOverlay)
}

// statusColorRect creates a small colored rectangle for status indication.
func statusColorRect(c color.Color, w, h float32) *canvas.Rectangle {
	r := canvas.NewRectangle(c)
	r.SetMinSize(fyne.NewSize(w, h))
	return r
}
