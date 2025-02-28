package scanner

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"
)

type FullResponse struct {
	Body          string
	Title         string
	StatusCode    int
	ContentLength int
}

type SlimResponse struct {
	Title         string
	StatusCode    int
	ContentLength int
}

type Requester struct {
	Scanner *Scanner
}

func NewRequester(scanner *Scanner) *Requester {
	return &Requester{Scanner: scanner}
}

func (r *Requester) newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			DisableKeepAlives:     true,
			MaxIdleConnsPerHost:   -1,
			ResponseHeaderTimeout: 7 * time.Second,
			TLSHandshakeTimeout:   7 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (r *Requester) RequestVHost(url string, vhost string) (*FullResponse, error) {
	r.Scanner.Log(fmt.Sprintf("Requesting %s with vhost %s", url, vhost))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Host = vhost
	req.Header.Set("User-Agent", "go-vhosts/1.0")
	req.Header.Set("Connection", "close")

	resp, err := r.Scanner.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var bodyString string
	var bodyBytes []byte
	var contentLength int

	if r.Scanner.Options.Minimal {
		bodyBytes = make([]byte, 8192)
		n, _ := resp.Body.Read(bodyBytes)
		bodyBytes = bodyBytes[:n]
		bodyString = string(bodyBytes)

		if resp.ContentLength > 0 {
			contentLength = int(resp.ContentLength)
		} else {
			contentLength = n
		}
	} else {
		bodyBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		bodyString = string(bodyBytes)
		contentLength = len(bodyBytes)
		if resp.ContentLength > 0 {
			contentLength = int(resp.ContentLength)
		}
	}

	title := ExtractTitle(bodyString)

	return &FullResponse{
		Body:          bodyString,
		Title:         title,
		StatusCode:    resp.StatusCode,
		ContentLength: contentLength,
	}, nil
}
