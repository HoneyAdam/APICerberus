# WebAssembly Plugin Support

APICerebrus supports loading and executing WebAssembly (WASM) plugins, allowing third-party developers to extend gateway functionality using any language that compiles to WASM.

## Overview

WebAssembly plugins provide:

- **Language Agnostic**: Write plugins in Rust, C/C++, Go, AssemblyScript, or any WASM-targeting language
- **Sandboxed Execution**: Plugins run in a secure sandbox with limited memory and execution time
- **Hot Reloading**: Load and unload plugins without restarting the gateway
- **Performance**: Near-native execution speed with minimal overhead
- **Isolation**: Plugin crashes don't affect the gateway

## How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│                    APICEREBRUS GATEWAY                          │
│                                                                 │
│  ┌──────────────┐      ┌──────────────┐     ┌──────────────┐   │
│  │   Pipeline   │─────►│ WASM Runtime │────►│ WASM Plugin  │   │
│  │              │◄─────│   (Wazero)   │◄────│   (.wasm)    │   │
│  └──────────────┘      └──────────────┘     └──────────────┘   │
│         │                                                      │
│         │    Host Functions                                     │
│         │    - log()                                            │
│         │    - get_header()                                     │
│         │    - set_header()                                     │
│         │    - get_metadata()                                   │
│         │    - abort()                                          │
│         ▼                                                      │
│  ┌──────────────┐                                              │
│  │   Upstream   │                                              │
│  └──────────────┘                                              │
└─────────────────────────────────────────────────────────────────┘
```

## Configuration

Add WASM configuration to your gateway config:

```yaml
wasm:
  enabled: true
  module_dir: "./plugins/wasm"
  max_memory: 134217728  # 128MB per plugin
  max_execution: "30s"   # Maximum execution time
  allow_filesystem: false
  allowed_paths:
    "/cache": "./wasm-cache"
  env_vars:
    ENV: "production"
    LOG_LEVEL: "info"
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable WASM plugins |
| `module_dir` | string | `"./plugins/wasm"` | Directory for WASM modules |
| `max_memory` | int64 | `134217728` | Max memory per plugin (bytes) |
| `max_execution` | duration | `"30s"` | Max execution time per call |
| `allow_filesystem` | bool | `false` | Allow filesystem access |
| `allowed_paths` | map | `{}` | Guest path → host path mappings |
| `env_vars` | map | `{}` | Environment variables for plugins |

## Writing a WASM Plugin

### Rust Example

```rust
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Serialize, Deserialize)]
struct PluginContext {
    method: String,
    path: String,
    query: String,
    headers: HashMap<String, String>,
    consumer_id: String,
    correlation_id: String,
    metadata: HashMap<String, serde_json::Value>,
}

#[derive(Serialize, Deserialize)]
struct PluginResponse {
    handled: bool,
    error: Option<String>,
    context: Option<PluginContext>,
}

// Host function imports
extern {
    fn host_log(level: i32, ptr: i32, len: i32);
    fn host_get_header(name_ptr: i32, name_len: i32) -> i64;
    fn host_set_header(name_ptr: i32, name_len: i32, val_ptr: i32, val_len: i32);
    fn host_abort(reason_ptr: i32, reason_len: i32);
}

#[no_mangle]
pub extern "C" fn process(ptr: i32, len: i32) -> i64 {
    // Read input context
    let input = unsafe {
        let slice = std::slice::from_raw_parts(ptr as *const u8, len as usize);
        String::from_utf8_lossy(slice)
    };
    
    let ctx: PluginContext = serde_json::from_str(&input).unwrap();
    
    // Plugin logic
    let mut response = PluginResponse {
        handled: false,
        error: None,
        context: Some(ctx),
    };
    
    // Example: Check custom header
    if ctx.headers.get("X-Custom-Auth").is_none() {
        response.error = Some("Missing X-Custom-Auth header".to_string());
        response.handled = true;
    }
    
    // Serialize response
    let output = serde_json::to_string(&response).unwrap();
    let bytes = output.into_bytes();
    
    // Return pointer/length packed into i64
    let ptr = bytes.as_ptr() as i64;
    let len = bytes.len() as i64;
    std::mem::forget(bytes);
    
    (ptr << 32) | len
}
```

### Go Example (using TinyGo)

```go
package main

import (
    "encoding/json"
    "unsafe"
)

//go:wasmimport env host_log
func host_log(level int32, ptr int32, len int32)

//go:wasmimport env host_abort
func host_abort(ptr int32, len int32)

func logInfo(msg string) {
    buf := []byte(msg)
    host_log(1, int32(uintptr(unsafe.Pointer(&buf[0]))), int32(len(buf)))
}

type Context struct {
    Method        string                 `json:"method"`
    Path          string                 `json:"path"`
    Headers       map[string]string      `json:"headers"`
    ConsumerID    string                 `json:"consumer_id"`
    CorrelationID string                 `json:"correlation_id"`
    Metadata      map[string]interface{} `json:"metadata"`
}

type Response struct {
    Handled bool    `json:"handled"`
    Error   string  `json:"error,omitempty"`
}

//export process
func process(ptr int32, len int32) int64 {
    // Read input
    input := make([]byte, len)
    copy(input, unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), len))
    
    var ctx Context
    json.Unmarshal(input, &ctx)
    
    logInfo("Processing request: " + ctx.Path)
    
    response := Response{Handled: false}
    
    // Your logic here
    if ctx.Path == "/admin" && ctx.ConsumerID == "" {
        response.Handled = true
        response.Error = "Admin access denied"
    }
    
    // Return response
    output, _ := json.Marshal(response)
    
    // Allocate and copy output
    outPtr := malloc(int32(len(output)))
    copy(unsafe.Slice((*byte)(unsafe.Pointer(uintptr(outPtr))), len(output)), output)
    
    return (int64(outPtr) << 32) | int64(len(output))
}

//export malloc
func malloc(size int32) int32 {
    buf := make([]byte, size)
    return int32(uintptr(unsafe.Pointer(&buf[0])))
}

func main() {}
```

