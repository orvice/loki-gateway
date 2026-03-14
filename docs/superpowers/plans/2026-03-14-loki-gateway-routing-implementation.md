# Loki Gateway Routing Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Loki-compatible push routing with exact label matching and default-target query proxy behavior for v1.

**Architecture:** Add a focused HTTP handler + service + matcher + forwarder stack inside `internal/`, with startup-time validated config loaded via existing butterfly `AppConfig`. Push requests are split by stream labels and multicast to matched targets (fallback to default). Query endpoints are passthrough to the default target with `X-Scope-OrgID` header forwarding.

**Tech Stack:** Go, gin, net/http, butterfly framework, httptest, testing package

---

## Chunk 1: Config Model and Routing Core

### Task 1: Define and validate gateway config model

**Files:**
- Create: `internal/config/loki.go`
- Modify: `cmd/loki-gateway/main.go`
- Modify: `config/service.yaml`
- Test: `internal/config/loki_test.go`

- [ ] **Step 1: Write failing config validation tests**

```go
func TestValidateRejectsUnknownDefaultTarget(t *testing.T) { /* ... */ }
func TestValidateRejectsRuleWithUnknownTarget(t *testing.T) { /* ... */ }
func TestValidateAcceptsMinimalValidConfig(t *testing.T) { /* ... */ }
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/config -run TestValidate -v`
Expected: FAIL with undefined types/functions.

- [ ] **Step 3: Implement config structs and validation**

```go
type LokiConfig struct {
    DefaultTarget string       `yaml:"default_target"`
    Targets       []LokiTarget `yaml:"targets"`
    Rules         []RouteRule  `yaml:"rules"`
}
```

Add `Validate() error` that checks required fields and target references.

- [ ] **Step 4: Wire config into app bootstrap**

Update `AppConfig` in `cmd/loki-gateway/main.go` to include `Loki internalconfig.LokiConfig` and call `cfg.Loki.Validate()` during startup.

- [ ] **Step 5: Update example service config**

Add `default_target`, `targets`, `rules` to `config/service.yaml` with local-safe defaults.

- [ ] **Step 6: Run config tests**

Run: `go test ./internal/config -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/config/loki.go internal/config/loki_test.go cmd/loki-gateway/main.go config/service.yaml
git commit -m "feat: add loki routing config model and validation"
```

### Task 2: Implement exact-match routing matcher

**Files:**
- Create: `internal/routing/matcher.go`
- Test: `internal/routing/matcher_test.go`

- [ ] **Step 1: Write failing matcher tests**

Cover cases:
- exact match
- multiple rule hits
- duplicate target deduplication
- no match

- [ ] **Step 2: Run matcher tests to verify failure**

Run: `go test ./internal/routing -v`
Expected: FAIL with undefined matcher.

- [ ] **Step 3: Implement matcher**

Expose:

```go
func MatchTargets(labels map[string]string, rules []config.RouteRule) []string
```

Behavior: exact-equals, AND semantics, return deduplicated ordered targets.

- [ ] **Step 4: Run matcher tests**

Run: `go test ./internal/routing -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/routing/matcher.go internal/routing/matcher_test.go
git commit -m "feat: add exact label matcher for loki targets"
```

## Chunk 2: Push Path (Loki /push)

### Task 3: Build downstream forwarder client

**Files:**
- Create: `internal/forwarder/client.go`
- Test: `internal/forwarder/client_test.go`

- [ ] **Step 1: Write failing client tests with httptest server**

Cases:
- forwards body to downstream URL
- applies request timeout
- forwards `X-Scope-OrgID` when present

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/forwarder -v`
Expected: FAIL with missing client implementation.

- [ ] **Step 3: Implement HTTP forwarder**

Add a small interface:

```go
type Client interface {
    PostPush(ctx context.Context, target config.LokiTarget, body []byte, headers http.Header) error
    ProxyQuery(ctx context.Context, target config.LokiTarget, in *http.Request) (*http.Response, error)
}
```

- [ ] **Step 4: Run forwarder tests**

Run: `go test ./internal/forwarder -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/forwarder/client.go internal/forwarder/client_test.go
git commit -m "feat: add loki downstream forwarder client"
```

### Task 4: Implement push service routing and fanout

**Files:**
- Create: `internal/service/push.go`
- Test: `internal/service/push_test.go`

- [ ] **Step 1: Write failing push service tests**

Cases:
- route by rule to single target
- multicast to multiple matched targets
- fallback to default target when no match
- invalid payload returns error

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/service -run TestPush -v`
Expected: FAIL with undefined service.

