package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"bufio"
	"math"
	"os/exec"
	"runtime"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"io"
	"log/slog"
	"strconv"
	"sort"
)

var LogFile *os.File

var rootCmd = &cobra.Command{
	Use:   "CursorGO",
	Short: "A CLI file explorer",
	Run:   run,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	fmt.Println("Initializing application...")
	var err error
	LogFile, err = os.OpenFile("CursorGO.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening log file:", err)
		os.Exit(1)
	}

	opts := slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewTextHandler(LogFile, &opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	slog.Info("Application initialized")
	fmt.Println("Application initialized")
}

func getBreadcrumb(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) <= 4 {
		return path
	}
	return filepath.FromSlash(fmt.Sprintf("%s/.../%s/%s/%s", parts[0], parts[len(parts)-3], parts[len(parts)-2], parts[len(parts)-1]))
}

func run(cmd *cobra.Command, args []string) {
	fmt.Println("Starting run function...")
	currentPath, err := os.Getwd()
	if err != nil {
		slog.Error("Failed to get current directory", "error", err)
		fmt.Println("Error getting current directory:", err)
		return
	}
	slog.Info("Starting file explorer", "path", currentPath)

	fmt.Println("Select a file or directory (use arrow keys and Enter to select)")
	fmt.Println("Type to search, use '/' to start search from the beginning")

	for {
		files, err := os.ReadDir(currentPath)
		if err != nil {
			slog.Error("Failed to read directory", "path", currentPath, "error", err)
			fmt.Println("Error reading directory:", err)
			currentPath = filepath.Dir(currentPath) // Go up one level
			continue
		}
		slog.Info("Entered directory", "path", currentPath)

		var items []string
		var dirs []string
		var regularFiles []string

		items = append(items, "0. ..")
		items = append(items, "1. cd (Change Directory)")

		for _, file := range files {
			info, err := file.Info()
			if err != nil {
				slog.Error("Failed to get file info", "error", err)
				continue
			}
			size := formatSize(info.Size())
			if file.IsDir() {
				dirs = append(dirs, fmt.Sprintf("%s (DIR)", file.Name()))
			} else {
				regularFiles = append(regularFiles, fmt.Sprintf("%s (%s)", file.Name(), size))
			}
		}

		// Sort directories and files alphabetically
		sort.Strings(dirs)
		sort.Strings(regularFiles)

		// Add sorted directories to items
		for i, dir := range dirs {
			items = append(items, fmt.Sprintf("%d. %s", i+2, dir))
		}

		// Add sorted files to items
		for i, file := range regularFiles {
			items = append(items, fmt.Sprintf("%d. %s", i+len(dirs)+2, file))
		}

		items = append(items, fmt.Sprintf("%d. Quit", len(items)))

		fmt.Printf("\nCurrent directory: %s\n", getBreadcrumb(currentPath))

		prompt := promptui.Select{
			Label:        "Select a file or directory (use arrow keys, Enter to select, or type the number)",
			Items:        items,
			Size:         10,
			HideSelected: true,
			Searcher: func(input string, index int) bool {
				item := items[index]
				if input == "/" {
					return strings.HasPrefix(strings.ToLower(item), strings.ToLower(input[1:]))
				}
				return strings.Contains(strings.ToLower(item), strings.ToLower(input))
			},
			StartInSearchMode: true,
			Stdout:            &bellSkipper{out: os.Stdout},
		}

		_, result, err := prompt.Run()
		if err != nil {
			if err == promptui.ErrInterrupt || err == promptui.ErrEOF {
				slog.Info("User interrupted the program")
				fmt.Println("Exiting...")
				return
			}
			slog.Error("Prompt failed", "error", err)
			fmt.Printf("Prompt failed: %v\n", err)
			fmt.Println("Press Enter to continue...")
			bufio.NewReader(os.Stdin).ReadBytes('\n')
			continue
		}

		slog.Info("User selected item", "item", result)

		if strings.HasPrefix(result, "1. cd") {
			numberedParts := displayNumberedPath(currentPath)
			for _, part := range numberedParts {
				fmt.Println(part)
			}
			fmt.Print("Enter the number of the directory you want to navigate to (or 'c' to cancel): ")
			var input string
			fmt.Scanln(&input)
			if input == "c" {
				fmt.Println("Cancelled directory change.")
				continue
			}
			if index, err := strconv.Atoi(input); err == nil && index >= 0 && index < len(numberedParts) {
				parts := strings.Split(filepath.ToSlash(currentPath), "/")
				currentPath = filepath.FromSlash(strings.Join(parts[:index+1], "/"))
				slog.Info("Changed directory", "newPath", currentPath)
				fmt.Printf("Changed directory to: %s\n", currentPath)
			} else {
				fmt.Println("Invalid input. Staying in current directory.")
				slog.Info("Invalid input for cd command", "input", input)
			}
			continue
		}

		if strings.HasSuffix(result, ". Quit") {
			slog.Info("User chose to quit")
			fmt.Println("Exiting...")
			return
		}

		// Extract the selected item, removing the number prefix
		selectedItem := strings.SplitN(result, ". ", 2)[1]

		if selectedItem == ".." {
			currentPath = filepath.Dir(currentPath)
			slog.Info("Moved up one directory", "newPath", currentPath)
			continue
		}

		// Extract the file name, removing the size or (DIR) suffix
		selectedName := strings.TrimSpace(strings.Split(selectedItem, " (")[0])
		fullPath := filepath.Join(currentPath, selectedName)
		fileInfo, err := os.Stat(fullPath)
		if err != nil {
			slog.Error("Failed to get file info", "file", selectedName, "error", err)
			fmt.Printf("Error getting file info for '%s': %v\n", selectedName, err)
			continue
		}

		if fileInfo.IsDir() {
			currentPath = fullPath
			slog.Info("Entered directory", "path", currentPath)
		} else {
			action := showFileOptions(fullPath)
			slog.Info("File action selected", "file", fullPath, "action", action)
			switch action {
			case "Open":
				if err := handleOpenFile(fullPath); err != nil {
					slog.Error("Failed to open file", "file", fullPath, "error", err)
					fmt.Printf("Error opening file: %v\n", err)
				}
			case "Delete":
				if deleted, err := deleteFile(fullPath); err != nil {
					slog.Error("Failed to delete file", "file", fullPath, "error", err)
					fmt.Printf("Error deleting file: %v\n", err)
				} else if deleted {
					slog.Info("File deleted", "file", filepath.Base(fullPath))
					fmt.Printf("File deleted: %s\n", filepath.Base(fullPath))
				}
			case "Copy":
				copied, err := copyFile(fullPath)
				if err != nil {
					slog.Error("Failed to copy file", "file", fullPath, "error", err)
					fmt.Printf("Error copying file: %v\n", err)
				} else if copied {
					slog.Info("File copied", "file", filepath.Base(fullPath))
					fmt.Printf("File copied successfully: %s\n", filepath.Base(fullPath))
				}
			case "Back":
				// Do nothing, continue browsing
			}
		}
	}
}

