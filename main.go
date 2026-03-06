package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func init() {
	runtime.LockOSThread()
}

const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_TABSTOP          = 0x00010000
	WS_VSCROLL          = 0x00200000
	ES_MULTILINE        = 0x0004
	ES_AUTOVSCROLL      = 0x0040
	ES_READONLY         = 0x0800
	WS_EX_CLIENTEDGE    = 0x00000200

	WM_DESTROY = 0x0002
	WM_COMMAND = 0x0111
	WM_SETTEXT = 0x000C
	WM_GETTEXT = 0x000D
	WM_TIMER   = 0x0113
	WM_CREATE  = 0x0001
	WM_KEYDOWN = 0x0100
	WM_KEYUP   = 0x0101

	EM_SETSEL     = 0x00B1
	EM_REPLACESEL = 0x00C2
	EM_SCROLL     = 0x00B5
	SB_BOTTOM     = 7

	SW_SHOWNORMAL = 1

	ID_BUTTON_START    = 1001
	ID_BUTTON_BROWSE   = 1003
	ID_BUTTON_CLEARLOG = 1004
	ID_EDIT_PATH       = 2002
	ID_LOG_BOX         = 2003
	ID_PROCESS_LABEL   = 2004

	CS_VREDRAW = 0x0001
	CS_HREDRAW = 0x0002

	OFN_FILEMUSTEXIST = 0x00001000
	OFN_PATHMUSTEXIST = 0x00000800
	OFN_HIDEREADONLY  = 0x00000004
	OFN_EXPLORER      = 0x00080000

	EN_CHANGE = 0x0300

	TIMER_ID         = 42
	TIMER_PROCESS_ID = 43

	COINIT_APARTMENTTHREADED = 0x2

	TH32CS_SNAPPROCESS = 0x00000002

	VK_R = 0x52

	SW_RESTORE = 9

	// Target process name — only this exe is considered valid
	ARCHEAGE_EXE = "archeage.exe"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	comdlg32 = windows.NewLazySystemDLL("comdlg32.dll")
	ole32    = windows.NewLazySystemDLL("ole32.dll")

	procCreateWindowExW          = user32.NewProc("CreateWindowExW")
	procRegisterClassExW         = user32.NewProc("RegisterClassExW")
	procDefWindowProcW           = user32.NewProc("DefWindowProcW")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procPostQuitMessage          = user32.NewProc("PostQuitMessage")
	procSendMessageW             = user32.NewProc("SendMessageW")
	procShowWindow               = user32.NewProc("ShowWindow")
	procUpdateWindow             = user32.NewProc("UpdateWindow")
	procGetModuleHandleW         = kernel32.NewProc("GetModuleHandleW")
	procGetOpenFileNameW         = comdlg32.NewProc("GetOpenFileNameW")
	procCommDlgExtError          = comdlg32.NewProc("CommDlgExtendedError")
	procEnableWindow             = user32.NewProc("EnableWindow")
	procSetTimer                 = user32.NewProc("SetTimer")
	procKillTimer                = user32.NewProc("KillTimer")
	procCoInitializeEx           = ole32.NewProc("CoInitializeEx")
	procCoUninitialize           = ole32.NewProc("CoUninitialize")
	shell32                      = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW            = shell32.NewProc("ShellExecuteW")
	procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = kernel32.NewProc("Process32FirstW")
	procProcess32NextW           = kernel32.NewProc("Process32NextW")
	procCloseHandle              = kernel32.NewProc("CloseHandle")
	procFindWindowW              = user32.NewProc("FindWindowW")
	procPostMessageW             = user32.NewProc("PostMessageW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
)

