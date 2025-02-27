package scanner

import (
	"context"
	"crypto/tls"
	"fmt"
	"go-vhosts/pkg/utils"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
)

type Scanner struct {
	Options          ScanOptions
	Results          []ScanResult
	Baselines        map[string]BaselineResponse
	UnstableHosts    map[string]bool
	client           *http.Client
	mutex            sync.Mutex
	consecutiveHits  map[string]int
	consecutiveMutex sync.Mutex
	progressBar      *progressbar.ProgressBar
	OutputFile       string
	checkedTargets   map[string]bool
}

type ScanResult struct {
	Target   string
	VHost    string
	Response Response
	IsVHost  bool
}

type Response struct {
	StatusCode int
	Body       string
	Title      string
	Length     int
}

type ScanOptions struct {
	Targets         []string
	Wordlist        []string
	Threads         int
	ConcurrentHosts int
	Silent          bool
	NoProgress      bool
	UserAgent       string
	CustomHeaders   map[string]string
	Proxy           string
	AutoPilot       bool
}

func NewScanner(options ScanOptions) *Scanner {
	if options.UserAgent == "" {
		options.UserAgent = "go-vhosts/1.0"
	}

	if options.ConcurrentHosts <= 0 {
		options.ConcurrentHosts = 1
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:    100,
		IdleConnTimeout: 30 * time.Second,
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &Scanner{
		Options:         options,
		Baselines:       make(map[string]BaselineResponse),
		UnstableHosts:   make(map[string]bool),
		client:          client,
		consecutiveHits: make(map[string]int),
		checkedTargets:  make(map[string]bool),
	}
}

func (s *Scanner) isVHost(target string, response Response) bool {
	baseline, exists := s.Baselines[target]
	if !exists {
		return false
	}

	if len(baseline.RandomResults) == 0 {
		return true
	}

	if !slices.Contains(baseline.StatusCodes, response.StatusCode) {
		return true
	}

	if response.Title != "" && !slices.Contains(baseline.Titles, response.Title) {
		return true
	}

	isSignificantlyDifferent := true
	for _, baselineBody := range baseline.Bodies {
		similarity := utils.CalculateSimilarity(response.Body, baselineBody)
		if similarity > 50 {
			isSignificantlyDifferent = false
			break
		}
	}

	return isSignificantlyDifferent
}

func (s *Scanner) scanTarget(target string, resultChan chan<- ScanResult, targetWg *sync.WaitGroup, targetResults *sync.Map) {
	defer targetWg.Done()

	if !s.learnTargetBaseline(target) {
		return
	}

	targetResultsList := s.processVhostsForTarget(target, resultChan)

	s.storeAndSaveResults(target, targetResultsList, targetResults)
}

func (s *Scanner) learnTargetBaseline(target string) bool {
	baselineChan := make(chan BaselineResponse, 1)

	go func() {
		baseline := s.learnBaseline(target)
		baselineChan <- baseline
	}()

	select {
	case baseline := <-baselineChan:
		s.mutex.Lock()
		s.Baselines[target] = baseline
		s.mutex.Unlock()
	case <-time.After(30 * time.Second):
		s.mutex.Lock()
		s.UnstableHosts[target] = true
		s.mutex.Unlock()
		return false
	}

	return !s.isHostUnstable(target)
}

func (s *Scanner) processVhostsForTarget(target string, resultChan chan<- ScanResult) []ScanResult {
	var targetResultsList []ScanResult
	var targetResultsMutex sync.Mutex

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := s.monitorUnstableHost(target, cancel)
	defer close(done)

	s.scanVhostsWithWorkerPool(target, resultChan, ctx, &targetResultsList, &targetResultsMutex)

	return targetResultsList
}

func (s *Scanner) monitorUnstableHost(target string, cancel context.CancelFunc) chan struct{} {
	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if s.isHostUnstable(target) {
					cancel()
					return
				}
			case <-done:
				return
			}
		}
	}()

	return done
}

func (s *Scanner) scanVhostsWithWorkerPool(
	target string,
	resultChan chan<- ScanResult,
	ctx context.Context,
	targetResultsList *[]ScanResult,
	targetResultsMutex *sync.Mutex,
) {
	var vhostWg sync.WaitGroup
	semaphore := make(chan struct{}, s.Options.Threads)

	for _, word := range s.Options.Wordlist {
		if ctx.Err() != nil || s.isHostUnstable(target) {
			break
		}

		vhostWg.Add(1)
		semaphore <- struct{}{}

		go func(t, w string) {
			defer func() {
				<-semaphore
				vhostWg.Done()
			}()

			if ctx.Err() != nil {
				return
			}

			result := s.scanVhost(t, w, resultChan, ctx)

			if result != nil && result.IsVHost {
				targetResultsMutex.Lock()
				*targetResultsList = append(*targetResultsList, *result)
				targetResultsMutex.Unlock()
			}
		}(target, word)
	}

	vhostWg.Wait()
}
func (s *Scanner) storeAndSaveResults(target string, targetResultsList []ScanResult, targetResults *sync.Map) {
	targetResults.Store(target, targetResultsList)

	var allResults []ScanResult
	targetResults.Range(func(key, value interface{}) bool {
		if results, ok := value.([]ScanResult); ok {
			allResults = append(allResults, results...)
		}
		return true
	})

	if s.OutputFile != "" {
		if !s.Options.Silent && !s.Options.NoProgress && s.progressBar != nil {
			s.progressBar.Clear()
		}

		err := s.SaveResultsIncremental(allResults)
		if err != nil && !s.Options.Silent {
			fmt.Printf("Warning: Failed to save incremental results: %s\n", err)
		}

		if !s.Options.Silent && !s.Options.NoProgress && s.progressBar != nil {
			s.progressBar.RenderBlank()
		}
	}
}

