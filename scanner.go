package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/sirupsen/logrus"
)

type Scanner struct {
	hostsFile    string
	wordlistFile string
	outputFile   string
	inputFile    string
	concurrency  int
	log          *logrus.Logger
	client       *http.Client
	results      []ScanResult
	shadows      []ShadowResult
	mu           sync.Mutex
	hostBar      *progressbar.ProgressBar
}

type ScanResult struct {
	Host   string   `json:"host"`
	VHosts []string `json:"vhosts"`
}

type BaselineResult struct {
	statusCodes    map[int]int
	contentLengths map[int64]int
}

type ShadowResult struct {
	Host         string   `json:"host"`
	ShadowVHosts []string `json:"shadow_vhosts"`
}

func NewScanner(hostsFile, wordlistFile, outputFile string, concurrency int, log *logrus.Logger) *Scanner {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:    100,
		IdleConnTimeout: 10 * time.Second,
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &Scanner{
		hostsFile:    hostsFile,
		wordlistFile: wordlistFile,
		outputFile:   outputFile,
		concurrency:  concurrency,
		log:          log,
		client:       client,
		results:      make([]ScanResult, 0),
		shadows:      make([]ShadowResult, 0),
	}
}

func (s *Scanner) Start() error {
	if err := s.saveResults(); err != nil {
		return fmt.Errorf("failed to initialize results file: %v", err)
	}

	hosts, err := s.readLines(s.hostsFile)
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %v", err)
	}
	s.log.Debugf("Read hosts: %v", hosts)

	wordlist, err := s.readLines(s.wordlistFile)
	if err != nil {
		return fmt.Errorf("failed to read wordlist file: %v", err)
	}
	s.log.Debugf("Read wordlist: %v", wordlist)

	s.hostBar = progressbar.NewOptions(len(hosts),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(15),
		progressbar.OptionSetDescription("Scanning hosts"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, s.concurrency)

	for _, host := range hosts {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(host string) {
			defer wg.Done()
			defer func() {
				<-semaphore
				s.hostBar.Add(1)
			}()

			s.scanHost(host, wordlist)
		}(host)
	}

	wg.Wait()
	fmt.Println()
	return nil
}

func (s *Scanner) scanHost(host string, wordlist []string) {
	s.log.Debugf("Starting scan for host: %s with wordlist: %v", host, wordlist)

	baseDomain := extractDomain(host)
	if baseDomain == "" {
		s.log.Warnf("Could not extract domain from host: %s", host)
		return
	}

	baseline := s.performLearningPhase(host)
	if baseline == nil {
		s.log.Warnf("Learning phase failed for %s, skipping...", host)
		return
	}

	s.log.Debugf("Baseline results - Status codes: %v, Content lengths: %v",
		baseline.statusCodes, baseline.contentLengths)

	result := ScanResult{
		Host:   host,
		VHosts: make([]string, 0),
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10)
	resultsChan := make(chan string, len(wordlist))

	for _, word := range wordlist {
		wg.Add(1)
		semaphore <- struct{}{}

		var vhost string
		if strings.Contains(word, ".") {
			vhost = word
		} else {
			vhost = fmt.Sprintf("%s.%s", word, baseDomain)
		}

		go func(vhost string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			s.log.Debugf("Testing vhost: %s against host: %s", vhost, host)

			if s.checkVHost(host, vhost, baseline) {
				resultsChan <- vhost
			}
		}(vhost)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	for subdomain := range resultsChan {
		result.VHosts = append(result.VHosts, subdomain)
	}

	s.mu.Lock()
	s.results = append(s.results, result)
	if err := s.saveResults(); err != nil {
		s.log.Warnf("Failed to save results for host %s: %v", host, err)
	}
	s.mu.Unlock()
}

func (s *Scanner) performLearningPhase(host string) *BaselineResult {
	baseline := &BaselineResult{
		statusCodes:    make(map[int]int),
		contentLengths: make(map[int64]int),
	}

	baseDomain := extractDomain(host)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			randomHost := fmt.Sprintf("random%d-%d.%s", index, rand.Int(), baseDomain)
			resp, err := s.makeRequest(host, randomHost)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return
			}

			contentLength := int64(len(body))

			mu.Lock()
			baseline.statusCodes[resp.StatusCode]++
			baseline.contentLengths[contentLength]++
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	if len(baseline.statusCodes) == 0 || len(baseline.contentLengths) == 0 {
		return nil
	}

	return baseline
}

