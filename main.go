package main

import (
	"flag"
	"fmt"
	"os"

	"go-vhosts/pkg/scanner"
	"go-vhosts/pkg/utils"
)

func main() {
	// Flags
	targetURL := flag.String("u", "", "Target URL (e.g., https://example.com)")
	targetFile := flag.String("l", "", "Path to file containing target URLs (one per line)")
	wordlistPath := flag.String("w", "", "Path to wordlist file")
	threads := flag.Int("t", 25, "Number of concurrent threads per target")
	concurrentHosts := flag.Int("c", 5, "Number of targets to scan concurrently")
	silent := flag.Bool("silent", false, "Disable colored output")
	noProgress := flag.Bool("no-progress", false, "Disable progress bar")
	userAgent := flag.String("ua", "go-vhosts/1.0", "User-Agent string")
	proxy := flag.String("proxy", "", "Proxy URL (e.g., http://127.0.0.1:8080)")
	autopilot := flag.Bool("autopilot", true, "Enable autopilot mode to detect and skip unstable hosts")
	outputFile := flag.String("o", "", "Output results to JSON file")
	headers := make(utils.HeadersFlag)
	flag.Var(&headers, "H", "Custom header in format \"Name: Value\" (can be used multiple times)")

	flag.Parse()

	// Check if required flags are provided
	if (*targetURL == "" && *targetFile == "") || *wordlistPath == "" {
		fmt.Println("Error: Either Target URL (-u) or Target File (-l) must be provided along with wordlist (-w)")
		flag.Usage()
		os.Exit(1)
	}

	// Load targets
	var targets []string
	if *targetURL != "" {
		// Single target from command line
		targets = append(targets, utils.NormalizeURL(*targetURL))
	}

	if *targetFile != "" {
		// Multiple targets from file
		fileTargets, err := utils.LoadWordlist(*targetFile)
		if err != nil {
			fmt.Printf("Error loading target file: %s\n", err)
			os.Exit(1)
		}

		for _, target := range fileTargets {
			if target != "" {
				targets = append(targets, utils.NormalizeURL(target))
			}
		}
	}

	if len(targets) == 0 {
		fmt.Println("Error: No valid targets found")
		os.Exit(1)
	}

	// Load wordlist
	wordlist, err := utils.LoadWordlist(*wordlistPath)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	// Create scanner options
	options := scanner.ScanOptions{
		Targets:         targets,
		Wordlist:        wordlist,
		Threads:         *threads,
		ConcurrentHosts: *concurrentHosts,
		Silent:          *silent,
		NoProgress:      *noProgress,
		UserAgent:       *userAgent,
		CustomHeaders:   headers,
		Proxy:           *proxy,
		AutoPilot:       *autopilot,
	}

	// Create and start scanner
	scannerInstance := scanner.NewScanner(options)

	// Set output file for incremental saving
	if *outputFile != "" {
		scannerInstance.OutputFile = *outputFile
	}

	results := scannerInstance.Start()

	// Print summary
	if !*silent {
		fmt.Printf("\nScan completed. Found %d virtual hosts.\n", len(results))
		if *outputFile != "" {
			fmt.Printf("Results saved to %s\n", *outputFile)
		}
	}
}