type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type OPENFILENAME struct {
	LStructSize       uint32
	HwndOwner         uintptr
	HInstance         uintptr
	LpstrFilter       *uint16
	LpstrCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilerIndex       uint32
	LpstrFile         *uint16
	NMaxFile          uint32
	LpstrFileTitle    *uint16
	NMaxFileTitle     uint32
	LpstrInitialDir   *uint16
	LpstrTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LpstrDefExt       *uint16
	LCustData         uintptr
	LpfnHook          uintptr
	LpTemplateName    *uint16
	PvReserved        uintptr
	DwReserved        uint32
	FlagsEx           uint32
}

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type PROCESSENTRY32W struct {
	DwSize              uint32
	CntUsage            uint32
	Th32ProcessID       uint32
	Th32DefaultHeapID   uintptr
	Th32ModuleID        uint32
	CntThreads          uint32
	Th32ParentProcessID uint32
	PcPriClassBase      int32
	DwFlags             uint32
	SzExeFile           [260]uint16
}

// KEYBDINPUT / rawInput / makeKeyInput removed — using PostMessage instead

type Window struct {
	hwnd            uintptr
	filePath        string
	logHwnd         uintptr
	pathEditHwnd    uintptr
	startBtnHwnd    uintptr
	browseBtnHwnd   uintptr
	clearLogBtnHwnd uintptr
	processLblHwnd  uintptr
	instance        uintptr
	isRunning       bool
	gameRunning     bool
}

var gw *Window

var wndProcCallback uintptr

func init() {
	wndProcCallback = windows.NewCallback(wndProc)
}

// isArcheAgeRunning checks strictly for "archeage.exe" by exact process name.
func isArcheAgeRunning() bool {
	hSnap, _, _ := procCreateToolhelp32Snapshot.Call(TH32CS_SNAPPROCESS, 0)
	if hSnap == ^uintptr(0) {
		logLine("ERROR: CreateToolhelp32Snapshot failed.")
		return false
	}
	defer procCloseHandle.Call(hSnap)

	var pe PROCESSENTRY32W
	pe.DwSize = uint32(unsafe.Sizeof(pe))

	ret, _, _ := procProcess32FirstW.Call(hSnap, uintptr(unsafe.Pointer(&pe)))
	for ret != 0 {
		name := strings.ToLower(windows.UTF16ToString(pe.SzExeFile[:]))
		if name == ARCHEAGE_EXE {
			return true
		}
		ret, _, _ = procProcess32NextW.Call(hSnap, uintptr(unsafe.Pointer(&pe)))
	}
	return false
}

// isProcessRunning kept for generic use (e.g. dump), but game detection uses isArcheAgeRunning.
func isProcessRunning(substr string) bool {
	hSnap, _, _ := procCreateToolhelp32Snapshot.Call(TH32CS_SNAPPROCESS, 0)
	if hSnap == ^uintptr(0) {
		logLine("ERROR: CreateToolhelp32Snapshot failed.")
		return false
	}
	defer procCloseHandle.Call(hSnap)

	var pe PROCESSENTRY32W
	pe.DwSize = uint32(unsafe.Sizeof(pe))

	ret, _, _ := procProcess32FirstW.Call(hSnap, uintptr(unsafe.Pointer(&pe)))
	for ret != 0 {
		name := strings.ToLower(windows.UTF16ToString(pe.SzExeFile[:]))
		if strings.Contains(name, substr) {
			return true
		}
		ret, _, _ = procProcess32NextW.Call(hSnap, uintptr(unsafe.Pointer(&pe)))
	}
	return false
}

// checkProcess now uses isArcheAgeRunning (exact match on archeage.exe) instead of
// FindWindowW by class name, so it will not trigger on unrelated processes.
func checkProcess() {
	running := isArcheAgeRunning()
	if running == gw.gameRunning {
		return
	}
	gw.gameRunning = running

	if running {
		setCtrlText(gw.processLblHwnd, "ArcheAge: ● Running")
		logLine("ArcheAge process detected (archeage.exe).")
		enableCtrl(gw.browseBtnHwnd, true)
		enableCtrl(gw.startBtnHwnd, gw.filePath != "")
	} else {
		setCtrlText(gw.processLblHwnd, "ArcheAge: ○ Not Running")
		logLine("ArcheAge process not found.")
		if gw.isRunning {
			doStop()
		}
		enableCtrl(gw.startBtnHwnd, false)
		enableCtrl(gw.browseBtnHwnd, false)
	}
}