func showFileOptions(filePath string) string {
	prompt := promptui.Select{
		Label: fmt.Sprintf("Choose an action for %s", filepath.Base(filePath)),
		Items: []string{"Open", "Delete", "Copy", "Back"},
	}

	_, result, err := prompt.Run()
	if err != nil {
		slog.Error("Prompt failed", "error", err)
		fmt.Printf("Prompt failed %v\n", err)
		return "Back"
	}

	return result
}

func handleOpenFile(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt", ".csv", ".tex", ".log", ".md", ".pdf", ".docx", ".xlsx", ".pptx", ".zip", ".rar", ".7z":
		return openWithDefaultEditor(path)
	default:
		return fmt.Errorf("Cannot open file: %s\nFile type not supported for opening.", path)
	}
}

func openWithDefaultEditor(path string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default: // Linux and other Unix-like systems
		cmd = exec.Command("xdg-open", path)
	}

	err := cmd.Start()
	if err != nil {
		slog.Error("Failed to open file", "file", path, "error", err)
		return fmt.Errorf("Error opening file: %v", err)
	}
	fmt.Printf("Opening file: %s\n", path)
	return nil
}

func deleteFile(path string) (bool, error) {
	prompt := promptui.Prompt{
		Label:     fmt.Sprintf("Are you sure you want to delete %s", filepath.Base(path)),
		IsConfirm: true,
	}

	result, err := prompt.Run()
	if err != nil {
		slog.Info("Cancelled file deletion", "file", filepath.Base(path))
		return false, fmt.Errorf("Cancelled deletion of %s", filepath.Base(path))
	}

	if strings.ToLower(result) == "y" {
		err := os.Remove(path)
		if err != nil {
			slog.Error("Failed to delete file", "file", filepath.Base(path), "error", err)
			return false, fmt.Errorf("Error: Unable to delete %s (%v)", filepath.Base(path), err)
		}
		slog.Info("File deleted", "file", filepath.Base(path))
		return true, nil
	}

	slog.Info("Cancelled file deletion", "file", filepath.Base(path))
	return false, fmt.Errorf("Cancelled deletion of %s", filepath.Base(path))
}

