//go:build windows

package install

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TryElevate attempts to re-launch the current process with elevated
// (Administrator) privileges. It returns (true, nil) when it successfully
// re-launched an elevated child and the caller should exit. It returns
// (false, nil) when the current process is already elevated (no action taken).
//
// Elevation order:
//  1. gsudo  – third-party sudo for Windows; widely available via winget.
//  2. sudo   – built-in on Windows 11 24H2+.
//  3. UAC ShellExecuteExW "runas" – triggers the standard UAC prompt.
func TryElevate(args []string) (relaunched bool, err error) {
	if defaultIsAdmin() {
		return false, nil
	}

	self, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("resolve current executable: %w", err)
	}

	// 1. Try gsudo.
	if gsudo, lookErr := exec.LookPath("gsudo"); lookErr == nil {
		cmdArgs := append([]string{self}, args...)
		cmd := exec.Command(gsudo, cmdArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if runErr := cmd.Run(); runErr != nil {
			return false, fmt.Errorf("gsudo elevation failed: %w", runErr)
		}
		return true, nil
	}

	// 2. Try sudo (Windows 11 24H2+ built-in).
	if sudo, lookErr := exec.LookPath("sudo"); lookErr == nil {
		cmdArgs := append([]string{self}, args...)
		cmd := exec.Command(sudo, cmdArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if runErr := cmd.Run(); runErr != nil {
			return false, fmt.Errorf("sudo elevation failed: %w", runErr)
		}
		return true, nil
	}

	// 3. UAC ShellExecuteExW "runas" – opens the UAC prompt then waits for the
	// elevated child to finish.
	if uacErr := shellExecuteElevate(self, args); uacErr != nil {
		return false, uacErr
	}
	return true, nil
}

// shellExecuteInfoW mirrors the Windows SHELLEXECUTEINFOW structure.
// See: https://learn.microsoft.com/en-us/windows/win32/api/shellapi/ns-shellapi-shellexecuteinfow
type shellExecuteInfoW struct {
	CbSize     uint32
	Mask       uint32
	Hwnd       syscall.Handle
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	InstApp    syscall.Handle
	IDList     uintptr
	Class      *uint16
	HkeyClass  syscall.Handle
	HotKey     uint32
	_          [8]byte // union: hIcon / hMonitor / padding
	Process    syscall.Handle
}

const (
	seeMaskNocloseprocess = 0x00000040
	swShownormal          = 1
)

var (
	shell32         = windows.NewLazySystemDLL("shell32.dll")
	shellExecuteExW = shell32.NewProc("ShellExecuteExW")
)

// shellExecuteElevate triggers a UAC elevation prompt via ShellExecuteExW and
// waits for the elevated child process to finish.
func shellExecuteElevate(exe string, args []string) error {
	verbPtr, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return fmt.Errorf("encode verb: %w", err)
	}
	exePtr, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return fmt.Errorf("encode exe path: %w", err)
	}

	var paramsPtr *uint16
	if len(args) > 0 {
		p, pErr := windows.UTF16PtrFromString(buildParams(args))
		if pErr != nil {
			return fmt.Errorf("encode args: %w", pErr)
		}
		paramsPtr = p
	}

	info := shellExecuteInfoW{
		Mask:       seeMaskNocloseprocess,
		Hwnd:       0,
		Verb:       verbPtr,
		File:       exePtr,
		Parameters: paramsPtr,
		Show:       swShownormal,
	}
	info.CbSize = uint32(unsafe.Sizeof(info))

	ret, _, callErr := shellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		return fmt.Errorf("ShellExecuteExW failed: %w", callErr)
	}

	// Wait for the elevated child to exit.
	if info.Process != 0 {
		defer func() {
			_ = windows.CloseHandle(windows.Handle(info.Process))
		}()
		_, _ = windows.WaitForSingleObject(windows.Handle(info.Process), windows.INFINITE)
	}

	return nil
}

// buildParams converts a slice of arguments into a single command-line string
// compatible with ShellExecuteExW. Arguments that contain spaces are quoted.
func buildParams(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t") {
			quoted[i] = `"` + a + `"`
		} else {
			quoted[i] = a
		}
	}
	return strings.Join(quoted, " ")
}
