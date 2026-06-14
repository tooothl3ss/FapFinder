# FapFinder
![badge](https://img.shields.io/badge/Built%20for-degenerates-critical?style=flat-square&logo=hell) ![NSFW](https://img.shields.io/badge/NSFW-yes-critical?style=flat-square) ![Badge](https://img.shields.io/badge/Senpai-approves-blueviolet?style=flat-square)

> A shameless sensitive-file hunter with needs.  
> He doesn't search for files. He *craves* them.

FapFinder v2.0 crawls your filesystem hunting for credentials, keys, configs, archives, databases, and anything else worth craving.  
Cross-platform, fully configurable, and equipped with four independent search modes that can be combined freely.

---

## Search modes

| Mode | Flag | What it does |
|------|------|--------------|
| **Glob** | `-ext` | Match filenames against glob patterns (`*.key`, `*.sql`, …) |
| **Regex** | `-regex` | Match filenames against a regular expression |
| **No-extension** | `-no-ext` | Catch extensionless files (`id_rsa`, `shadow`, …) |
| **Known names** | `-names` | Match against a built-in list of sensitive filenames and path fragments |
| **All** | `-all` | Enable Glob + No-extension + Known names at once |

> `-all` does **not** enable regex — pass `-regex` explicitly if you need it.  
> Running without any search flag is equivalent to `-all` — everything is on by default.

---

## Default behaviour

**Scan paths**

| OS | Default roots |
|----|---------------|
| Linux | `/home`, `/root`, `/etc`, `/var`, `/tmp` |
| Windows | `C:\` |

**Excluded directories** (skipped unless overridden)

| OS | Excluded by default |
|----|---------------------|
| Linux | `/proc`, `/sys`, `/dev`, `/run`, `/snap`, `/boot`, `/lib`, `/lib64`, `/usr/lib`, `/usr/lib64` |
| Windows | `C:\Windows`, `C:\Program Files`, `C:\Program Files (x86)` |

**Force-included subdirs inside excluded dirs** (Windows only)

`C:\Windows\Temp`, `C:\Windows\Tasks` — these are always scanned even though `C:\Windows` is excluded.

> **Note:** if `-path` is specified, OS default excludes are **not applied** — only `-exclude` matters.  
> Doing `-path C:\` will scan everything including `C:\Windows` unless you also pass `-exclude`.

**Default glob patterns** (used when `-ext` is not specified)

```
*.txt  *.csv  *.log  *.kdbx  *.kdb  *.config  *.conf  *.cfg
*.key  *.pem  *.crt  *.cer   *.p12  *.pfx     *.jks   *.rsa
*.zip  *.tar  *.rar  *.gz    *.7z   *.ini      *.yaml  *.yml
*.toml *.json *.xml  *.env.* *.bak  *.old      *.sql   *.dump
*.sqlite *.db *.ovpn *.rdp   *.pcap
```

**Built-in known filenames** (`-names` / `-all`)

| Category | Files |
|----------|-------|
| SSH | `id_rsa`, `id_dsa`, `id_ecdsa`, `id_ed25519`, `authorized_keys`, `authorized_keys2`, `known_hosts` |
| System auth | `shadow`, `passwd`, `master.passwd`, `opasswd`, `login.defs` |
| Credentials / tokens | `credentials`, `token`, `secrets`, `.env`, `.env.local`, `.env.production`, `.env.development`, `.netrc`, `.pgpass`, `.my.cnf` |
| Docker | `.docker/config.json` |
| Shell history | `.bash_history`, `.zsh_history`, `.sh_history`, `.python_history`, `.mysql_history`, `.psql_history` |
| Shell config | `.bashrc`, `.bash_profile`, `.zshrc`, `.profile` |
| Web | `.htpasswd`, `.htaccess` |
| Cloud | `.aws/credentials`, `.aws/config`, `.azure/accessTokens.json` |
| Infra / build | `Dockerfile`, `Makefile`, `Vagrantfile`, `docker-compose.yml`, `docker-compose.yaml` |
| Git | `.git-credentials`, `.gitconfig` |
| GPG | `trustdb.gpg` |
| Misc | `wp-config.php`, `LocalSettings.php` |

Path fragments (e.g. `.aws/credentials`, `.docker/config.json`) are matched against the full file path, not just the filename.

---

## Options

```
-path   string   Comma-separated root paths to scan; supports glob patterns (e.g. 'C:\Users\*\Downloads')
-ext    string   Comma-separated glob patterns, e.g. '*.txt,*.csv'
-regex  string   Regex to match filenames, e.g. '(?i)pass'
-exclude string  Comma-separated dirs to exclude; supports glob patterns (e.g. 'C:\Users\*\AppData')
-include string  Comma-separated dirs to force-include inside excluded dirs
-no-ext          Search for files without an extension
-names           Search for known sensitive filenames
-all             Enable all search modes (ext + no-ext + names)
-out    string   Write results to a file, e.g. -out results.txt
-help            Show help
```

If `-exclude` conflicts with a default force-include, the exclude wins.

---

## Examples

```bash
# Default scan — all modes, OS-default paths
./FapFinder

# Regex only — filenames containing "pass", "secret", or "token"
./FapFinder -regex '(?i)(pass|secret|token)'

# Custom extensions + extensionless files
./FapFinder -ext '*.pem,*.key' -no-ext

# Custom paths, known filenames only
./FapFinder -path /opt,/srv -names

# Regex combined with glob
./FapFinder -regex '.*\.db$' -ext '*.sql'

# Exclude a dir and save results to file
./FapFinder -exclude 'C:\Temp' -out results.txt

# Force-include a subdir inside an excluded dir
./FapFinder -exclude /etc -include /etc/nginx

# Scan Downloads for every user on Windows (glob path)
./FapFinder -path 'C:\Users\*\Downloads'

# Exclude AppData for every user (glob exclude)
./FapFinder -exclude 'C:\Users\*\AppData'

# Full Windows scan, all modes, save output
./FapFinder -path 'C:\' -exclude 'C:\Windows,C:\Program Files,C:\Program Files (x86)' -all -out loot.txt
```

---

Built with ❤️ in Go.  
Disrespectfully cross-platform.