func copyFile(sourcePath string) (bool, error) {
	prompt := promptui.Prompt{
		Label: "Enter destination path",
		Validate: func(input string) error {
			if input == "" {
				return fmt.Errorf("Destination path cannot be empty")
			}
			return nil
		},
	}

	destPath, err := prompt.Run()
	if err != nil {
		slog.Info("Cancelled file copy", "file", filepath.Base(sourcePath))
		return false, fmt.Errorf("Cancelled copying of %s", filepath.Base(sourcePath))
	}

	// Check if destination is a directory
	destInfo, err := os.Stat(destPath)
	if err == nil && destInfo.IsDir() {
		// If destination is a directory, append the source file name
		destPath = filepath.Join(destPath, filepath.Base(sourcePath))
	} else if os.IsNotExist(err) {
		// Ensure the destination directory exists
		destDir := filepath.Dir(destPath)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			slog.Error("Failed to create destination directory", "directory", destDir, "error", err)
			return false, fmt.Errorf("Unable to create destination directory %s: %v", destDir, err)
		}
	} else if err != nil {
		slog.Error("Error checking destination path", "path", destPath, "error", err)
		return false, fmt.Errorf("Error checking destination path: %v", err)
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		slog.Error("Failed to open source file", "file", filepath.Base(sourcePath), "error", err)
		return false, fmt.Errorf("Unable to open source file %s: %v", filepath.Base(sourcePath), err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		slog.Error("Failed to create destination file", "file", filepath.Base(destPath), "error", err)
		return false, fmt.Errorf("Unable to create destination file %s: %v", filepath.Base(destPath), err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		slog.Error("Failed to copy file", "source", filepath.Base(sourcePath), "destination", filepath.Base(destPath), "error", err)
		return false, fmt.Errorf("Unable to copy %s to %s: %v", filepath.Base(sourcePath), filepath.Base(destPath), err)
	}

	slog.Info("File copied", "source", filepath.Base(sourcePath), "destination", destPath)
	return true, nil
}

func continuePrompt() bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Do you want to continue? (y/n): ")
	response, err := reader.ReadString('\n')
	if err != nil {
		slog.Error("Error reading input", "error", err)
		return false
	}
	return strings.ToLower(strings.TrimSpace(response)) == "y"
}

func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	i := int(math.Log2(float64(size)) / 10)
	return fmt.Sprintf("%.2f %s", float64(size)/math.Pow(1024, float64(i)), units[i])
}

type bellSkipper struct {
	out io.WriteCloser
}

func (bs *bellSkipper) Write(b []byte) (int, error) {
	const bell = '\a'
	if len(b) == 1 && b[0] == bell {
		return 0, nil
	}
	return bs.out.Write(b)
}

func (bs *bellSkipper) Close() error {
	return bs.out.Close()
}

func displayNumberedPath(path string) []string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	var numberedParts []string
	for i, part := range parts {
		numberedParts = append(numberedParts, fmt.Sprintf("%d: %s", i, part))
	}
	return numberedParts
}
