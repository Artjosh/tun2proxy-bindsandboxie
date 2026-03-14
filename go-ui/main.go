package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
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
	api         *APIClient
	backendProc *exec.Cmd
	logFile     *os.File
)

// setupLogging redirects all log/fmt output to a file instead of a console window.
func setupLogging() {
	exePath, _ := os.Executable()
	logPath := filepath.Join(filepath.Dir(exePath), "manager_gui.log")

	// Fallback if exe path detection fails
	if _, err := os.Stat(filepath.Dir(exePath)); os.IsNotExist(err) {
		logPath = "manager_gui.log"
	}

	var err error
	logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}

	// Do NOT redirect os.Stdout/Stderr here as it causes Fyne UI to crash on Windows when using Windows subsystem (-H=windowsgui)
	// Just use a standard logger for manual statements if needed, but since we use `log.Println`, we just let it go to default (or nowhere).
	log.SetOutput(logFile)
	log.Println("=== Manager GUI started ===")
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

// moveWindow repositions the window to ensure it fits on screen, centering it.
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

// startBackend launches the Go manager.exe as a child process.
func startBackend() error {
	exePath, _ := os.Executable()
	rootDir := filepath.Dir(filepath.Dir(exePath))
	scriptPath := filepath.Join(rootDir, "manager_go", "manager.exe")

	// If running from source (go run), use cwd-based detection
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		cwd, _ := os.Getwd()
		rootDir = filepath.Dir(cwd)
		scriptPath = filepath.Join(rootDir, "manager_go", "manager.exe")
	}
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		scriptPath = filepath.Join(".", "..", "manager_go", "manager.exe")
	}

	log.Printf("Starting backend: %s\n", scriptPath)

	backendProc = exec.Command(scriptPath)
	backendProc.Dir = filepath.Dir(scriptPath)
	backendProc.Stdout = logFile
	backendProc.Stderr = logFile

	// Hide console window on Windows
	backendProc.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	// Kill existing manager.exe processes to prevent port conflicts
	killCmd := exec.Command("taskkill", "/F", "/IM", "manager.exe")
	killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	killCmd.Run()

	// Wait a moment for the port to be released
	time.Sleep(1 * time.Second)

	if err := backendProc.Start(); err != nil {
		return fmt.Errorf("failed to start backend: %w", err)
	}

	log.Printf("Backend started with PID %d\n", backendProc.Process.Pid)
	return nil
}

func stopBackend() {
	log.Println("Stopping backend...")
	if backendProc != nil && backendProc.Process != nil {
		backendProc.Process.Kill()
	}
	
	killCmd := exec.Command("taskkill", "/F", "/IM", "manager.exe")
	killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	killCmd.Run()

	if logFile != nil {
		logFile.Close()
	}
}

func main() {
	// Admin check
	if !isAdmin() {
		relaunchAsAdmin()
		return
	}

	// Setup file logging (must be before any log/fmt calls)
	setupLogging()

	// Initialize API client
	api = NewAPIClient(backendURL)

	// Start Python backend if not already running
	if !api.IsBackendAlive() {
		if err := startBackend(); err != nil {
			log.Println("Warning: Could not auto-start backend:", err)
		} else {
			// Wait for backend to be ready (increased timeout + retry)
			if err := api.WaitForBackend(30 * time.Second); err != nil {
				log.Println("Warning: Backend did not become ready:", err)
			} else {
				log.Println("Backend is ready.")
			}
		}
	} else {
		log.Println("Backend already running.")
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
			// Reposition to ensure it fits on screen
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

	// Cleanup on close
	myWindow.SetOnClosed(func() {
		stopBackend()
	})

	myWindow.ShowAndRun()
}
