package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bebiksior/go-vhosts/pkg/scanner"
)

var args struct {
	targets          string
	targetsList      string
	wordlist         string
	threads          int
	concurrentVHosts int
	verbose          bool
	internal         bool
	outputFile       string
	minimal          bool
}

func main() {
	flag.StringVar(&args.targets, "u", "", "Comma-separated list of targets to scan")
	flag.StringVar(&args.targetsList, "l", "", "Path to file containing targets (one per line)")
	flag.StringVar(&args.wordlist, "w", "", "Path to file containing vhosts (one per line)")
	flag.IntVar(&args.threads, "t", 3, "Number of concurrent target scans")
	flag.IntVar(&args.concurrentVHosts, "c", 5, "Number of concurrent vhost checks per target")
	flag.BoolVar(&args.verbose, "verbose", false, "Enable verbose output")
	flag.BoolVar(&args.internal, "internal", false, "Filter wordlist to only include internal hosts")
	flag.StringVar(&args.outputFile, "o", "", "Path to save JSON results (one result per line)")
	flag.BoolVar(&args.minimal, "minimal", false, "Skip similarity comparison for faster scanning with less CPU usage")
	flag.Parse()

	if args.targets == "" && args.targetsList == "" {
		fmt.Println("Error: either -u or -l parameter is required")
		flag.Usage()
		os.Exit(1)
	}

	var targets []string
	if args.targets != "" {
		targets = strings.Split(args.targets, ",")
	} else {
		content, err := os.ReadFile(args.targetsList)
		if err != nil {
			fmt.Printf("Error reading targets file: %v\n", err)
			os.Exit(1)
		}
		targets = strings.Split(strings.TrimSpace(string(content)), "\n")
	}

	var wordlist []string

	if args.wordlist != "" {
		content, err := os.ReadFile(args.wordlist)
		if err != nil {
			fmt.Printf("Error reading wordlist file: %v\n", err)
			os.Exit(1)
		}
		wordlist = strings.Split(strings.TrimSpace(string(content)), "\n")
	} else {
		fmt.Println("Error: wordlist parameter is required")
		flag.Usage()
		os.Exit(1)
	}

	scannerInstance := scanner.NewScanner(
		targets,
		wordlist,
		scanner.ScannerOptions{
			Threads:          args.threads,
			ConcurrentVHosts: args.concurrentVHosts,
			Verbose:          args.verbose,
			Internal:         args.internal,
			OutputFile:       args.outputFile,
			Minimal:          args.minimal,
		},
	)
	defer scannerInstance.Close()

	if args.internal {
		scannerInstance.RemoveNonInternalHosts()
	}

	scannerInstance.Scan()
}
