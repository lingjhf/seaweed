package httpx

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
)

type EndpointSet struct {
	mu     sync.RWMutex
	urls   []string
	active int
}

func NewEndpointSet(rawURLs []string) (*EndpointSet, error) {
	urls, err := NormalizeBaseURLs(rawURLs)
	if err != nil {
		return nil, err
	}
	return &EndpointSet{urls: urls}, nil
}

func NormalizeBaseURLs(rawURLs []string) ([]string, error) {
	if len(rawURLs) == 0 {
		return nil, fmt.Errorf("httpx: base urls are required")
	}
	urls := make([]string, 0, len(rawURLs))
	seen := map[string]struct{}{}
	for _, raw := range rawURLs {
		normalized, err := NormalizeBaseURL(raw)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		urls = append(urls, normalized)
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("httpx: base urls are required")
	}
	return urls, nil
}

func NormalizeBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("expected absolute http url")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (s *EndpointSet) URLs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	urls := make([]string, len(s.urls))
	copy(urls, s.urls)
	return urls
}

func (s *EndpointSet) URL(path string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.urls[s.active] + path
}
