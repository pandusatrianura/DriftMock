# Implementation Architecture: DriftMock

> **Document Version:** 1.0.0  
> **Status:** Draft for Review (Phase 1: Specify)  
> **Complements:** [specification.md](file:///e:/github/pandusatrianura/DriftMock/docs/specification.md) | [driftmock.md](file:///e:/github/pandusatrianura/DriftMock/docs/ideas/driftmock.md)

---

## 1. System Architecture Overview

DriftMock operates as a modular, local-first reverse proxy written in Go. The architecture consists of six decoupled packages in `internal/`, communicating through well-defined Go interfaces.

```
                  ┌──────────────────────────────────────────┐
                  │          Client Application              │
                  │  (Web Frontend / Mobile App / Test Suite) │
                  └────────────────────┬─────────────────────┘
                                       │ HTTP Requests
                                       ▼
                  ┌──────────────────────────────────────────┐
                  │         DriftMock CLI (Cobra)            │
                  │        (cmd/driftmock/main.go)           │
                  └────────────────────┬─────────────────────┘
                                       │
                ┌──────────────────────┴──────────────────────┐
                │                                             │
      [ Record / Replay Mode ]                        [ Check Mode ]
                │                                             │
                ▼                                             ▼
  ┌───────────────────────────┐                 ┌───────────────────────────┐
  │   Proxy & Recording       │                 │   Contract Checker        │
  │   Engine (internal/proxy) │                 │   (internal/cli/check)    │
  └─────────────┬─────────────┘                 └─────────────┬─────────────┘
                │                                             │
     Forward /  │ Intercept                                   │ Compare
     Response   ▼                                             ▼
  ┌───────────────────────────┐                 ┌───────────────────────────┐
  │   Sanitizer Pipeline      │                 │   Semantic Diff Engine    │
  │  (internal/sanitizer)     │                 │   (internal/diff)         │
  └─────────────┬─────────────┘                 └─────────────┬─────────────┘
                │ Clean Payload                               │
                ▼                                             │
  ┌───────────────────────────┐                               │
  │   Storage Engine          │◄──────────────────────────────┘
  │   (internal/storage)      │       Load Stubs (.drift/stubs/*.json)
  └─────────────┬─────────────┘
                ▲
                │ Match Request
  ┌─────────────┴─────────────┐
  │   Deterministic Matcher   │
  │   (internal/matcher)      │
  └───────────────────────────┘
```

---

## 2. Stub File Schema Definition

All recorded interactions are stored as standalone, formatted JSON files in `.drift/stubs/` using the following schema:

```json
{
  "version": "1.0",
  "id": "get_api_v1_users_sort_asc",
  "recorded_at": "2026-11-17T18:00:00Z",
  "request": {
    "method": "GET",
    "path": "/api/v1/users",
    "normalized_path": "/api/v1/users",
    "query": {
      "limit": ["10"],
      "sort": ["asc"]
    },
    "headers": {
      "Accept": ["application/json"]
    },
    "body_hash": ""
  },
  "response": {
    "status_code": 200,
    "headers": {
      "Content-Type": ["application/json; charset=utf-8"]
    },
    "body": {
      "data": [
        {
          "id": 1,
          "name": "Jane Doe",
          "email": "jane@example.com",
          "role": "admin"
        }
      ],
      "meta": {
        "total": 1,
        "page": 1
      }
    },
    "latency_ms": 142
  }
}
```

### Go Model Representation (`internal/model/stub.go`):
```go
package model

import "time"

type Stub struct {
	Version    string          `json:"version"`
	ID         string          `json:"id"`
	RecordedAt time.Time       `json:"recorded_at"`
	Request    RequestPattern  `json:"request"`
	Response   ResponsePattern `json:"response"`
}

type RequestPattern struct {
	Method         string              `json:"method"`
	Path           string              `json:"path"`
	NormalizedPath string              `json:"normalized_path"`
	Query          map[string][]string `json:"query,omitempty"`
	Headers        map[string][]string `json:"headers,omitempty"`
	BodyHash       string              `json:"body_hash,omitempty"`
}

type ResponsePattern struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Body       any                 `json:"body"`
	LatencyMs  int64               `json:"latency_ms"`
}
```

---

## 3. Subsystem Breakdown & Algorithms

### 3.1. Reverse Proxy & Recording Engine (`internal/proxy`)
- Utilizes Go's standard `net/http/httputil.ReverseProxy`.
- Custom `Director` rewrites the request `URL.Scheme`, `URL.Host`, and `Host` header to match the target staging URL.
- Custom `ModifyResponse` intercepts the `http.Response`:
  1. Inspects `Content-Encoding` (handles transparent gzip/deflate decompression if needed so payload is saved as readable JSON).
  2. Reads and duplicates response body using `io.TeeReader` or `io.ReadAll` with `bytes.NewBuffer`.
  3. Measures round-trip duration in milliseconds (`latency_ms`).
  4. Passes request and response to `SanitizerPipeline`.
  5. Asynchronously dispatches the sanitized `Stub` to `FileStore` for persistence.
  6. Restores the uncompressed or original stream to the downstream client.

### 3.2. Sanitizer Pipeline (`internal/sanitizer`)
Security and privacy must be guaranteed prior to any disk write.
1. **Header Sanitization:**
   - Drops or redacts: `Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, `X-Auth-Token`, `Proxy-Authorization`.
   - Headers are replaced with `"[REDACTED]"` or filtered completely depending on category.
2. **JSON Body Masking:**
   - Recursive depth-first traversal of parsed `map[string]any` and `[]any`.
   - Checks keys (case-insensitive substring and exact matches) against sensitive key blacklist:
     - `password`, `pass`, `secret`, `token`, `access_token`, `refresh_token`, `auth`, `api_key`, `credit_card`, `cvv`, `ssn`, `nik`.
   - Replaces leaf scalar value with `"[REDACTED]"`.

### 3.3. Deterministic Matcher & Path Collapsing (`internal/matcher`)
When running in `replay` mode, incoming requests must be matched deterministically against saved stubs using a hybrid two-tier approach.

#### Matching Priority Hierarchy:
1. **Tier 1 — Exact Match:**
   - `HTTP Method` matches.
   - Exact `Path` matches (e.g. `/api/v1/users/42`).
   - `Query Parameters` match exactly (ignoring parameter order; normalized by sorting keys).
   - If `POST`/`PUT`/`PATCH`: matches `BodyHash` (SHA-256 of normalized body) or falls back to path match if body is empty.
2. **Tier 2 — Pattern Fallback Match (Heuristic):**
   - Heuristic rule detects numeric identifiers (`/users/\d+`), UUIDs (`[0-9a-fA-F-]{36}`), and object IDs (`[0-9a-fA-F]{24}`).
   - During recording, computes and persists `normalized_path: "/api/v1/users/:id"` alongside concrete `path`.
   - If an incoming request path (e.g. `/api/v1/users/99`) has no Tier 1 exact stub, it matches against stubs sharing `normalized_path: "/api/v1/users/:id"`.
3. **Fallback Match:**
   - Matches method and path base, regardless of non-essential query parameters.

### 3.4. Semantic JSON Tree Diff Engine (`internal/diff`)
Comparing live staging responses with recorded snapshots requires semantic structural analysis rather than line-by-line text diffing.

#### Diff Algorithm Steps:
1. Parse both baseline stub body (`Expected`) and live staging response body (`Received`) into AST / generic representations.
2. Recursively traverse keys and indices:
   - **Key Missing in Received:** The key exists in Expected but not in Received.
     $\rightarrow$ Severity: 🔴 **BREAKING CHANGE** (`- path.to.field: expected <type>, got missing`).
   - **Type Mismatch:** The key exists, but type changed (e.g. expected `int`, got `string`, or expected `object`, got `array`).
     $\rightarrow$ Severity: 🔴 **BREAKING CHANGE** (`~ path.to.field: expected number, received string`).
   - **Nullability Shift:** Non-null field received as `null` without optional declaration.
     $\rightarrow$ Severity: 🔴 **BREAKING CHANGE** (`~ path.to.field: expected non-null string, received null`).
   - **Enum / Value Drift (Configurable):** Constant value or status changed.
     $\rightarrow$ Severity: 🟡 **WARNING** (`~ path.to.status: expected "ACTIVE", received "SUSPENDED"`).
   - **New Key Added:** The key exists in Received but not in Expected.
     $\rightarrow$ Severity: 🟢 **NON-BREAKING ADDITION** (`+ path.to.new_field: string`).
3. Output Formatter:
   - Formats results into high-contrast ANSI colors (Red, Yellow, Green) with Jest/Vitest style indicators (`-`, `~`, `+`).
   - Summarizes total changes and issues exit status (`0` for clean/compatible, `1` for breaking changes detected).

---

## 4. Operational Workflows

### 4.1. Record Workflow
```text
Client Application                 DriftMock Proxy                  Target Staging
       │                                  │                                │
       │──── 1. HTTP Request ────────────►│                                │
       │    (e.g. GET /api/v1/users)      │──── 2. Forward Request ───────►│
       │                                  │                                │
       │                                  │◄─── 3. Response + Payload ─────│
       │                                  │                                │
       │                                  ├──── 4. Sanitizer Pipeline      │
       │                                  │    (strip auth, mask keys)     │
       │                                  │                                │
       │                                  ├──── 5. Save to FileStore       │
       │                                  │    (.drift/stubs/GET_users.json)
       │                                  │                                │
       │◄─── 6. Return Original Resp ─────│                                │
```

### 4.2. Replay Workflow
```text
Client Application                 DriftMock Replay Server
       │                                  │
       │──── 1. HTTP Request ────────────►│
       │    (e.g. GET /api/v1/users)      ├──── 2. Matcher Engine
       │                                  │    (Method + Normalized Path + Sorted Query)
       │                                  │
       │                                  ├──── 3. Lookup in FileStore
       │                                  │    (Load .drift/stubs/GET_users.json)
       │                                  │
       │                                  ├──── 4. Latency Handling
       │                                  │    (Default: Snappy <5ms; if --simulate-delay: sleep latency_ms; if --latency <ms>: sleep <ms>)
       │                                  │
       │◄─── 5. Return Stored Response ───│ (Status Code, Headers, Mock Body)
```

### 4.3. Check Workflow
```text
DriftMock CLI                      Target Staging Server
       │                                    │
       ├──── 1. Load All Recorded Stubs     │
       │     from .drift/stubs/*.json       │
       │                                    │
       │──── 2. Send Live HTTP Request ────►│
       │     (using stored method & path)   │
       │                                    │
       │◄─── 3. Live Staging Response ──────│
       │                                    │
       ├──── 4. Execute Semantic Tree Diff  │
       │     (Baseline vs Live AST)         │
       │                                    │
       ├──── 5. Grade Severity              │
       │     (Breaking / Warning / Addition)│
       │                                    │
       ▼                                    │
 Terminal Report & Exit Code (0 or 1)       │
```

---

## 5. Storage & Naming Conventions

### Filename Generation Algorithm:
To maintain deterministic filenames across Windows and Unix file systems:
1. Normalize method to uppercase (e.g. `GET`).
2. Replace slashes `/` in path with underscores `_`.
3. If query parameters exist, compute an 8-character SHA-256 hash suffix of the sorted query string.
4. Format: `<METHOD>_<sanitized_path>[_<query_hash>].json`.

Examples:
- `GET /api/v1/users` $\rightarrow$ `GET_api_v1_users.json`
- `GET /api/v1/users?role=admin&sort=asc` $\rightarrow$ `GET_api_v1_users_a1b2c3d4.json`
- `POST /api/v1/auth/login` $\rightarrow$ `POST_api_v1_auth_login.json`

---

## 6. Resilience & Edge Cases

| Scenario | Handling Strategy |
|---|---|
| Target Server Offline during `record` | Return `502 Bad Gateway` to client with clean error JSON; do not write invalid stub to disk. |
| Stub Not Found during `replay` | Return `404 Not Found` with helpful diagnostic payload listing unmatched path and available registered stubs. |
| Large / Binary Payloads | Check `Content-Type`. If image/binary/octet-stream, record metadata but store payload as base64 string or skip recording if exceeding size limit (e.g. 5MB). |
| Compressed Responses (`gzip`, `br`) | Decompress in proxy pipeline so stored stubs remain readable JSON text; strip or adjust `Content-Encoding` header when saving stub. |
| CORS Preflight (`OPTIONS`) | Automatically respond with permissive CORS headers during `replay` mode so browser frontend doesn't block local requests. |
| Dynamic Timestamp Drift | In `check` mode, allow configurable ignoring of volatile timestamp fields (e.g. `updated_at`, `timestamp`, `created_at`) to avoid false-positive drift alerts. |

---

## 7. Dependencies Review

| Package | Purpose | Justification |
|---|---|---|
| `github.com/spf13/cobra` | CLI application framework | Industry standard for Go CLIs (Kubernetes, Hugo, GitHub CLI). POSIX flags, autocompletion. |
| `github.com/fatih/color` | Terminal color printing | Reliable ANSI escape code handling on Windows (VT100 enabled) and Unix. |
| `github.com/stretchr/testify` | Test assertions & suites | High-readability test assertions for unit and integration testing. |
| Go Standard Library | Everything else (`net/http`, `crypto`, `os`, `encoding/json`) | Eliminates bloat, guarantees lightning-fast compilation, zero external runtime dependency. |
