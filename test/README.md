# APICerebrus Testing Guide

This directory contains test helpers and fixtures to make testing APICerebrus easier and more consistent.

## Overview

The `testhelpers` package provides utilities for:

- **Mock Store**: In-memory SQLite database with test data
- **HTTP Testing**: Test servers and request/response assertions
- **Fixtures**: Pre-built test data factories for users, API keys, routes, services, and consumers
- **Plugin Testing**: Plugin registry and pipeline execution helpers

## Quick Start

```go
package mypackage_test

import (
    "testing"
    "github.com/APICerberus/APICerebrus/test/helpers"
)

func TestMyFeature(t *testing.T) {
    // Create a mock store with test data
    store := testhelpers.NewMockStoreWithData(t)
    
    // Create a test HTTP server
    server := testhelpers.NewTestServer(t, myHandler)
    
    // Make requests and assert responses
    resp := server.GET("/api/test", nil)
    testhelpers.AssertStatusOK(t, resp)
}
```

## Store Helpers (`test/helpers/store.go`)

### Creating a Mock Store

```go
// Empty store
store := testhelpers.NewMockStore(t)

// Store with pre-populated test data (users, API keys)
store := testhelpers.NewMockStoreWithData(t)
```

### Working with Users

```go
// Create a user
user := &store.User{
    Email: "test@example.com",
    Name:  "Test User",
    // ... other fields
}
store.MustCreateUser(user)

// Find users
user := store.MustFindUserByEmail("test@example.com")
user := store.MustFindUserByID("user-123")

// Assertions
store.AssertUserExists("test@example.com")
store.AssertUserNotExists("missing@example.com")
```

### Working with API Keys

```go
// Create an API key
fullKey, apiKey := store.MustCreateAPIKey(user.ID, "My Key", "live")

// Resolve an API key
user, key := store.MustResolveAPIKey("ck_live_xxxx")
```

### Direct Database Operations

```go
// Execute SQL
store.Exec("INSERT INTO users (id, email) VALUES (?, ?)", id, email)

// Query
rows := store.Query("SELECT * FROM users WHERE status = ?", "active")

// Count rows
count := store.Count("users", "status = ?", "active")

// Truncate tables
store.Truncate("users", "api_keys")

// Transactions
err := store.WithTransaction(func(tx *sql.Tx) error {
    // ... use tx for operations
    return nil
})
```

## HTTP Helpers (`test/helpers/http.go`)

### Creating a Test Server

```go
server := testhelpers.NewTestServer(t, myHandler)
defer server.Close()

// Or use TLS
server := testhelpers.NewTestTLSServer(t, myHandler)
```

### Making Requests

```go
// Simple requests
resp := server.GET("/api/users", nil)
resp := server.POST("/api/users", userData, nil)
resp := server.PUT("/api/users/1", updateData, nil)
resp := server.PATCH("/api/users/1", patchData, nil)
resp := server.DELETE("/api/users/1", nil)

// With headers
resp := server.GET("/api/protected", map[string]string{
    "Authorization": "Bearer token123",
})

// With custom method and body
resp := server.MakeRequest("POST", "/api/upload", fileData, headers)
```

### Response Assertions

```go
// Status codes
testhelpers.AssertStatusOK(t, resp)
testhelpers.AssertStatusCreated(t, resp)
testhelpers.AssertStatusNoContent(t, resp)
testhelpers.AssertStatusBadRequest(t, resp)
testhelpers.AssertStatusUnauthorized(t, resp)
testhelpers.AssertStatusForbidden(t, resp)
testhelpers.AssertStatusNotFound(t, resp)
testhelpers.AssertStatusInternalServerError(t, resp)
testhelpers.AssertStatus(t, resp, http.StatusTeapot)

// JSON
testhelpers.AssertJSON(t, resp, `{"id": "123", "name": "Test"}`)
testhelpers.AssertJSONContains(t, resp, map[string]any{
    "name": "Test",
    "status": "active",
})

// Headers
testhelpers.AssertHeader(t, resp, "Content-Type", "application/json")
testhelpers.AssertHeaderContains(t, resp, "Content-Type", "json")

// Body
testhelpers.AssertBodyContains(t, resp, "expected content")

// Parse JSON
var result MyStruct
testhelpers.ParseJSON(t, resp, &result)
```

### Request Building

```go
req := testhelpers.NewRequest(t, "POST", "http://example.com/api", body)
req = testhelpers.WithBearerAuth(req, "token123")
req = testhelpers.WithAPIKey(req, "ck_live_xxx")
req = testhelpers.WithJSONContentType(req)
req = testhelpers.WithHeaders(req, map[string]string{
    "X-Custom": "value",
})
```

### Direct Handler Testing

```go
recorder := testhelpers.NewRecorder()
handler.ServeHTTP(recorder, req)
testhelpers.AssertStatus(t, recorder.Result(), http.StatusOK)
```

## Fixtures (`test/helpers/fixtures.go`)

### Users