var (
	procEnumWindows      = user32.NewProc("EnumWindows")
	procGetWindowTextW   = user32.NewProc("GetWindowTextW")
	procGetWindowLongW   = user32.NewProc("GetWindowLongW")
	procGetClassNameW    = user32.NewProc("GetClassNameW")
	procEnumChildWindows = user32.NewProc("EnumChildWindows")
)

var foundHwnd uintptr

var enumTopCallback uintptr

func init() {
	enumTopCallback = windows.NewCallback(func(hwnd, lParam uintptr) uintptr {
		buf := make([]uint16, 512)
		procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), 512)
		title := strings.ToLower(windows.UTF16ToString(buf))
		search := strings.ToLower(windows.UTF16ToString((*[256]uint16)(unsafe.Pointer(lParam))[:]))
		if strings.Contains(title, search) {
			foundHwnd = hwnd
			return 0
		}
		return 1
	})
}

var enumChildCallback uintptr
var foundChildHwnd uintptr

func init() {
	enumChildCallback = windows.NewCallback(func(hwnd, lParam uintptr) uintptr {
		style, _, _ := procGetWindowLongW.Call(hwnd, uintptr(0xFFFFFFF0))
		if style&WS_VISIBLE == 0 {
			return 1
		}
		buf := make([]uint16, 256)
		procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), 256)
		class := strings.ToLower(windows.UTF16ToString(buf))
		logLine("  child hwnd=0x%X class=%s", hwnd, class)

		if foundChildHwnd == 0 {
			foundChildHwnd = hwnd
		}
		return 1
	})
}

var allTopWindowsCallback uintptr

func init() {
	allTopWindowsCallback = windows.NewCallback(func(hwnd, lParam uintptr) uintptr {
		titleBuf := make([]uint16, 256)
		procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&titleBuf[0])), 256)
		title := windows.UTF16ToString(titleBuf)

		classBuf := make([]uint16, 256)
		procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&classBuf[0])), 256)
		class := windows.UTF16ToString(classBuf)

		if title != "" {
			logLine("  TOP hwnd=0x%X class=%q title=%q", hwnd, class, title)
		}
		return 1
	})
}

// findArcheAgeWindow looks up the window belonging to archeage.exe specifically.
// It first snapshots processes to get the PID of archeage.exe, then finds the
// top-level window owned by that PID via EnumWindows.
func findArcheAgeWindow() uintptr {
	// 1. Find PID of archeage.exe
	hSnap, _, _ := procCreateToolhelp32Snapshot.Call(TH32CS_SNAPPROCESS, 0)
	if hSnap == ^uintptr(0) {
		logLine("ERROR: CreateToolhelp32Snapshot failed.")
		return 0
	}
	defer procCloseHandle.Call(hSnap)

	var pe PROCESSENTRY32W
	pe.DwSize = uint32(unsafe.Sizeof(pe))
	var archeagePID uint32

	ret, _, _ := procProcess32FirstW.Call(hSnap, uintptr(unsafe.Pointer(&pe)))
	for ret != 0 {
		name := strings.ToLower(windows.UTF16ToString(pe.SzExeFile[:]))
		if name == ARCHEAGE_EXE {
			archeagePID = pe.Th32ProcessID
			break
		}
		ret, _, _ = procProcess32NextW.Call(hSnap, uintptr(unsafe.Pointer(&pe)))
	}

	if archeagePID == 0 {
		logLine("archeage.exe not found in process list.")
		return 0
	}
	logLine("Found archeage.exe PID=%d, searching for its window...", archeagePID)

	// 2. Find the main window that belongs to that PID
	type searchData struct {
		pid  uint32
		hwnd uintptr
	}
	sd := searchData{pid: archeagePID}

	cb := windows.NewCallback(func(hwnd, lParam uintptr) uintptr {
		data := (*searchData)(unsafe.Pointer(lParam))
		var pid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == data.pid {
			// Only pick visible top-level windows
			style, _, _ := procGetWindowLongW.Call(hwnd, uintptr(0xFFFFFFF0))
			if style&WS_VISIBLE != 0 {
				data.hwnd = hwnd
				return 0 // stop enumeration
			}
		}
		return 1
	})

	procEnumWindows.Call(cb, uintptr(unsafe.Pointer(&sd)))

	if sd.hwnd == 0 {
		logLine("No visible window found for archeage.exe (PID=%d).", archeagePID)
		return 0
	}
	logLine("Found ArcheAge window hwnd=0x%X for PID=%d.", sd.hwnd, archeagePID)
	return sd.hwnd
}

