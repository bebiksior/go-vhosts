package scanner

import (
	"strings"

	"github.com/bebiksior/go-vhosts/pkg/utils"
)

func (s *Scanner) onVHostHit(target string, isVHost bool, vhost string) {
	if !s.Options.AutoPilot {
		return
	}

	if !isVHost {
		s.consecutiveMutex.Lock()
		s.consecutiveHits[target] = 0
		s.consecutiveMutex.Unlock()
		return
	}

	s.mutex.Lock()
	alreadyChecked := s.checkedTargets[target]
	s.mutex.Unlock()

	if !alreadyChecked {
		s.mutex.Lock()
		s.checkedTargets[target] = true
		s.mutex.Unlock()

		isUnstable := s.checkSimilarPatternVhost(target, vhost)
		if isUnstable {
			s.mutex.Lock()
			s.UnstableHosts[target] = true
			s.mutex.Unlock()
			return
		}
	}

	s.consecutiveMutex.Lock()
	s.consecutiveHits[target]++
	hitCount := s.consecutiveHits[target]
	s.consecutiveMutex.Unlock()

	if hitCount == 3 {
		isUnstable := s.verifyUnstableHost(target)
		if isUnstable {
			s.mutex.Lock()
			s.UnstableHosts[target] = true
			s.mutex.Unlock()
		}
	}
}

func (s *Scanner) checkSimilarPatternVhost(target string, vhost string) bool {
	var similarVhost string
	parts := strings.Split(vhost, ".")
	if len(parts) >= 2 {
		randomStr := utils.GenerateRandomString(8)
		domainParts := parts[1:]
		similarVhost = parts[0] + randomStr + "." + strings.Join(domainParts, ".")
	} else {
		similarVhost = utils.GenerateRandomString(8) + "." + vhost
	}

	resp, err := s.sendRequest(target, similarVhost)
	if err != nil {
		return false
	}

	isVHost := s.isVHost(target, resp)

	return isVHost
}

func (s *Scanner) verifyUnstableHost(target string) bool {
	randomVHosts := []string{
		utils.GenerateRandomString(12) + ".com",
		utils.GenerateRandomString(8) + "." + utils.GenerateRandomString(6) + ".org",
		utils.GenerateRandomString(10) + "." + utils.GenerateRandomString(7) + ".net",
	}

	vhostCount := 0
	for _, vhost := range randomVHosts {
		resp, err := s.sendRequest(target, vhost)
		if err != nil {
			continue
		}

		if s.isVHost(target, resp) {
			vhostCount++
		}
	}

	isUnstable := vhostCount >= 2
	return isUnstable
}
