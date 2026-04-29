package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
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
	`C:\Windows`, `C:\Program Files`, `C:\Program Files (x86)`,
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
// Dotfiles like ".bashrc" are considered extensionless; ".env.local" is not.
func hasNoExtension(name string) bool {
	if name == "" {
		return false
	}
	stripped := strings.TrimLeft(name, ".")
	if stripped == "" {
		return true
	}
	return !strings.Contains(stripped, ".")
}

// normPath returns a cleaned, lowercase path for plain-string comparison.
func normPath(p string) string {
	return strings.ToLower(filepath.Clean(p))
}

// isGlobPattern reports whether s contains glob metacharacters.
func isGlobPattern(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// splitSlashPath splits a slash-normalized path into non-empty segments.
func splitSlashPath(p string) []string {
	var parts []string
	for _, seg := range strings.Split(p, "/") {
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	return parts
}

// matchPathPattern does segment-by-segment glob matching between a concrete path
// and a pattern (e.g. "C:\Users\*\AppData" or "/home/*/Downloads").
//
// Returns:
//
//	matched=true    → path exactly matches pattern, or is a descendant of a matched dir
//	isAncestor=true → path is shallower and its segments match the pattern prefix —
//	                  must be traversed to reach potential matches below
func matchPathPattern(pathStr, pattern string) (matched, isAncestor bool) {
	normP := filepath.ToSlash(strings.ToLower(filepath.Clean(pathStr)))
	normQ := filepath.ToSlash(strings.ToLower(filepath.Clean(pattern)))

	pParts := splitSlashPath(normP)
	qParts := splitSlashPath(normQ)

	minLen := len(pParts)
	if len(qParts) < minLen {
		minLen = len(qParts)
	}

	for i := 0; i < minLen; i++ {
		ok, err := filepath.Match(qParts[i], pParts[i])
		if err != nil || !ok {
			return false, false
		}
	}

	switch {
	case len(pParts) == len(qParts):
		return true, false // exact depth — matched
	case len(pParts) < len(qParts):
		return false, true // path is shallower — ancestor
	default:
		return true, false // path is deeper — descendant of matched dir
	}
}

// expandGlobPaths expands paths containing glob metacharacters into actual directories.
// Plain paths are passed through unchanged.
func expandGlobPaths(paths []string) []string {
	var result []string
	for _, p := range paths {
		if !isGlobPattern(p) {
			result = append(result, p)
			continue
		}
		matches, err := filepath.Glob(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] Invalid path pattern %q: %v\n", p, err)
			continue
		}
		if len(matches) == 0 {
			fmt.Fprintf(os.Stderr, "[!] No directories matched pattern: %s\n", p)
			continue
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err == nil && info.IsDir() {
				result = append(result, m)
			}
		}
	}
	return result
}

// isDirExcluded returns true if dirPath is covered by excl (plain path or glob pattern).
// For plain paths it checks exact match or subdir prefix.
// For glob patterns it uses segment-by-segment matching (matched or descendant).
func isDirExcluded(dirPath, excl string) bool {
	if isGlobPattern(excl) {
		matched, _ := matchPathPattern(dirPath, excl)
		return matched
	}
	clean := normPath(dirPath)
	exclClean := normPath(excl)
	return clean == exclClean || strings.HasPrefix(clean, exclClean+string(os.PathSeparator))
}

// shouldSkipDir decides whether to skip a directory during walk.
// It handles the "exclude dir but force-include certain subdirs" logic.
// Supports both plain paths and glob patterns in excludeDirs.
func shouldSkipDir(dirPath string, excludeDirs []string, forceInclude []string) bool {
	clean := normPath(dirPath)

	isExcluded := false
	for _, exc := range excludeDirs {
		if isDirExcluded(dirPath, exc) {
			isExcluded = true
			break
		}
		// If isDirExcluded returned false for a pattern, the dir may be an ancestor
		// that needs to be traversed — isExcluded stays false, which is correct.
	}
	if !isExcluded {
		return false
	}

	// Check if this dir IS a force-include or is an ancestor/descendant of one.
	for _, inc := range forceInclude {
		incClean := normPath(inc)
		if clean == incClean || strings.HasPrefix(clean, incClean+string(os.PathSeparator)) {
			return false
		}
		if strings.HasPrefix(incClean, clean+string(os.PathSeparator)) {
			return false
		}
	}

	return true
}

// isInsideExcludedZone returns true if a FILE is inside an excluded dir
// but NOT inside a force-included subdir.
// Supports both plain paths and glob patterns in excludeDirs.
func isInsideExcludedZone(filePath string, excludeDirs []string, forceInclude []string) bool {
	clean := normPath(filePath)

	inExclude := false
	for _, exc := range excludeDirs {
		if isGlobPattern(exc) {
			// Check whether the file's parent directory (or any ancestor) is inside
			// a pattern-matched dir. matchPathPattern handles the descendant case:
			// a parent deeper than the pattern still returns matched=true.
			matched, _ := matchPathPattern(filepath.Dir(filePath), exc)
			if matched {
				inExclude = true
				break
			}
		} else {
			excClean := normPath(exc)
			if strings.HasPrefix(clean, excClean+string(os.PathSeparator)) {
				inExclude = true
				break
			}
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
	pathFlag := flag.String("path", "", "Comma-separated root paths to scan; supports glob patterns (e.g. 'C:\\Users\\*\\Downloads')")
	extFlag := flag.String("ext", "", "Comma-separated glob patterns (e.g. '*.txt,*.csv')")
	regexFlag := flag.String("regex", "", "Regex to match filenames (e.g. '(?i)pass')")
	excludeFlag := flag.String("exclude", "", "Comma-separated dirs to exclude; supports glob patterns (e.g. 'C:\\Users\\*\\AppData')")
	includeFlag := flag.String("include", "", "Comma-separated dirs to force-include inside excluded dirs")
	noExtFlag := flag.Bool("no-ext", false, "Search for files without extension")
	namesFlag := flag.Bool("names", false, "Search for known sensitive filenames")
	allFlag := flag.Bool("all", false, "Enable all search modes (ext + no-ext + names)")
	outFlag := flag.String("out", "", "Write results to file (e.g. -out results.txt)")
	helpFlag := flag.Bool("help", false, "Show help")

	flag.CommandLine.SetOutput(os.Stderr)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\nExamples:")
		fmt.Fprintf(os.Stderr, "  %s                                          # default scan (ext + names + no-ext)\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -regex '(?i)(pass|secret|token)'         # regex only\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -ext '*.pem,*.key' -no-ext               # custom extensions + extensionless\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -path /opt,/srv -names                   # custom paths, known names only\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -path 'C:\\Users\\*\\Downloads'           # scan Downloads for every user\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -exclude 'C:\\Users\\*\\AppData'          # exclude AppData for every user\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -regex '.*\\.db$' -ext '*.sql'           # regex + glob combined\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -exclude 'C:\\Temp' -out res.txt         # exclude dir + save to file\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -exclude /etc -include /etc/nginx        # force-include inside excluded dir\n", os.Args[0])
	}

	// Print banner before Parse so it appears even on flag errors.
	fmt.Fprint(os.Stderr, banner)

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
	customPaths := false
	scanPaths := defaultPaths
	if *pathFlag != "" {
		customPaths = true
		var rawPaths []string
		for _, p := range strings.Split(*pathFlag, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				rawPaths = append(rawPaths, p)
			}
		}
		scanPaths = expandGlobPaths(rawPaths)
	}

	// ── Resolve exclude / force-include dirs ───────────────────────
	// If user passed -path explicitly, don't apply default OS excludes —
	// they know what they're scanning. Only -exclude flag applies.
	var excludeDirs []string
	if !customPaths {
		excludeDirs = append(excludeDirs, defaultExclude...)
	}

	var userExcludes []string
	if *excludeFlag != "" {
		for _, d := range strings.Split(*excludeFlag, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				excludeDirs = append(excludeDirs, d)
				userExcludes = append(userExcludes, d)
			}
		}
	}

	var forceIncludeDirs []string
	if !customPaths {
		forceIncludeDirs = append(forceIncludeDirs, defaultForceInclude...)
	}
	if *includeFlag != "" {
		for _, d := range strings.Split(*includeFlag, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				forceIncludeDirs = append(forceIncludeDirs, d)
			}
		}
	}

	// User's -exclude overrides default force-includes:
	// if user explicitly excluded a dir, remove it from force-includes.
	if len(userExcludes) > 0 {
		var filtered []string
		for _, inc := range forceIncludeDirs {
			incClean := normPath(inc)
			keep := true
			for _, exc := range userExcludes {
				excClean := normPath(exc)
				if incClean == excClean || strings.HasPrefix(incClean, excClean+string(os.PathSeparator)) {
					keep = false
					break
				}
			}
			if keep {
				filtered = append(filtered, inc)
			}
		}
		forceIncludeDirs = filtered
	}

	// ── Resolve search modes ───────────────────────────────────────
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
		} else if !explicitFlags["ext"] {
			patterns = defaultExtPatterns
		}
	}

	// ── Prepare output file if requested ───────────────────────────
	var outFile *os.File
	if *outFlag != "" {
		var err error
		outFile, err = os.Create(*outFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] Cannot create output file: %v\n", err)
			os.Exit(1)
		}
		defer outFile.Close()
		fmt.Fprintf(os.Stderr, "[*] Writing results to %s\n", *outFlag)
	}

	// ── Walk ───────────────────────────────────────────────────────
	seen := make(map[string]struct{})
	scannedDirs := 0
	matchCount := 0
	start := time.Now()

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

		fmt.Fprintf(os.Stderr, "[*] Scanning %s ...\n", root)

		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return filepath.SkipDir
			}

			if d.IsDir() {
				if shouldSkipDir(path, excludeDirs, forceIncludeDirs) {
					return filepath.SkipDir
				}
				scannedDirs++
				if scannedDirs%500 == 0 {
					fmt.Fprintf(os.Stderr, "[*] Scanned %d dirs, found %d files...\n", scannedDirs, matchCount)
				}
				return nil
			}

			// Skip files directly inside excluded zones (but outside force-includes).
			if isInsideExcludedZone(path, excludeDirs, forceIncludeDirs) {
				return nil
			}

			// Only process regular files.
			// Symlinks → resolve, skip if target is dir or broken.
			// Junctions/reparse points on Windows can report as non-dir non-regular → skip.
			if !d.Type().IsRegular() {
				if d.Type()&fs.ModeSymlink != 0 {
					target, err := os.Stat(path)
					if err != nil || target.IsDir() || !target.Mode().IsRegular() {
						return nil
					}
				} else {
					return nil
				}
			}

			name := d.Name()
			matched := false

			if useGlob && len(patterns) > 0 && matchGlob(name, patterns) {
				matched = true
			}
			if useRegex && compiledRegex.MatchString(name) {
				matched = true
			}
			if useNoExt && hasNoExtension(name) {
				matched = true
			}
			if useNames && matchKnownName(path, name) {
				matched = true
			}

			if matched {
				if _, ok := seen[path]; !ok {
					seen[path] = struct{}{}
					matchCount++
					fmt.Println(path)
					if outFile != nil {
						fmt.Fprintln(outFile, path)
					}
				}
			}

			return nil
		})
	}

	fmt.Fprintf(os.Stderr, "[+] Done. Scanned %d dirs, found %d matches in %v.\n",
		scannedDirs, matchCount, time.Since(start).Round(time.Millisecond))
}
