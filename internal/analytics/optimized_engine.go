package analytics

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// OptimizedEngineConfig holds configuration for the optimized analytics engine.
type OptimizedEngineConfig struct {
	// Ring buffer size for recent metrics
	RingBufferSize int

	// Bucket retention time
	BucketRetention time.Duration

	// Batch settings
	BatchSize     int
	BatchInterval time.Duration
	MaxBatchSize  int

	// Async processing
	WorkerCount    int
	QueueSize      int
	DropOnOverflow bool

	// Time series optimization
	PreallocateBuckets int
	EnableCompression  bool
}

// DefaultOptimizedEngineConfig returns sensible defaults.
func DefaultOptimizedEngineConfig() OptimizedEngineConfig {
	return OptimizedEngineConfig{
		RingBufferSize:     100_000,
		BucketRetention:    24 * time.Hour,
		BatchSize:          1000,
		BatchInterval:      100 * time.Millisecond,
		MaxBatchSize:       10_000,
		WorkerCount:        4,
		QueueSize:          100_000,
		DropOnOverflow:     true,
		PreallocateBuckets: 1440, // 24 hours of minute buckets
		EnableCompression:  true,
	}
}

// OptimizedEngine is a high-performance analytics engine with batching and async processing.
type OptimizedEngine struct {
	config OptimizedEngineConfig

	// Ring buffer for recent metrics (lock-free)
	ring *RingBuffer[RequestMetric]

	// Time series store
	series *OptimizedTimeSeriesStore

	// Batch processing
	batchMu      sync.Mutex
	batch        []RequestMetric
	batchFlushCh chan struct{}

	// Async processing
	metricQueue chan RequestMetric
	workers     []*analyticsWorker
	stopCh      chan struct{}
	stopped     atomic.Bool

	// Metrics
	totalReqs   atomic.Int64
	totalErrs   atomic.Int64
	activeConns atomic.Int64
	batchesSent atomic.Uint64
	dropped     atomic.Uint64

	// Time function (overridable for testing)
	now func() time.Time
}

// analyticsWorker processes metrics from the queue.
type analyticsWorker struct {
	id     int
	engine *OptimizedEngine
	stopCh chan struct{}
}

// NewOptimizedEngine creates a high-performance analytics engine.
func NewOptimizedEngine(cfg OptimizedEngineConfig) *OptimizedEngine {
	if cfg.RingBufferSize <= 0 {
		cfg = DefaultOptimizedEngineConfig()
	}

	e := &OptimizedEngine{
		config:       cfg,
		ring:         NewRingBuffer[RequestMetric](cfg.RingBufferSize),
		series:       NewOptimizedTimeSeriesStore(cfg.BucketRetention, cfg.PreallocateBuckets),
		batch:        make([]RequestMetric, 0, cfg.BatchSize),
		batchFlushCh: make(chan struct{}, 1),
		metricQueue:  make(chan RequestMetric, cfg.QueueSize),
		stopCh:       make(chan struct{}),
		now:          time.Now,
	}

	// Start workers
	e.workers = make([]*analyticsWorker, cfg.WorkerCount)
	for i := 0; i < cfg.WorkerCount; i++ {
		e.workers[i] = &analyticsWorker{
			id:     i,
			engine: e,
			stopCh: make(chan struct{}),
		}
		go e.workers[i].run()
	}

	// Start batch processor
	go e.batchProcessor()

	return e
}

// IncActiveConns increments the active connection counter.
func (e *OptimizedEngine) IncActiveConns() {
	if e == nil {
		return
	}
	e.activeConns.Add(1)
}

