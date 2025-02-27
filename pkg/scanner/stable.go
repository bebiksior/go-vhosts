package scanner

import (
	"go-vhosts/pkg/utils"
	"slices"
)

type BaselineResponse struct {
	StatusCodes   []int
	Titles        []string
	Bodies        []string
	RandomVHosts  []string
	RandomResults []Response
}

func (s *Scanner) learnBaseline(target string) BaselineResponse {
	targetHost := utils.GetHostFromURL(target)

	randomVHosts := []string{
		"testing.com",
		utils.GenerateRandomString(10) + "." + targetHost,
		utils.GenerateRandomString(5) + "." + utils.GenerateRandomString(5) + ".com",
	}

	var randomResults []Response
	var statusCodes []int
	var titles []string
	var bodies []string

	for _, vhost := range randomVHosts {
		resp, err := s.sendRequest(target, vhost)
		if err != nil {
			continue
		}

		randomResults = append(randomResults, resp)

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

	baseline := BaselineResponse{
		StatusCodes:   statusCodes,
		Titles:        titles,
		Bodies:        bodies,
		RandomVHosts:  randomVHosts,
		RandomResults: randomResults,
	}

	return baseline
}

func (s *Scanner) isHostUnstable(target string) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.UnstableHosts[target]
}
