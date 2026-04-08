package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"
)

// Executor executes federated GraphQL queries.
type Executor struct {
	client            *http.Client
	subscriptions     map[string]*SubscriptionConnection
	subscriptionsMu   sync.RWMutex
	queryCache        *QueryCache
	circuitBreakers   map[string]*CircuitBreaker
	circuitBreakersMu sync.RWMutex
}

// SubscriptionConnection represents an active subscription to a subgraph.
type SubscriptionConnection struct {
	ID         string
	Subgraph   *Subgraph
	Query      string
	Variables  map[string]any
	Conn       *websocket.Conn
	CancelFunc context.CancelFunc
	Messages   chan *SubscriptionMessage
	Errors     chan error
	Done       chan struct{}
}

// SubscriptionMessage represents a message from a subscription.
type SubscriptionMessage struct {
	ID    string
	Data  map[string]any
	Error error
}

// QueryCache caches parsed query plans.
type QueryCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
	maxSize int
}

// CacheEntry represents a cached query plan.
type CacheEntry struct {
	Plan      *Plan
	Timestamp time.Time
	HitCount  atomic.Int32
}

// CircuitBreaker implements circuit breaker pattern for subgraph calls.
type CircuitBreaker struct {
	mu              sync.Mutex
	failures        int
	lastFailureTime time.Time
	state           CircuitState
	threshold       int
	resetTimeout    time.Duration
}

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// QueryOptimizer optimizes query execution plans.
type QueryOptimizer struct {
	enabled bool
}

// OptimizedPlan represents an optimized execution plan.
type OptimizedPlan struct {
	Plan           *Plan
	ExecutionOrder []string
	ParallelGroups [][]string
	EstimatedCost  int
}

// ExecutionResult represents the result of executing a plan.
type ExecutionResult struct {
	Data   map[string]any `json:"data,omitempty"`
	Errors []ExecutionError       `json:"errors,omitempty"`
}

