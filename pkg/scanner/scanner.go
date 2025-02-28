package scanner

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
)

type Scanner struct {
	Targets  []string
	Wordlist []string
	Options  ScannerOptions

	httpClient         *http.Client
	requester          *Requester
	progressBar        *progressbar.ProgressBar
	totalVHosts        int
	outputWriter       *OutputWriter
	accessibilityCache map[string]bool
	cacheMutex         sync.RWMutex
}

type ScannerOptions struct {
	Threads          int
	ConcurrentVHosts int
	Verbose          bool
	Internal         bool
	OutputFile       string
	Minimal          bool
}

func NewScanner(targets []string, wordlist []string, options ScannerOptions) *Scanner {
	scanner := &Scanner{
		Targets:            targets,
		Wordlist:           wordlist,
		Options:            options,
		accessibilityCache: make(map[string]bool),
	}

	if scanner.Options.Threads <= 0 {
		scanner.Options.Threads = 1
	}

	scanner.requester = NewRequester(scanner)
	scanner.httpClient = scanner.requester.newHTTPClient()
	scanner.totalVHosts = len(targets) * len(wordlist)

	if options.OutputFile != "" {
		var err error
		scanner.outputWriter, err = NewOutputWriter(options.OutputFile)
		if err != nil {
			fmt.Printf("Warning: Failed to initialize output writer: %v\n", err)
		} else {
			fmt.Printf("Results will be saved to %s\n", options.OutputFile)
		}
	}

	return scanner
}

func (s *Scanner) SetOutputFile(filePath string) error {
	if s.outputWriter != nil {
		s.outputWriter.Close()
	}

	var err error
	s.outputWriter, err = NewOutputWriter(filePath)
	if err != nil {
		return fmt.Errorf("failed to set output file: %w", err)
	}

	return nil
}

func (s *Scanner) Scan() {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, s.Options.Threads)

	s.progressBar = progressbar.NewOptions(s.totalVHosts,
		progressbar.OptionSetDescription("Scanning virtual hosts"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionSetItsString("vhosts"),
		progressbar.OptionShowCount(),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionEnableColorCodes(true),
	)

	if s.Options.Internal {
		fmt.Println("Using internal hosts filter - only hosts that are NOT directly accessible will be checked")
	}

	fmt.Printf("Starting scan with %d targets and %d hostnames in wordlist (%d total vhosts)\n",
		len(s.Targets), len(s.Wordlist), s.totalVHosts)

	for _, target := range s.Targets {
		wg.Add(1)
		semaphore <- struct{}{}
		go func(target string) {
			defer wg.Done()
			defer func() { <-semaphore }()
			s.Log(fmt.Sprintf("Scanning %s", target))
			s.scanTarget(target)
		}(target)
	}

	wg.Wait()

	if s.progressBar != nil {
		s.progressBar.Clear()
		s.progressBar.Close()
	}

	fmt.Printf("\nScan completed!\n")

	if s.outputWriter != nil && s.Options.OutputFile != "" {
		fmt.Printf("Results saved to %s\n", s.Options.OutputFile)
	}
}

func (s *Scanner) scanTarget(target string) {

	session := NewSession(s, target)
	go func() {
		results := session.Scan()
		if s.outputWriter != nil {
			s.outputWriter.WriteResults(target, results)
		}
	}()

	for result := range session.Results {
		s.printResult(result, target)
	}
}

func (s *Scanner) printResult(result SessionResult, target string) {
	if result.IsVHost {
		accessibleStr := "✓"
		if !result.IsAccessible {
			accessibleStr = "✗"
		}

		var resultStr string
		statusColor := color.New(color.FgGreen)
		if result.Response.StatusCode >= 400 {
			statusColor = color.New(color.FgRed)
		} else if result.Response.StatusCode >= 300 {
			statusColor = color.New(color.FgYellow)
		}

		accessibleColor := color.New(color.FgGreen)
		if !result.IsAccessible {
			accessibleColor = color.New(color.FgRed)
		}

		resultStr = fmt.Sprintf("%s - %s [%s] [%s] [Accessible: %s]",
			color.YellowString(target),
			color.CyanString(result.VHost),
			statusColor.Sprintf("%d", result.Response.StatusCode),
			color.WhiteString(result.Response.Title),
			accessibleColor.Sprint(accessibleStr),
		)

		s.progressBar.Clear()
		fmt.Println(resultStr)
		s.progressBar.RenderBlank()
	}
}

func (s *Scanner) UpdateProgress(count int) {
	if s.progressBar != nil {
		s.progressBar.Add(count)
	}
}

func (s *Scanner) Log(message string) {
	if s.Options.Verbose {
		if s.progressBar != nil {
			s.progressBar.Clear()
		}
		fmt.Println(message)
		if s.progressBar != nil {
			s.progressBar.RenderBlank()
		}
	}
}

func (s *Scanner) Close() {
	if s.outputWriter != nil {
		s.outputWriter.Close()
	}
}
