# Architecture Decision Records

## ADR-001: SQLite as Primary Database

**Status**: Accepted
**Date**: 2026-04-10
**Context**: Database selection for API Gateway state storage

### Decision
Use `modernc.org/sqlite` (pure Go, no CGO) as the primary database for all gateway state: users, API keys, routes, services, upstreams, audit logs, billing credits, sessions.

### Rationale
- **Zero external dependencies**: Pure Go implementation means no CGO, no system-level SQLite installation needed, and simpler cross-compilation
- **Deployment simplicity**: Single binary + data file — no separate database server to provision, monitor, or fail over
- **Gateway workload profile**: API gateways are read-heavy (route lookup, auth validation) with occasional writes (config changes, credit deductions). SQLite handles 100K+ reads/s easily
- **WAL mode**: Write-ahead logging enables concurrent readers with minimal writer contention
- **BUSY retry**: Implemented exponential backoff on SQLITE_BUSY for concurrent write paths (billing, audit batching)

### Trade-offs
- **Write contention**: SQLite serializes writes. Under very high write throughput (audit logging, billing at 10K+ req/s with body capture), this becomes a bottleneck
- **No horizontal scaling**: Single-writer limitation means this doesn't scale across multiple gateway instances without Raft replication
- **Mitigation**: Audit logging uses async buffered batch writes; billing uses atomic transactions with retry; Raft clustering replicates state across nodes
- **Future**: Consider PostgreSQL migration for v2.0 if multi-writer throughput becomes a bottleneck

---

## ADR-002: Custom YAML Parser

**Status**: Accepted
**Date**: 2026-04-10
**Context**: Configuration file parsing for gateway, admin, and portal settings

### Decision
Implement a custom YAML parser (`internal/pkg/yaml/`) rather than using `gopkg.in/yaml.v3` or similar third-party library.

### Rationale
- **Security**: Third-party YAML parsers have had CVEs related to billion-laughs attacks, entity expansion bombs, and unsafe type resolution. Custom parser eliminates this attack surface
- **Scope control**: Gateway config YAML has a known, bounded schema — we don't need full YAML spec compliance, just enough for our config structure
- **Dependency reduction**: Fewer external dependencies means smaller binary, fewer CVEs to track, simpler supply chain security
- **Fuzz-tested**: Custom parser has adversarial fuzz tests (YAML bombs, deeply nested structures, malformed anchors/aliases)

### Trade-offs
- **Maintenance**: Must handle edge cases manually
- **Limited features**: No support for complex YAML features (multi-doc, tags, custom resolvers) — but these aren't needed for config

---

## ADR-003: 5-Phase Plugin Pipeline

**Status**: Accepted
**Date**: 2026-04-10
**Context**: Plugin execution ordering for request processing

### Decision
Execute plugins in 5 fixed phases: `PRE_AUTH → AUTH → PRE_PROXY → PROXY → POST_PROXY`

### Rationale
- **Clear separation of concerns**: Each phase has a distinct purpose — PRE_AUTH handles correlation IDs and IP restrictions, AUTH validates credentials, PRE_PROXY applies rate limiting and transforms, PROXY manages circuit breaking and retries, POST_PROXY handles response transforms
- **Fail-fast**: Auth failures short-circuit before proxy work begins, saving resources
- **Deterministic ordering**: Fixed phase order eliminates plugin ordering ambiguity and race conditions
- **Parallelizable**: Independent plugins within a phase can execute concurrently (via `OptimizedPipeline.executeParallel()`)

### Trade-offs
- **Sequential cross-phase**: Plugins in later phases can't influence earlier phases (e.g., a PROXY plugin can't affect AUTH behavior)
- **Rigidity**: Adding a new phase requires code changes, not just config

---

## ADR-004: SubnetAware Load Balancing over Geo-aware

**Status**: Accepted
**Date**: 2026-04-10
**Context**: Default geographic/network-aware routing algorithm

### Decision
Use `SubnetAware` balancer (IP first-two-octet grouping) as the default network-aware algorithm. `geo_aware` is deprecated as an alias. True MaxMind GeoIP2 integration deferred.

### Rationale
- **No external dependencies**: Subnet grouping requires no GeoIP database, no license management, no database updates
- **Predictable behavior**: First-two-octet grouping maps to `/16` subnets, which often aligns with organizational network boundaries
- **Low overhead**: Simple string comparison, no database lookups per request
- **Configurable**: Can be extended to true GeoIP in the future by swapping the grouping function

### Trade-offs
- **Coarse granularity**: `/16` subnets don't map to geographic regions — two IPs in `10.0.x.x` could be on different continents
- **Private IP bias**: Most useful for internal gateway deployments where IP ranges map to datacenters
- **Future**: MaxMind GeoIP2 integration planned for true geographic routing (see Beyond v1.0 roadmap)