// ExecutionError represents an execution error.
type ExecutionError struct {
	Message    string                 `json:"message"`
	Path       []string               `json:"path,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

// NewExecutor creates a new executor.
func NewExecutor() *Executor {
	return &Executor{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		subscriptions:   make(map[string]*SubscriptionConnection),
		queryCache:      NewQueryCache(1000),
		circuitBreakers: make(map[string]*CircuitBreaker),
	}
}

// NewQueryCache creates a new query cache.
func NewQueryCache(maxSize int) *QueryCache {
	return &QueryCache{
		entries: make(map[string]*CacheEntry),
		maxSize: maxSize,
	}
}

// Get retrieves a cached plan.
func (qc *QueryCache) Get(query string) (*Plan, bool) {
	qc.mu.RLock()
	defer qc.mu.RUnlock()

	entry, ok := qc.entries[query]
	if !ok {
		return nil, false
	}

	entry.HitCount.Add(1)
	return entry.Plan, true
}

// Set stores a plan in the cache.
func (qc *QueryCache) Set(query string, plan *Plan) {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	if len(qc.entries) >= qc.maxSize {
		qc.evictOldest()
	}

	qc.entries[query] = &CacheEntry{
		Plan:      plan,
		Timestamp: time.Now(),
		
	}
}

// evictOldest removes the oldest entry from the cache.
func (qc *QueryCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for k, v := range qc.entries {
		if oldestKey == "" || v.Timestamp.Before(oldestTime) {
			oldestKey = k
			oldestTime = v.Timestamp
		}
	}

	if oldestKey != "" {
		delete(qc.entries, oldestKey)
	}
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(threshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:    threshold,
		resetTimeout: resetTimeout,
		state:        CircuitClosed,
	}
}

// CanExecute checks if a request can be executed.
func (cb *CircuitBreaker) CanExecute() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = CircuitHalfOpen
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	}

	return true
}

// RecordSuccess records a successful execution.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
	}
	cb.failures = 0
}

// RecordFailure records a failed execution.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	if cb.failures >= cb.threshold {
		cb.state = CircuitOpen
	}
}

// getCircuitBreaker gets or creates a circuit breaker for a subgraph.
func (e *Executor) getCircuitBreaker(subgraphID string) *CircuitBreaker {
	e.circuitBreakersMu.RLock()
	cb, ok := e.circuitBreakers[subgraphID]
	e.circuitBreakersMu.RUnlock()

	if !ok {
		e.circuitBreakersMu.Lock()
		cb = NewCircuitBreaker(5, 30*time.Second)
		e.circuitBreakers[subgraphID] = cb
		e.circuitBreakersMu.Unlock()
	}

	return cb
}

// Execute executes a plan.
func (e *Executor) Execute(ctx context.Context, plan *Plan) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Data:   make(map[string]any),
		Errors: make([]ExecutionError, 0),
	}

	// Execute steps in dependency order
	executedSteps := make(map[string]map[string]any)
	stepResults := make(map[string]map[string]any)

	for _, step := range plan.Steps {
		// Check if dependencies are met
		deps := plan.DependsOn[step.ID]
		depData := make(map[string]any)
		for _, depID := range deps {
			if data, ok := executedSteps[depID]; ok {
				// Merge dependency data
				for k, v := range data {
					depData[k] = v
				}
			}
		}

		// Execute step
		stepData, err := e.executeStep(ctx, step, depData)
		if err != nil {
			result.Errors = append(result.Errors, ExecutionError{
				Message: fmt.Sprintf("step %s failed: %v", step.ID, err),
				Path:    step.Path,
			})
			continue
		}

		executedSteps[step.ID] = stepData
		stepResults[step.ID] = stepData

		// Merge into final result
		e.mergeResult(result.Data, stepData, step.Path)
	}

	return result, nil
}

// executeStep executes a single plan step.
func (e *Executor) executeStep(ctx context.Context, step *PlanStep, depData map[string]any) (map[string]any, error) {
	// Prepare variables
	variables := make(map[string]any)
	for k, v := range step.Variables {
		variables[k] = v
	}

	// Add dependency data as variables if this is an entity resolution
	if len(depData) > 0 && step.ResultType != "scalar" {
		variables["representations"] = e.buildRepresentations(depData, step.ResultType)
	}

	// Build request
	reqBody := map[string]any{
		"query":     step.Query,
		"variables": variables,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", step.Subgraph.URL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range step.Subgraph.Headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subgraph returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return nil, err
	}

	var subgraphResp struct {
		Data   map[string]any `json:"data"`
		Errors []struct {
			Message string   `json:"message"`
			Path    []string `json:"path"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &subgraphResp); err != nil {
		return nil, err
	}

	// Extract data
	if subgraphResp.Data != nil {
		// Handle _entities response
		if entities, ok := subgraphResp.Data["_entities"].([]any); ok {
			if len(entities) > 0 {
				if entity, ok := entities[0].(map[string]any); ok {
					return entity, nil
				}
			}
		}

		// Return first field result
		for _, v := range subgraphResp.Data {
			if data, ok := v.(map[string]any); ok {
				return data, nil
			}
		}
	}

	return subgraphResp.Data, nil
}

// buildRepresentations builds entity representations for Apollo Federation.
func (e *Executor) buildRepresentations(depData map[string]any, typeName string) []any {
	representations := make([]any, 0)

	// Build representation with __typename and key fields
	rep := make(map[string]any)
	rep["__typename"] = typeName
	for k, v := range depData {
		rep[k] = v
	}

	representations = append(representations, rep)
	return representations
}

// mergeResult merges step result into the final result.
func (e *Executor) mergeResult(data map[string]any, stepData map[string]any, path []string) {
	if len(path) == 0 {
		for k, v := range stepData {
			data[k] = v
		}
		return
	}

	// Navigate to the correct position in the data tree
	current := data
	for i, key := range path[:len(path)-1] {
		if _, ok := current[key]; !ok {
			current[key] = make(map[string]any)
		}
		if next, ok := current[key].(map[string]any); ok {
			current = next
		} else {
			// Cannot navigate further
			return
		}

		_ = i
	}

	// Set the final value
	lastKey := path[len(path)-1]
	if existing, ok := current[lastKey].(map[string]any); ok {
		// Merge with existing data
		for k, v := range stepData {
			existing[k] = v
		}
	} else {
		current[lastKey] = stepData
	}
}

