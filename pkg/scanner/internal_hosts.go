package scanner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
)

func (s *Scanner) RemoveNonInternalHosts() {
	var internalHosts []string
	var mutex sync.Mutex

	fmt.Println("Filtering wordlist to only include internal hosts (not directly accessible)...")
	bar := progressbar.NewOptions(len(s.Wordlist),
		progressbar.OptionSetDescription("Filtering wordlist"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionSetItsString("host"),
		progressbar.OptionShowCount(),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionEnableColorCodes(true),
	)

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, s.Options.ConcurrentVHosts)

	for _, host := range s.Wordlist {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(host string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			if !s.isVHostDirectlyAccessible(host) {
				mutex.Lock()
				internalHosts = append(internalHosts, host)
				mutex.Unlock()
			}
			bar.Add(1)
		}(host)
	}

	wg.Wait()

	originalCount := len(s.Wordlist)
	s.Wordlist = internalHosts

	s.totalVHosts = len(s.Targets) * len(s.Wordlist)

	fmt.Printf("\nFiltered wordlist from %d to %d internal hosts (%.1f%% reduction)\n",
		originalCount,
		len(internalHosts),
		100.0-(float64(len(internalHosts))/float64(originalCount)*100.0))
}

func (s *Scanner) isVHostDirectlyAccessible(vhost string) bool {
	s.cacheMutex.RLock()
	result, exists := s.accessibilityCache[vhost]
	s.cacheMutex.RUnlock()

	if exists {
		return result
	}

	ips, err := net.LookupHost(vhost)
	if err != nil {
		s.cacheMutex.Lock()
		s.accessibilityCache[vhost] = false
		s.cacheMutex.Unlock()
		return false
	}

	for _, ip := range ips {
		parsedIP := net.ParseIP(ip)
		if parsedIP.IsLoopback() || parsedIP.IsPrivate() {
			s.cacheMutex.Lock()
			s.accessibilityCache[vhost] = false
			s.cacheMutex.Unlock()
			return false
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://%s", vhost)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		s.cacheMutex.Lock()
		s.accessibilityCache[vhost] = false
		s.cacheMutex.Unlock()
		return false
	}

	req.Header.Set("User-Agent", "go-vhosts/1.0")

	resp, err := s.httpClient.Do(req)
	if err == nil {
		defer resp.Body.Close()
		result := resp.StatusCode > 0
		s.cacheMutex.Lock()
		s.accessibilityCache[vhost] = result
		s.cacheMutex.Unlock()
		return result
	}

	url = fmt.Sprintf("https://%s", vhost)
	req, err = http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		s.cacheMutex.Lock()
		s.accessibilityCache[vhost] = false
		s.cacheMutex.Unlock()
		return false
	}

	req.Header.Set("User-Agent", "go-vhosts/1.0")

	resp, err = s.httpClient.Do(req)
	if err != nil {
		s.cacheMutex.Lock()
		s.accessibilityCache[vhost] = false
		s.cacheMutex.Unlock()
		return false
	}

	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	result = resp != nil && resp.StatusCode > 0
	s.cacheMutex.Lock()
	s.accessibilityCache[vhost] = result
	s.cacheMutex.Unlock()
	return result
}
