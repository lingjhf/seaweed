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

// EndpointPolicyMode selects the endpoint selection strategy.
type EndpointPolicyMode string

const (
	// EndpointPolicyFailover keeps using the active endpoint until it fails.
	EndpointPolicyFailover EndpointPolicyMode = "failover"
	// EndpointPolicyRoundRobin rotates the starting endpoint for each selection.
	EndpointPolicyRoundRobin EndpointPolicyMode = "round-robin"
)

// EndpointHealthCheckPolicy configures background endpoint health probes.
type EndpointHealthCheckPolicy struct {
	Enabled          bool
	Interval         time.Duration
	Timeout          time.Duration
	FailureThreshold int
	SuccessThreshold int
}

// EndpointCircuitBreakerPolicy configures endpoint failure isolation.
type EndpointCircuitBreakerPolicy struct {
	Enabled             bool
	FailureThreshold    int
	OpenTimeout         time.Duration
	HalfOpenMaxRequests int
}

// EndpointPolicy controls how an endpoint set chooses and marks endpoints.
type EndpointPolicy struct {
	Mode           EndpointPolicyMode
	HealthCheck    EndpointHealthCheckPolicy
	CircuitBreaker EndpointCircuitBreakerPolicy
}

// EndpointSet stores normalized endpoints and their health state.
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

// EndpointCandidate is one endpoint candidate for a request path.
type EndpointCandidate struct {
	Index int
	URL   string
}

// EndpointLease represents one selected endpoint that must be finished.
type EndpointLease struct {
	Index int
	URL   string

	set      *EndpointSet
	halfOpen bool
}

type endpointState struct {
	failures         int
	successes        int
	unhealthy        bool
	openUntil        time.Time
	halfOpenRequests int
}

// NewEndpointSet creates an endpoint set with the default endpoint policy.
func NewEndpointSet(rawURLs []string) (*EndpointSet, error) {
	return NewEndpointSetWithPolicy(rawURLs, EndpointPolicy{})
}

// NewEndpointSetWithPolicy creates an endpoint set with policy.
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

// NormalizeEndpointPolicy applies policy defaults and validates the mode.
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

// NormalizeBaseURLs validates, normalizes, and deduplicates base URLs.
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

// NormalizeBaseURL validates and normalizes one absolute HTTP base URL.
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

// URLs returns a snapshot of normalized base URLs.
func (s *EndpointSet) URLs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	urls := make([]string, len(s.urls))
	copy(urls, s.urls)
	return urls
}

// URL joins path with the currently active base URL.
func (s *EndpointSet) URL(path string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.urls[s.active] + path
}

// Candidates returns endpoint candidates for path in policy order.
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

// Lease selects one available endpoint lease for path.
func (s *EndpointSet) Lease(path string) (*EndpointLease, error) {
	for _, candidate := range s.Candidates(path) {
		available, halfOpen := s.beginCandidate(candidate.Index)
		if !available {
			continue
		}
		return &EndpointLease{
			Index:    candidate.Index,
			URL:      candidate.URL,
			set:      s,
			halfOpen: halfOpen,
		}, nil
	}
	return nil, fmt.Errorf("httpx: no available endpoints")
}

// Finish records whether the leased endpoint attempt succeeded.
func (l *EndpointLease) Finish(success bool) {
	if l == nil || l.set == nil {
		return
	}
	l.set.finishCandidate(l.Index, l.halfOpen, success)
}

// MarkSuccess records a successful endpoint attempt.
func (s *EndpointSet) MarkSuccess(index int) {
	s.RecordSuccess(index)
}

// RecordSuccess records a successful endpoint attempt.
func (s *EndpointSet) RecordSuccess(index int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.urls) {
		return
	}
	s.recordSuccessLocked(index)
}

// RecordFailure records a failed endpoint attempt.
func (s *EndpointSet) RecordFailure(index int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.urls) {
		return
	}
	s.recordFailureLocked(index, time.Now())
}

// FinishCandidate records whether the endpoint candidate succeeded.
func (s *EndpointSet) FinishCandidate(index int, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.urls) {
		return
	}
	if s.states[index].halfOpenRequests > 0 {
		s.states[index].halfOpenRequests--
	}
	if success {
		s.recordSuccessLocked(index)
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

// StartHealthCheck starts background health probes when health checks are enabled.
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

// Close stops background health checks.
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
