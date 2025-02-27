# vhosts-go

A fast virtual host scanner written in Go.

## Installation

```bash
go install github.com/bebiksior/go-vhosts@latest
```

## Usage

```bash
# Basic usage
vhosts-go -u https://example.com -w wordlist.txt

# Scan multiple targets from a file
vhosts-go -l targets.txt -w wordlist.txt

# Save results to a JSON file
vhosts-go -u https://example.com -w wordlist.txt -o results.json

# Adjust concurrency
vhosts-go -u https://example.com -w wordlist.txt -t 50 -c 10
```

### Options

```
-u          Target URL (e.g., https://example.com)
-l          Path to file containing target URLs (one per line)
-w          Path to wordlist file
-t          Number of concurrent threads per target (default: 25)
-c          Number of targets to scan concurrently (default: 5)
-o          Output results to JSON file
-silent     Disable colored output
-no-progress Disable progress bar
-ua         User-Agent string (default: go-vhosts/1.0)
-proxy      Proxy URL (e.g., http://127.0.0.1:8080)
-H          Custom header in format "Name: Value" (can be used multiple times)
```