```go
// Basic user with defaults
user := testhelpers.FixtureUser()

// With options
user := testhelpers.FixtureUser(
    testhelpers.WithEmail("custom@example.com"),
    testhelpers.WithName("Custom Name"),
    testhelpers.WithRole("admin"),
    testhelpers.WithStatus("active"),
    testhelpers.WithCreditBalance(5000),
    testhelpers.WithCompany("Acme Inc"),
)

// Predefined fixtures
admin := testhelpers.FixtureAdmin()
suspended := testhelpers.FixtureSuspendedUser()
```

### API Keys

```go
key := testhelpers.FixtureAPIKey(
    testhelpers.WithUserID("user-123"),
    testhelpers.WithKeyName("Production Key"),
    testhelpers.WithKeyPrefix("ck_live_"),
)
```

### Routes

```go
route := testhelpers.FixtureRoute()

route := testhelpers.FixtureRoute(
    testhelpers.WithRouteName("api-route"),
    testhelpers.WithRouteService("api-service"),
    testhelpers.WithRouteHosts([]string{"api.example.com"}),
    testhelpers.WithRoutePaths([]string{"/v1/*"}),
    testhelpers.WithRouteMethods([]string{"GET", "POST"}),
    testhelpers.WithRouteStripPath(true),
    testhelpers.WithRoutePriority(100),
    testhelpers.AddRoutePlugin(testhelpers.FixtureCORSPlugin(
        []string{"*"},
        []string{"GET", "POST"},
        []string{"Content-Type"},
    )),
)
```

### Services

```go
service := testhelpers.FixtureService()

service := testhelpers.FixtureService(
    testhelpers.WithServiceName("my-service"),
    testhelpers.WithServiceProtocol("https"),
    testhelpers.WithServiceUpstream("my-upstream"),
    testhelpers.WithServiceTimeouts(5*time.Second, 30*time.Second, 30*time.Second),
)
```

### Consumers

```go
consumer := testhelpers.FixtureConsumer()

consumer := testhelpers.FixtureConsumer(
    testhelpers.WithConsumerName("my-consumer"),
    testhelpers.WithConsumerRateLimit(1000, 1500),
    testhelpers.WithConsumerACLGroups([]string{"premium"}),
    testhelpers.AddConsumerAPIKey(config.ConsumerAPIKey{
        Key: "ck_live_custom",
    }),
)
```

### Upstreams

```go
upstream := testhelpers.FixtureUpstream()

upstream := testhelpers.FixtureUpstream(
    testhelpers.WithUpstreamName("my-upstream"),
    testhelpers.WithUpstreamAlgorithm("least_conn"),
    testhelpers.AddUpstreamTarget(config.UpstreamTarget{
        Address: "localhost:8081",
        Weight:  100,
    }),
)
```

### Plugin Configurations

```go
// CORS
cors := testhelpers.FixtureCORSPlugin(
    []string{"https://example.com"},
    []string{"GET", "POST"},
    []string{"Content-Type", "Authorization"},
)

// Rate Limit
rateLimit := testhelpers.FixtureRateLimitPlugin(100, 150)

// Auth API Key
auth := testhelpers.FixtureAuthAPIKeyPlugin()

// Request Size Limit
sizeLimit := testhelpers.FixtureRequestSizeLimitPlugin(1024 * 1024) // 1MB

// Custom plugin
custom := testhelpers.FixturePluginConfig("my-plugin", true, map[string]any{
    "setting": "value",
})
```

### Full Configuration

```go
cfg := testhelpers.FixtureConfig()

cfg := testhelpers.FixtureConfig(
    testhelpers.WithGatewayAddr("localhost:9090"),
    testhelpers.WithAdminAPIKey("secret-key"),
    testhelpers.WithRoutes([]config.Route{
        testhelpers.FixtureRoute(),
    }),
    testhelpers.WithGlobalPlugins([]config.PluginConfig{
        testhelpers.FixtureCORSPlugin([]string{"*"}, []string{"GET"}, []string{}),
    }),
)
```

## Plugin Helpers (`test/helpers/plugin.go`)

### Creating a Test Context

```go
// Basic context
ctx := testhelpers.NewTestPipelineContext(t)

// With route
ctx := testhelpers.NewTestPipelineContextWithRoute(t, route)

// With consumer
ctx := testhelpers.NewTestPipelineContextWithConsumer(t, consumer)

// Customizing
ctx := testhelpers.NewTestPipelineContext(t).
    WithRoute(&route).
    WithConsumer(&consumer).
    WithMetadata("key", "value")
```

### Running Plugins

```go
// Single plugin
handled, err := ctx.RunPlugin(plugin)

// Plugin chain
plugins := []plugin.PipelinePlugin{plugin1, plugin2, plugin3}
handled, err := ctx.RunPluginChain(plugins)

// Post-proxy hooks
ctx.RunPostProxy(plugins, nil) // or pass proxy error
```

### Assertions

```go
ctx.AssertAborted()
ctx.AssertNotAborted()
ctx.AssertAbortReason("rate limit exceeded")
ctx.AssertStatus(http.StatusTooManyRequests)
ctx.AssertHeader("X-RateLimit-Limit", "100")
ctx.AssertHeaderContains("Content-Type", "json")
ctx.AssertBodyContains("error message")
```

