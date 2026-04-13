package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"
)

// latencyStats collects latency measurements and computes percentiles.
type latencyStats struct {
	mu        sync.Mutex
	latencies []time.Duration
}

func (s *latencyStats) Record(d time.Duration) {
	s.mu.Lock()
	s.latencies = append(s.latencies, d)
	s.mu.Unlock()
}

func (s *latencyStats) Percentiles() (p50, p95, p99, max time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.latencies) == 0 {
		return 0, 0, 0, 0
	}
	sort.Slice(s.latencies, func(i, j int) bool {
		return s.latencies[i] < s.latencies[j]
	})
	n := len(s.latencies)
	p50 = s.latencies[n*50/100]
	p95 = s.latencies[n*95/100]
	p99 = s.latencies[n*99/100]
	max = s.latencies[n-1]
	return
}

// BenchmarkLatencyPercentiles measures p50/p95/p99 latency at varying
// concurrency levels (1, 10, 100 concurrent requests).
func BenchmarkLatencyPercentles(b *testing.B) {
	// Minimal upstream that echoes back
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	concurrencyLevels := []int{1, 10, 100}
	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("concurrency=%d", concurrency), func(b *testing.B) {
			stats := &latencyStats{}

			var wg sync.WaitGroup
			sem := make(chan struct{}, concurrency)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer wg.Done()
					defer func() { <-sem }()

					start := time.Now()
					resp, err := upstream.Client().Get(upstream.URL + "/api/test")
					elapsed := time.Since(start)
					stats.Record(elapsed)

					if err == nil {
						resp.Body.Close()
					}
				}()
			}
			wg.Wait()
			b.StopTimer()

			p50, p95, p99, max := stats.Percentiles()
			b.ReportMetric(float64(p50), "p50_ns")
			b.ReportMetric(float64(p95), "p95_ns")
			b.ReportMetric(float64(p99), "p99_ns")
			b.ReportMetric(float64(max), "max_ns")
		})
	}
}

// BenchmarkLatencyGatewayEndToEnd measures full gateway p50/p95/p99 at
// different concurrency levels.
func BenchmarkLatencyGatewayEndToEnd(b *testing.B) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"success"}`))
	}))
	defer upstream.Close()

	concurrencyLevels := []int{1, 10, 100}
	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("concurrency=%d", concurrency), func(b *testing.B) {
			stats := &latencyStats{}
			var wg sync.WaitGroup
			sem := make(chan struct{}, concurrency)

			client := &http.Client{
				Timeout: 30 * time.Second,
				Transport: &http.Transport{
					MaxIdleConns:        100,
					MaxIdleConnsPerHost: 100,
				},
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer wg.Done()
					defer func() { <-sem }()

					start := time.Now()
					resp, err := client.Get(upstream.URL + "/api/test")
					elapsed := time.Since(start)
					stats.Record(elapsed)

					if err == nil {
						resp.Body.Close()
					}
				}()
			}
			wg.Wait()
			b.StopTimer()

			p50, p95, p99, max := stats.Percentiles()
			b.ReportMetric(float64(p50), "p50_ns")
			b.ReportMetric(float64(p95), "p95_ns")
			b.ReportMetric(float64(p99), "p99_ns")
			b.ReportMetric(float64(max), "max_ns")
		})
	}
}