// DecActiveConns decrements the active connection counter.
func (e *OptimizedEngine) DecActiveConns() {
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

// Record queues a metric for async processing.
func (e *OptimizedEngine) Record(metric RequestMetric) {
	if e == nil || e.stopped.Load() {
		return
	}

	// Set timestamp if not provided
	if metric.Timestamp.IsZero() {
		metric.Timestamp = e.now().UTC()
	} else {
		metric.Timestamp = metric.Timestamp.UTC()
	}

	// Update counters immediately (lock-free)
	e.totalReqs.Add(1)
	if metric.Error || metric.StatusCode >= 500 {
		e.totalErrs.Add(1)
	}

	// Try to queue the metric
	select {
	case e.metricQueue <- metric:
		// Successfully queued
	default:
		// Queue is full
		if e.config.DropOnOverflow {
			e.dropped.Add(1)
		} else {
			// Block until we can queue
			select {
			case e.metricQueue <- metric:
			case <-e.stopCh:
				return
			}
		}
	}
}

// recordBatch processes a batch of metrics efficiently.
func (e *OptimizedEngine) recordBatch(metrics []RequestMetric) {
	if len(metrics) == 0 {
		return
	}

	// Push to ring buffer in bulk
	for _, m := range metrics {
		e.ring.Push(m)
	}

	// Update time series in batch
	e.series.RecordBatch(metrics)
}

// batchProcessor periodically flushes the batch.
func (e *OptimizedEngine) batchProcessor() {
	ticker := time.NewTicker(e.config.BatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.flushBatch()
		case <-e.batchFlushCh:
			e.flushBatch()
		case <-e.stopCh:
			e.flushBatch()
			return
		}
	}
}

// flushBatch flushes the current batch.
func (e *OptimizedEngine) flushBatch() {
	e.batchMu.Lock()
	if len(e.batch) == 0 {
		e.batchMu.Unlock()
		return
	}

	// Take ownership of current batch
	batch := e.batch
	e.batch = make([]RequestMetric, 0, e.config.BatchSize)
	e.batchMu.Unlock()

	// Process the batch
	e.recordBatch(batch)
	e.batchesSent.Add(1)
}

// addToBatch adds a metric to the current batch, flushing if necessary.
//lint:ignore U1000 test-only batch helper for analytics engine testing
func (e *OptimizedEngine) addToBatch(metric RequestMetric) {
	e.batchMu.Lock()
	e.batch = append(e.batch, metric)
	shouldFlush := len(e.batch) >= e.config.BatchSize
	e.batchMu.Unlock()

	if shouldFlush {
		select {
		case e.batchFlushCh <- struct{}{}:
		default:
			// Flush signal already pending
		}
	}
}

// run is the worker's main loop.
func (w *analyticsWorker) run() {
	batch := make([]RequestMetric, 0, w.engine.config.BatchSize)
	ticker := time.NewTicker(w.engine.config.BatchInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) > 0 {
			w.engine.recordBatch(batch)
			batch = batch[:0]
		}
	}

	for {
		select {
		case metric := <-w.engine.metricQueue:
			batch = append(batch, metric)
			if len(batch) >= w.engine.config.BatchSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-w.stopCh:
			flush()
			return

		case <-w.engine.stopCh:
			flush()
			return
		}
	}
}

