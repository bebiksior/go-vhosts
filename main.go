package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

var (
	hostsFile    string
	wordlistFile string
	outputFile   string
	inputFile    string
	debug        bool
	concurrency  int
	mode         string
	log          *logrus.Logger
)

func init() {
	// Initialize logger
	log = logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})
}

func main() {
	// Parse command line flags
	flag.StringVar(&hostsFile, "h", "", "Path to hosts file (required for discover mode)")
	flag.StringVar(&wordlistFile, "w", "", "Path to wordlist file (required for discover mode)")
	flag.StringVar(&outputFile, "o", "", "Path to output file (required)")
	flag.StringVar(&inputFile, "i", "", "Path to input file (required for shadow mode)")
	flag.BoolVar(&debug, "debug", false, "Enable debug output")
	flag.IntVar(&concurrency, "c", 10, "Number of concurrent requests")
	flag.Parse()

	// Get the mode from remaining arguments
	args := flag.Args()
	if len(args) != 1 || (args[0] != "discover" && args[0] != "shadow") {
		fmt.Println("Usage:")
		fmt.Println("  Discover mode: ./vhosts-go -h hosts.txt -w wordlist.txt -o output.json -c 10 discover")
		fmt.Println("  Shadow mode: ./vhosts-go -i input.json -o shadows.json -c 10 shadow")
		os.Exit(1)
	}
	mode = args[0]

	// Validate required flags based on mode
	if mode == "discover" {
		if hostsFile == "" || wordlistFile == "" || outputFile == "" {
			fmt.Println("Error: hosts file (-h), wordlist file (-w), and output file (-o) are required for discover mode")
			flag.Usage()
			os.Exit(1)
		}
	} else if mode == "shadow" {
		if inputFile == "" || outputFile == "" {
			fmt.Println("Error: input file (-i) and output file (-o) are required for shadow mode")
			flag.Usage()
			os.Exit(1)
		}
	}

	// Set log level based on debug flag
	if debug {
		log.SetLevel(logrus.DebugLevel)
	} else {
		log.SetLevel(logrus.InfoLevel)
	}

	// Create scanner with appropriate configuration
	scanner := NewScanner(hostsFile, wordlistFile, outputFile, concurrency, log)
	scanner.inputFile = inputFile

	// Run the appropriate mode
	var err error
	if mode == "discover" {
		err = scanner.Start()
	} else {
		err = scanner.StartShadow()
	}

	if err != nil {
		log.Fatalf("Scanning failed: %v", err)
	}
}
