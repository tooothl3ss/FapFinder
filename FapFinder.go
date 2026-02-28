package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

const banner = `
  **********************************************
  *        ^_^ FapFinder v1.2 ^_^              *
  *   UwU scanning your folders, senpai~       *
  **********************************************
`

var defaultPath string

func init() {
	if runtime.GOOS == "windows" {
		defaultPath = "C:\\Users"
	} else {
		defaultPath = "./"
	}
}

func matchPattern(filename string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, filename)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func prioritySort(files []string) {
	sort.SliceStable(files, func(i, j int) bool {
		f1 := strings.ToLower(files[i])
		f2 := strings.ToLower(files[j])

		iLow := strings.HasSuffix(f1, ".kdbx") || strings.HasSuffix(f1, ".conf")
		jLow := strings.HasSuffix(f2, ".kdbx") || strings.HasSuffix(f2, ".conf")

		if iLow != jLow {
			return !iLow
		}
		return files[i] < files[j]
	})
}

func main() {
	pathFlag := flag.String("path", defaultPath, fmt.Sprintf("Root path to start scanning from (default: %s)", defaultPath))

	defaultExt := "*.txt,*.csv,*.kdbx,*.config,*.conf,*.key,*.rsa,*.ini"
	extFlag := flag.String("ext", defaultExt, "Comma separated list of file glob patterns.")
	regexFlag := flag.String("regex", "", "Regex pattern to match filenames (e.g. 'passw.*\\.txt'). Can be used together with -ext.")

	helpFlag := flag.Bool("help", false, "Show help message")

	flag.Usage = func() {
		fmt.Println(banner)
		fmt.Printf("Usage: %s [options]\n", os.Args[0])
		fmt.Println("Options:")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *helpFlag {
		flag.Usage()
		return
	}

	// Компилируем регексп если задан
	var compiledRegex *regexp.Regexp
	if *regexFlag != "" {
		var err error
		compiledRegex, err = regexp.Compile(*regexFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid regex pattern: %v\n", err)
			os.Exit(1)
		}
	}

	// Подготовка glob-паттернов — пропускаем если пользователь не передавал -ext явно
	usingDefaultExt := *extFlag == defaultExt
	var patterns []string
	if *extFlag != "" {
		for _, p := range strings.Split(*extFlag, ",") {
			patterns = append(patterns, strings.TrimSpace(p))
		}
	}

	var filesFound []string

	err := filepath.WalkDir(*pathFlag, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}

		name := d.Name()

		// OR-логика: файл подходит если совпал glob ИЛИ regex
		matchedExt := matchPattern(name, patterns)
		matchedRegex := compiledRegex != nil && compiledRegex.MatchString(name)

		if matchedExt || matchedRegex {
			filesFound = append(filesFound, path)
		}

		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error walking the path: %v\n", err)
	}

	// Приоритетная сортировка только если используется дефолтный список расширений
	if usingDefaultExt {
		prioritySort(filesFound)
	} else {
		sort.Strings(filesFound)
	}

	for _, file := range filesFound {
		fmt.Println(file)
	}
}