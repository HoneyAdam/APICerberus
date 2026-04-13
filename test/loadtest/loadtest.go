// Package loadtest provides sustained HTTP load testing for APICerebrus.
//
// It supports configurable attack profiles (constant rate, ramp-up, spike)
// with per-request latency tracking and percentile reporting.
//
// Usage:
//
//	attacker := &loadtest.HTTPAttacker{
//	    Target:  "http://localhost:9876",
//	    Timeout: 10 * time.Second,
//	}
//	report := loadtest.ConstantRate(attacker, 1000, 30*time.Second)
//	report.Print()
package loadtest

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Result holds a single request outcome.
type Result struct {
	Latency    time.Duration
	StatusCode int
	BytesOut   int64
	BytesIn    int64
	Err        error
	Timestamp  time.Time
}

// Report aggregates results from a load test run.
type Report struct {
	StartTime      time.Time
	EndTime        time.Time
	Total          int64
	Successes      int64
	Failures       int64
	BytesIn        int64
	BytesOut       int64
	Latencies      []time.Duration
	StatusCodes    map[int]int64
	Duration       time.Duration
	RequestsPerSec float64
	Errors         []string
}

// Print writes a human-readable summary to stdout.
func (r *Report) Print() {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("LOAD TEST REPORT")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Duration:        %s\n", r.Duration.Truncate(time.Millisecond))
	fmt.Printf("Total Requests:  %d\n", r.Total)
	fmt.Printf("Successes:       %d\n", r.Successes)
	fmt.Printf("Failures:        %d\n", r.Failures)
	fmt.Printf("Throughput:      %.1f req/s\n", r.RequestsPerSec)
	fmt.Printf("Bytes In:        %d\n", r.BytesIn)
	fmt.Printf("Bytes Out:       %d\n", r.BytesOut)
	if len(r.Latencies) > 0 {
		fmt.Printf("\nLatency:\n")
		fmt.Printf("  Min:     %s\n", r.Latencies[0].Truncate(time.Microsecond))
		fmt.Printf("  p50:     %s\n", r.Percentile(50).Truncate(time.Microsecond))
		fmt.Printf("  p95:     %s\n", r.Percentile(95).Truncate(time.Microsecond))
		fmt.Printf("  p99:     %s\n", r.Percentile(99).Truncate(time.Microsecond))
		fmt.Printf("  Max:     %s\n", r.Latencies[len(r.Latencies)-1].Truncate(time.Microsecond))
		fmt.Printf("  Mean:    %s\n", r.Mean().Truncate(time.Microsecond))
	}
	if len(r.StatusCodes) > 0 {
		fmt.Printf("\nStatus Codes:\n")
		codes := make([]int, 0, len(r.StatusCodes))
		for c := range r.StatusCodes {
			codes = append(codes, c)
		}
		sort.Ints(codes)
		for _, c := range codes {
			fmt.Printf("  %d: %d\n", c, r.StatusCodes[c])
		}
	}
	if len(r.Errors) > 0 {
		fmt.Printf("\nErrors (top %d):\n", min(len(r.Errors), 10))
		for i, e := range r.Errors {
			if i >= 10 {
				break
			}
			fmt.Printf("  - %s\n", e)
		}
	}
	fmt.Println(strings.Repeat("=", 60))
}

// Percentile returns the p-th percentile latency (0-100).
func (r *Report) Percentile(p int) time.Duration {
	if len(r.Latencies) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(p)/100*float64(len(r.Latencies)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(r.Latencies) {
		idx = len(r.Latencies) - 1
	}
	return r.Latencies[idx]
}

// Mean returns average latency.
func (r *Report) Mean() time.Duration {
	if len(r.Latencies) == 0 {
		return 0
	}
	var sum time.Duration
	for _, l := range r.Latencies {
		sum += l
	}
	return sum / time.Duration(len(r.Latencies))
}

// FailureRate returns the fraction of requests that failed (0.0 - 1.0).
func (r *Report) FailureRate() float64 {
	if r.Total == 0 {
		return 0
	}
	return float64(r.Failures) / float64(r.Total)
}

// HTTPAttacker performs HTTP requests for load testing.
type HTTPAttacker struct {
	Client  *http.Client
	Target  string
	Headers map[string]string
}

// NewAttacker returns an attacker with sensible defaults.
func NewAttacker(target string) *HTTPAttacker {
	return &HTTPAttacker{
		Target: target,
		Client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        200,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		Headers: make(map[string]string),
	}
}

// Attack sends a single HTTP request and returns the result.
func (a *HTTPAttacker) Attack(ctx context.Context, method, path string, body []byte) Result {
	req, err := http.NewRequestWithContext(ctx, method, a.Target+path, nil)
	if err != nil {
		return Result{Err: err, Timestamp: time.Now()}
	}
	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}
	if len(body) > 0 {
		req.ContentLength = int64(len(body))
	}

	start := time.Now()
	resp, err := a.Client.Do(req)
	latency := time.Since(start)

	r := Result{
		Latency:   latency,
		BytesOut:  req.ContentLength,
		Timestamp: start,
	}
	if err != nil {
		r.Err = err
		return r
	}
	defer resp.Body.Close()

	r.StatusCode = resp.StatusCode
	r.BytesIn = resp.ContentLength
	return r
}

