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
  *        ^_^ FapFinder v2.0 ^_^              *
  *   UwU scanning your folders, senpai~       *
  **********************************************
`

// ─── Default scan paths ────────────────────────────────────────────

var defaultPathsLinux = []string{"/home", "/root", "/etc", "/var", "/tmp"}
var defaultPathsWindows = []string{`C:\`}

// ─── Default exclude dirs ──────────────────────────────────────────

var defaultExcludeLinux = []string{
	"/proc", "/sys", "/dev", "/run",
	"/snap", "/boot",
	"/lib", "/lib64", "/usr/lib", "/usr/lib64",
}

var defaultExcludeWindows = []string{
	`C:\Windows`,
}

// ─── Force-include subdirs inside excluded dirs ────────────────────

var forceIncludeWindows = []string{
	`C:\Windows\Temp`,
	`C:\Windows\Tasks`,
}

// ─── Known filenames (no extension / dotfiles) ─────────────────────

var knownFileNames = []string{
	// SSH
	"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519",
	"authorized_keys", "authorized_keys2", "known_hosts",
	// System auth
	"shadow", "passwd", "master.passwd", "opasswd",
	"login.defs",
	// Credentials / tokens
	"credentials", "token", "secrets",
	".env", ".env.local", ".env.production", ".env.development",
	".netrc", ".pgpass", ".my.cnf",
	".docker/config.json",
	// Shell history
	".bash_history", ".zsh_history", ".sh_history",
	".python_history", ".mysql_history", ".psql_history",
	// Shell config (may contain aliases/exports with creds)
	".bashrc", ".bash_profile", ".zshrc", ".profile",
	// Web / services
	".htpasswd", ".htaccess",
	// Cloud
	".aws/credentials", ".aws/config",
	".azure/accessTokens.json",
	// Build / infra
	"Dockerfile", "Makefile", "Vagrantfile",
	"docker-compose.yml", "docker-compose.yaml",
	// Git
	".git-credentials", ".gitconfig",
	// GPG
	"trustdb.gpg",
	// Misc
	"wp-config.php", "LocalSettings.php",
}

// ─── Default glob patterns ─────────────────────────────────────────

var defaultExtPatterns = []string{
	"*.txt", "*.csv", "*.log",
	"*.kdbx", "*.kdb",
	"*.config", "*.conf", "*.cfg",
	"*.key", "*.pem", "*.crt", "*.cer", "*.p12", "*.pfx", "*.jks",
	"*.rsa", "*.zip", "*.tar", "*.rar", "*.gz", "*.7z",
	"*.ini", "*.yaml", "*.yml", "*.toml", "*.json", "*.xml",
	"*.env.*",
	"*.bak", "*.old", 
	"*.sql", "*.dump", "*.sqlite", "*.db",
	"*.ovpn", "*.rdp",
	"*.pcap",
}

// ═══════════════════════════════════════════════════════════════════
//  Helpers
// ═══════════════════════════════════════════════════════════════════

// matchGlob checks filename against a list of glob patterns.
func matchGlob(filename string, patterns []string) bool {
	lower := strings.ToLower(filename)
	for _, p := range patterns {
		if matched, _ := filepath.Match(strings.ToLower(p), lower); matched {
			return true
		}
	}
	return false
}

// matchKnownName checks if the full path ends with any known filename.
// Supports both plain names ("shadow") and path fragments (".aws/credentials").
func matchKnownName(filePath string, name string) bool {
	norm := filepath.ToSlash(filePath)
	for _, known := range knownFileNames {
		if strings.Contains(known, "/") {
			// Path fragment — check suffix
			if strings.HasSuffix(norm, "/"+known) {
				return true
			}
		} else {
			if strings.EqualFold(name, known) {
				return true
			}
		}
	}
	return false
}

// hasNoExtension returns true if filename has no extension.
// Dotfiles like ".env" are considered extensionless here.
// "file.txt" → false, "id_rsa" → true, ".bashrc" → true
func hasNoExtension(name string) bool {
	if name == "" {
		return false
	}
	// Strip leading dots to handle dotfiles
	stripped := strings.TrimLeft(name, ".")
	if stripped == "" {
		return true // e.g. "..." — weird but extensionless
	}
	return !strings.Contains(stripped, ".")
}

// normPath returns cleaned lowercase path for comparison.
func normPath(p string) string {
	return strings.ToLower(filepath.Clean(p))
}

// shouldSkipDir decides whether to skip a directory during walk.
// It handles the "exclude dir but force-include certain subdirs" logic.
func shouldSkipDir(dirPath string, excludeDirs []string, forceInclude []string) bool {
	clean := normPath(dirPath)

	isExcluded := false
	for _, exc := range excludeDirs {
		excClean := normPath(exc)
		if clean == excClean || strings.HasPrefix(clean, excClean+string(os.PathSeparator)) {
			isExcluded = true
			break
		}
	}
	if !isExcluded {
		return false
	}

	// Check if this dir IS a force-include or is an ancestor leading to one
	for _, inc := range forceInclude {
		incClean := normPath(inc)
		// This dir is the included path or inside it → don't skip
		if clean == incClean || strings.HasPrefix(clean, incClean+string(os.PathSeparator)) {
			return false
		}
		// This dir is an ancestor of the included path → don't skip (need to traverse through)
		if strings.HasPrefix(incClean, clean+string(os.PathSeparator)) {
			return false
		}
	}

	return true
}

// isInsideExcludedZone returns true if a FILE is inside an excluded dir
// but NOT inside a force-included subdir. Used to filter out files that
// live in excluded dirs outside of force-included subdirs
// (e.g. C:\Windows\explorer.exe should be skipped, but C:\Windows\Temp\log.txt should not).
func isInsideExcludedZone(filePath string, excludeDirs []string, forceInclude []string) bool {
	clean := normPath(filePath)

	inExclude := false
	for _, exc := range excludeDirs {
		excClean := normPath(exc)
		if strings.HasPrefix(clean, excClean+string(os.PathSeparator)) {
			inExclude = true
			break
		}
	}
	if !inExclude {
		return false
	}

	for _, inc := range forceInclude {
		incClean := normPath(inc)
		if strings.HasPrefix(clean, incClean+string(os.PathSeparator)) || filepath.Dir(clean) == incClean {
			return false
		}
	}
	return true
}

// prioritySort puts high-value findings first, low-noise stuff later.
func prioritySort(files []string) {
	sort.SliceStable(files, func(i, j int) bool {
		f1 := strings.ToLower(files[i])
		f2 := strings.ToLower(files[j])

		lowPriority := func(s string) bool {
			return strings.HasSuffix(s, ".kdbx") ||
				strings.HasSuffix(s, ".conf") ||
				strings.HasSuffix(s, ".cfg")
			
		}
		iLow := lowPriority(f1)
		jLow := lowPriority(f2)
		if iLow != jLow {
			return !iLow
		}
		return files[i] < files[j]
	})
}

// ═══════════════════════════════════════════════════════════════════
//  Main
// ═══════════════════════════════════════════════════════════════════

func main() {
	// ── Determine OS-specific defaults ─────────────────────────────
	var defaultPaths, defaultExclude, defaultForceInclude []string

	if runtime.GOOS == "windows" {
		defaultPaths = defaultPathsWindows
		defaultExclude = defaultExcludeWindows
		defaultForceInclude = forceIncludeWindows
	} else {
		defaultPaths = defaultPathsLinux
		defaultExclude = defaultExcludeLinux
		defaultForceInclude = nil
	}

	// ── Flags ──────────────────────────────────────────────────────
	pathFlag := flag.String("path", "", "Comma-separated root paths to scan (overrides OS defaults)")
	extFlag := flag.String("ext", "", "Comma-separated glob patterns (e.g. '*.txt,*.csv')")
	regexFlag := flag.String("regex", "", "Regex to match filenames (e.g. '(?i)pass')")
	excludeFlag := flag.String("exclude", "", "Comma-separated extra dirs to exclude")
	includeFlag := flag.String("include", "", "Comma-separated dirs to force-include inside excluded dirs")
	noExtFlag := flag.Bool("no-ext", false, "Search for files without extension")
	namesFlag := flag.Bool("names", false, "Search for known sensitive filenames")
	allFlag := flag.Bool("all", false, "Enable all search modes (ext + no-ext + names)")
	helpFlag := flag.Bool("help", false, "Show help")


	flag.Usage = func() {
		fmt.Println(banner)
		fmt.Printf("Usage: %s [options]\n\n", os.Args[0])
		fmt.Println("Options:")
		flag.PrintDefaults()
		fmt.Println("\nExamples:")
		fmt.Printf("  %s                                  # default scan (ext + names + no-ext)\n", os.Args[0])
		fmt.Printf("  %s -regex '(?i)(pass|secret|token)' # regex only\n", os.Args[0])
		fmt.Printf("  %s -ext '*.pem,*.key' -no-ext       # custom extensions + extensionless files\n", os.Args[0])
		fmt.Printf("  %s -path /opt,/srv -names            # custom paths, known filenames only\n", os.Args[0])
		fmt.Printf("  %s -regex '.*\\.db$' -ext '*.sql'    # regex + glob combined\n", os.Args[0])
	}

	flag.Parse()

	if *helpFlag {
		flag.Usage()
		return
	}

	// ── Figure out which flags the user actually passed ────────────
	explicitFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		explicitFlags[f.Name] = true
	})

	// ── Compile regex ──────────────────────────────────────────────
	var compiledRegex *regexp.Regexp
	if *regexFlag != "" {
		var err error
		compiledRegex, err = regexp.Compile(*regexFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] Invalid regex: %v\n", err)
			os.Exit(1)
		}
	}

	// ── Resolve scan paths ─────────────────────────────────────────
	scanPaths := defaultPaths
	if *pathFlag != "" {
		scanPaths = nil
		for _, p := range strings.Split(*pathFlag, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				scanPaths = append(scanPaths, p)
			}
		}
	}

	// ── Resolve exclude / force-include dirs ───────────────────────
	excludeDirs := append([]string{}, defaultExclude...)
	if *excludeFlag != "" {
		for _, d := range strings.Split(*excludeFlag, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				excludeDirs = append(excludeDirs, d)
			}
		}
	}

	forceIncludeDirs := append([]string{}, defaultForceInclude...)
	if *includeFlag != "" {
		for _, d := range strings.Split(*includeFlag, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				forceIncludeDirs = append(forceIncludeDirs, d)
			}
		}
	}

	// ── Resolve search modes ───────────────────────────────────────
	// No explicit search flags → default mode (all three on)
	anySearchFlag := explicitFlags["ext"] || explicitFlags["regex"] ||
		explicitFlags["no-ext"] || explicitFlags["names"] || explicitFlags["all"]

	useGlob := false
	useNoExt := false
	useNames := false
	useRegex := compiledRegex != nil

	if *allFlag {
		useGlob = true
		useNoExt = true
		useNames = true
	} else if !anySearchFlag {
		// Default mode: everything on
		useGlob = true
		useNoExt = true
		useNames = true
	} else {
		// User picked specific modes
		if explicitFlags["ext"] {
			useGlob = true
		}
		if *noExtFlag {
			useNoExt = true
		}
		if *namesFlag {
			useNames = true
		}
	}

	// ── Build glob patterns ────────────────────────────────────────
	var patterns []string
	if useGlob {
		if explicitFlags["ext"] && *extFlag != "" {
			for _, p := range strings.Split(*extFlag, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					patterns = append(patterns, p)
				}
			}
		} else if useGlob && !explicitFlags["ext"] {
			patterns = defaultExtPatterns
		}
	}

	// ── Walk ───────────────────────────────────────────────────────
	usingDefaultSort := !anySearchFlag

	var filesFound []string
	seen := make(map[string]bool)

	for _, root := range scanPaths {
		info, err := os.Stat(root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] Cannot access %s: %v\n", root, err)
			continue
		}
		if !info.IsDir() {
			fmt.Fprintf(os.Stderr, "[!] Not a directory: %s\n", root)
			continue
		}

		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return filepath.SkipDir
			}

			if d.IsDir() {
				if shouldSkipDir(path, excludeDirs, forceIncludeDirs) {
					return filepath.SkipDir
				}
				return nil
			}

			// Skip files directly inside excluded zones (but outside force-includes)
			if isInsideExcludedZone(path, excludeDirs, forceIncludeDirs) {
				return nil
			}

			name := d.Name()
			matched := false

			// 1. Glob patterns
			if useGlob && len(patterns) > 0 && matchGlob(name, patterns) {
				matched = true
			}

			// 2. Regex
			if useRegex && compiledRegex.MatchString(name) {
				matched = true
			}

			// 3. Files without extension
			if useNoExt && hasNoExtension(name) {
				matched = true
			}

			// 4. Known filenames
			if useNames && matchKnownName(path, name) {
				matched = true
			}

			if matched && !seen[path] {
				seen[path] = true
				filesFound = append(filesFound, path)
			}

			return nil
		})
	}

	// ── Sort & output ──────────────────────────────────────────────
	if usingDefaultSort {
		prioritySort(filesFound)
	} else {
		sort.Strings(filesFound)
	}

	for _, f := range filesFound {
		fmt.Println(f)
	}

	fmt.Fprintf(os.Stderr, "\n[*] Done. Found %d files.\n", len(filesFound))
}