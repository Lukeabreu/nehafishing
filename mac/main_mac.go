package mac

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var globalState = struct {
	mu       sync.Mutex
	isActive bool
	cancel   context.CancelFunc
}{isActive: false}

func sendKeyR() {
	scriptContent := `
	tell application "System Events"
		keystroke "r"
	end tell
	`

	tmpFile, err := os.CreateTemp("", "send_key_*.scpt")
	if err != nil {
		fmt.Printf("❌ Failed to create temp script: %v\n", err)
		return
	}
	defer os.Remove(tmpFile.Name()) // Clean up after execution

	if _, err := tmpFile.WriteString(scriptContent); err != nil {
		fmt.Printf("❌ Failed to write script: %v\n", err)
		return
	}
	tmpFile.Close()

	// Run the script file directly
	cmd := exec.Command("osascript", tmpFile.Name())
	output, err := cmd.CombinedOutput()

	if err != nil {
		fmt.Printf("❌ AppleScript Execution Failed: %v\n", err)
		fmt.Printf("   Output/Debug Info: %s\n", string(output))
		return
	}

	fmt.Println("✅ Key 'R' sent successfully.")
}

func main() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	defaultFile := filepath.Join(exeDir, "actions.txt")

	fmt.Println("=== ArcheAgent macOS Test ===")
	fmt.Printf("📂 Looking for file at: %s\n", defaultFile)

	if _, err := os.Stat(defaultFile); os.IsNotExist(err) {
		os.WriteFile(defaultFile, []byte("action"), 0644)
		fmt.Println("📝 Created default actions.txt with 'action'.")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		startProcessingWorker(defaultFile)
	}()

	select {
	case <-done:
		fmt.Println("🛑 Worker finished. Exiting gracefully.")
	case <-time.After(60 * time.Second):
		fmt.Println("⏱️ Timeout reached after 60 seconds.")
	}
}

func startProcessingWorker(filePath string) {
	globalState.mu.Lock()
	if globalState.isActive {
		globalState.mu.Unlock()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	globalState.cancel = cancel
	globalState.isActive = true
	globalState.mu.Unlock()

	defer func() {
		globalState.mu.Lock()
		globalState.isActive = false
		globalState.mu.Unlock()
	}()

	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("❌ Error opening file: %v\n", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("🛑 Process cancelled.")
			return
		case <-ticker.C:
			file.Seek(0, 0)
			scanner = bufio.NewScanner(file)

			foundAction := false
			foundStop := false

			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				lowerLine := strings.ToLower(line)

				if lowerLine == "action" || lowerLine == "r" {
					foundAction = true
				}
				if lowerLine == "stop" {
					foundStop = true
					break
				}
			}

			if foundStop {
				fmt.Println("🛑 STOP command detected! Disabling controls...")
				return
			}

			if foundAction {
				sendKeyR()
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
}
