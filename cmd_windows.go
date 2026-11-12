//go:build windows
// +build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

const adbCommandTimeout = 12 * time.Second
const emulatorDiscoveryTimeout = 4 * time.Second

type mumuADBInfo struct {
	Host         string `json:"adb_host"`
	HostIP       string `json:"adb_host_ip"`
	Port         int    `json:"adb_port"`
	AndroidStart bool   `json:"is_android_started"`
	ProcessStart bool   `json:"is_process_started"`
}

func runWindowsToolCommand(path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), emulatorDiscoveryTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.CombinedOutput()
	text := strings.TrimRight(string(output), "\r\n")
	if ctx.Err() == context.DeadlineExceeded {
		return text, fmt.Errorf("emulator command timeout")
	}
	return text, err
}

func appendUniqueADBTarget(targets *[]string, seen map[string]struct{}, host string, port int) {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	if port <= 0 || port > 65535 {
		return
	}

	target := fmt.Sprintf("%s:%d", host, port)
	if _, exists := seen[target]; exists {
		return
	}
	seen[target] = struct{}{}
	*targets = append(*targets, target)
}

func findMuMuManagerPath() string {
	dir := filepath.Dir(adb)
	for i := 0; i < 7 && dir != "" && dir != filepath.Dir(dir); i++ {
		candidates := []string{
			filepath.Join(dir, "MuMuManager.exe"),
			filepath.Join(dir, "nx_main", "MuMuManager.exe"),
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

func discoverMuMuADBTargets() []string {
	managerPath := findMuMuManagerPath()
	if managerPath == "" {
		return nil
	}

	output, err := runWindowsToolCommand(managerPath, "info", "--vmindex", "all")
	if err != nil {
		log.Printf("[device] MuMuManager info failed: path=%q err=%v output=%q", managerPath, err, logPreview(output, 500))
		return nil
	}

	targets := make([]string, 0)
	seen := make(map[string]struct{})
	var players map[string]mumuADBInfo
	if json.Unmarshal([]byte(output), &players) == nil {
		keys := make([]string, 0, len(players))
		for key := range players {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			info := players[key]
			if !info.AndroidStart && !info.ProcessStart {
				continue
			}
			host := info.HostIP
			if host == "" {
				host = info.Host
			}
			appendUniqueADBTarget(&targets, seen, host, info.Port)
		}
	}

	if len(targets) == 0 {
		var info mumuADBInfo
		if json.Unmarshal([]byte(output), &info) == nil {
			host := info.HostIP
			if host == "" {
				host = info.Host
			}
			appendUniqueADBTarget(&targets, seen, host, info.Port)
		}
	}

	if len(targets) > 0 {
		log.Printf("[device] MuMuManager discovered adb targets: %v", targets)
	}
	return targets
}

func findLDConsolePath() string {
	for _, name := range []string{"ldconsole.exe", "dnconsole.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}

	dir := filepath.Dir(adb)
	for i := 0; i < 7 && dir != "" && dir != filepath.Dir(dir); i++ {
		for _, name := range []string{"ldconsole.exe", "dnconsole.exe"} {
			candidate := filepath.Join(dir, name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
		dir = filepath.Dir(dir)
	}

	processes, err := process.Processes()
	if err != nil {
		return ""
	}
	for _, p := range processes {
		name, nameErr := p.Name()
		if nameErr != nil {
			continue
		}
		lowerName := strings.ToLower(name)
		if lowerName != "dnplayer.exe" &&
			lowerName != "dnmultiplayer.exe" &&
			lowerName != "ldvboxheadless.exe" {
			continue
		}

		exePath, exeErr := p.Exe()
		if exeErr != nil || exePath == "" {
			continue
		}
		dir := filepath.Dir(exePath)
		for _, consoleName := range []string{"ldconsole.exe", "dnconsole.exe"} {
			candidate := filepath.Join(dir, consoleName)
			if _, statErr := os.Stat(candidate); statErr == nil {
				return candidate
			}
		}
	}
	return ""
}

func discoverLDPlayerADBTargets() []string {
	consolePath := findLDConsolePath()
	if consolePath == "" {
		return nil
	}

	output, err := runWindowsToolCommand(consolePath, "runninglist")
	if err != nil {
		log.Printf("[device] LDPlayer runninglist failed: path=%q err=%v output=%q", consolePath, err, logPreview(output, 500))
		return nil
	}

	targets := make([]string, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(output, "\n") {
		indexText := strings.TrimSpace(strings.SplitN(line, ",", 2)[0])
		index, parseErr := strconv.Atoi(indexText)
		if parseErr != nil || index < 0 {
			continue
		}

		appendUniqueADBTarget(&targets, seen, "127.0.0.1", 5555+index*2)
		appendUniqueADBTarget(&targets, seen, "127.0.0.1", 7555+index*2)
	}

	if len(targets) > 0 {
		log.Printf("[device] LDPlayer discovered adb targets: %v", targets)
	}
	return targets
}

func discoverEmulatorADBTargets() []string {
	targets := make([]string, 0)
	seen := make(map[string]struct{})

	for _, target := range discoverMuMuADBTargets() {
		if _, exists := seen[target]; !exists {
			seen[target] = struct{}{}
			targets = append(targets, target)
		}
	}
	for _, target := range discoverLDPlayerADBTargets() {
		if _, exists := seen[target]; !exists {
			seen[target] = struct{}{}
			targets = append(targets, target)
		}
	}

	for _, port := range []int{5555, 5557, 7555, 7557, 16384} {
		appendUniqueADBTarget(&targets, seen, "127.0.0.1", port)
	}
	return targets
}

func screenSizePixels() (int, int) {
	user32 := syscall.NewLazyDLL("user32.dll")
	getSystemMetrics := user32.NewProc("GetSystemMetrics")

	width, _, _ := getSystemMetrics.Call(0)  // SM_CXSCREEN
	height, _, _ := getSystemMetrics.Call(1) // SM_CYSCREEN
	return int(width), int(height)
}

func screenScale() float32 {
	user32 := syscall.NewLazyDLL("user32.dll")
	getDpiForSystem := user32.NewProc("GetDpiForSystem")

	dpi, _, _ := getDpiForSystem.Call()
	if dpi <= 0 {
		return 1
	}
	return float32(dpi) / 96
}

func adbExec(str ...string) string {
	output, err := runADBCommand(false, str...)
	if err != nil {
		return err.Error()
	}
	return output
}

func adbExecCombined(str ...string) (string, error) {
	return runADBCommand(true, str...)
}

func runADBCommand(combined bool, str ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), adbCommandTimeout)
	defer cancel()

	start := time.Now()
	log.Printf("[adb] start path=%q args=%q combined=%v", adb, str, combined)

	cmd := exec.CommandContext(ctx, adb, str...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	var output []byte
	var err error
	if combined {
		output, err = cmd.CombinedOutput()
	} else {
		output, err = cmd.Output()
	}

	text := strings.TrimRight(string(output), "\r\n")
	elapsed := time.Since(start)
	if ctx.Err() == context.DeadlineExceeded {
		log.Printf("[adb] timeout path=%q args=%q elapsed=%s output=%q", adb, str, elapsed, logPreview(text, 500))
		return text, fmt.Errorf("adb 命令超时（%s）", adbCommandTimeout)
	}
	if err != nil {
		log.Printf("[adb] failed path=%q args=%q elapsed=%s err=%v output=%q", adb, str, elapsed, err, logPreview(text, 500))
		return text, err
	}
	log.Printf("[adb] ok path=%q args=%q elapsed=%s output=%q", adb, str, elapsed, logPreview(text, 500))
	return text, nil
}

// 查找 ADB 可执行文件路径
func findADBPath() string {
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		localAdbPath := filepath.Join(exeDir, "adb.exe")
		if _, err := os.Stat(localAdbPath); err == nil {
			return localAdbPath
		}
	}

	if path, err := exec.LookPath("adb"); err == nil {
		return path
	}

	processes, err := process.Processes()
	if err == nil {
		for _, p := range processes {
			name, err := p.Name()
			if err != nil || strings.ToLower(name) != "adb.exe" {
				continue
			}
			path, err := p.Exe()
			if err == nil && path != "" {
				return path
			}
		}
	}

	return "adb"
}