func (s *Scanner) checkVHost(targetURL, vhost string, baseline *BaselineResult) bool {
	resp, err := s.makeRequest(targetURL, vhost)
	if err != nil {
		s.log.Debugf("Request failed for vhost %s: %v", vhost, err)
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.log.Debugf("Failed to read body for vhost %s: %v", vhost, err)
		return false
	}

	bodyStr := string(body)
	s.log.Debugf("Response for %s - Status: %d, Body length: %d, Body preview: %.100s...",
		vhost, resp.StatusCode, len(body), bodyStr)

	if strings.Contains(string(body), "<TITLE>Access Denied</TITLE>") && resp.StatusCode == 403 && strings.Contains(string(body), "errors&#46;edgesuite&#46;net") {
		return false
	}

	if len(baseline.statusCodes) > 1 {
		s.log.Debugf("Status code is unstable for %s, ignoring status code checks", targetURL)
	} else {
		if _, exists := baseline.statusCodes[resp.StatusCode]; !exists {
			s.log.Debugf("Found different status code for %s: %d", vhost, resp.StatusCode)
			return true
		}
	}

	contentLength := int64(len(body))
	if len(baseline.contentLengths) > 1 {
		s.log.Debugf("Content length is unstable for %s, ignoring content length checks", targetURL)
	} else {
		if _, exists := baseline.contentLengths[contentLength]; !exists {
			s.log.Debugf("Found different content length for %s: %d", vhost, contentLength)
			return true
		}
	}

	return false
}

func (s *Scanner) makeRequest(targetURL, vhost string) (*http.Response, error) {
	s.log.Debugf("Making request - URL: %s, Host header: %s", targetURL, vhost)

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		s.log.Debugf("Failed to create request: %v", err)
		return nil, err
	}

	req.Host = vhost
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "close")

	resp, err := s.client.Do(req)
	if err != nil {
		s.log.Debugf("Request failed: %v", err)
		return nil, err
	}

	s.log.Debugf("Got response - Status: %d, Headers: %v", resp.StatusCode, resp.Header)
	return resp, nil
}

func (s *Scanner) readLines(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func (s *Scanner) saveResults() error {
	dir := filepath.Dir(s.outputFile)
	tmpFile, err := os.CreateTemp(dir, "vhosts-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	tmpName := tmpFile.Name()

	encoder := json.NewEncoder(tmpFile)
	encoder.SetIndent("", "  ")

	var data interface{}
	if len(s.shadows) > 0 {
		data = s.shadows
	} else {
		data = s.results
	}

	if err := encoder.Encode(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpName)
		return fmt.Errorf("failed to encode results: %v", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to close temp file: %v", err)
	}

	if err := os.Rename(tmpName, s.outputFile); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to rename temp file: %v", err)
	}

	return nil
}

func extractDomain(urlStr string) string {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}
	return parsedURL.Hostname()
}

func (s *Scanner) StartShadow() error {
	if err := s.saveResults(); err != nil {
		return fmt.Errorf("failed to initialize results file: %v", err)
	}

	data, err := os.ReadFile(s.inputFile)
	if err != nil {
		return fmt.Errorf("failed to read input file: %v", err)
	}

	var results []ScanResult
	if err := json.Unmarshal(data, &results); err != nil {
		return fmt.Errorf("failed to parse input JSON: %v", err)
	}

	totalHosts := len(results)
	s.hostBar = progressbar.NewOptions(totalHosts,
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(15),
		progressbar.OptionSetDescription("Checking shadow vhosts"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, s.concurrency)

	for _, result := range results {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(result ScanResult) {
			defer wg.Done()
			defer func() {
				<-semaphore
				s.hostBar.Add(1)
			}()

			s.checkShadowVHosts(result)
		}(result)
	}

	wg.Wait()
	fmt.Println()

	return s.saveResults()
}

func (s *Scanner) checkShadowVHosts(result ScanResult) {
	var shadowVHosts []string

	for _, vhost := range result.VHosts {
		if !s.isVHostDirectlyAccessible(vhost) {
			shadowVHosts = append(shadowVHosts, vhost)
		}
	}

	if len(shadowVHosts) > 0 {
		s.mu.Lock()
		s.shadows = append(s.shadows, ShadowResult{
			Host:         result.Host,
			ShadowVHosts: shadowVHosts,
		})
		if err := s.saveResults(); err != nil {
			s.log.Warnf("Failed to save shadow results for host %s: %v", result.Host, err)
		}
		s.mu.Unlock()
	}
}

func (s *Scanner) isVHostDirectlyAccessible(vhost string) bool {
	httpsURL := fmt.Sprintf("https://%s", vhost)
	if s.canConnect(httpsURL) {
		return true
	}

	httpURL := fmt.Sprintf("http://%s", vhost)
	return s.canConnect(httpURL)
}

func (s *Scanner) canConnect(urlStr string) bool {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return false
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "close")

	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		ForceAttemptHTTP2: false,
		MaxIdleConns:      100,
		IdleConnTimeout:   10 * time.Second,
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		if urlErr, ok := err.(*url.Error); ok {
			if urlErr.Timeout() || strings.Contains(err.Error(), "connection refused") ||
				strings.Contains(err.Error(), "no such host") ||
				strings.Contains(err.Error(), "cannot assign requested address") ||
				strings.Contains(err.Error(), "network is unreachable") {
				return false
			}
			return true
		}
		return true
	}
	defer resp.Body.Close()

	return true
}
