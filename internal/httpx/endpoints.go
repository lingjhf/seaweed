package httpx

import (
	"context"
	"fmt"
	"net/http"
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
	states []endpointState

	closeOnce sync.Once
	closeCh   chan struct{}
}

type EndpointCandidate struct {
	Index int
	URL   string
}

type endpointState struct {
	failures         int
	successes        int
	unhealthy        bool
	openUntil        time.Time
	halfOpenRequests int
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
	return &EndpointSet{
		urls:    urls,
		policy:  policy,
		states:  make([]endpointState, len(urls)),
		closeCh: make(chan struct{}),
	}, nil
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
	if policy.HealthCheck.Enabled {
		if policy.HealthCheck.Interval == 0 {
			policy.HealthCheck.Interval = 30 * time.Second
		}
		if policy.HealthCheck.Timeout == 0 {
			policy.HealthCheck.Timeout = 2 * time.Second
		}
		if policy.HealthCheck.FailureThreshold == 0 {
			policy.HealthCheck.FailureThreshold = 1
		}
		if policy.HealthCheck.SuccessThreshold == 0 {
			policy.HealthCheck.SuccessThreshold = 1
		}
	}
	if policy.CircuitBreaker.Enabled {
		if policy.CircuitBreaker.FailureThreshold == 0 {
			policy.CircuitBreaker.FailureThreshold = 3
		}
		if policy.CircuitBreaker.OpenTimeout == 0 {
			policy.CircuitBreaker.OpenTimeout = 30 * time.Second
		}
		if policy.CircuitBreaker.HalfOpenMaxRequests == 0 {
			policy.CircuitBreaker.HalfOpenMaxRequests = 1
		}
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
	halfOpen := make([]EndpointCandidate, 0, len(s.urls))
	now := time.Now()
	start := s.active
	if s.policy.Mode == EndpointPolicyRoundRobin {
		start = s.next
		s.next = (s.next + 1) % len(s.urls)
	}
	for offset := range s.urls {
		index := (start + offset) % len(s.urls)
		if s.skipLocked(index, now) {
			continue
		}
		candidate := EndpointCandidate{
			Index: index,
			URL:   s.urls[index] + path,
		}
		if s.halfOpenLocked(index, now) {
			halfOpen = append(halfOpen, candidate)
			continue
		}
		candidates = append(candidates, candidate)
	}
	return append(halfOpen, candidates...)
}

func (s *EndpointSet) MarkSuccess(index int) {
	s.RecordSuccess(index)
}

func (s *EndpointSet) RecordSuccess(index int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.urls) {
		return
	}
	s.recordSuccessLocked(index)
}

func (s *EndpointSet) RecordFailure(index int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.urls) {
		return
	}
	s.recordFailureLocked(index, time.Now())
}

func (s *EndpointSet) beginCandidate(index int) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.urls) {
		return false, false
	}
	now := time.Now()
	if s.skipLocked(index, now) {
		return false, false
	}
	if !s.halfOpenLocked(index, now) {
		return true, false
	}
	state := &s.states[index]
	if state.halfOpenRequests >= s.policy.CircuitBreaker.HalfOpenMaxRequests {
		return false, false
	}
	state.halfOpenRequests++
	return true, true
}

func (s *EndpointSet) finishCandidate(index int, halfOpen bool, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.urls) {
		return
	}
	if halfOpen && s.states[index].halfOpenRequests > 0 {
		s.states[index].halfOpenRequests--
	}
	if success {
		s.recordSuccessLocked(index)
		return
	}
	s.recordFailureLocked(index, time.Now())
}

func (s *EndpointSet) recordSuccessLocked(index int) {
	state := &s.states[index]
	state.failures = 0
	state.successes++
	if !s.policy.HealthCheck.Enabled || state.successes >= s.policy.HealthCheck.SuccessThreshold {
		state.unhealthy = false
	}
	state.openUntil = time.Time{}
	state.halfOpenRequests = 0
	if s.policy.Mode == EndpointPolicyFailover && index >= 0 && index < len(s.urls) {
		s.active = index
	}
}

func (s *EndpointSet) recordFailureLocked(index int, now time.Time) {
	state := &s.states[index]
	state.failures++
	state.successes = 0
	if s.policy.HealthCheck.Enabled && state.failures >= s.policy.HealthCheck.FailureThreshold {
		state.unhealthy = true
	}
	if s.policy.CircuitBreaker.Enabled && state.failures >= s.policy.CircuitBreaker.FailureThreshold {
		state.openUntil = now.Add(s.policy.CircuitBreaker.OpenTimeout)
		state.halfOpenRequests = 0
	}
}

func (s *EndpointSet) StartHealthCheck(client *http.Client, method string, path string) {
	if !s.policy.HealthCheck.Enabled {
		return
	}
	if client == nil {
		client = http.DefaultClient
	}
	probeClient := *client
	probeClient.Timeout = s.policy.HealthCheck.Timeout
	ticker := time.NewTicker(s.policy.HealthCheck.Interval)
	go func() {
		defer ticker.Stop()
		s.probeAll(&probeClient, method, path)
		for {
			select {
			case <-ticker.C:
				s.probeAll(&probeClient, method, path)
			case <-s.closeCh:
				return
			}
		}
	}()
}

func (s *EndpointSet) Close() {
	s.closeOnce.Do(func() {
		close(s.closeCh)
	})
}

func (s *EndpointSet) skipLocked(index int, now time.Time) bool {
	state := s.states[index]
	if s.policy.HealthCheck.Enabled && state.unhealthy {
		return true
	}
	if !s.policy.CircuitBreaker.Enabled {
		return false
	}
	if state.openUntil.After(now) {
		return true
	}
	return !state.openUntil.IsZero() && state.halfOpenRequests >= s.policy.CircuitBreaker.HalfOpenMaxRequests
}

func (s *EndpointSet) halfOpenLocked(index int, now time.Time) bool {
	state := s.states[index]
	return s.policy.CircuitBreaker.Enabled && !state.openUntil.IsZero() && !state.openUntil.After(now)
}

func (s *EndpointSet) probeAll(client *http.Client, method string, path string) {
	urls := s.URLs()
	for index, baseURL := range urls {
		ctx, cancel := context.WithTimeout(context.Background(), s.policy.HealthCheck.Timeout)
		req, err := http.NewRequestWithContext(ctx, method, baseURL+path, nil)
		if err != nil {
			cancel()
			s.RecordFailure(index)
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			s.RecordFailure(index)
			continue
		}
		resp.Body.Close()
		cancel()
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusInternalServerError {
			s.RecordSuccess(index)
			continue
		}
		s.RecordFailure(index)
	}
}
