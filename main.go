package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// --- Constants for Windows API ---
const (
	CW_USEDEFAULT   = 0x80000000
	CS_VREDRAW      = 0x0001
	CS_HREDRAW      = 0x0002
	SW_HIDE         = 0
	SW_SHOW         = 5
	WM_DESTROY      = 0x0002
	WM_PAINT        = 0x000F
	WM_CREATE       = 0x0001
	WM_COMMAND      = 0x0111
	BTN_PUSHED      = 1
	ID_BUTTON_START = 1001
	ID_BUTTON_STOP  = 1002
	ID_EDIT_FILE    = 2001
	ID_STATIC_TXT   = 3001

	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_BORDER           = 0x00800000
	WS_CAPTION          = 0x00C00000
	WS_CLIPSIBLINGS     = 0x04000000
	WS_CLIPCHILDREN     = 0x02000000
	WS_DISABLED         = 0x08000000
	WS_GROUP            = 0x00020000
	WS_TABSTOP          = 0x00010000
	WS_THICKFRAME       = 0x00040000
	WS_POPUP            = 0x80000000
	WS_SYSMENU          = 0x00080000
	WS_MINIMIZEBOX      = 0x00020000
	WS_MAXIMIZEBOX      = 0x00010000

	BS_PUSHBUTTON    = 0
	BS_DEFPUSHBUTTON = 1
	ES_AUTOHSCROLL   = 0x00000080
	ES_MULTILINE     = 0x00000200
	ES_WANTRETURN    = 0x00001000

	WM_USER = 0x0400
)

var (
	user32DLL   = windows.NewLazySystemDLL("user32.dll")
	kernel32DLL = windows.NewLazySystemDLL("kernel32.dll")

	procCreateWindowExA     = user32DLL.NewProc("CreateWindowExA")
	procRegisterClassA      = user32DLL.NewProc("RegisterClassExA")
	procPostQuitMessage     = user32DLL.NewProc("PostQuitMessage")
	procDispatchMessageA    = user32DLL.NewProc("DispatchMessageA")
	procGetMessageA         = user32DLL.NewProc("GetMessageA")
	procTranslateMessage    = user32DLL.NewProc("TranslateMessage")
	procSendMessageA        = user32DLL.NewProc("SendMessageA")
	procFindWindowA         = user32DLL.NewProc("FindWindowA")
	procSetForegroundWindow = user32DLL.NewProc("SetForegroundWindow")
	procSendInput           = user32DLL.NewProc("SendInput")
	procShowWindow          = user32DLL.NewProc("ShowWindow")
	procUpdateWindow        = user32DLL.NewProc("UpdateWindow")
	procKillTimer           = user32DLL.NewProc("KillTimer")
	procSetTimer            = user32DLL.NewProc("SetTimer")
	procDefWindowProcA      = user32DLL.NewProc("DefWindowProcA")

	procGetModuleHandleA = kernel32DLL.NewProc("GetModuleHandleA")
)

type Window struct {
	hwnd      HWND
	filePath  string
	isRunning bool
	cancelCtx context.Context // We need context, so we import it
	ctxCancel context.CancelFunc
}

// Global state for the worker goroutine
var globalState = struct {
	mu       sync.Mutex
	isActive bool
	cancel   context.CancelFunc
}{isActive: false}

// --- Input Simulation Structures ---
type INPUT struct {
	Type  uint32
	Union unsafe.Pointer
}

type KEYBDINPUT struct {
	Vk          uint16
	Scan        uint16
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

const INPUT_KEYBOARD = 1
const KEYEVENTF_KEYUP = 0x0002

func sendKeyR() {
	var inputs [2]INPUT
	keyDown := KEYBDINPUT{Vk: 0x52} // R key virtual code
	inputs[0].Type = INPUT_KEYBOARD
	inputs[0].Union = (*KEYBDINPUT)(unsafe.Pointer(&keyDown))

	keyUp := KEYBDINPUT{Vk: 0x52, Flags: KEYEVENTF_KEYUP}
	inputs[1].Type = INPUT_KEYBOARD
	inputs[1].Union = (*KEYBDINPUT)(unsafe.Pointer(&keyUp))

	ret, _, _ := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		uintptr(unsafe.Sizeof(INPUT{})),
	)
	if ret == 0 {
		fmt.Println("Failed to send input.")
	}
}

