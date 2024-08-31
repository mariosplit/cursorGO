package main

import (
	"fmt"
	"git.com/mariosplit/CursurGO/cmd"
	"log/slog"
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procAllocConsole     = kernel32.NewProc("AllocConsole")
	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")
)

func main() {
	fmt.Println("Starting application...")

	// Check if we already have a console
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		fmt.Println("No console detected. Attempting to allocate a new console...")
		r, _, err := procAllocConsole.Call()
		if r == 0 {
			fmt.Printf("Failed to allocate console: %v\n", err)
			// Continue execution even if console allocation fails
		} else {
			fmt.Println("Console allocated successfully.")
		}
	} else {
		fmt.Println("Console already exists, skipping allocation.")
	}

	// Get handles for standard input, output, and error
	stdin, err := syscall.GetStdHandle(syscall.STD_INPUT_HANDLE)
	if err != nil {
		fmt.Printf("Failed to get stdin handle: %v\n", err)
	}
	stdout, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		fmt.Printf("Failed to get stdout handle: %v\n", err)
	}
	stderr, err := syscall.GetStdHandle(syscall.STD_ERROR_HANDLE)
	if err != nil {
		fmt.Printf("Failed to get stderr handle: %v\n", err)
	}

	// Redirect standard input, output, and error
	os.Stdin = os.NewFile(uintptr(stdin), "/dev/stdin")
	os.Stdout = os.NewFile(uintptr(stdout), "/dev/stdout")
	os.Stderr = os.NewFile(uintptr(stderr), "/dev/stderr")

	// Set console title
	setConsoleTitle("CursorGO File Explorer")

	fmt.Println("Initializing application...")
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
