# VHosts-Go

Fast virtual host scanner that finds hidden vhosts and identifies which ones are only accessible through Host header manipulation.

> [!NOTE]
> This tool is currently in beta


## Install
```bash
go install github.com/bebiksior/vhosts-go@latest
```

## Usage
Two modes available:

```bash
# Discover vhosts
vhosts-go -h hosts.txt -w wordlist.txt -o out.json -c 10 discover

# Find shadow vhosts (only accessible via Host header), takes output from discover mode as input
vhosts-go -i out.json -o shadows.json -c 10 shadow
```

### Flags
- `-h` - Hosts file (one per line)
- `-w` - Wordlist file
- `-o` - Output file
- `-i` - Input file (for shadow mode)
- `-c` - Concurrent requests (default: 10)
- `--debug` - Enable debug output
- `--proxy` - Proxy URL (e.g. http://127.0.0.1:8080)

### Example Output
Discover mode:
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

Shadow mode:
```json
[
  {
    "host": "https://example.com",
    "shadow_vhosts": [
      "internal.example.com",
      "staging.example.com"
    ]
  }
]
```

## Features
- Fast concurrent scanning
- Smart baseline detection
- Progress bars
- Shadow vhost detection
- JSON output
- HTTP/HTTPS support

## License
MIT