// postKeyToWindow sends WM_KEYDOWN + WM_KEYUP directly to the target window
// handle via PostMessage — no focus stealing required.
func postKeyToWindow(hwnd uintptr, vk uintptr) {
	procMapVirtualKeyW := user32.NewProc("MapVirtualKeyW")
	scan, _, _ := procMapVirtualKeyW.Call(vk, 0) // MAPVK_VK_TO_VSC

	lParamDown := uintptr(1) | (scan << 16)
	lParamUp := uintptr(1) | (scan << 16) | (1 << 30) | (1 << 31)

	procPostMessageW.Call(hwnd, WM_KEYDOWN, vk, lParamDown)
	time.Sleep(50 * time.Millisecond)
	procPostMessageW.Call(hwnd, WM_KEYUP, vk, lParamUp)
}

func sendRToArcheAge() {
	if !isArcheAgeRunning() {
		logLine("WARNING: archeage.exe is not running. Skipping R key send.")
		return
	}

	gameHwnd := findArcheAgeWindow()
	if gameHwnd == 0 {
		logLine("WARNING: ArcheAge window not found.")
		return
	}

	go func() {
		logLine("Sending R via PostMessage to hwnd=0x%X (archeage.exe).", gameHwnd)
		postKeyToWindow(gameHwnd, VK_R)
		logLine("R sent successfully via PostMessage.")
	}()
}

func logLine(format string, args ...interface{}) {
	if gw == nil || gw.logHwnd == 0 {
		return
	}
	msg := fmt.Sprintf("[%s] %s\r\n",
		time.Now().Format("15:04:05"),
		fmt.Sprintf(format, args...))
	ptr := windows.StringToUTF16Ptr(msg)
	procSendMessageW.Call(gw.logHwnd, EM_SETSEL, ^uintptr(0), ^uintptr(0))
	procSendMessageW.Call(gw.logHwnd, EM_REPLACESEL, 0, uintptr(unsafe.Pointer(ptr)))
	procSendMessageW.Call(gw.logHwnd, EM_SCROLL, SB_BOTTOM, 0)
}

func setCtrlText(hwnd uintptr, text string) {
	procSendMessageW.Call(hwnd, WM_SETTEXT, 0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(text))))
}

func getCtrlText(hwnd uintptr) string {
	buf := make([]uint16, 1024)
	procSendMessageW.Call(hwnd, WM_GETTEXT,
		uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	return windows.UTF16ToString(buf)
}

func enableCtrl(hwnd uintptr, enable bool) {
	v := uintptr(0)
	if enable {
		v = 1
	}
	procEnableWindow.Call(hwnd, v)
}

func scanFile() {
	data, err := os.ReadFile(gw.filePath)
	if err != nil {
		logLine("ERROR reading file: %v", err)
		doStop()
		return
	}

	text := strings.ToLower(strings.TrimSpace(string(data)))

	actionPending := false
	stopFound := false

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "action:1" {
			actionPending = true
		}
		if line == "stop" {
			stopFound = true
		}
	}

	if actionPending {
		logLine("Action detected (action:1) — sending R.")
		sendRToArcheAge()
		// Reset flag to 0 so Lua knows the action was consumed
		if err := os.WriteFile(gw.filePath, []byte("action:0"), 0644); err != nil {
			logLine("WARNING: could not reset action flag: %v", err)
		} else {
			logLine("Action flag reset to action:0.")
		}
	}

	if stopFound {
		doStop()
	}
}

