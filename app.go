package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

func init() {
	runtime.LockOSThread()
}

const (
	ACTION_FILENAME = "neha-fishing-actions.txt"
	LOG_FILENAME    = "neha-fishing-log.txt"
	ARCHEAGE_EXE    = "archeage.exe"

	WM_KEYDOWN         = 0x0100
	WM_KEYUP           = 0x0101
	WS_VISIBLE         = 0x10000000
	VK_R               = 0x52
	TH32CS_SNAPPROCESS = 0x00000002
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procPostMessageW             = user32.NewProc("PostMessageW")
	procMapVirtualKeyW           = user32.NewProc("MapVirtualKeyW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowLongW           = user32.NewProc("GetWindowLongW")
	procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = kernel32.NewProc("Process32FirstW")
	procProcess32NextW           = kernel32.NewProc("Process32NextW")
	procCloseHandle              = kernel32.NewProc("CloseHandle")
)

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

// FishingPayload matches the JSON the Lua addon writes.
type FishingPayload struct {
	Action    string `json:"action"`
	BuffID    string `json:"buff_id"`
	SlotKey   string `json:"slot_key"`
	AutoKey   string `json:"auto_key"`
	HasTarget bool   `json:"has_target"`
	FishSize  string `json:"fish_size"`
	Consumed  bool   `json:"consumed"`
}

// FolderStatus is returned by SetFolder and PickFolder so the frontend
// knows both the resolved path and whether each expected file exists.
type FolderStatus struct {
	Path          string `json:"path"`
	StatusMsg     string `json:"status_msg"`
	HasActionFile bool   `json:"has_action_file"`
	HasLogFile    bool   `json:"has_log_file"`
}

// App is the main application struct. Public methods are exposed to the frontend.
type App struct {
	ctx            context.Context
	folderPath     string
	actionFilePath string
	logFilePath    string
	lastLogSize    int64
	stopCh         chan struct{}
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	a.stopWatching()
}

// ── Public methods (auto-bound to JS by Wails) ────────────────────────────

// PickFolder opens a native Windows folder-picker dialog and, if the user
// selects a folder, calls SetFolder on it. Returns a FolderStatus so the
// frontend can update its UI in one round-trip. Returns an empty FolderStatus
// (Path == "") when the user cancels.
func (a *App) PickFolder() FolderStatus {
	defaultDir := a.folderPath
	if defaultDir == "" {
		defaultDir, _ = os.UserHomeDir()
	}

	chosen, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		DefaultDirectory:     defaultDir,
		Title:                "Select your ArcheAge addon folder",
		CanCreateDirectories: false,
	})
	if err != nil || chosen == "" {
		// User cancelled or error — return empty path so JS does nothing.
		return FolderStatus{}
	}

	return a.applyFolder(chosen)
}

// SetFolder validates the given folder path and starts watching it.
// Returns a FolderStatus with a human-readable status string.
func (a *App) SetFolder(path string) FolderStatus {
	path = strings.TrimSpace(strings.Trim(path, `"`))
	return a.applyFolder(path)
}

// applyFolder is the shared implementation used by both SetFolder and PickFolder.
func (a *App) applyFolder(path string) FolderStatus {
	a.stopWatching()

	if path == "" {
		a.folderPath = ""
		return FolderStatus{Path: "", StatusMsg: "No folder set"}
	}

	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return FolderStatus{Path: path, StatusMsg: "❌ Folder not found: " + path}
	}

	a.folderPath = path
	a.actionFilePath = filepath.Join(path, ACTION_FILENAME)
	a.logFilePath = filepath.Join(path, LOG_FILENAME)
	a.lastLogSize = 0

	_, errA := os.Stat(a.actionFilePath)
	_, errL := os.Stat(a.logFilePath)

	hasAction := errA == nil
	hasLog := errL == nil

	var missing []string
	if !hasAction {
		missing = append(missing, ACTION_FILENAME)
	}
	if !hasLog {
		missing = append(missing, LOG_FILENAME)
	}

	a.startWatching()

	var msg string
	if len(missing) > 0 {
		msg = "⏳ Watching — waiting for: " + strings.Join(missing, ", ") + " (load into game first)"
	} else {
		msg = "✅ Watching: " + path
	}

	return FolderStatus{
		Path:          path,
		StatusMsg:     msg,
		HasActionFile: hasAction,
		HasLogFile:    hasLog,
	}
}

// GetArcheAgeStatus returns whether archeage.exe is currently running.
func (a *App) GetArcheAgeStatus() bool {
	return isArcheAgeRunning()
}

// OpenExplorer opens Windows Explorer at the currently set folder (or home dir).
func (a *App) OpenExplorer() {
	target := a.folderPath
	if target == "" {
		target, _ = os.UserHomeDir()
	}
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	proc := shell32.NewProc("ShellExecuteW")
	proc.Call(
		0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("explore"))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(target))),
		0, 0, 5,
	)
}

// ── Internal watch loop ───────────────────────────────────────────────────

