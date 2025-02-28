package scanner

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"strings"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
)

func NormalizeURL(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return ""
	}

	return u.String()
}

func ReadByLine(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open wordlist file: %w", err)
	}
	defer file.Close()

	var words []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word != "" && !strings.HasPrefix(word, "#") {
			words = append(words, word)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading wordlist file: %w", err)
	}

	return words, nil
}

func CalculateSimilarity(text1, text2 string) float64 {
	similarity := strutil.Similarity(text1, text2, metrics.NewJaccard())
	return similarity * 100.0
}

func GenerateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[n.Int64()]
	}
	return string(result)
}

func GetHostFromURL(targetURL string) string {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return ""
	}
	return parsedURL.Hostname()
}

func ExtractTitle(body string) string {
	titleStart := strings.Index(strings.ToLower(body), "<title>")
	if titleStart == -1 {
		return ""
	}
	titleStart += 7

	titleEnd := strings.Index(strings.ToLower(body[titleStart:]), "</title>")
	if titleEnd == -1 {
		return ""
	}

	return strings.TrimSpace(body[titleStart : titleStart+titleEnd])
}
