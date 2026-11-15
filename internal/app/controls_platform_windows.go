//go:build windows

package app

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Journal timestamps can lag the running client or contain a previous
// session. Query the process table for every guarded write and fail closed.
func eliteMayBeRunning() bool {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return true
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return true
	}
	for {
		switch strings.ToLower(windows.UTF16ToString(entry.ExeFile[:])) {
		case "elitedangerous64.exe", "elitedangerous32.exe", "elitedangerous.exe":
			return true
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			return !errors.Is(err, windows.ERROR_NO_MORE_FILES)
		}
	}
}

var steamLibraryPath = regexp.MustCompile(`(?m)^\s*"path"\s+"([^"\r\n]+)"`)

func eliteControlSchemes() string {
	var candidates []string
	if key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE); err == nil {
		steam, _, _ := key.GetStringValue("SteamPath")
		key.Close()
		if steam != "" {
			f, err := os.Open(filepath.Join(steam, "steamapps", "libraryfolders.vdf"))
			libraries := []string{steam}
			if err == nil {
				body, _ := io.ReadAll(io.LimitReader(f, 1<<20))
				f.Close()
				for _, path := range steamLibraryPath.FindAllStringSubmatch(string(body), -1) {
					libraries = append(libraries, strings.ReplaceAll(path[1], `\\`, `\`))
				}
			}
			for _, library := range libraries {
				candidates = append(candidates, filepath.Join(library, "steamapps", "common", "Elite Dangerous", "Products"))
			}
		}
	}
	for _, path := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
		if path != "" {
			candidates = append(candidates,
				filepath.Join(path, "Frontier", "EDLaunch", "Products"),
				filepath.Join(path, "Frontier Developments", "Elite Dangerous", "Products"))
		}
	}
	for _, root := range candidates {
		for _, build := range []string{"elite-dangerous-odyssey-64", "elite-dangerous-64", "FORC-FDEV-D-1010"} {
			path := filepath.Join(root, build, "ControlSchemes")
			if info, err := os.Stat(filepath.Join(path, "ConsoleX360.binds")); err == nil && info.Mode().IsRegular() {
				return path
			}
		}
	}
	return ""
}
