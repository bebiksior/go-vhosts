package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func setupTestServer() *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := strings.ToLower(r.Host)
		log.Debugf("Test server received request - Host: %s, URL: %s", host, r.URL.String())

		switch host {
		case "admin.example.com":
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "Welcome to admin panel")
			log.Debugf("Responding with admin panel content")
		case "admin2.example.com":
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "Welcome to admin2 panel")
			log.Debugf("Responding with admin2 panel content")
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "Default website")
			log.Debugf("Responding with default content")
		}
	})

	server := httptest.NewTLSServer(handler)
	log.Debugf("Test server started at: %s", server.URL)
	return server
}

func TestVHostScanner(t *testing.T) {
	log.SetLevel(logrus.DebugLevel)
	log.Debug("Starting VHost Scanner test")

	server := setupTestServer()
	defer server.Close()

	time.Sleep(1 * time.Second)

	t.Logf("Test server URL: %s", server.URL)

	hostsContent := []byte(server.URL + "\n")
	wordlistContent := []byte("admin\nadmin2\nnonexistent\nadmin.example.com\nadmin2.example.com")
	outputPath := "test_output.json"

	t.Run("Scanner finds valid vhosts", func(t *testing.T) {
		hostsFile := createTempFile(t, "hosts", hostsContent)
		wordlistFile := createTempFile(t, "wordlist", wordlistContent)

		t.Logf("Created test files - Hosts: %s, Wordlist: %s", hostsFile, wordlistFile)

		// Make a test request to verify server is working
		client := server.Client()
		req, err := http.NewRequest("GET", server.URL, nil)
		if err != nil {
			t.Fatalf("Failed to create test request: %v", err)
		}
		req.Host = "admin.example.com"

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to make test request: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		t.Logf("Test request response - Status: %d, Body: %s", resp.StatusCode, string(body))

		scanner := NewScanner(hostsFile, wordlistFile, outputPath, 10, log)
		scanner.client = server.Client()

		err = scanner.Start()
		if err != nil {
			t.Fatalf("Scanner failed: %v", err)
		}

		t.Logf("Scanner results: %+v", scanner.results)

		if len(scanner.results) == 0 {
			t.Fatal("No results found")
		}

		for _, result := range scanner.results {
			foundAdmin := false
			foundAdmin2 := false

			t.Logf("Checking result for host: %s", result.Host)
			t.Logf("Found vhosts: %v", result.VHosts)

			for _, vhost := range result.VHosts {
				switch vhost {
				case "admin.example.com":
					foundAdmin = true
				case "admin2.example.com":
					foundAdmin2 = true
				}
			}

			if !foundAdmin {
				t.Error("Failed to find admin.example.com vhost")
			}
			if !foundAdmin2 {
				t.Error("Failed to find admin2.example.com vhost")
			}
		}
	})
}

func createTempFile(t *testing.T, prefix string, content []byte) string {
	t.Helper()
	tmpfile, err := os.CreateTemp("", prefix)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	if _, err := tmpfile.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}

	if err := tmpfile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Schedule cleanup
	t.Cleanup(func() {
		os.Remove(tmpfile.Name())
	})

	return tmpfile.Name()
}
