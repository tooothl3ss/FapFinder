package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Твой фирменный баннер
const banner = `
  **********************************************
  *        ^_^ FapFinder v1.1 ^_^              *
  *   UwU scanning your folders, senpai~       *
  **********************************************
`

var defaultPath string

func init() {
	// Определяем путь по умолчанию в зависимости от ОС
	if runtime.GOOS == "windows" {
		defaultPath = "C:\\Users"
	} else {
		defaultPath = "./"
	}
}

// Проверяет, совпадает ли имя файла с одним из паттернов
func matchPattern(filename string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, filename)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// Сортировка: файлы .kdbx и .conf отправляются в конец списка
func prioritySort(files []string) {
	sort.SliceStable(files, func(i, j int) bool {
		// Приводим к нижнему регистру для проверки
		f1 := strings.ToLower(files[i])
		f2 := strings.ToLower(files[j])

		iLow := strings.HasSuffix(f1, ".kdbx") || strings.HasSuffix(f1, ".conf")
		jLow := strings.HasSuffix(f2, ".kdbx") || strings.HasSuffix(f2, ".conf")

		if iLow != jLow {
			// Если один файл низкоприоритетный, а другой нет -> низкоприоритетный идет позже
			return !iLow
		}
		// В остальных случаях обычная сортировка по алфавиту
		return files[i] < files[j]
	})
}

func main() {
	// Настройка флагов
	pathFlag := flag.String("path", defaultPath, fmt.Sprintf("Root path to start scanning from (default: %s)", defaultPath))
	// Твой список расширений
	defaultExt := "*.txt,*.csv,*.kdbx,*.config,*.conf,*.key,*.rsa,*.ini"
	extFlag := flag.String("ext", defaultExt, "Comma separated list of file patterns.")
	helpFlag := flag.Bool("help", false, "Show help message")

	// Настройка Usage для вывода баннера
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

	// Подготовка паттернов поиска
	rawPatterns := strings.Split(*extFlag, ",")
	var patterns []string
	for _, p := range rawPatterns {
		patterns = append(patterns, strings.TrimSpace(p))
	}

	var filesFound []string

	// Используем стандартный WalkDir — это надежнее, чем ручная рекурсия с горутинами
	err := filepath.WalkDir(*pathFlag, func(path string, d fs.DirEntry, err error) error {
		// Если у нас нет прав на чтение папки, просто пропускаем её (без паники и ошибок в консоль)
		if err != nil {
			return filepath.SkipDir
		}

		// Если это папка, идем дальше
		if d.IsDir() {
			return nil
		}

		// Проверяем имя файла на совпадение с расширениями
		if matchPattern(d.Name(), patterns) {
			filesFound = append(filesFound, path)
		}

		return nil
	})

	if err != nil {
		// Выводим ошибку, только если упал сам процесс обхода (редко)
		fmt.Fprintf(os.Stderr, "Error walking the path: %v\n", err)
	}

	// Если используется стандартный список, применяем твою хитрую сортировку
	if *extFlag == defaultExt {
		prioritySort(filesFound)
	} else {
		// Иначе просто сортируем по алфавиту для красоты
		sort.Strings(filesFound)
	}

	// Вывод результатов
	for _, file := range filesFound {
		fmt.Println(file)
	}
}