### Mock Plugins

```go
// Simple mock
mock := testhelpers.NewMockPlugin("my-plugin", plugin.PhasePreAuth).
    WithRun(func(ctx *plugin.PipelineContext) (bool, error) {
        // Plugin logic
        return false, nil
    })

// Aborting plugin
abortPlugin := testhelpers.AbortingMockPlugin(
    "rate-limiter",
    plugin.PhasePreAuth,
    http.StatusTooManyRequests,
    "rate limit exceeded",
)

// Error plugin
errorPlugin := testhelpers.ErrorMockPlugin(
    "auth",
    plugin.PhaseAuth,
    errors.New("authentication failed"),
)

// Recording plugin (tracks if called)
plugin, called := testhelpers.RecordingMockPlugin("recorder", plugin.PhasePreProxy)
// ... run plugin ...
if !*called {
    t.Error("Plugin was not called")
}
```

### Plugin Registry

```go
// Empty registry
registry := testhelpers.NewTestPluginRegistry(t)

// Default registry with built-in plugins
registry := testhelpers.NewDefaultTestRegistry(t)

// Register custom plugin
registry.MustRegister("custom", func(spec config.PluginConfig, ctx plugin.BuilderContext) (plugin.PipelinePlugin, error) {
    // Factory implementation
})

// Build plugin
p := registry.MustBuild(config.PluginConfig{Name: "cors"}, ctx)
```

### Building Pipelines

```go
// Build from configs
plugins := testhelpers.BuildDefaultTestPipeline(t, route.Plugins, consumers)

// Build with custom registry
plugins := testhelpers.BuildTestPipeline(t, registry, specs, builderCtx)

// Build all route pipelines
pipelines, hasAuth := testhelpers.MustBuildRoutePipelines(t, cfg, consumers)
```

### Builder Context

```go
// Basic context
ctx := testhelpers.NewTestBuilderContext(consumers)

// Custom permission lookup
ctx := plugin.BuilderContext{
    Consumers:        consumers,
    APIKeyLookup:     myKeyLookup,
    PermissionLookup: testhelpers.NewDenyAllPermissionLookup(),
}
```

### Test Utilities

```go
// Create test consumer
consumer := testhelpers.TestConsumer("id", "name", "key1", "key2")

// Create test route
route := testhelpers.TestRoute("name", "service", []string{"GET"}, []string{"/test"})

// Common errors
err := testhelpers.ErrTestPluginError
```

## Best Practices

1. **Always use `t.Helper()`**: All helper functions call `t.Helper()` so failures report the correct line number.

2. **Use `t.Cleanup()`**: Mock stores and test servers automatically clean up when the test completes.

3. **Prefer fixtures over manual creation**: Use fixture functions to ensure consistent test data.

4. **Use options pattern**: When customizing fixtures, use the provided option functions rather than modifying fields directly.

5. **Assert specific values**: Use `AssertJSONContains` instead of `AssertJSON` when you only care about specific fields.

6. **Test one thing at a time**: Create fresh contexts for each plugin test to avoid interference.

## Examples

### Integration Test Example

```go
func TestCreateUser(t *testing.T) {
    // Setup
    store := testhelpers.NewMockStore(t)
    handler := NewUserHandler(store)
    server := testhelpers.NewTestServer(t, handler)
    
    // Execute
    newUser := map[string]string{
        "email": "new@example.com",
        "name":  "New User",
    }
    resp := server.POST("/api/users", newUser, map[string]string{
        "Authorization": "Bearer admin-token",
    })
    
    // Assert
    testhelpers.AssertStatusCreated(t, resp)
    testhelpers.AssertJSONContains(t, resp, map[string]any{
        "email": "new@example.com",
        "name":  "New User",
    })
    
    // Verify database state
    store.AssertUserExists("new@example.com")
}
```

### Plugin Test Example

```go
func TestRateLimitPlugin(t *testing.T) {
    // Setup
    ctx := testhelpers.NewTestPipelineContext(t)
    ctx.WithConsumer(&testhelpers.FixtureConsumer())
    
    spec := testhelpers.FixtureRateLimitPlugin(10, 15)
    registry := testhelpers.NewDefaultTestRegistry(t)
    p := registry.MustBuild(spec, testhelpers.NewTestBuilderContext(nil))
    
    // Execute
    handled, err := ctx.RunPlugin(p)
    
    // Assert
    if err != nil {
        t.Fatalf("Unexpected error: %v", err)
    }
    if handled {
        t.Error("Expected plugin not to handle request")
    }
    ctx.AssertNotAborted()
}
```

### Unit Test Example

```go
func TestUserValidation(t *testing.T) {
    // Setup
    user := testhelpers.FixtureUser(
        testhelpers.WithEmail("invalid-email"),
    )
    
    // Execute
    err := ValidateUser(user)
    
    // Assert
    if err == nil {
        t.Error("Expected validation error for invalid email")
    }
}
```
