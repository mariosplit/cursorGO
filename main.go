//CLI explorer for win; a dir will be created when changing directory(cd) you enter path to non existent dir
package main

import (
	"fmt"
	"os"
	"git.com/mariosplit/CursurGO/cmd"
	"log/slog"
	"syscall"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	procAllocConsole = kernel32.NewProc("AllocConsole")
)

func main() {
	// Allocate a console if one doesn't exist
	r, _, _ := procAllocConsole.Call()
	if r == 0 {
		os.Stderr.WriteString("Failed to allocate console\n")
		return
	}

	// Redirect standard input, output, and error to the new console
	os.Stdin = os.NewFile(uintptr(syscall.STD_INPUT_HANDLE), "/dev/stdin")
	os.Stdout = os.NewFile(uintptr(syscall.STD_OUTPUT_HANDLE), "/dev/stdout")
	os.Stderr = os.NewFile(uintptr(syscall.STD_ERROR_HANDLE), "/dev/stderr")

	// Set console title
	setConsoleTitle("CursorGO File Explorer")

	// Rest of your main function
	defer cmd.LogFile.Close()
	if err := cmd.Execute(); err != nil {
		slog.Error("Failed to execute command", "error", err)
		fmt.Println(err)
		os.Exit(1)
	}
}

func setConsoleTitle(title string) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleTitle := kernel32.NewProc("SetConsoleTitleW")
	utf16Title, _ := syscall.UTF16PtrFromString(title)
	procSetConsoleTitle.Call(uintptr(unsafe.Pointer(utf16Title)))
}