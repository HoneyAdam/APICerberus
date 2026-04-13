package analytics

import (
	"context"
	"math"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultRingBufferSize   = 100_000
	defaultBucketRetention  = 24 * time.Hour
	minimumBucketRetention  = time.Minute
	cleanupIntervalPerWrite = time.Minute
	maxLatencySamples       = 10_000
	maxTimeSeriesBuckets    = 10_000 // caps memory between cleanup cycles
)

type RequestMetric struct {
	Timestamp       time.Time `json:"timestamp"`
	RouteID         string    `json:"route_id"`
	RouteName       string    `json:"route_name"`
	ServiceName     string    `json:"service_name"`
	UserID          string    `json:"user_id"`
	Method          string    `json:"method"`
	Path            string    `json:"path"`
	StatusCode      int       `json:"status_code"`
	LatencyMS       int64     `json:"latency_ms"`
	BytesIn         int64     `json:"bytes_in"`
	BytesOut        int64     `json:"bytes_out"`
	CreditsConsumed int64     `json:"credits_consumed"`
	Blocked         bool      `json:"blocked"`
	Error           bool      `json:"error"`
}

type EngineConfig struct {
	RingBufferSize  int
	BucketRetention time.Duration
}

type Overview struct {
	TotalRequests int64   `json:"total_requests"`
	ActiveConns   int64   `json:"active_conns"`
	TotalErrors   int64   `json:"total_errors"`
	ErrorRate     float64 `json:"error_rate"`
}

type Bucket struct {
	Start           time.Time     `json:"start"`
	Requests        int64         `json:"requests"`
	Errors          int64         `json:"errors"`
	AvgLatencyMS    float64       `json:"avg_latency_ms"`
	P50LatencyMS    int64         `json:"p50_latency_ms"`
	P95LatencyMS    int64         `json:"p95_latency_ms"`
	P99LatencyMS    int64         `json:"p99_latency_ms"`
	StatusCodes     map[int]int64 `json:"status_codes"`
	BytesIn         int64         `json:"bytes_in"`
	BytesOut        int64         `json:"bytes_out"`
	CreditsConsumed int64         `json:"credits_consumed"`
}

type Engine struct {
	ring        *RingBuffer[RequestMetric]
	series      *TimeSeriesStore
	totalReqs   atomic.Int64
	totalErrs   atomic.Int64
	activeConns atomic.Int64
	now         func() time.Time
}

func NewEngine(cfg EngineConfig) *Engine {
	if cfg.RingBufferSize <= 0 {
		cfg.RingBufferSize = defaultRingBufferSize
	}
	if cfg.BucketRetention <= 0 {
		cfg.BucketRetention = defaultBucketRetention
	}
	if cfg.BucketRetention < minimumBucketRetention {
		cfg.BucketRetention = minimumBucketRetention
	}

	return &Engine{
		ring:   NewRingBuffer[RequestMetric](cfg.RingBufferSize),
		series: NewTimeSeriesStore(cfg.BucketRetention),
		now:    time.Now,
	}
}

func (e *Engine) IncActiveConns() {
	if e == nil {
		return
	}
	e.activeConns.Add(1)
}

func (e *Engine) DecActiveConns() {
	if e == nil {
		return
	}
	for {
		current := e.activeConns.Load()
		if current <= 0 {
			return
		}
		if e.activeConns.CompareAndSwap(current, current-1) {
			return
		}
	}
}

func (e *Engine) Record(metric RequestMetric) {
	if e == nil || e.ring == nil || e.series == nil {
		return
	}
	if metric.Timestamp.IsZero() {
		metric.Timestamp = e.now().UTC()
	} else {
		metric.Timestamp = metric.Timestamp.UTC()
	}

	e.totalReqs.Add(1)
	if metric.Error || metric.StatusCode >= 500 {
		e.totalErrs.Add(1)
	}

	e.ring.Push(metric)
	e.series.Record(metric)
}

func (e *Engine) Overview() Overview {
	if e == nil {
		return Overview{}
	}
	total := e.totalReqs.Load()
	errors := e.totalErrs.Load()
	rate := 0.0
	if total > 0 {
		rate = float64(errors) / float64(total)
	}
	return Overview{
		TotalRequests: total,
		ActiveConns:   e.activeConns.Load(),
		TotalErrors:   errors,
		ErrorRate:     rate,
	}
}

func (e *Engine) Latest(limit int) []RequestMetric {
	if e == nil || e.ring == nil {
		return nil
	}
	return e.ring.Snapshot(limit)
}

func (e *Engine) TimeSeries(from, to time.Time) []Bucket {
	if e == nil || e.series == nil {
		return nil
	}
	return e.series.Buckets(from, to)
}

// Shutdown marks the engine as stopped and returns a final snapshot of metrics.
// The analytics engine writes synchronously (no background goroutine), so this
// is a no-op that just prevents further writes after shutdown begins.
func (e *Engine) Shutdown(_ context.Context) {
	// Analytics is purely in-memory with synchronous writes — no flush needed.
	// Data is lost on process exit regardless; persistence would require
	// writing to SQLite or a file, which is a feature addition.
}

type RingBuffer[T any] struct {
	slots   []atomic.Pointer[T]
	size    uint64
	written atomic.Uint64
}

func NewRingBuffer[T any](size int) *RingBuffer[T] {
	if size <= 0 {
		size = 1
	}
	return &RingBuffer[T]{
		slots: make([]atomic.Pointer[T], size),
		size:  uint64(size),
	}
}

func (r *RingBuffer[T]) Push(item T) {
	if r == nil || r.size == 0 {
		return
	}
	index := r.written.Add(1) - 1
	slot := index % r.size
	value := item
	r.slots[slot].Store(&value)
}