func (s *Scanner) scanVhost(target string, vhost string, resultChan chan<- ScanResult, ctx context.Context) *ScanResult {
	if s.isHostUnstable(target) {
		s.updateProgressBar()
		return nil
	}

	select {
	case <-ctx.Done():
		s.updateProgressBar()
		return nil
	default:
	}

	response, err := s.sendRequestWithTimeout(target, vhost, ctx)
	if err != nil {
		s.updateProgressBar()
		return nil
	}

	result := s.processVhostResponse(target, vhost, response)

	s.updateProgressBar()

	select {
	case resultChan <- result:
	case <-ctx.Done():
		return nil
	}

	return &result
}

func (s *Scanner) sendRequestWithTimeout(target string, vhost string, ctx context.Context) (Response, error) {
	respChan := make(chan Response, 1)
	errChan := make(chan error, 1)

	go func() {
		resp, err := s.sendRequest(target, vhost)
		if err != nil {
			errChan <- err
			return
		}
		respChan <- resp
	}()

	select {
	case response := <-respChan:
		return response, nil
	case err := <-errChan:
		return Response{}, err
	case <-time.After(10 * time.Second):
		return Response{}, fmt.Errorf("request timed out")
	case <-ctx.Done():
		return Response{}, fmt.Errorf("request cancelled")
	}
}

func (s *Scanner) processVhostResponse(target string, vhost string, response Response) ScanResult {
	isVHost := s.isVHost(target, response)

	s.onVHostHit(target, isVHost, vhost)

	result := ScanResult{
		Target:   target,
		VHost:    vhost,
		Response: response,
		IsVHost:  isVHost,
	}

	if isVHost {
		s.mutex.Lock()
		s.printResult(result)
		s.mutex.Unlock()
	}

	return result
}

func (s *Scanner) updateProgressBar() {
	if !s.Options.Silent && !s.Options.NoProgress && s.progressBar != nil {
		s.mutex.Lock()
		s.progressBar.Add(1)
		s.mutex.Unlock()
	}
}

func (s *Scanner) Start() []ScanResult {
	resultChan := make(chan ScanResult)
	results := []ScanResult{}

	s.initializeProgressBar()

	collectorDone := s.startResultCollector(resultChan, &results)

	s.processTargetsWithConcurrency(resultChan)

	close(resultChan)
	<-collectorDone

	if !s.Options.Silent && !s.Options.NoProgress && s.progressBar != nil {
		s.progressBar.Finish()
	}

	return s.filterDiscoveredVhosts(results)
}

func (s *Scanner) initializeProgressBar() {
	totalVhosts := len(s.Options.Targets) * len(s.Options.Wordlist)

	if !s.Options.Silent && !s.Options.NoProgress {
		s.progressBar = progressbar.NewOptions(
			totalVhosts,
			progressbar.OptionSetDescription("Scanning vhosts..."),
			progressbar.OptionSetWidth(50),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
			progressbar.OptionSetItsString("vhosts"),
			progressbar.OptionClearOnFinish(),
			progressbar.OptionOnCompletion(func() {
				fmt.Println()
			}),
		)
	}
}

func (s *Scanner) startResultCollector(resultChan <-chan ScanResult, results *[]ScanResult) chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)
		for result := range resultChan {
			s.mutex.Lock()
			*results = append(*results, result)
			s.mutex.Unlock()
		}
	}()

	return done
}

func (s *Scanner) processTargetsWithConcurrency(resultChan chan<- ScanResult) {
	var targetWg sync.WaitGroup
	targetSemaphore := make(chan struct{}, s.Options.ConcurrentHosts)
	targetResults := &sync.Map{}

	for _, target := range s.Options.Targets {
		targetWg.Add(1)
		targetSemaphore <- struct{}{}

		go func(t string) {
			s.scanTarget(t, resultChan, &targetWg, targetResults)
			<-targetSemaphore
		}(target)
	}

	targetWg.Wait()
}

func (s *Scanner) filterDiscoveredVhosts(results []ScanResult) []ScanResult {
	var discoveredResults []ScanResult

	for _, result := range results {
		if result.IsVHost {
			discoveredResults = append(discoveredResults, result)
		}
	}

	return discoveredResults
}