func (a *App) startWatching() {
	a.stopCh = make(chan struct{})
	ch := a.stopCh

	// Scan action file every second
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ch:
				return
			case <-t.C:
				if msg := a.scanAction(); msg != "" {
					wailsRuntime.EventsEmit(a.ctx, "scan:result", msg)
				}
				wailsRuntime.EventsEmit(a.ctx, "process:status", isArcheAgeRunning())
			}
		}
	}()

	// Tail log file every 500ms
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ch:
				return
			case <-t.C:
				if lines := a.tailLog(); len(lines) > 0 {
					wailsRuntime.EventsEmit(a.ctx, "log:lines", lines)
				}
			}
		}
	}()
}

func (a *App) stopWatching() {
	if a.stopCh != nil {
		close(a.stopCh)
		a.stopCh = nil
	}
}

func (a *App) scanAction() string {
	if a.actionFilePath == "" {
		return ""
	}
	data, err := os.ReadFile(a.actionFilePath)
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return ""
	}

	var p FishingPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return fmt.Sprintf("JSON error: %v", err)
	}

	if p.Action != "fish_detected" || p.Consumed {
		return ""
	}

	size := p.FishSize
	if size == "" {
		size = "unknown"
	}
	msg := fmt.Sprintf("🐟 Fish! buff=%s slot=%s size=%s — sending R", p.BuffID, p.SlotKey, size)

	go sendRToArcheAge()

	p.Consumed = true
	if out, err := json.Marshal(p); err == nil {
		os.WriteFile(a.actionFilePath, out, 0644)
	}
	return msg
}

func (a *App) tailLog() []string {
	if a.logFilePath == "" {
		return nil
	}
	f, err := os.Open(a.logFilePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	info, _ := f.Stat()
	size := info.Size()
	if size < a.lastLogSize {
		a.lastLogSize = 0
	}
	if size == a.lastLogSize {
		return nil
	}

	f.Seek(a.lastLogSize, 0)
	buf := make([]byte, size-a.lastLogSize)
	n, _ := f.Read(buf)
	a.lastLogSize += int64(n)

	var lines []string
	for _, line := range strings.Split(string(buf[:n]), "\n") {
		line = strings.TrimRight(line, "\r")
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// ── Win32 key sending ─────────────────────────────────────────────────────

func isArcheAgeRunning() bool {
	hSnap, _, _ := procCreateToolhelp32Snapshot.Call(TH32CS_SNAPPROCESS, 0)
	if hSnap == ^uintptr(0) {
		return false
	}
	defer procCloseHandle.Call(hSnap)
	var pe PROCESSENTRY32W
	pe.DwSize = uint32(unsafe.Sizeof(pe))
	ret, _, _ := procProcess32FirstW.Call(hSnap, uintptr(unsafe.Pointer(&pe)))
	for ret != 0 {
		if strings.ToLower(windows.UTF16ToString(pe.SzExeFile[:])) == ARCHEAGE_EXE {
			return true
		}
		ret, _, _ = procProcess32NextW.Call(hSnap, uintptr(unsafe.Pointer(&pe)))
	}
	return false
}

func findArcheAgeWindow() uintptr {
	hSnap, _, _ := procCreateToolhelp32Snapshot.Call(TH32CS_SNAPPROCESS, 0)
	if hSnap == ^uintptr(0) {
		return 0
	}
	defer procCloseHandle.Call(hSnap)
	var pe PROCESSENTRY32W
	pe.DwSize = uint32(unsafe.Sizeof(pe))
	var pid uint32
	ret, _, _ := procProcess32FirstW.Call(hSnap, uintptr(unsafe.Pointer(&pe)))
	for ret != 0 {
		if strings.ToLower(windows.UTF16ToString(pe.SzExeFile[:])) == ARCHEAGE_EXE {
			pid = pe.Th32ProcessID
			break
		}
		ret, _, _ = procProcess32NextW.Call(hSnap, uintptr(unsafe.Pointer(&pe)))
	}
	if pid == 0 {
		return 0
	}

	type sd struct {
		pid  uint32
		hwnd uintptr
	}
	data := sd{pid: pid}
	cb := windows.NewCallback(func(hwnd, lParam uintptr) uintptr {
		d := (*sd)(unsafe.Pointer(lParam))
		var wpid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&wpid)))
		if wpid == d.pid {
			style, _, _ := procGetWindowLongW.Call(hwnd, uintptr(0xFFFFFFF0))
			if style&WS_VISIBLE != 0 {
				d.hwnd = hwnd
				return 0
			}
		}
		return 1
	})
	procEnumWindows.Call(cb, uintptr(unsafe.Pointer(&data)))
	return data.hwnd
}

func sendRToArcheAge() {
	hwnd := findArcheAgeWindow()
	if hwnd == 0 {
		return
	}
	scan, _, _ := procMapVirtualKeyW.Call(VK_R, 0)
	lDown := uintptr(1) | (scan << 16)
	lUp := uintptr(1) | (scan << 16) | (1 << 30) | (1 << 31)
	procPostMessageW.Call(hwnd, WM_KEYDOWN, VK_R, lDown)
	time.Sleep(50 * time.Millisecond)
	procPostMessageW.Call(hwnd, WM_KEYUP, VK_R, lUp)
}
