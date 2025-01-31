# VHosts-Go

A fast and concurrent virtual host scanner written in Go. This tool helps discover virtual hosts (vhosts) by sending HTTP requests with different Host headers and analyzing the responses.

## Features

- Fast concurrent scanning with rate limiting
- Automatic learning phase to detect baseline behavior
- Smart detection of unstable responses
- Colored output with detailed logging
- JSON output format
- Support for both HTTP and HTTPS
- Configurable concurrency

## Installation

```bash
go install github.com/yourusername/vhosts-go@latest
```

Or build from source:

```bash
git clone https://github.com/yourusername/vhosts-go.git
cd vhosts-go
go build
```

## Usage

```bash
./vhosts-go -h hosts.txt -w wordlist.txt -o output.txt [--debug]
```

### Parameters

- `-h`: Path to hosts file (one host per line)
- `-w`: Path to wordlist file (subdomains to test)
- `-o`: Output file path (JSON format)
- `--debug`: Enable debug output

### Input File Formats

hosts.txt:
```
https://example.com
https://another-domain.com
```

wordlist.txt:
```
admin
dev
staging
test
```

### Output Format

The tool generates a JSON file with the following structure:

```json
[
  {
    "host": "https://example.com",
    "vhosts": [
      "admin.example.com",
      "dev.example.com"
    ]
  }
]
```

## How it Works

1. For each target host, the tool performs a learning phase by sending requests with random Host headers
2. It analyzes the responses to determine if status codes and content lengths are stable
3. For each subdomain in the wordlist, it sends a request with the subdomain as the Host header
4. If the response differs from the baseline (and the difference is reliable), it's marked as a potential virtual host
5. Results are saved in JSON format

## License

MIT License