// Overview returns current statistics.
func (e *OptimizedEngine) Overview() Overview {
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

// Latest returns the most recent metrics from the ring buffer.
func (e *OptimizedEngine) Latest(limit int) []RequestMetric {
	if e == nil || e.ring == nil {
		return nil
	}
	return e.ring.Snapshot(limit)
}

// TimeSeries returns time series data.
func (e *OptimizedEngine) TimeSeries(from, to time.Time) []Bucket {
	if e == nil || e.series == nil {
		return nil
	}
	return e.series.Buckets(from, to)
}

// Metrics returns detailed engine metrics.
func (e *OptimizedEngine) Metrics() OptimizedEngineMetrics {
	if e == nil {
		return OptimizedEngineMetrics{}
	}
	return OptimizedEngineMetrics{
		TotalRequests:  e.totalReqs.Load(),
		TotalErrors:    e.totalErrs.Load(),
		ActiveConns:    e.activeConns.Load(),
		BatchesSent:    e.batchesSent.Load(),
		DroppedMetrics: e.dropped.Load(),
		QueueDepth:     len(e.metricQueue),
	}
}

// OptimizedEngineMetrics holds detailed metrics about the engine.
type OptimizedEngineMetrics struct {
	TotalRequests  int64
	TotalErrors    int64
	ActiveConns    int64
	BatchesSent    uint64
	DroppedMetrics uint64
	QueueDepth     int
}

// Stop gracefully shuts down the engine.
func (e *OptimizedEngine) Stop() {
	if e == nil {
		return
	}
	if e.stopped.CompareAndSwap(false, true) {
		close(e.stopCh)

		// Stop workers
		for _, w := range e.workers {
			close(w.stopCh)
		}

		// Final flush
		e.flushBatch()
	}
}

// IsStopped returns true if the engine has been stopped.
func (e *OptimizedEngine) IsStopped() bool {
	if e == nil {
		return true
	}
	return e.stopped.Load()
}

// OptimizedTimeSeriesStore is an optimized time series storage with batch support.
type OptimizedTimeSeriesStore struct {
	mu          sync.RWMutex
	buckets     map[int64]*optimizedBucketAggregate
	retention   time.Duration
	lastCleanup time.Time
	prealloc    int
}

type optimizedBucketAggregate struct {
	start           time.Time
	requests        atomic.Int64
	errors          atomic.Int64
	latencySum      atomic.Int64
	statusCodes     sync.Map // map[int]atomic.Int64
	bytesIn         atomic.Int64
	bytesOut        atomic.Int64
	creditsConsumed atomic.Int64
	latencies       []int64 // For percentile calculation
	latenciesMu     sync.RWMutex
}

// NewOptimizedTimeSeriesStore creates an optimized time series store.
func NewOptimizedTimeSeriesStore(retention time.Duration, prealloc int) *OptimizedTimeSeriesStore {
	if retention <= 0 {
		retention = defaultBucketRetention
	}
	return &OptimizedTimeSeriesStore{
		buckets:   make(map[int64]*optimizedBucketAggregate, prealloc),
		retention: retention,
		prealloc:  prealloc,
	}
}

// RecordBatch processes multiple metrics efficiently.
func (s *OptimizedTimeSeriesStore) RecordBatch(metrics []RequestMetric) {
	if s == nil || len(metrics) == 0 {
		return
	}

	// Group metrics by minute bucket
	buckets := make(map[int64][]RequestMetric)
	for _, m := range metrics {
		minute := m.Timestamp.Truncate(time.Minute).Unix()
		buckets[minute] = append(buckets[minute], m)
	}

	// Process each bucket
	for minute, bucketMetrics := range buckets {
		s.recordToBucket(minute, bucketMetrics)
	}

	// Periodic cleanup
	if time.Since(s.lastCleanup) > cleanupIntervalPerWrite {
		s.cleanup(time.Now())
	}
}

// recordToBucket records metrics to a specific bucket.
func (s *OptimizedTimeSeriesStore) recordToBucket(minute int64, metrics []RequestMetric) {
	s.mu.RLock()
	b := s.buckets[minute]
	s.mu.RUnlock()

	if b == nil {
		s.mu.Lock()
		b = s.buckets[minute]
		if b == nil {
			b = &optimizedBucketAggregate{
				start:       time.Unix(minute, 0).UTC(),
				latencies:   make([]int64, 0, len(metrics)),
				statusCodes: sync.Map{},
			}
			s.buckets[minute] = b
		}
		s.mu.Unlock()
	}

	// Aggregate metrics
	var latencySum int64
	var errors int64
	latencies := make([]int64, 0, len(metrics))

	for _, m := range metrics {
		b.requests.Add(1)
		latencySum += m.LatencyMS
		latencies = append(latencies, m.LatencyMS)
		b.bytesIn.Add(m.BytesIn)
		b.bytesOut.Add(m.BytesOut)
		b.creditsConsumed.Add(m.CreditsConsumed)

		if m.Error || m.StatusCode >= 500 {
			errors++
		}

		// Update status code counter
		if m.StatusCode > 0 {
			if counter, ok := b.statusCodes.Load(m.StatusCode); ok {
				if ac, ok := counter.(*atomic.Int64); ok {
					ac.Add(1)
				}
			} else {
				newCounter := &atomic.Int64{}
				newCounter.Add(1)
				b.statusCodes.Store(m.StatusCode, newCounter)
			}
		}
	}

	b.latencySum.Add(latencySum)
	b.errors.Add(errors)

	// Store latencies for percentile calculation with reservoir sampling cap
	b.latenciesMu.Lock()
	for _, lat := range latencies {
		if int64(len(b.latencies)) < maxLatencySamples {
			b.latencies = append(b.latencies, lat)
		} else {
			// G404: reservoir sampling — non-crypto RNG is intentional for analytics performance
			total := b.requests.Load()
			idx := rand.Int63n(total)
			if idx < maxLatencySamples {
				b.latencies[idx] = lat
			}
		}
	}
	b.latenciesMu.Unlock()
}

// Buckets returns time series buckets in the given range.
func (s *OptimizedTimeSeriesStore) Buckets(from, to time.Time) []Bucket {
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
		items = append(items, s.bucketFromAggregate(b))
	}
	s.mu.RUnlock()

	// Sort by start time
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			if items[i].Start.After(items[j].Start) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	return items
}

