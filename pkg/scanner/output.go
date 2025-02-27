package scanner

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fatih/color"
)

func (s *Scanner) SaveResultsIncremental(results []ScanResult) error {
	if s.OutputFile == "" {
		return nil
	}

	if !s.Options.Silent && !s.Options.NoProgress && s.progressBar != nil {
		s.progressBar.Clear()
	}

	type VHostInfo struct {
		VHost      string `json:"vhost"`
		StatusCode int    `json:"status_code"`
		Title      string `json:"title"`
		Length     int    `json:"content_length"`
	}

	type TargetResult struct {
		Target string      `json:"target"`
		VHosts []VHostInfo `json:"vhosts"`
	}

	targetMap := make(map[string]*TargetResult)

	var existingTargets []TargetResult
	existingData, err := os.ReadFile(s.OutputFile)
	if err == nil && len(existingData) > 0 {
		if err := json.Unmarshal(existingData, &existingTargets); err == nil {
			for i := range existingTargets {
				targetMap[existingTargets[i].Target] = &existingTargets[i]
			}
		}
	}

	existingVhosts := make(map[string]bool)
	for _, target := range targetMap {
		for _, vhost := range target.VHosts {
			key := fmt.Sprintf("%s:%s", target.Target, vhost.VHost)
			existingVhosts[key] = true
		}
	}

	for _, result := range results {
		if s.UnstableHosts[result.Target] || !result.IsVHost {
			continue
		}

		key := fmt.Sprintf("%s:%s", result.Target, result.VHost)
		if existingVhosts[key] {
			continue
		}

		if _, exists := targetMap[result.Target]; !exists {
			targetMap[result.Target] = &TargetResult{
				Target: result.Target,
				VHosts: []VHostInfo{},
			}
		}

		vhostInfo := VHostInfo{
			VHost:      result.VHost,
			StatusCode: result.Response.StatusCode,
			Title:      result.Response.Title,
			Length:     result.Response.Length,
		}

		targetMap[result.Target].VHosts = append(targetMap[result.Target].VHosts, vhostInfo)
		existingVhosts[key] = true
	}

	var outputResults []TargetResult
	for _, target := range targetMap {
		if len(target.VHosts) > 0 {
			outputResults = append(outputResults, *target)
		}
	}

	jsonData, err := json.MarshalIndent(outputResults, "", "  ")
	if err != nil {
		return fmt.Errorf("error creating JSON output: %s", err)
	}

	err = os.WriteFile(s.OutputFile, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("error writing to output file: %s", err)
	}

	if !s.Options.Silent && !s.Options.NoProgress && s.progressBar != nil {
		s.progressBar.RenderBlank()
	}

	return nil
}

func (s *Scanner) printResult(result ScanResult) {
	if !result.IsVHost {
		return
	}

	if !s.Options.Silent && !s.Options.NoProgress && s.progressBar != nil {
		s.progressBar.Clear()
	}

	var resultStr string
	if s.Options.Silent {
		resultStr = fmt.Sprintf("[%d] %s - %s - %s",
			result.Response.StatusCode,
			result.Target,
			result.VHost,
			result.Response.Title,
		)
	} else {
		statusColor := color.New(color.FgGreen)
		if result.Response.StatusCode >= 400 {
			statusColor = color.New(color.FgRed)
		} else if result.Response.StatusCode >= 300 {
			statusColor = color.New(color.FgYellow)
		}

		resultStr = fmt.Sprintf("%s - %s [%s] [%s]",
			color.YellowString(result.Target),
			color.CyanString(result.VHost),
			statusColor.Sprintf("%d", result.Response.StatusCode),
			color.WhiteString(result.Response.Title),
		)
	}

	fmt.Println(resultStr)

	if !s.Options.Silent && !s.Options.NoProgress && s.progressBar != nil {
		s.progressBar.RenderBlank()
	}
}
