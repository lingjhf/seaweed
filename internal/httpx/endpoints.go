package httpx

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

type EndpointPolicyMode string

const (
	EndpointPolicyFailover   EndpointPolicyMode = "failover"
	EndpointPolicyRoundRobin EndpointPolicyMode = "round-robin"
)

type EndpointHealthCheckPolicy struct {
	Enabled          bool
	Interval         time.Duration
	Timeout          time.Duration
	FailureThreshold int
	SuccessThreshold int
}

type EndpointCircuitBreakerPolicy struct {
	Enabled             bool
	FailureThreshold    int
	OpenTimeout         time.Duration
	HalfOpenMaxRequests int
}

type EndpointPolicy struct {
	Mode           EndpointPolicyMode
	HealthCheck    EndpointHealthCheckPolicy
	CircuitBreaker EndpointCircuitBreakerPolicy
}

type EndpointSet struct {
	mu     sync.RWMutex
	urls   []string
	active int
	next   int
	policy EndpointPolicy
}

type EndpointCandidate struct {
	Index int
	URL   string
}

func NewEndpointSet(rawURLs []string) (*EndpointSet, error) {
	return NewEndpointSetWithPolicy(rawURLs, EndpointPolicy{})
}

func NewEndpointSetWithPolicy(rawURLs []string, policy EndpointPolicy) (*EndpointSet, error) {
	urls, err := NormalizeBaseURLs(rawURLs)
	if err != nil {
		return nil, err
	}
	policy, err = NormalizeEndpointPolicy(policy)
	if err != nil {
		return nil, err
	}
	return &EndpointSet{urls: urls, policy: policy}, nil
}

func NormalizeEndpointPolicy(policy EndpointPolicy) (EndpointPolicy, error) {
	if policy.Mode == "" {
		policy.Mode = EndpointPolicyFailover
	}
	switch policy.Mode {
	case EndpointPolicyFailover, EndpointPolicyRoundRobin:
	default:
		return EndpointPolicy{}, fmt.Errorf("httpx: unsupported endpoint policy mode %q", policy.Mode)
	}
	return policy, nil
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

func (s *EndpointSet) Candidates(path string) []EndpointCandidate {
	s.mu.Lock()
	defer s.mu.Unlock()

	candidates := make([]EndpointCandidate, 0, len(s.urls))
	start := s.active
	if s.policy.Mode == EndpointPolicyRoundRobin {
		start = s.next
		s.next = (s.next + 1) % len(s.urls)
	}
	for offset := range s.urls {
		index := (start + offset) % len(s.urls)
		candidates = append(candidates, EndpointCandidate{
			Index: index,
			URL:   s.urls[index] + path,
		})
	}
	return candidates
}

func (s *EndpointSet) MarkSuccess(index int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.policy.Mode == EndpointPolicyFailover && index >= 0 && index < len(s.urls) {
		s.active = index
	}
}