// AttackFunc is a function that performs a single attack iteration.
type AttackFunc func(ctx context.Context) Result

// ConstantRate runs a constant rate of requests for the given duration.
func ConstantRate(attacker AttackFunc, rate int, duration time.Duration) *Report {
	interval := time.Second / time.Duration(rate)
	return run(attacker, duration, interval)
}

// RampUp gradually increases the rate from startRate to endRate over the duration.
func RampUp(attacker AttackFunc, startRate, endRate int, duration time.Duration, steps int) *Report {
	var results []Result
	var mu sync.Mutex
	var total, successes, failures, bytesIn, bytesOut int64
	statusCodes := make(map[int]int64)

	start := time.Now()
	stepDuration := duration / time.Duration(steps)
	rateStep := (endRate - startRate) / steps

	var wg sync.WaitGroup
	for s := 0; s < steps; s++ {
		rate := startRate + s*rateStep
		if rate <= 0 {
			rate = 1
		}
		interval := time.Second / time.Duration(rate)

		// Calculate how many requests to send during this step
		stepMs := stepDuration.Milliseconds()
		numRequests := int(int64(rate) * stepMs / 1000)
		if numRequests <= 0 {
			numRequests = 1
		}

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r := attacker(context.Background())
				mu.Lock()
				results = append(results, r)
				total++
				if r.Err == nil && r.StatusCode < 500 {
					successes++
				} else {
					failures++
				}
				bytesIn += r.BytesIn
				bytesOut += r.BytesOut
				if r.StatusCode > 0 {
					statusCodes[r.StatusCode]++
				}
				mu.Unlock()
				time.Sleep(interval)
			}()
		}
		time.Sleep(stepDuration)
	}
	wg.Wait()

	elapsed := time.Since(start)
	sort.Slice(results, func(i, j int) bool { return results[i].Latency < results[j].Latency })

	latencies := make([]time.Duration, 0, len(results))
	var errs []string
	for _, r := range results {
		latencies = append(latencies, r.Latency)
		if r.Err != nil {
			errs = append(errs, r.Err.Error())
		}
	}

	return &Report{
		StartTime:      start,
		EndTime:        start.Add(elapsed),
		Total:          total,
		Successes:      successes,
		Failures:       failures,
		BytesIn:        bytesIn,
		BytesOut:       bytesOut,
		Latencies:      latencies,
		StatusCodes:    statusCodes,
		Duration:       elapsed,
		RequestsPerSec: float64(total) / elapsed.Seconds(),
		Errors:         errs,
	}
}

func run(attacker AttackFunc, duration time.Duration, interval time.Duration) *Report {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var (
		mu          sync.Mutex
		results     []Result
		total       int64
		successes   int64
		failures    int64
		bytesIn     int64
		bytesOut    int64
		statusCodes = make(map[int]int64)
		wg          sync.WaitGroup
		ticker      = time.NewTicker(interval)
	)
	defer ticker.Stop()

	start := time.Now()

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			elapsed := time.Since(start)
			sort.Slice(results, func(i, j int) bool { return results[i].Latency < results[j].Latency })

			latencies := make([]time.Duration, 0, len(results))
			var errs []string
			for _, r := range results {
				latencies = append(latencies, r.Latency)
				if r.Err != nil {
					errs = append(errs, r.Err.Error())
				}
			}

			return &Report{
				StartTime:      start,
				EndTime:        start.Add(elapsed),
				Total:          total,
				Successes:      successes,
				Failures:       failures,
				BytesIn:        bytesIn,
				BytesOut:       bytesOut,
				Latencies:      latencies,
				StatusCodes:    statusCodes,
				Duration:       elapsed,
				RequestsPerSec: float64(total) / elapsed.Seconds(),
				Errors:         errs,
			}
		case <-ticker.C:
			wg.Add(1)
			go func() {
				defer wg.Done()
				r := attacker(ctx)
				atomic.AddInt64(&total, 1)
				if r.Err == nil && r.StatusCode < 500 {
					atomic.AddInt64(&successes, 1)
				} else {
					atomic.AddInt64(&failures, 1)
				}
				atomic.AddInt64(&bytesIn, r.BytesIn)
				atomic.AddInt64(&bytesOut, r.BytesOut)
				if r.StatusCode > 0 {
					mu.Lock()
					statusCodes[r.StatusCode]++
					mu.Unlock()
				}
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
			}()
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
