# Loki Gateway Routing Design

Date: 2026-03-14
Status: Reviewed draft

## 1. Goal and Scope

Build a Grafana Loki compatible forwarding service that:

- Accepts Loki push traffic at `POST /loki/api/v1/push`.
- Reads routing rules from config.
- Routes streams by exact label matching (`label == value`) to one or more downstream Loki targets.
- Falls back to a default Loki target when no rule matches.
- Supports query endpoints by forwarding all queries to the default Loki target only.

In scope for v1:

- `POST /loki/api/v1/push` with rule-based routing.
- Query passthrough endpoints:
  - `GET /loki/api/v1/query`
  - `GET /loki/api/v1/query_range`
  - `GET /loki/api/v1/labels`
  - `GET /loki/api/v1/label/{name}/values`
- Static config loaded at startup only (config changes require restart).

Out of scope for v1:

- Non-exact matching (regex, expression language, OR syntax).
- Dynamic config reload.
- Durable queueing/replay for failed forwarding.
- Query routing by labels.

## 2. Requirements

### Functional requirements

- Push traffic must be accepted via Loki-compatible endpoint.
- Routing decision is based on stream labels.
- A stream can match multiple rules; forwarding must multicast to all matched targets.
- If no rule matches, the stream must be sent to `default_target`.
- Query traffic must be proxied to `default_target` only.
- `X-Scope-OrgID` header must be forwarded for query requests.

### Behavioral requirements

- Push API response to client is always fast-success (gateway does not block on downstream result).
- Downstream push failures are recorded in logs and metrics.
- Query API returns downstream response status/body from default Loki.

### Validation requirements

- Startup fails if `default_target` is missing or references an unknown target.
- Startup fails if any rule references an unknown target.

## 3. Configuration Model

Top-level YAML structure:

```yaml
default_target: "loki-a"

targets:
  - name: "loki-a"
    url: "http://loki-a:3100"
    tenant_id: ""
    timeout_ms: 3000
  - name: "loki-b"
    url: "http://loki-b:3100"
    tenant_id: ""

rules:
  - name: "prod-core"
    match:
      cluster: "prod"
      team: "core"
    target: "loki-b"
  - name: "staging"
    match:
      env: "staging"
    target: "loki-a"
```

### Config semantics

- `default_target`: target name used when no routing rule matches.
- `targets[]`: downstream Loki instances.
- `rules[]`:
  - `match` uses exact equality and AND semantics across all listed labels.
  - `target` refers to one item in `targets[]`.

## 4. API Behavior

### 4.1 Push: `POST /loki/api/v1/push`

Flow:

1. Decode Loki push payload.
2. For each stream, parse labels and evaluate routing rules.
3. Collect all matched targets for that stream.
4. If none matched, assign stream to `default_target`.
5. Group streams by target and build one push payload per target.
6. Dispatch forwarding concurrently.
7. Return success to caller without waiting for downstream completion.

Response behavior:

- Valid request: return `204 No Content` immediately.
- Invalid payload format: return `400`.

Notes:

- If a stream matches multiple rules mapping to same target, target list must be deduplicated.
- Grouping by target reduces duplicate network calls.

### 4.2 Query passthrough

Supported endpoints:

- `GET /loki/api/v1/query`
- `GET /loki/api/v1/query_range`
- `GET /loki/api/v1/labels`
- `GET /loki/api/v1/label/{name}/values`

Flow:

1. Accept incoming query request and keep query parameters unchanged.
2. Proxy request to `default_target`.
3. Forward `X-Scope-OrgID` header.
4. Return downstream status and body directly.

Failure behavior:

- If default Loki is unreachable/timeout: return `502`.

## 5. Internal Components and Boundaries

### `http/handler`

Responsibilities:

- Route the 5 supported endpoints.
- Validate minimal request shape.
- Invoke service layer.
- Produce HTTP responses.

Non-responsibilities:

- No routing decisions.
- No downstream HTTP business logic.

### `routing/matcher`

Responsibilities:

- Input: stream labels + in-memory rules.
- Output: list of matched target names.
- Apply exact match with AND semantics.

Non-responsibilities:

- No network logic.
- No payload mutations.

### `service/push_service`

Responsibilities:

- Parse stream-level labels.
- Call matcher and apply default fallback.
- Build per-target grouped push payloads.
- Trigger concurrent forwarding tasks.
- Emit logging/metrics for per-target forwarding outcomes.

Non-responsibilities:

- No direct HTTP route wiring.

### `service/query_service`

Responsibilities:

- Build passthrough requests to default Loki for supported query endpoints.
- Preserve query params and pass through `X-Scope-OrgID`.
- Return downstream status/body to handler.

### `forwarder/client`

Responsibilities:

- Execute HTTP requests to downstream Loki targets.
- Apply per-target timeout.
- Optional per-target tenant header behavior if configured.

Non-responsibilities:

- No routing policy.

### `config`

Responsibilities:

- Load config once at startup.
- Validate references and mandatory fields.
- Provide immutable runtime config objects.

## 6. Data Flow Summary

Push path:

- Client -> Push Handler -> Push Service -> Matcher -> Target Buckets -> Forwarder (parallel) -> Downstream Loki targets

Query path:

- Client -> Query Handler -> Query Service -> Forwarder -> Default Loki -> Client

## 7. Error Handling and Observability

### Push path

- Parse/validation error: `400`.
- Downstream forwarding error: log and metric only.
- Client-facing push result: success for accepted valid request.

### Query path

- Downstream unavailable or timeout: `502`.
- Otherwise mirror downstream response.

### Observability

Add structured logs and metrics with at least:

- `forward_attempt_total{target, endpoint}`
- `forward_success_total{target, endpoint}`
- `forward_fail_total{target, endpoint, reason}`
- Optional latency histogram per target.

## 8. Testing Strategy

### Unit tests

- `routing/matcher`
  - exact match success
  - multiple rules hit
  - no match fallback behavior trigger point
  - target deduplication
- `service/push_service`
  - stream-to-target bucketing
  - grouped payload per target
  - downstream failures do not change success path
- `service/query_service`
  - all four query endpoints proxy to default
  - query parameters preserved
  - `X-Scope-OrgID` forwarded

### Integration tests (httptest)

- `POST /loki/api/v1/push`
  - valid body -> success
  - invalid body -> `400`
- Query endpoints
  - downstream success passthrough
  - downstream unavailable -> `502`

## 9. Acceptance Criteria

- Supports `POST /loki/api/v1/push` rule-based routing.
- Exact label matching (`label == value`) is implemented with AND semantics.
- Multi-match multicast works (one stream to multiple targets).
- No-match fallback routes to `default_target`.
- Supports all 4 query endpoints and always proxies to `default_target`.
- Query request forwards `X-Scope-OrgID`.
- Push downstream failure is observable in logs/metrics and does not fail client request.
- Tests pass via `make test`.

## 10. Future Extensions (Non-blocking)

- Config hot reload.
- More expressive rule language.
- Durable async delivery for push reliability.
- Query-side routing strategies.