// ExecuteParallel executes steps in parallel where possible.
func (e *Executor) ExecuteParallel(ctx context.Context, plan *Plan) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Data:   make(map[string]any),
		Errors: make([]ExecutionError, 0),
	}

	// Build execution graph
	pendingSteps := make(map[string]*PlanStep)
	completedSteps := make(map[string]map[string]any)
	inProgress := make(map[string]bool)

	for _, step := range plan.Steps {
		pendingSteps[step.ID] = step
	}

	// Execute steps
	var wg sync.WaitGroup
	var mu sync.Mutex

	for len(pendingSteps) > 0 {
		// Find steps that can be executed (all dependencies met)
		executable := make([]*PlanStep, 0)
		for _, step := range pendingSteps {
			if inProgress[step.ID] {
				continue
			}

			canExecute := true
			for _, depID := range plan.DependsOn[step.ID] {
				if _, ok := completedSteps[depID]; !ok {
					canExecute = false
					break
				}
			}

			if canExecute {
				executable = append(executable, step)
			}
		}

		if len(executable) == 0 {
			// Check for deadlock
			if len(pendingSteps) > 0 && len(inProgress) == 0 {
				return nil, fmt.Errorf("deadlock detected: unable to execute remaining steps")
			}
			// Wait for some steps to complete
			break
		}

		// Execute steps in parallel
		for _, step := range executable {
			wg.Add(1)
			inProgress[step.ID] = true

			go func(s *PlanStep) {
				defer wg.Done()

				// Gather dependency data
				depData := make(map[string]any)
				for _, depID := range plan.DependsOn[s.ID] {
					if data, ok := completedSteps[depID]; ok {
						for k, v := range data {
							depData[k] = v
						}
					}
				}

				// Execute
				stepData, err := e.executeStep(ctx, s, depData)

				mu.Lock()
				defer mu.Unlock()

				if err != nil {
					result.Errors = append(result.Errors, ExecutionError{
						Message: fmt.Sprintf("step %s failed: %v", s.ID, err),
						Path:    s.Path,
					})
				} else {
					completedSteps[s.ID] = stepData
					e.mergeResult(result.Data, stepData, s.Path)
				}

				delete(inProgress, s.ID)
				delete(pendingSteps, s.ID)
			}(step)
		}

		// Wait for batch to complete
		wg.Wait()
	}

	return result, nil
}

// BatchRequest represents a batched GraphQL request.
type BatchRequest struct {
	Queries []string
}

// BatchResponse represents a batched GraphQL response.
type BatchResponse struct {
	Results []map[string]any
	Errors  []ExecutionError
}

// ExecuteBatch executes multiple queries in a batch.
func (e *Executor) ExecuteBatch(ctx context.Context, subgraph *Subgraph, batch *BatchRequest) (*BatchResponse, error) {
	response := &BatchResponse{
		Results: make([]map[string]any, 0, len(batch.Queries)),
		Errors:  make([]ExecutionError, 0),
	}

	// Build batched query
	var sb strings.Builder
	sb.WriteString("{\n")

	for i, query := range batch.Queries {
		sb.WriteString(fmt.Sprintf("  batch_%d: %s\n", i, query))
	}

	sb.WriteString("}")

	reqBody := map[string]any{
		"query": sb.String(),
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", subgraph.URL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return nil, err
	}

	var batchResp map[string]any
	if err := json.Unmarshal(body, &batchResp); err != nil {
		return nil, err
	}

	// Extract results
	for i := range batch.Queries {
		key := fmt.Sprintf("batch_%d", i)
		if data, ok := batchResp[key]; ok {
			if d, ok := data.(map[string]any); ok {
				response.Results = append(response.Results, d)
			}
		}
	}

	return response, nil
}

