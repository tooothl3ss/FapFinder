# FapFinder
![badge](https://img.shields.io/badge/Built%20for-degenerates-critical?style=flat-square&logo=hell) ![NSFW](https://img.shields.io/badge/NSFW-yes-critical?style=flat-square) ![Badge](https://img.shields.io/badge/Senpai-approves-blueviolet?style=flat-square)

> A shameless file extension scanner with needs.  
> He doesn't search for files. He *craves* them.

FapFinder crawls your filesystem in search of files with extensions it finds... exciting.  
Whether it's `.docx`, `.jpg`, or `.exe`, just tell FapFinder what you're into.  

It comes with a few hardcoded preferences but isn’t judgmental — you can always specify your own.  
Fully configurable, delightfully inappropriate, and dead simple to use.

**Usage:**  

`./FapFinder [options]`

**Options:**

- `-ext string`  Comma separated list of file patterns to search for.   
- `-path string` Root path to start scanning from
- `-h` Show help message

**Examples:**

`.\FapFinder.exe -ext "*.xls,*.xlsx,*.doc,*.docx"`

`.\FapFinder.exe -path "C:\\"`

`.\FapFinder.exe -path "C:\\" -ext "*.xls,*.xlsx,*.doc,*.docx"`

By default, FapFinder recursively scans from the **current directory** on Linux and from **`C:\Users\`** on Windows.  
You can override this using the `-path` option.

---
Built with ❤️ in Go.  
Disrespectfully cross-platform.