Compile with TinyGo:

```bash
tinygo build -o plugin.wasm -target wasm -no-debug plugin.go
```

## Host Functions

Plugins can call these host functions provided by the gateway:

### `host_log(level, ptr, len)`
Log a message from the plugin.

- `level`: 0=debug, 1=info, 2=warn, 3=error
- `ptr`: Pointer to message bytes
- `len`: Message length

### `host_get_header(name_ptr, name_len) -> i64`
Get a request header value.

Returns packed i64: `(ptr << 32) | len` or `-1` if not found.

### `host_set_header(name_ptr, name_len, val_ptr, val_len)`
Set a response header.

### `host_get_metadata(key_ptr, key_len) -> i64`
Get a metadata value.

Returns packed i64: `(ptr << 32) | len` or `-1` if not found.

### `host_set_metadata(key_ptr, key_len, val_ptr, val_len)`
Set a metadata value.

### `host_abort(reason_ptr, reason_len)`
Abort request processing with a reason.

## Loading Plugins

### Via Configuration

```yaml
routes:
  - name: "api-route"
    paths:
      - "/api/*"
    plugins:
      - name: "wasm"
        config:
          module_id: "custom-auth"
          module_path: "./plugins/custom-auth.wasm"
          phase: "pre-auth"
          priority: 10
          name: "Custom Auth"
          version: "1.0.0"
```

### Via Admin API

```bash
curl -X POST http://localhost:9876/admin/wasm/plugins \
  -H "Content-Type: application/json" \
  -H "X-Admin-Key: $ADMIN_KEY" \
  -d '{
    "id": "custom-auth",
    "path": "/plugins/custom-auth.wasm",
    "config": {
      "phase": "pre-auth",
      "priority": 10
    }
  }'
```

## Security Considerations

### Memory Limits

Each plugin is restricted to its configured memory limit. Exceeding this limit causes an immediate termination.

### Execution Time

Plugins that exceed `max_execution` are terminated.

### No Network Access

By default, plugins cannot make network connections. Use the main gateway plugins for external calls.

### Filesystem Isolation

When `allow_filesystem: true`, plugins can only access paths explicitly listed in `allowed_paths`.

### Capability-Based Security

Future versions will support WASI capabilities for fine-grained permission control.

## Performance

### Startup Time

- Module validation: ~1-5ms
- Compilation/instantiation: ~10-50ms

### Execution Overhead

- Function call overhead: ~1-5µs
- Context serialization: ~10-100µs (depends on size)
- Total per-request: ~50-500µs

### Memory

- Runtime overhead: ~5MB per plugin
- Per-request: ~10KB for context

## Debugging

### Enable Debug Logging

```yaml
logging:
  level: "debug"
```

### Plugin Logs

Plugin logs are prefixed with `[WASM:module-id]`:

```
2026-04-07T10:30:00Z [WASM:custom-auth] [INFO] Processing request: /api/users
2026-04-07T10:30:00Z [WASM:custom-auth] [ERROR] Invalid token format
```

### Testing Plugins

Use the `wasmtime` CLI for local testing:

```bash
wasmtime run --env ENV=test plugin.wasm
```

## Troubleshooting

### "wasm module not found"

Verify the path is correct and the module has been uploaded.

### "invalid wasm magic number"

The file is not a valid WASM module. Recompile with the correct target.

### "memory limit exceeded"

Reduce memory usage in your plugin or increase `max_memory`.

### "execution timeout"

Optimize your plugin or increase `max_execution`.

### "unknown import"

Your plugin imports a function not provided by the host. Check host function signatures.

## Example Use Cases

### Custom Authentication

```rust
// Validate custom JWT format or API key patterns
```

### Request Transformation

```rust
// Modify request body format between client and upstream
```

### Bot Detection

```rust
// Custom fingerprinting logic
```

### Geo-blocking

```rust
// IP-based geographic restrictions
```

### Content Filtering

```rust
// Scan request/response bodies for sensitive data
```

## Future Enhancements

- [ ] WASI support for standard I/O
- [ ] Plugin-to-plugin communication
- [ ] Shared memory regions
- [ ] Async/await support
- [ ] Plugin marketplace integration
- [ ] Hot code swapping

---

*Document Version: 1.0*  
*APICerebrus Version: 1.0.0*