// ExecuteSubscription executes a GraphQL subscription and returns a channel for receiving updates.
func (e *Executor) ExecuteSubscription(ctx context.Context, plan *Plan) (*SubscriptionConnection, error) {
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("no steps in subscription plan")
	}

	// Subscriptions typically have a single step
	step := plan.Steps[0]
	if step.Subgraph == nil {
		return nil, fmt.Errorf("subscription step has no subgraph")
	}

	// Generate subscription ID
	subID := fmt.Sprintf("sub_%d", time.Now().UnixNano())

	// Create subscription connection
	sub := &SubscriptionConnection{
		ID:        subID,
		Subgraph:  step.Subgraph,
		Query:     step.Query,
		Variables: step.Variables,
		Messages:  make(chan *SubscriptionMessage, 100),
		Errors:    make(chan error, 10),
		Done:      make(chan struct{}),
	}

	// Store subscription
	e.subscriptionsMu.Lock()
	e.subscriptions[subID] = sub
	e.subscriptionsMu.Unlock()

	// Start subscription goroutine
	go e.runSubscription(sub, step)

	return sub, nil
}

// runSubscription manages the WebSocket subscription connection.
func (e *Executor) runSubscription(sub *SubscriptionConnection, step *PlanStep) {
	defer close(sub.Done)
	defer close(sub.Messages)
	defer close(sub.Errors)

	// Convert HTTP URL to WebSocket URL
	wsURL := sub.Subgraph.URL
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)

	// Add /graphql or /subscriptions path if not present
	if !strings.Contains(wsURL, "/graphql") && !strings.Contains(wsURL, "/subscriptions") {
		wsURL = strings.TrimSuffix(wsURL, "/") + "/graphql"
	}

	// Connect to WebSocket
	ws, err := websocket.Dial(wsURL, "", wsURL)
	if err != nil {
		sub.Errors <- fmt.Errorf("websocket connection failed: %w", err)
		return
	}
	sub.Conn = ws

	// Send subscription initialization message
	initMsg := map[string]any{
		"type": "connection_init",
	}
	if err := websocket.JSON.Send(ws, initMsg); err != nil {
		sub.Errors <- fmt.Errorf("websocket init failed: %w", err)
		return
	}

	// Send subscription start message
	subMsg := map[string]any{
		"type": "start",
		"id":   sub.ID,
		"payload": map[string]any{
			"query":     step.Query,
			"variables": step.Variables,
		},
	}
	if err := websocket.JSON.Send(ws, subMsg); err != nil {
		sub.Errors <- fmt.Errorf("subscription start failed: %w", err)
		return
	}

	// Listen for messages
	for {
		var msg struct {
			Type    string                 `json:"type"`
			ID      string                 `json:"id"`
			Payload map[string]any `json:"payload"`
		}

		if err := websocket.JSON.Receive(ws, &msg); err != nil {
			if err != io.EOF {
				sub.Errors <- fmt.Errorf("websocket receive error: %w", err)
			}
			return
		}

		switch msg.Type {
		case "data":
			if msg.ID == sub.ID && msg.Payload != nil {
				sub.Messages <- &SubscriptionMessage{
					ID:   sub.ID,
					Data: msg.Payload,
				}
			}
		case "error":
			if msg.ID == sub.ID {
				sub.Errors <- fmt.Errorf("subscription error: %v", msg.Payload)
			}
		case "complete":
			if msg.ID == sub.ID {
				return
			}
		}
	}
}

// StopSubscription stops an active subscription.
func (e *Executor) StopSubscription(subID string) error {
	e.subscriptionsMu.Lock()
	sub, ok := e.subscriptions[subID]
	delete(e.subscriptions, subID)
	e.subscriptionsMu.Unlock()

	if !ok {
		return fmt.Errorf("subscription %s not found", subID)
	}

	if sub.Conn != nil {
		// Send stop message
		stopMsg := map[string]any{
			"type": "stop",
			"id":   subID,
		}
		_ = websocket.JSON.Send(sub.Conn, stopMsg)
		sub.Conn.Close()
	}

	return nil
}

