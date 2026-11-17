# Specification: DriftMock (Contract-Drift & Live Recording Proxy)

> **Document Version:** 1.0.0  
> **Status:** Draft for Review (Phase 1: Specify)  
> **Source Idea:** [driftmock.md](file:///e:/github/pandusatrianura/DriftMock/docs/ideas/driftmock.md)

---

## 1. Objective

DriftMock is a high-performance, single-binary CLI tool written in **Go** that acts as an intelligent, transparent reverse proxy between client applications (frontend web, mobile apps, QA automation test runners) and backend staging/development environments.

### Core Value Proposition:
1. **Zero-Effort Recording:** Passively records live HTTP traffic and generates clean, human-readable, Git-committable JSON mock stubs without requiring manual mock authoring.
2. **Instant Offline Mocking:** Serves recorded stubs deterministically when developing offline or when staging environments are degraded/unreachable.
3. **Automated Contract Drift Detection:** Performs semantic JSON tree comparisons between current backend responses and saved contract snapshots, highlighting breaking API changes directly in terminal output and CI pipelines.

### Primary Persona:
Frontend and Mobile Engineers, QA Automation Engineers, and Full-Stack Developers who need reliable API mocking without maintaining brittle mock fixtures or suffering silent API contract breakage.

---

## 2. Tech Stack

- **Language:** Go (1.24+)
- **CLI Framework:** `github.com/spf13/cobra` (POSIX-compliant CLI command structure)
- **Terminal Styling:** `github.com/fatih/color` (ANSI colored terminal output, Windows 10/11 VT100 support)
- **Testing & Assertions:** Standard Go `testing`, `net/http/httptest`, and `github.com/stretchr/testify`
- **Standard Library Centric:** `net/http`, `net/http/httputil`, `encoding/json`, `regexp`, `crypto/sha256`, `time`, `os`, `path/filepath`
- **Target Platforms:** Windows (`amd64`, `arm64`), macOS (`amd64`, `arm64`), Linux (`amd64`, `arm64`)

---

## 3. Commands

Full executable CLI commands and flags:

```bash
# Build
go build -o bin/driftmock cmd/driftmock/main.go

# Development Run
go run cmd/driftmock/main.go --help

# Test Suite with Coverage
go test -v -race -coverprofile=coverage.out ./...

# View Coverage in Browser
go tool cover -html=coverage.out

# Linter
golangci-lint run ./...

# Formatter
gofmt -s -w .
```

### CLI Command Interfaces:

#### 1. Record Mode
Intercepts outbound traffic, forwards to target staging, sanitizes sensitive data, and saves stubs to disk.
```bash
driftmock record --target https://api.staging.example.com --port 8080 --out ./.drift
# Short flags
driftmock record -t https://api.staging.example.com -p 8080 -o ./.drift
```

#### 2. Replay Mode
Serves recorded stubs offline with deterministic matching and simulated network latency.
```bash
driftmock replay --stubs ./.drift --port 8080
# Optional flag for simulated delay override (in ms, default uses recorded latency or 50ms)
driftmock replay --stubs ./.drift --port 8080 --latency 100
```

#### 3. Check Mode
Re-requests all recorded endpoints against live target and performs semantic tree diff to detect contract drift.
```bash
driftmock check --stubs ./.drift --target https://api.staging.example.com
# Fail CI build with exit code 1 if breaking changes exist
driftmock check --stubs ./.drift --target https://api.staging.example.com --strict
```

#### 4. Version & Info
```bash
driftmock version
```

---

## 4. Project Structure

```text
DriftMock/
├── .drift/                       # Default stubs directory (.gitignore candidate or committed for team sharing)
│   └── stubs/
│       ├── GET_api_v1_users.json
│       └── POST_api_v1_auth_login.json
├── cmd/
│   └── driftmock/
│       └── main.go               # Entry point
├── internal/
│   ├── cli/                      # Cobra command handlers
│   │   ├── root.go
│   │   ├── record.go
│   │   ├── replay.go
│   │   ├── check.go
│   │   └── version.go
│   ├── diff/                     # Semantic JSON tree diff engine
│   │   ├── differ.go
│   │   ├── differ_test.go
│   │   ├── formatter.go          # Jest/Vitest style colored output
│   │   └── severity.go           # Breaking, Warning, Non-Breaking categorization
│   ├── matcher/                  # Request deterministic matching
│   │   ├── matcher.go
│   │   ├── matcher_test.go
│   │   ├── normalizer.go         # Query param sorting, path normalization
│   │   └── path_collapse.go      # REST path parameter collapsing (/users/123 -> /users/:id)
│   ├── model/                    # Core domain entities & stub schemas
│   │   ├── stub.go
│   │   ├── request.go
│   │   ├── response.go
│   │   └── diff_result.go
│   ├── proxy/                    # Reverse proxy & recording engine
│   │   ├── handler.go
│   │   ├── recorder.go
│   │   ├── replayer.go
│   │   └── proxy_test.go
│   ├── sanitizer/                # Redaction & masking pipeline
│   │   ├── sanitizer.go
│   │   ├── sanitizer_test.go
│   │   ├── headers.go            # Auth header stripping
│   │   └── body_mask.go          # Recursive JSON key masking (passwords, tokens)
│   └── storage/                  # File I/O for stubs
│       ├── file_store.go
│       ├── file_store_test.go
│       └── naming.go             # Deterministic stub filename generator
├── tests/
│   ├── e2e/                      # End-to-end integration tests
│   │   ├── record_replay_test.go
│   │   └── drift_check_test.go
│   └── fixtures/                 # Mock payloads and expected diff fixtures
├── docs/
│   ├── ideas/
│   │   └── driftmock.md
│   ├── specification.md          # This specification document
│   └── implementation.md         # Technical architecture & design document
├── go.mod
├── go.sum
└── README.md
```

---

## 5. Code Style & Conventions

- Follow standard Go conventions (`effective_go`, idiomatic packages, single responsibility per file).
- Error handling: Never ignore errors. Wrap errors with contextual prefix using `fmt.Errorf("context: %w", err)`.
- Concurrency: Ensure goroutine safety for concurrent incoming HTTP proxy requests using `sync.RWMutex` or immutable structs.
- Logging: Clean, developer-friendly terminal output with structured colors. Avoid raw log spam.

### Reference Code Snippet:
```go
package sanitizer

import (
	"encoding/json"
	"strings"
)

var defaultSensitiveKeys = map[string]bool{
	"password":    true,
	"secret":      true,
	"token":       true,
	"access_token":true,
	"credit_card": true,
}

// MaskJSONBody traverses a JSON structure and replaces values of sensitive keys with "[REDACTED]".
func MaskJSONBody(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		// Non-JSON payload returned as is
		return data, nil
	}

	maskValue(root)
	return json.MarshalIndent(root, "", "  ")
}

func maskValue(val any) {
	switch v := val.(type) {
	case map[string]any:
		for k, child := range v {
			if defaultSensitiveKeys[strings.ToLower(k)] {
				v[k] = "[REDACTED]"
			} else {
				maskValue(child)
			}
		}
	case []any:
		for _, item := range v {
			maskValue(item)
		}
	}
}
```

---

## 6. Testing Strategy

1. **Unit Testing:**
   - Framework: Standard Go `testing` package with `testify/assert` and `testify/require`.
   - Coverage: Minimum 80% coverage on `internal/matcher`, `internal/sanitizer`, `internal/diff`, and `internal/storage`.
   - Table-driven tests for matcher edge cases, JSON diffing varieties (nulls, missing keys, array item shifts), and path parameter collapsing heuristics.
2. **Integration / HTTP Testing:**
   - Use `httptest.Server` to spin up ephemeral mock staging backends and test recording, forwarding, header stripping, and replay fidelity.
3. **End-to-End (E2E) CLI Testing:**
   - Execute CLI commands against a running test server to verify exit codes, file persistence on disk, and terminal drift output.

---

## 7. Boundaries & Guardrails

- **Always:**
  - Strip or redact authorization headers (`Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, `X-Auth-Token`) before writing stubs.
  - Return exit code `0` when no breaking changes are found in `check` mode, and exit code `1` when breaking changes exist and `--strict` is active.
  - Preserve HTTP response status codes, content-type headers, and payload schemas faithfully.
  - Keep stub files deterministic and human-readable with sorted JSON keys and 2-space indentation.
- **Ask First:**
  - Introducing external database engines or in-memory caches that mutate state across multiple requests.
  - Changing the stub storage schema or directory structure.
  - Adding non-standard CLI dependencies or external CGo bindings.
- **Never:**
  - Commit or persist plaintext tokens, passwords, or PII into `.drift/stubs`.
  - Require OS root/administrator privileges (no raw socket sniffing or TUN/TAP network interception).
  - Transmit user telemetry, analytics, or payload data to any remote cloud service.
  - Support binary protocols (gRPC, Protobuf, WebSockets) in v0.1 scope.

---

## 8. Success Criteria

1. **Single Binary Executable:** Builds into a standalone, portable binary without external runtime dependencies.
2. **Zero-Config Developer Setup:** Running `driftmock record --target https://api.staging.example.com` immediately proxies traffic on `localhost:8080`.
3. **Sanitization Guarantee:** 100% of defined sensitive headers and configured sensitive JSON fields are redacted prior to disk write.
4. **Deterministic Replay:** Replaying matching requests returns recorded status code and payload with `< 5ms` internal proxy routing overhead.
5. **Accurate Semantic Diff:**
   - Identifies removed fields (🔴 Breaking)
   - Identifies type mismatches e.g. number vs string (🔴 Breaking)
   - Identifies newly added fields (🟢 Non-breaking)
   - Ignores JSON key ordering differences (Zero false positives)
6. **Cross-Platform Compatibility:** Runs seamlessly on Windows 10/11, macOS, and Linux.

---

## 9. Confirmed Decisions & Design Specifications

### 1. Dynamic Path Parameter Collapsing (Hybrid Storage Strategy):
- **Recording:** Menyimpan URL konkret asli (`request.path: "/api/v1/users/42"`) dan pola normalisasi heuristik (`request.normalized_path: "/api/v1/users/:id"`).
- **Check Mode:** Menggunakan path konkret asli untuk melakukan re-request riil ke staging tanpa perlu mensintesis ID tiruan.
- **Replay Matcher:** Hierarki dua tingkat:
  1. *Exact Match:* Mencocokkan method, path konkret, dan query params.
  2. *Pattern Fallback:* Jika ID belum terekam (contoh `/api/v1/users/99`), cocokkan terhadap `normalized_path: "/api/v1/users/:id"` dan kembalikan stub template tersebut.

### 2. Replay Latency Simulation:
- **Default Replay:** Merespons instan/snappy (overhead proxy minimal `< 5ms`) untuk kecepatan optimal DX, Storybook, dan automated test suites.
- **Simulasi Latensi (Opt-in Flags):**
  - `--simulate-delay`: Menggunakan nilai latensi riil yang terekam di stub (`response.latency_ms`).
  - `--latency <ms>`: Memaksa latensi seragam untuk simulasi throttling/slow network.