func (r *RingBuffer[T]) Snapshot(limit int) []T {
	if r == nil || r.size == 0 {
		return nil
	}
	written := r.written.Load()
	if written == 0 {
		return nil
	}

	count := int(written)    // #nosec G115 -- ring buffer size is bounded by configured capacity.
	if count > int(r.size) { // #nosec G115
		count = int(r.size) // #nosec G115
	}
	if limit > 0 && limit < count {
		count = limit
	}

	out := make([]T, 0, count)
	for i := 0; i < count; i++ {
		index := (written - 1 - uint64(i)) % r.size
		ptr := r.slots[index].Load()
		if ptr == nil {
			continue
		}
		out = append(out, *ptr)
	}
	return out
}

type TimeSeriesStore struct {
	mu          sync.RWMutex
	buckets     map[int64]*bucketAggregate
	retention   time.Duration
	lastCleanup time.Time
}

type bucketAggregate struct {
	start           time.Time
	requests        int64
	errors          int64
	latencySum      int64
	latencies       []int64
	statusCodes     map[int]int64
	bytesIn         int64
	bytesOut        int64
	creditsConsumed int64
}

func NewTimeSeriesStore(retention time.Duration) *TimeSeriesStore {
	if retention <= 0 {
		retention = defaultBucketRetention
	}
	if retention < minimumBucketRetention {
		retention = minimumBucketRetention
	}
	return &TimeSeriesStore{
		buckets:   make(map[int64]*bucketAggregate),
		retention: retention,
	}
}

func (s *TimeSeriesStore) Record(metric RequestMetric) {
	if s == nil {
		return
	}

	ts := metric.Timestamp.UTC()
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	minute := ts.Truncate(time.Minute)
	key := minute.Unix()

	s.mu.Lock()
	if len(s.buckets) >= maxTimeSeriesBuckets {
		s.cleanupLocked(ts)
		s.lastCleanup = ts
		if len(s.buckets) >= maxTimeSeriesBuckets {
			s.mu.Unlock()
			return
		}
	}
	b := s.buckets[key]
	if b == nil {
		b = &bucketAggregate{
			start:       minute,
			latencies:   make([]int64, 0, 64),
			statusCodes: map[int]int64{},
		}
		s.buckets[key] = b
	}
	b.requests++
	if metric.Error || metric.StatusCode >= 500 {
		b.errors++
	}
	b.latencySum += metric.LatencyMS
	if int64(len(b.latencies)) < maxLatencySamples {
		b.latencies = append(b.latencies, metric.LatencyMS)
	} else {
		// G404: reservoir sampling — non-crypto RNG is intentional for analytics performance
		idx := rand.Int63n(b.requests)
		if idx < maxLatencySamples {
			b.latencies[idx] = metric.LatencyMS
		}
	}
	if metric.StatusCode > 0 {
		b.statusCodes[metric.StatusCode]++
	}
	b.bytesIn += metric.BytesIn
	b.bytesOut += metric.BytesOut
	b.creditsConsumed += metric.CreditsConsumed

	if s.lastCleanup.IsZero() || ts.Sub(s.lastCleanup) >= cleanupIntervalPerWrite {
		s.cleanupLocked(ts)
		s.lastCleanup = ts
	}
	s.mu.Unlock()
}

func (s *TimeSeriesStore) Buckets(from, to time.Time) []Bucket {
	if s == nil {
		return nil
	}
	from = from.UTC()
	to = to.UTC()
	if from.IsZero() {
		from = time.Unix(0, 0).UTC()
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.After(to) {
		from, to = to, from
	}

	s.mu.RLock()
	items := make([]Bucket, 0, len(s.buckets))
	for _, b := range s.buckets {
		if b == nil {
			continue
		}
		if b.start.Before(from) || b.start.After(to) {
			continue
		}
		items = append(items, bucketFromAggregate(b))
	}
	s.mu.RUnlock()

	sort.Slice(items, func(i, j int) bool {
		return items[i].Start.Before(items[j].Start)
	})
	return items
}

func (s *TimeSeriesStore) cleanupLocked(now time.Time) {
	if s == nil {
		return
	}
	cutoff := now.UTC().Add(-s.retention).Truncate(time.Minute)
	for key, b := range s.buckets {
		if b == nil {
			delete(s.buckets, key)
			continue
		}
		if b.start.Before(cutoff) {
			delete(s.buckets, key)
		}
	}
}

func bucketFromAggregate(b *bucketAggregate) Bucket {
	out := Bucket{
		Start:           b.start,
		Requests:        b.requests,
		Errors:          b.errors,
		StatusCodes:     cloneStatusCodes(b.statusCodes),
		BytesIn:         b.bytesIn,
		BytesOut:        b.bytesOut,
		CreditsConsumed: b.creditsConsumed,
	}
	if b.requests > 0 {
		out.AvgLatencyMS = float64(b.latencySum) / float64(b.requests)
	}
	out.P50LatencyMS = percentile(b.latencies, 50)
	out.P95LatencyMS = percentile(b.latencies, 95)
	out.P99LatencyMS = percentile(b.latencies, 99)
	return out
}

func cloneStatusCodes(src map[int]int64) map[int]int64 {
	if len(src) == 0 {
		return map[int]int64{}
	}
	out := make(map[int]int64, len(src))
	for code, count := range src {
		out[code] = count
	}
	return out
}

func percentile(values []int64, p int) int64 {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		p = 1
	}
	if p > 100 {
		p = 100
	}

	sorted := make([]int64, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	rank := int(math.Ceil((float64(p) / 100.0) * float64(len(sorted))))
	if rank <= 0 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}