// GetActiveSubscriptions returns a list of active subscription IDs.
func (e *Executor) GetActiveSubscriptions() []string {
	e.subscriptionsMu.RLock()
	defer e.subscriptionsMu.RUnlock()

	ids := make([]string, 0, len(e.subscriptions))
	for id := range e.subscriptions {
		ids = append(ids, id)
	}
	return ids
}

// OptimizePlan optimizes the execution plan for better performance.
func (e *Executor) OptimizePlan(plan *Plan) *OptimizedPlan {
	optimized := &OptimizedPlan{
		Plan:           plan,
		ExecutionOrder: make([]string, 0, len(plan.Steps)),
		ParallelGroups: make([][]string, 0),
	}

	// Group steps by dependencies for parallel execution
	executed := make(map[string]bool)
	pending := make(map[string]*PlanStep)

	for _, step := range plan.Steps {
		pending[step.ID] = step
	}

	for len(pending) > 0 {
		group := make([]string, 0)

		for id, _ := range pending {
			// Check if all dependencies are met
			canExecute := true
			for _, depID := range plan.DependsOn[id] {
				if !executed[depID] {
					canExecute = false
					break
				}
			}

			if canExecute {
				group = append(group, id)
			}
		}

		if len(group) == 0 && len(pending) > 0 {
			// Deadlock detected, break
			break
		}

		for _, id := range group {
			optimized.ExecutionOrder = append(optimized.ExecutionOrder, id)
			executed[id] = true
			delete(pending, id)
		}

		if len(group) > 0 {
			optimized.ParallelGroups = append(optimized.ParallelGroups, group)
		}
	}

	// Calculate estimated cost based on number of parallel groups
	optimized.EstimatedCost = len(optimized.ParallelGroups) * 10

	return optimized
}

// ExecuteOptimized executes an optimized plan.
func (e *Executor) ExecuteOptimized(ctx context.Context, optimized *OptimizedPlan) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Data:   make(map[string]any),
		Errors: make([]ExecutionError, 0),
	}

	completedSteps := make(map[string]map[string]any)
	stepMap := make(map[string]*PlanStep)

	for _, step := range optimized.Plan.Steps {
		stepMap[step.ID] = step
	}

	// Execute each parallel group
	for _, group := range optimized.ParallelGroups {
		var wg sync.WaitGroup
		var mu sync.Mutex
		groupResults := make(map[string]map[string]any)
		groupErrors := make([]ExecutionError, 0)

		for _, stepID := range group {
			step := stepMap[stepID]
			wg.Add(1)

			go func(s *PlanStep) {
				defer wg.Done()

				// Gather dependency data
				depData := make(map[string]any)
				for _, depID := range optimized.Plan.DependsOn[s.ID] {
					if data, ok := completedSteps[depID]; ok {
						for k, v := range data {
							depData[k] = v
						}
					}
				}

				// Check circuit breaker
				cb := e.getCircuitBreaker(s.Subgraph.ID)
				if !cb.CanExecute() {
					mu.Lock()
					groupErrors = append(groupErrors, ExecutionError{
						Message: fmt.Sprintf("circuit breaker open for subgraph %s", s.Subgraph.ID),
						Path:    s.Path,
					})
					mu.Unlock()
					return
				}

				// Execute step
				stepData, err := e.executeStep(ctx, s, depData)

				mu.Lock()
				defer mu.Unlock()

				if err != nil {
					cb.RecordFailure()
					groupErrors = append(groupErrors, ExecutionError{
						Message: fmt.Sprintf("step %s failed: %v", s.ID, err),
						Path:    s.Path,
					})
				} else {
					cb.RecordSuccess()
					groupResults[s.ID] = stepData
				}
			}(step)
		}

		wg.Wait()

		// Merge results
		for id, data := range groupResults {
			completedSteps[id] = data
			step := stepMap[id]
			e.mergeResult(result.Data, data, step.Path)
		}

		// Append errors
		result.Errors = append(result.Errors, groupErrors...)
	}

	return result, nil
}