func doStart() {
	if !gw.gameRunning {
		logLine("Cannot start — ArcheAge (archeage.exe) is not running.")
		return
	}
	logLine("Starting scan. File: %s", gw.filePath)
	gw.isRunning = true
	setCtrlText(gw.startBtnHwnd, "Stop")
	procSetTimer.Call(gw.hwnd, TIMER_ID, 1000, 0)
}

func doStop() {
	procKillTimer.Call(gw.hwnd, TIMER_ID)
	gw.isRunning = false
	setCtrlText(gw.startBtnHwnd, "Start")
	logLine("Scan stopped.")
}

func openFileDialog(owner uintptr) string {
	logLine("Opening file dialog...")

	exe, err := os.Executable()
	if err != nil {
		exe, _ = os.Getwd()
	} else {
		exe = filepath.Dir(exe)
	}

	buf := make([]uint16, windows.MAX_PATH)
	filter := utf16DoubleNull("Text Files", "*.txt", "All Files", "*.*")

	ofn := OPENFILENAME{}
	ofn.LStructSize = uint32(unsafe.Sizeof(ofn))
	ofn.HwndOwner = owner
	ofn.LpstrFilter = &filter[0]
	ofn.LpstrFile = &buf[0]
	ofn.NMaxFile = uint32(len(buf))
	ofn.LpstrInitialDir = windows.StringToUTF16Ptr(exe)
	ofn.LpstrTitle = windows.StringToUTF16Ptr("Select actions file")
	ofn.Flags = OFN_FILEMUSTEXIST | OFN_PATHMUSTEXIST | OFN_HIDEREADONLY | OFN_EXPLORER

	logLine("Calling GetOpenFileNameW (struct=%d bytes)...", ofn.LStructSize)

	ret, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		code, _, _ := procCommDlgExtError.Call()
		if code == 0 {
			logLine("Dialog cancelled.")
		} else {
			logLine("Dialog FAILED — CommDlgExtendedError: 0x%08X", code)
		}
		return ""
	}

	result := windows.UTF16ToString(buf)
	logLine("Selected: %s", result)
	return result
}

func utf16DoubleNull(pairs ...string) []uint16 {
	var out []uint16
	for _, s := range pairs {
		out = append(out, windows.StringToUTF16(s)...)
	}
	out = append(out, 0)
	return out
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {

	case WM_CREATE:
		buildControls(hwnd)
		procSetTimer.Call(hwnd, TIMER_PROCESS_ID, 2000, 0)
		return 0

	case WM_TIMER:
		switch wParam {
		case TIMER_ID:
			scanFile()
		case TIMER_PROCESS_ID:
			checkProcess()
		}
		return 0

	case WM_COMMAND:
		id := wParam & 0xFFFF
		notif := (wParam >> 16) & 0xFFFF

		switch id {
		case ID_BUTTON_START:
			if gw.isRunning {
				doStop()
			} else {
				doStart()
			}

		case ID_BUTTON_CLEARLOG:
			setCtrlText(gw.logHwnd, "")
			logLine("Log cleared.")

		case ID_BUTTON_BROWSE:
			logLine("Browse clicked.")
			path := openFileDialog(hwnd)
			if path != "" {
				gw.filePath = path
				setCtrlText(gw.pathEditHwnd, path)
				if gw.gameRunning {
					enableCtrl(gw.startBtnHwnd, true)
				}
				logLine("File set.")
			}

		case ID_EDIT_PATH:
			if notif == EN_CHANGE {
				text := strings.TrimSpace(getCtrlText(gw.pathEditHwnd))
				gw.filePath = text
				enableCtrl(gw.startBtnHwnd, text != "" && gw.gameRunning)
			}
		}
		return 0

	case WM_DESTROY:
		logLine("Closing.")
		procKillTimer.Call(hwnd, TIMER_ID)
		procKillTimer.Call(hwnd, TIMER_PROCESS_ID)
		procPostQuitMessage.Call(0)
		return 0
	}

	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func buildControls(hwnd uintptr) {
	inst := gw.instance

	newLabel(hwnd, 12, 14, 68, 22, "File path:")
	gw.pathEditHwnd = newEdit(hwnd, 82, 12, 300, 22, "")
	gw.browseBtnHwnd = newButton(hwnd, inst, 390, 10, 128, 26, "Browse...", ID_BUTTON_BROWSE)

	gw.processLblHwnd = newLabel(hwnd, 12, 52, 240, 22, "ArcheAge: ○ Checking...")
	gw.startBtnHwnd = newButton(hwnd, inst, 260, 48, 100, 28, "Start", ID_BUTTON_START)
	gw.clearLogBtnHwnd = newButton(hwnd, inst, 368, 48, 150, 28, "Clear Log", ID_BUTTON_CLEARLOG)
	enableCtrl(gw.startBtnHwnd, false)
	enableCtrl(gw.browseBtnHwnd, false)

	newLabel(hwnd, 12, 88, 100, 18, "Debug log:")
	gw.logHwnd = newLogBox(hwnd, inst, 12, 108, 506, 300)
}

func isElevated() bool {
	token := windows.Token(0)
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}

func relaunchAsAdmin() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	exePtr := windows.StringToUTF16Ptr(exe)
	verbPtr := windows.StringToUTF16Ptr("runas")
	procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(exePtr)), 0, 0, SW_SHOWNORMAL)
	os.Exit(0)
}

