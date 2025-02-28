package scanner

import (
	"fmt"
	"net"
	"net/http"
	"slices"
	"sync"
)

type SessionResult struct {
	VHost        string
	Response     *SlimResponse
	IsVHost      bool
	IsAccessible bool
}

type Session struct {
	Scanner          *Scanner
	Target           string
	Results          chan SessionResult
	WaitGroup        *sync.WaitGroup
	BaselineResponse BaselineResponse
}

func NewSession(scanner *Scanner, target string) *Session {
	return &Session{
		Scanner:          scanner,
		Target:           target,
		Results:          make(chan SessionResult, 100),
		BaselineResponse: BaselineResponse{},
		WaitGroup:        &sync.WaitGroup{},
	}
}

type BaselineResponse struct {
	StatusCodes []int
	Titles      []string
	Bodies      []string
}

func (s *Session) aliveCheck() bool {
	_, err := net.LookupHost(GetHostFromURL(s.Target))
	if err != nil {
		return false
	}

	req, err := http.NewRequest("GET", s.Target, nil)
	if err != nil {
		return false
	}

	req.Header.Set("User-Agent", "go-vhosts/1.0")
	req.Header.Set("Connection", "close")

	resp, err := s.Scanner.httpClient.Do(req)
	if err != nil {
		return false
	}

	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	return resp != nil && resp.StatusCode > 0
}

func (s *Session) Scan() []SessionResult {
	if !s.aliveCheck() {
		s.Scanner.Log(fmt.Sprintf("Target %s is not alive, skipping", s.Target))

		skippedCount := len(s.Scanner.Wordlist)
		s.Scanner.UpdateProgress(skippedCount)

		if s.Results != nil {
			close(s.Results)
		}
		return []SessionResult{}
	}

	s.BaselineResponse = s.learnBaseline()

	var results []SessionResult
	resultsChan := make(chan SessionResult, len(s.Scanner.Wordlist))

	done := make(chan struct{})

	completedCount := 0
	countMutex := &sync.Mutex{}

	go func() {
		for result := range resultsChan {
			results = append(results, result)
			if s.Results != nil {
				s.Results <- result
			}
		}

		close(done)

		if s.Results != nil {
			close(s.Results)
		}
	}()

	concurrentLimit := s.Scanner.Options.ConcurrentVHosts
	if concurrentLimit <= 0 {
		concurrentLimit = 10
	}
	semaphore := make(chan struct{}, concurrentLimit)

	for _, vhost := range s.Scanner.Wordlist {
		s.WaitGroup.Add(1)
		semaphore <- struct{}{}

		go func(vhost string) {
			defer s.WaitGroup.Done()
			defer func() {
				<-semaphore

				countMutex.Lock()
				completedCount++
				s.Scanner.UpdateProgress(1)
				countMutex.Unlock()
			}()

			fullResponse, err := s.Scanner.requester.RequestVHost(s.Target, vhost)
			if err != nil {
				return
			}

			if s.isDifferent(*fullResponse) {
				isAccessible := s.Scanner.isVHostDirectlyAccessible(vhost)

				result := SessionResult{
					VHost: vhost,
					Response: &SlimResponse{
						StatusCode:    fullResponse.StatusCode,
						Title:         fullResponse.Title,
						ContentLength: fullResponse.ContentLength,
					},
					IsVHost:      true,
					IsAccessible: isAccessible,
				}

				resultsChan <- result
			}
		}(vhost)
	}

	s.WaitGroup.Wait()
	close(resultsChan)

	<-done

	if s.Scanner.Options.Verbose && len(results) > 0 {
		s.Scanner.Log(fmt.Sprintf("Found %d vhosts for %s", len(results), s.Target))
	}

	return results
}

func (s *Session) learnBaseline() BaselineResponse {
	targetHost := GetHostFromURL(s.Target)

	randomVHosts := []string{
		"testing.com",
		GenerateRandomString(10) + "." + targetHost,
		GenerateRandomString(5) + "." + GenerateRandomString(5) + ".com",
	}

	var randomResults []FullResponse
	var statusCodes []int
	var titles []string
	var bodies []string

	for _, vhost := range randomVHosts {
		resp, err := s.Scanner.requester.RequestVHost(s.Target, vhost)
		if err != nil {
			continue
		}

		randomResults = append(randomResults, *resp)

		if !slices.Contains(statusCodes, resp.StatusCode) {
			statusCodes = append(statusCodes, resp.StatusCode)
		}

		if resp.Title != "" && !slices.Contains(titles, resp.Title) {
			titles = append(titles, resp.Title)
		}

		bodies = append(bodies, resp.Body)
	}

	if len(randomResults) == 0 {
		return BaselineResponse{}
	}

	return BaselineResponse{
		StatusCodes: statusCodes,
		Titles:      titles,
		Bodies:      bodies,
	}
}

func (s *Session) isDifferent(response FullResponse) bool {
	if s.BaselineResponse.StatusCodes == nil {
		return false
	}

	if len(s.BaselineResponse.StatusCodes) == 0 {
		return true
	}

	if !slices.Contains(s.BaselineResponse.StatusCodes, response.StatusCode) {
		return true
	}

	if response.Title != "" && !slices.Contains(s.BaselineResponse.Titles, response.Title) {
		return true
	}

	isSignificantlyDifferent := true
	for _, baselineBody := range s.BaselineResponse.Bodies {
		similarity := CalculateSimilarity(response.Body, baselineBody)
		if similarity > 40 {
			isSignificantlyDifferent = false
			break
		}
	}

	return isSignificantlyDifferent
}
