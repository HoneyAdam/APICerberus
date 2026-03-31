package cache

import (
	"sync"
	"time"
)

// ObjectPool provides a generic object pool for reducing GC pressure
type ObjectPool[T any] struct {
	pool sync.Pool
	new  func() T
	reset func(T)
}

// NewObjectPool creates a new object pool
func NewObjectPool[T any](newFunc func() T, resetFunc func(T)) *ObjectPool[T] {
	return &ObjectPool[T]{
		pool: sync.Pool{
			New: func() interface{} {
				return newFunc()
			},
		},
		new:   newFunc,
		reset: resetFunc,
	}
}

// Get retrieves an object from the pool
func (p *ObjectPool[T]) Get() T {
	return p.pool.Get().(T)
}

// Put returns an object to the pool
func (p *ObjectPool[T]) Put(obj T) {
	if p.reset != nil {
		p.reset(obj)
	}
	p.pool.Put(obj)
}

// BufferPool provides a pool of byte slices
type BufferPool struct {
	pool sync.Pool
	size int
}

// NewBufferPool creates a new buffer pool with specified size
func NewBufferPool(size int) *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, size)
			},
		},
		size: size,
	}
}

// Get retrieves a buffer from the pool
func (p *BufferPool) Get() []byte {
	return p.pool.Get().([]byte)
}

// Put returns a buffer to the pool
func (p *BufferPool) Put(buf []byte) {
	if cap(buf) >= p.size {
		p.pool.Put(buf[:p.size])
	}
}

// SizedBufferPool provides buffers of different sizes
type SizedBufferPool struct {
	pools map[int]*BufferPool
	sizes []int
	mu    sync.RWMutex
}

// NewSizedBufferPool creates a pool with multiple buffer sizes
func NewSizedBufferPool(sizes ...int) *SizedBufferPool {
	if len(sizes) == 0 {
		sizes = []int{1024, 4096, 16384, 65536, 262144}
	}

	p := &SizedBufferPool{
		pools: make(map[int]*BufferPool),
		sizes: sizes,
	}

	for _, size := range sizes {
		p.pools[size] = NewBufferPool(size)
	}

	return p
}

// Get gets a buffer closest to the requested size
func (p *SizedBufferPool) Get(size int) []byte {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Find the smallest buffer that fits
	for _, s := range p.sizes {
		if s >= size {
			return p.pools[s].Get()
		}
	}

	// Return largest if nothing fits
	return p.pools[p.sizes[len(p.sizes)-1]].Get()
}

// Put returns a buffer to the appropriate pool
func (p *SizedBufferPool) Put(buf []byte) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	cap := cap(buf)
	for _, size := range p.sizes {
		if cap <= size {
			p.pools[size].Put(buf)
			return
		}
	}
}

// ResponseBufferPool provides buffers for HTTP responses
type ResponseBufferPool struct {
	bodyPool   *SizedBufferPool
	headerPool *sync.Pool
}

// NewResponseBufferPool creates a response buffer pool
func NewResponseBufferPool() *ResponseBufferPool {
	return &ResponseBufferPool{
		bodyPool: NewSizedBufferPool(1024, 4096, 16384, 65536, 262144, 1048576),
		headerPool: &sync.Pool{
			New: func() interface{} {
				return make(map[string][]string, 32)
			},
		},
	}
}

// GetBodyBuffer gets a body buffer
func (p *ResponseBufferPool) GetBodyBuffer(size int) []byte {
	return p.bodyPool.Get(size)
}

// PutBodyBuffer returns a body buffer
func (p *ResponseBufferPool) PutBodyBuffer(buf []byte) {
	p.bodyPool.Put(buf)
}

// GetHeaderMap gets a header map
func (p *ResponseBufferPool) GetHeaderMap() map[string][]string {
	return p.headerPool.Get().(map[string][]string)
}

// PutHeaderMap returns a header map
func (p *ResponseBufferPool) PutHeaderMap(headers map[string][]string) {
	// Clear the map
	for k := range headers {
		delete(headers, k)
	}
	p.headerPool.Put(headers)
}

// JSONParserPool provides pooled JSON decoders
type JSONParserPool struct {
	pool sync.Pool
}

// NewJSONParserPool creates a JSON parser pool
func NewJSONParserPool() *JSONParserPool {
	return &JSONParserPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make(map[string]interface{}, 32)
			},
		},
	}
}

// Get gets a JSON object from pool
func (p *JSONParserPool) Get() map[string]interface{} {
	return p.pool.Get().(map[string]interface{})
}

// Put returns a JSON object to pool
func (p *JSONParserPool) Put(v map[string]interface{}) {
	// Clear the map
	for k := range v {
		delete(v, k)
	}
	p.pool.Put(v)
}

// CacheEntryPool provides pooled cache entries
type CacheEntryPool struct {
	pool sync.Pool
}

// PooledCacheEntry represents a cached response (pooled version)
type PooledCacheEntry struct {
	Key        string
	Body       []byte
	Headers    map[string]string
	StatusCode int
	Expiration time.Time
	ETag       string
	Compressed bool
}

// NewCacheEntryPool creates a cache entry pool
func NewCacheEntryPool() *CacheEntryPool {
	return &CacheEntryPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &PooledCacheEntry{
					Headers: make(map[string]string, 16),
				}
			},
		},
	}
}

// Get retrieves a cache entry
func (p *CacheEntryPool) Get() *PooledCacheEntry {
	return p.pool.Get().(*PooledCacheEntry)
}

// Put returns a cache entry
func (p *CacheEntryPool) Put(e *PooledCacheEntry) {
	e.Key = ""
	e.Body = nil
	e.StatusCode = 0
	e.Expiration = time.Time{}
	e.ETag = ""
	e.Compressed = false
	for k := range e.Headers {
		delete(e.Headers, k)
	}
	p.pool.Put(e)
}

// Global pools for common use cases
var (
	// GlobalResponsePool is the global response buffer pool
	GlobalResponsePool = NewResponseBufferPool()

	// GlobalJSONPool is the global JSON parser pool
	GlobalJSONPool = NewJSONParserPool()

	// GlobalCacheEntryPool is the global cache entry pool
	GlobalCacheEntryPool = NewCacheEntryPool()
)

// RequestContextPool provides pooled request contexts
type RequestContextPool struct {
	pool sync.Pool
}

// RequestContext holds per-request data
type RequestContext struct {
	StartTime  time.Time
	RequestID  string
	UserID     string
	APIKeyID   string
	RouteID    string
	ServiceID  string
	Metadata   map[string]interface{}
}

// NewRequestContextPool creates a request context pool
func NewRequestContextPool() *RequestContextPool {
	return &RequestContextPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &RequestContext{
					Metadata: make(map[string]interface{}, 8),
				}
			},
		},
	}
}

// Get retrieves a context
func (p *RequestContextPool) Get() *RequestContext {
	ctx := p.pool.Get().(*RequestContext)
	ctx.StartTime = time.Now()
	return ctx
}

// Put returns a context
func (p *RequestContextPool) Put(ctx *RequestContext) {
	ctx.RequestID = ""
	ctx.UserID = ""
	ctx.APIKeyID = ""
	ctx.RouteID = ""
	ctx.ServiceID = ""
	for k := range ctx.Metadata {
		delete(ctx.Metadata, k)
	}
	p.pool.Put(ctx)
}

// Global request context pool
var GlobalRequestContextPool = NewRequestContextPool()