func main() {
	if !isElevated() {
		relaunchAsAdmin()
		return
	}

	hr, _, _ := procCoInitializeEx.Call(0, COINIT_APARTMENTTHREADED)
	if hr != 0 && hr != 1 {
		fmt.Printf("CoInitializeEx failed: 0x%08X\n", hr)
	}
	defer procCoUninitialize.Call()

	instance, _, _ := procGetModuleHandleW.Call(0)
	gw = &Window{instance: instance}

	className := windows.StringToUTF16Ptr("NehaFishingTool")

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		Style:         CS_VREDRAW | CS_HREDRAW,
		LpfnWndProc:   wndProcCallback,
		HInstance:     instance,
		HbrBackground: 6,
		LpszClassName: className,
	}

	ret, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if ret == 0 {
		fmt.Printf("RegisterClassExW failed: %v\n", err)
		return
	}

	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("NehaFishing"))),
		WS_OVERLAPPEDWINDOW|WS_VISIBLE,
		200, 100, 560, 480,
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		fmt.Printf("CreateWindowExW failed: %v\n", err)
		return
	}
	gw.hwnd = hwnd

	procShowWindow.Call(hwnd, SW_SHOWNORMAL)
	procUpdateWindow.Call(hwnd)

	logLine("ArcheAgent started. Waiting for archeage.exe process...")

	var msg MSG
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || r == ^uintptr(0) {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func newLabel(parent uintptr, x, y, w, h int, text string) uintptr {
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("STATIC"))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(text))),
		WS_CHILD|WS_VISIBLE,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, 0, 0, 0,
	)
	return hwnd
}

func newEdit(parent uintptr, x, y, w, h int, text string) uintptr {
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("EDIT"))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(text))),
		WS_CHILD|WS_VISIBLE|WS_TABSTOP,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, ID_EDIT_PATH, 0, 0,
	)
	return hwnd
}

func newLogBox(parent, instance uintptr, x, y, w, h int) uintptr {
	hwnd, _, _ := procCreateWindowExW.Call(
		WS_EX_CLIENTEDGE,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("EDIT"))),
		0,
		WS_CHILD|WS_VISIBLE|WS_VSCROLL|ES_MULTILINE|ES_AUTOVSCROLL|ES_READONLY,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, ID_LOG_BOX, instance, 0,
	)
	return hwnd
}

func newButton(parent, instance uintptr, x, y, w, h int, label string, id uintptr) uintptr {
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("BUTTON"))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(label))),
		WS_CHILD|WS_VISIBLE|WS_TABSTOP,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, id, instance, 0,
	)
	return hwnd
}