- [ ] **Step 3: Implement minimal push service**

Parse Loki `streams` payload, build per-target payload buckets, call forwarder concurrently, return immediately for valid requests.

- [ ] **Step 4: Add non-blocking failure logging hook**

Ensure downstream errors are logged/metric-hooked but do not fail accepted push requests.

- [ ] **Step 5: Run push service tests**

Run: `go test ./internal/service -run TestPush -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/service/push.go internal/service/push_test.go
git commit -m "feat: implement loki push routing and multicast"
```

### Task 5: Expose `POST /loki/api/v1/push`

**Files:**
- Create: `internal/httpapi/handler_push.go`
- Modify: `cmd/loki-gateway/main.go`
- Test: `internal/httpapi/handler_push_test.go`

- [ ] **Step 1: Write failing HTTP handler tests**

Cases:
- valid push returns `204`
- invalid payload returns `400`

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/httpapi -run TestPushHandler -v`
Expected: FAIL due to missing route/handler.

- [ ] **Step 3: Implement push handler and route wiring**

Wire `POST /loki/api/v1/push` into gin with push service dependency.

- [ ] **Step 4: Run push handler tests**

Run: `go test ./internal/httpapi -run TestPushHandler -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/handler_push.go internal/httpapi/handler_push_test.go cmd/loki-gateway/main.go
git commit -m "feat: expose loki push endpoint"
```

## Chunk 3: Query Path + End-to-End Verification

### Task 6: Implement query proxy service

**Files:**
- Create: `internal/service/query.go`
- Test: `internal/service/query_test.go`

- [ ] **Step 1: Write failing query service tests**

Cover supported endpoints:
- `/loki/api/v1/query`
- `/loki/api/v1/query_range`
- `/loki/api/v1/labels`
- `/loki/api/v1/label/{name}/values`

Verify proxy uses `default_target` and forwards `X-Scope-OrgID`.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/service -run TestQuery -v`
Expected: FAIL with missing query service.

- [ ] **Step 3: Implement query service passthrough**

Return downstream response status/body unchanged; map downstream unreachable to typed error for handler `502`.

- [ ] **Step 4: Run query service tests**

Run: `go test ./internal/service -run TestQuery -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/query.go internal/service/query_test.go
git commit -m "feat: add default-target loki query proxy service"
```

### Task 7: Expose 4 query endpoints in HTTP handler

**Files:**
- Create: `internal/httpapi/handler_query.go`
- Modify: `cmd/loki-gateway/main.go`
- Test: `internal/httpapi/handler_query_test.go`

- [ ] **Step 1: Write failing handler tests**

Cases:
- each supported query path returns downstream response
- downstream unavailable returns `502`

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/httpapi -run TestQueryHandler -v`
Expected: FAIL due to missing routes.

- [ ] **Step 3: Implement query handlers and route registration**

Register exact 4 GET paths only.

- [ ] **Step 4: Run query handler tests**

Run: `go test ./internal/httpapi -run TestQueryHandler -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/handler_query.go internal/httpapi/handler_query_test.go cmd/loki-gateway/main.go
git commit -m "feat: expose loki query passthrough endpoints"
```

### Task 8: Add integrated behavior tests and final verification

**Files:**
- Create: `internal/httpapi/integration_test.go`
- Modify: `README.md`

- [ ] **Step 1: Write failing integration tests**

Scenarios:
- push request with mixed streams routes to matched + default targets
- query endpoints proxy to default target

- [ ] **Step 2: Run integration tests to verify failure**

Run: `go test ./internal/httpapi -run TestIntegration -v`
Expected: FAIL before wiring completion.

- [ ] **Step 3: Implement test fixtures/mocks and pass tests**

Use `httptest` downstream Loki servers and assert received paths/bodies/headers.

- [ ] **Step 4: Run full test suite**

Run: `make test`
Expected: all tests PASS.

- [ ] **Step 5: Update README usage section**

Document supported endpoints, routing behavior, and config example.

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/integration_test.go README.md
git commit -m "test: add integration coverage for loki routing gateway"
```

## Execution Notes

- Keep each task independently releasable.
- Do not add config hot-reload or expression-based routing in this plan.
- If payload parsing needs gzip/snappy handling not currently in scope, capture it in a follow-up spec rather than expanding this plan.
