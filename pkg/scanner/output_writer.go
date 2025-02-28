package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type OutputWriter struct {
	filePath      string
	fileMutex     sync.Mutex
	enabled       bool
	file          *os.File
	targetEntries map[string]bool
}

type VHostResult struct {
	VHost         string `json:"vhost"`
	StatusCode    int    `json:"status_code"`
	Title         string `json:"title"`
	ContentLength int64  `json:"content_length"`
	IsAccessible  bool   `json:"is_accessible"`
}

type TargetResult struct {
	Target string        `json:"target"`
	VHosts []VHostResult `json:"vhosts"`
}

func NewOutputWriter(filePath string) (*OutputWriter, error) {
	if filePath == "" {
		return &OutputWriter{enabled: false}, nil
	}

	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}

	return &OutputWriter{
		filePath:      filePath,
		enabled:       true,
		file:          file,
		targetEntries: make(map[string]bool),
	}, nil
}

func (w *OutputWriter) WriteResults(target string, results []SessionResult) error {
	if !w.enabled || len(results) == 0 {
		return nil
	}

	w.fileMutex.Lock()
	defer w.fileMutex.Unlock()

	if w.file == nil {
		return fmt.Errorf("output file is not open")
	}

	var vhostResults []VHostResult
	for _, result := range results {
		if !result.IsVHost {
			continue
		}

		vhostResults = append(vhostResults, VHostResult{
			VHost:         result.VHost,
			StatusCode:    result.Response.StatusCode,
			Title:         result.Response.Title,
			ContentLength: int64(result.Response.ContentLength),
			IsAccessible:  result.IsAccessible,
		})
	}

	if len(vhostResults) == 0 {
		return nil
	}

	targetResult := TargetResult{
		Target: target,
		VHosts: vhostResults,
	}

	resultJSON, err := json.Marshal(targetResult)
	if err != nil {
		return fmt.Errorf("failed to marshal target result: %w", err)
	}

	if _, err := w.file.Write(resultJSON); err != nil {
		return fmt.Errorf("failed to write target result: %w", err)
	}

	if _, err := w.file.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	w.targetEntries[target] = true

	return w.file.Sync()
}

func (w *OutputWriter) Close() error {
	if !w.enabled || w.file == nil {
		return nil
	}

	w.fileMutex.Lock()
	defer w.fileMutex.Unlock()

	err := w.file.Close()
	w.file = nil
	w.targetEntries = nil

	return err
}