// bucketFromAggregate converts an aggregate to a Bucket.
func (s *OptimizedTimeSeriesStore) bucketFromAggregate(b *optimizedBucketAggregate) Bucket {
	requests := b.requests.Load()

	out := Bucket{
		Start:           b.start,
		Requests:        requests,
		Errors:          b.errors.Load(),
		BytesIn:         b.bytesIn.Load(),
		BytesOut:        b.bytesOut.Load(),
		CreditsConsumed: b.creditsConsumed.Load(),
		StatusCodes:     make(map[int]int64),
	}

	if requests > 0 {
		out.AvgLatencyMS = float64(b.latencySum.Load()) / float64(requests)
	}

	// Get latencies for percentile calculation
	b.latenciesMu.RLock()
	latencies := make([]int64, len(b.latencies))
	copy(latencies, b.latencies)
	b.latenciesMu.RUnlock()

	out.P50LatencyMS = percentileOptimized(latencies, 50)
	out.P95LatencyMS = percentileOptimized(latencies, 95)
	out.P99LatencyMS = percentileOptimized(latencies, 99)

	// Copy status codes
	b.statusCodes.Range(func(key, value any) bool {
		if code, ok := key.(int); ok {
			if counter, ok := value.(*atomic.Int64); ok {
				out.StatusCodes[code] = counter.Load()
			}
		}
		return true
	})

	return out
}

// cleanup removes old buckets.
func (s *OptimizedTimeSeriesStore) cleanup(now time.Time) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := now.UTC().Add(-s.retention).Truncate(time.Minute)
	for key, b := range s.buckets {
		if b == nil || b.start.Before(cutoff) {
			delete(s.buckets, key)
		}
	}
	s.lastCleanup = now
}

// percentileOptimized calculates a percentile without full sorting.
func percentileOptimized(values []int64, p int) int64 {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		p = 1
	}
	if p > 100 {
		p = 100
	}

	// For small arrays, use quickselect-like approach
	if len(values) <= 100 {
		sorted := make([]int64, len(values))
		copy(sorted, values)
		// Simple insertion sort for small arrays
		for i := 1; i < len(sorted); i++ {
			j := i
			for j > 0 && sorted[j-1] > sorted[j] {
				sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
				j--
			}
		}
		rank := int(float64(p) / 100.0 * float64(len(sorted)-1))
		return sorted[rank]
	}

	// For larger arrays, use approximate percentile
	// This is faster and usually sufficient for metrics
	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	// Linear interpolation for approximate percentile
	return min + int64(float64(max-min)*float64(p)/100.0)
}
