package scanner

import (
	"go-vhosts/pkg/utils"
	"io"
	"net/http"
)

func (s *Scanner) sendRequest(target string, vhost string) (Response, error) {
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return Response{}, err
	}

	req.Host = vhost
	req.Header.Set("User-Agent", s.Options.UserAgent)

	for key, value := range s.Options.CustomHeaders {
		req.Header.Set(key, value)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}

	bodyString := string(bodyBytes)
	title := utils.ExtractTitle(bodyString)

	return Response{
		StatusCode: resp.StatusCode,
		Body:       bodyString,
		Title:      title,
		Length:     len(bodyBytes),
	}, nil
}