func findArcheageWindow() HWND {
	// Try common names. Adjust if the actual window title differs.
	// ArcheAge usually has "ArcheAge" or similar in the title.
	names := []string{"ArcheAge", "ArcheAge - ", "ArcheAge.exe"}
	for _, name := range names {
		hwnd, _, _ := procFindWindowA.Call(
			0,
			StringToUTF16Ptr(name),
		)
		if hwnd != 0 {
			return HWND(hwnd)
		}
	}
	return 0
}

// --- Main Logic ---

func main() {
	// Get the directory of the executable
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	defaultFile := filepath.Join(exeDir, "actions.txt")

	// Create the window
	win := &Window{filePath: defaultFile}
	go win.Run()

	// Wait for exit (blocking call)
	select {}
}

func (w *Window) Run() {
	// Register Class
	className := StringToUTF16Ptr("ArcheActionTool")
	instance, _, _ := procGetModuleHandleA.Call(0)

	wc := struct {
		Style         uint32
		CbClsExtra    int32
		CbWndExtra    int32
		HInstance     HINSTANCE
		HIcon         HICON
		HCursor       HCURSOR
		HbrBackground HBRUSH
		LpszMenuName  *uint16
		LpszClassName *uint16
		HIconSm       HICON
	}{
		Style:         CS_VREDRAW | CS_HREDRAW,
		HInstance:     HINSTANCE(instance),
		HbrBackground: 6, // COLOR_WINDOW + 1
		LpszClassName: className,
	}

	_, _, _ = procRegisterClassA.Call(uintptr(unsafe.Pointer(&wc)))

	// Create Window (Hidden initially, then shown minimized/overlay)
	title := StringToUTF16Ptr("ArcheAgent Overlay")
	hwnd, _, _ := procCreateWindowExA.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		WS_OVERLAPPEDWINDOW|WS_VISIBLE,
		CW_USEDEFAULT, CW_USEDEFAULT, 300, 150,
		0, 0, HINSTANCE(instance), 0,
	)

	w.hwnd = hwnd

	// Add Controls programmatically (Button, Edit Box)
	// Button Start
	procSendMessageA.Call(uintptr(hwnd), WM_CREATE, 0, 0)

	// Simple layout: Button at bottom, Text box above
	// We'll use SendMessage to create child controls
	createChildControl(hwnd, ID_BUTTON_START, "Start", 20, 100, 100, 30)
	createChildControl(hwnd, ID_BUTTON_STOP, "Stop", 130, 100, 100, 30)
	createChildControl(hwnd, ID_EDIT_FILE, "", 20, 20, 260, 70)

	// Set initial text in edit box
	procSendMessageA.Call(uintptr(hwnd), WM_SETTEXT, 0, uintptr(unsafe.Pointer(StringToUTF16Ptr(defaultFile))))

	// Show window (We can try to make it overlay-like by making it topmost or just visible)
	procShowWindow.Call(uintptr(hwnd), SW_SHOWMINIMIZED)
	// Note: True "overlay" (transparent/always on top) requires more complex registry/settings or setting WS_EX_TOPMOST.
	// For now, we minimize it but keep it running.

	// Message Loop
	msg := struct {
		Hwnd    HWND
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      struct {
			X int32
			Y int32
		}
	}{}

	for {
		ret, _, _ := procGetMessageA.Call(
			uintptr(unsafe.Pointer(&msg)),
			0, 0, 0,
		)
		if ret <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageA.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func createChildControl(parentHWND HWND, id int, text string, x, y, w, h int) {
	textPtr := StringToUTF16Ptr(text)
	style := BS_PUSHBUTTON | WS_CHILD | WS_VISIBLE | WS_BORDER | WS_TABSTOP
	if id == ID_EDIT_FILE {
		style = ES_MULTILINE | ES_AUTOHSCROLL | WS_CHILD | WS_VISIBLE | WS_BORDER | WS_TABSTOP
	}

	hwnd, _, _ := procCreateWindowExA.Call(
		0,
		uintptr(unsafe.Pointer(StringToUTF16Ptr("BUTTON"))),
		uintptr(unsafe.Pointer(textPtr)),
		style,
		x, y, w, h,
		parentHWND,
		HMENU(id),
		0,
		0,
	)
	_ = hwnd
}

// This function handles messages from the OS
// In a real app, we'd parse WM_COMMAND here.
// For simplicity in this snippet, we rely on a global timer or polling if needed,
// but let's implement a basic message handler loop inside the Run function properly.
// Actually, the above Run function is incomplete regarding message dispatching for buttons.
// Let's refactor the Run to include a proper WndProc.

// Re-implementing Run with a proper WndProc closure
func (w *Window) RunWithWndProc() {
	className := "ArcheActionTool"
	instance, _, _ := procGetModuleHandleA.Call(0)

	wc := struct {
		Style         uint32
		CbClsExtra    int32
		CbWndExtra    int32
		HInstance     HINSTANCE
		HIcon         HICON
		HCursor       HCURSOR
		HbrBackground HBRUSH
		LpszMenuName  *uint16
		LpszClassName *uint16
		HIconSm       HICON
	}{
		Style:         CS_VREDRAW | CS_HREDRAW,
		HInstance:     HINSTANCE(instance),
		HbrBackground: 6,
		LpszClassName: StringToUTF16Ptr(className),
	}
	procRegisterClassA.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowExA.Call(
		0,
		uintptr(unsafe.Pointer(StringToUTF16Ptr(className))),
		uintptr(unsafe.Pointer(StringToUTF16Ptr("ArcheAgent"))),
		WS_OVERLAPPEDWINDOW|WS_VISIBLE,
		CW_USEDEFAULT, CW_USEDEFAULT, 300, 150,
		0, 0, HINSTANCE(instance), 0,
	)
	w.hwnd = hwnd

	// Create Controls
	createChildControl(hwnd, ID_BUTTON_START, "Start", 20, 100, 100, 30)
	createChildControl(hwnd, ID_BUTTON_STOP, "Stop", 130, 100, 100, 30)
	createChildControl(hwnd, ID_EDIT_FILE, "", 20, 20, 260, 70)

	// Initial File Path
	initialPath := "actions.txt"
	if exePath, err := os.Executable(); err == nil {
		initialPath = filepath.Join(filepath.Dir(exePath), "actions.txt")
	}
	procSendMessageA.Call(uintptr(hwnd), WM_SETTEXT, 0, uintptr(unsafe.Pointer(StringToUTF16Ptr(initialPath))))

	// Timer to check file changes or process?
	// We will use a separate goroutine for reading the file.

	// Message Loop
	msg := struct {
		Hwnd    HWND
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      struct {
			X int32
			Y int32
		}
	}{}

	for {
		ret, _, _ := procGetMessageA.Call(
			uintptr(unsafe.Pointer(&msg)),
			0, 0, 0,
		)
		if ret <= 0 {
			break
		}

		// Handle Commands (Buttons)
		if msg.Message == WM_COMMAND {
			id := int(msg.WParam & 0xFFFF)
			if id == ID_BUTTON_START {
				startProcessing(hwnd)
			} else if id == ID_BUTTON_STOP {
				stopProcessing()
			}
		}

		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageA.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func startProcessing(hwnd HWND) {
	globalState.mu.Lock()
	if globalState.isActive {
		globalState.mu.Unlock()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	globalState.cancel = cancel
	globalState.isActive = true
	globalState.mu.Unlock()

	go func() {
		defer func() {
			globalState.mu.Lock()
			globalState.isActive = false
			globalState.mu.Unlock()
		}()

		// Read file path from the control
		// In a real app, we'd get the text from the edit control.
		// For simplicity, assume "actions.txt" in current dir or pass it via global.
		// Let's read the file "actions.txt" relative to exe.
		exePath, _ := os.Executable()
		filePath := filepath.Join(filepath.Dir(exePath), "actions.txt")

		file, err := os.Open(filePath)
		if err != nil {
			fmt.Printf("Error opening file: %v\n", err)
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		ticker := time.NewTicker(1 * time.Second) // Check every second
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Check if file has new content or just loop through lines?
				// Requirement: "reads a txt file for an action string"
				// Interpretation: If the file contains "action", press R.
				// Or: Read line by line.

				file.Seek(0, 0)
				scanner = bufio.NewScanner(file)
				foundAction := false

				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if strings.EqualFold(line, "action") || line == "r" || line == "press r" {
						foundAction = true
						break
					}
				}

				if foundAction {
					sendKeyR()
					time.Sleep(200 * time.Millisecond) // Debounce
				}
			}
		}
	}()
}

func stopProcessing() {
	globalState.mu.Lock()
	if !globalState.isActive {
		globalState.mu.Unlock()
		return
	}
	if globalState.cancel != nil {
		globalState.cancel()
	}
	globalState.isActive = false
	globalState.mu.Unlock()
}
