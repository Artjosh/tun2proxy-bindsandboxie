package main

import (
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	backendURL = "http://127.0.0.1:8000"
	appTitle   = "Joshboxie Tun2socks adapter manager and HWID changer"
	normalW    = 1000
	normalH    = 800
	ipcheckW   = 1800
	ipcheckH   = 800
)

var (
	api     *APIClient
	logFile *os.File
)

// setupLogging redirects log output to a file.
func setupLogging() {
	exePath, _ := os.Executable()
	logPath := filepath.Join(filepath.Dir(exePath), "manager_gui.log")
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		logFile = f
		// Do NOT redirect os.Stdout/Stderr — crashes Fyne with -H=windowsgui
		log.SetOutput(logFile)
		log.Println("=== Manager GUI started ===")
	}
}

// isAdmin checks if the current process is running with admin privileges.
func isAdmin() bool {
	shell32 := syscall.NewLazyDLL("shell32.dll")
	proc := shell32.NewProc("IsUserAnAdmin")
	ret, _, _ := proc.Call()
	return ret != 0
}

// relaunchAsAdmin re-launches this executable with admin rights.
func relaunchAsAdmin() {
	exe, _ := os.Executable()
	verb := "runas"
	cwd, _ := os.Getwd()

	shell32 := syscall.NewLazyDLL("shell32.dll")
	proc := shell32.NewProc("ShellExecuteW")

	verbPtr, _ := syscall.UTF16PtrFromString(verb)
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	argsPtr, _ := syscall.UTF16PtrFromString("")
	cwdPtr, _ := syscall.UTF16PtrFromString(cwd)

	proc.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(exePtr)),
		uintptr(unsafe.Pointer(argsPtr)),
		uintptr(unsafe.Pointer(cwdPtr)),
		1, // SW_SHOWNORMAL
	)
	os.Exit(0)
}

// getScreenSize returns the primary monitor resolution using user32.dll
func getScreenSize() (int, int) {
	user32 := syscall.NewLazyDLL("user32.dll")
	getSystemMetrics := user32.NewProc("GetSystemMetrics")
	w, _, _ := getSystemMetrics.Call(0) // SM_CXSCREEN
	h, _, _ := getSystemMetrics.Call(1) // SM_CYSCREEN
	return int(w), int(h)
}

// moveWindow repositions the window to center it on screen.
func moveWindow(hwnd uintptr, width, height int) {
	user32 := syscall.NewLazyDLL("user32.dll")
	moveWin := user32.NewProc("MoveWindow")
	screenW, screenH := getScreenSize()
	x := (screenW - width) / 2
	y := (screenH - height) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	moveWin.Call(hwnd, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 1)
}

// findMyWindow finds this application's window handle by title.
func findMyWindow(title string) uintptr {
	user32 := syscall.NewLazyDLL("user32.dll")
	findWindow := user32.NewProc("FindWindowW")
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	hwnd, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	return hwnd
}

// centerWindowOnScreen centers and resizes the window to given dimensions.
func centerWindowOnScreen(title string, width, height int) {
	hwnd := findMyWindow(title)
	if hwnd != 0 {
		moveWindow(hwnd, width, height)
	}
}

func main() {
	// Admin check
	if !isAdmin() {
		relaunchAsAdmin()
		return
	}

	// Setup file logging (before any log calls)
	setupLogging()

	// Determine root dir (where config.json, tun2socks.exe, etc. live)
	// When compiled as a single exe, the exe lives alongside those files
	exePath, _ := os.Executable()
	rootDir := filepath.Dir(exePath)

	// Start the HTTP backend server as a goroutine in this same process
	log.Println("Starting embedded HTTP server...")
	go runServer(8000, rootDir)

	// Give the server a moment to bind the port before UI tries to call it
	time.Sleep(500 * time.Millisecond)

	// Initialize API client
	api = NewAPIClient(backendURL)

	// Wait for backend to be ready (it's in-process so should be fast)
	if err := api.WaitForBackend(15 * time.Second); err != nil {
		log.Println("Warning: embedded backend did not become ready:", err)
	} else {
		log.Println("Backend ready.")
	}

	// Create Fyne application
	myApp := app.New()
	myApp.Settings().SetTheme(theme.DarkTheme())

	myWindow := myApp.NewWindow(appTitle)
	myWindow.Resize(fyne.NewSize(normalW, normalH))

	// Admin status label
	adminText := "ADMIN: NO (Re-run as Admin)"
	adminOk := false
	if adm, err := api.CheckAdmin(); err == nil {
		adminOk = adm
	}
	if adminOk {
		adminText = "ADMIN: YES"
	}
	adminLabel := widget.NewLabel(adminText)
	if adminOk {
		adminLabel.Importance = widget.SuccessImportance
	} else {
		adminLabel.Importance = widget.DangerImportance
	}

	// Header
	titleLabel := widget.NewLabel(appTitle)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	header := container.NewBorder(nil, nil, titleLabel, adminLabel)

	// Tabs
	dashboardTab := buildDashboardTab(myWindow)
	ipcheckTab := buildIPCheckTab()
	configTab := buildConfigTab(myWindow)

	tabs := container.NewAppTabs(
		container.NewTabItem("Dashboard", dashboardTab),
		container.NewTabItem("IP Check", ipcheckTab),
		container.NewTabItem("Configuration", configTab),
	)

	tabs.OnSelected = func(tab *container.TabItem) {
		if tab.Text == "IP Check" {
			myWindow.Resize(fyne.NewSize(ipcheckW, ipcheckH))
			go func() {
				time.Sleep(100 * time.Millisecond)
				centerWindowOnScreen(appTitle, ipcheckW, ipcheckH)
			}()
		} else {
			myWindow.Resize(fyne.NewSize(normalW, normalH))
			go func() {
				time.Sleep(100 * time.Millisecond)
				centerWindowOnScreen(appTitle, normalW, normalH)
			}()
		}
	}

	// Main layout
	mainContent := container.NewBorder(header, nil, nil, nil, tabs)
	myWindow.SetContent(mainContent)

	myWindow.ShowAndRun()
	// When the window closes, the server goroutine dies with the process automatically
}
