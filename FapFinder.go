package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
)

var defaultPath string

// Banner to display in the help message.
const banner = `
  **********************************************
  *        ^_^ FapFinder v1.0 ^_^              *
  *   UwU scanning your folders, senpai~       *
  **********************************************
`

func init() {
	// Set default path based on OS.
	if runtime.GOOS == "windows" {
		// Change default Windows root to "C:\Users" so that user files are likely present.
		defaultPath = "C:\\Users"
	} else {
		defaultPath = "./"
	}

	// Disable runtime stack trace printing.
	debug.SetTraceback("none")

	// Set flag package's output to stdout for full help output.
	flag.CommandLine.SetOutput(os.Stdout)
	flag.Usage = func() {
		fmt.Println(banner)
		fmt.Printf("Usage: %s [options]\n", os.Args[0])
		fmt.Println("Options:")
		flag.PrintDefaults()
	}

	// Redirect os.Stderr to the null device.
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err == nil {
		os.Stderr = null
	}
}

// scanDir recursively scans the given directory for files matching any of the provided patterns.
// It uses a deferred recover to catch panics within this goroutine.
func scanDir(path string, patterns []string, wg *sync.WaitGroup, files chan<- string) {
	defer func() {
		if r := recover(); r != nil {
			// Silently recover from any panic.
		}
		wg.Done()
	}()

	entries, err := os.ReadDir(path)
	if err != nil {
		// Silently skip directories that cannot be read.
		return
	}

	for _, entry := range entries {
		fullPath := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			wg.Add(1)
			go scanDir(fullPath, patterns, wg, files)
		} else {
			for _, pattern := range patterns {
				match, err := filepath.Match(pattern, entry.Name())
				if err != nil {
					// Silently skip pattern errors.
					continue
				}
				if match {
					files <- fullPath
					break
				}
			}
		}
	}
}

// prioritySort sorts files so that files ending in ".kdbx" or ".conf" come last.
func prioritySort(files []string) {
	sort.SliceStable(files, func(i, j int) bool {
		// Determine if the files are low priority.
		iLow := strings.HasSuffix(strings.ToLower(files[i]), ".kdbx") || strings.HasSuffix(strings.ToLower(files[i]), ".conf")
		jLow := strings.HasSuffix(strings.ToLower(files[j]), ".kdbx") || strings.HasSuffix(strings.ToLower(files[j]), ".conf")
		if iLow != jLow {
			// If i is low priority, it should come after j.
			return !iLow
		}
		// Otherwise, sort lexicographically.
		return files[i] < files[j]
	})
}

func main() {
	// Use OS-dependent default path.
	pathFlag := flag.String("path", defaultPath, fmt.Sprintf("Root path to start scanning from (default: %s)", defaultPath))
	extFlag := flag.String("ext", "*.txt,*.csv,*.kdbx,*.config,*.conf,*.key,*.rsa,*.ini", "Comma separated list of file patterns to search for. Order defines reverse priority.")
	helpFlag := flag.Bool("help", false, "Show help message")
	flag.Parse()

	if *helpFlag {
		flag.Usage()
		return
	}

	// Check if the default extension list is used.
	defaultExt := "*.txt,*.csv,*.kdbx,*.config,*.conf,*.key,*.rsa,*.ini"
	usingDefault := (*extFlag == defaultExt)

	// Split the extension flag into a slice.
	patterns := strings.Split(*extFlag, ",")
	for i := range patterns {
		patterns[i] = strings.TrimSpace(patterns[i])
	}

	filesFoundChan := make(chan string, 100)
	var wg sync.WaitGroup
	wg.Add(1)
	go scanDir(*pathFlag, patterns, &wg, filesFoundChan)

	go func() {
		wg.Wait()
		close(filesFoundChan)
	}()

	var filesFound []string
	for file := range filesFoundChan {
		filesFound = append(filesFound, file)
	}

	// When using the default extension list, sort files so that low-priority files come last.
	if usingDefault {
		prioritySort(filesFound)
	}

	for _, file := range filesFound {
		fmt.Println(file)
	}
}
