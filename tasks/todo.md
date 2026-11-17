# Task List: DriftMock (Contract-Drift & Live Recording Proxy)

> **Document Version:** 1.0.0  
> **Complements:** [tasks/plan.md](file:///e:/github/pandusatrianura/DriftMock/tasks/plan.md) | [docs/specification.md](file:///e:/github/pandusatrianura/DriftMock/docs/specification.md)

---

## Phase 1: Foundation & Data Models

### Task 1: Go Module Setup & Domain Models
- **Description:** Initialize Go module `github.com/pandusatrianura/driftmock` with required external dependencies (`spf13/cobra`, `fatih/color`, `stretchr/testify`), create standard directory layout, and define domain models for Stubs, RequestPatterns, ResponsePatterns, and DiffResults.
- **Acceptance criteria:**
  - [x] `go.mod` and `go.sum` are initialized and tidy.
  - [x] `internal/model/stub.go` contains structs for `Stub`, `RequestPattern`, and `ResponsePattern` matching the specification schema.
  - [x] `internal/model/diff.go` defines `DiffSeverity` (Breaking, Warning, Addition) and `DiffItem` structures.
- **Verification:**
  - [x] Build succeeds: `go build ./...`
  - [x] Unit tests pass: `go test -v ./internal/model/...`
- **Dependencies:** None
- **Files likely touched:**
  - `go.mod`
  - `internal/model/stub.go`
  - `internal/model/diff.go`
- **Estimated scope:** S (3 files)

---

### Task 2: File Storage & Deterministic Naming Engine
- **Description:** Implement `internal/storage` to handle reading, writing, and listing JSON stubs on disk, along with deterministic filename generation (`<METHOD>_<path_underscored>[_<query_hash>].json`).
- **Acceptance criteria:**
  - [x] `naming.go` produces deterministic, filesystem-safe filenames for arbitrary paths and query parameters.
  - [x] `file_store.go` creates `.drift/stubs/` directory if absent, writes formatted JSON with 2-space indentation, and lists/loads all existing stubs.
  - [x] Unit tests cover special characters, query string sorting, and file read/write roundtrip.
- **Verification:**
  - [x] Tests pass: `go test -v ./internal/storage/...` (84.0% coverage)
- **Dependencies:** Task 1
- **Files likely touched:**
  - `internal/storage/naming.go`
  - `internal/storage/file_store.go`
  - `internal/storage/file_store_test.go`
- **Estimated scope:** M (3 files)

---

### 🏁 Checkpoint 1: Foundation
- [x] `go build ./...` compiles cleanly without warnings.
- [x] Storage engine passes all unit tests with 100% roundtrip persistence fidelity.

---

## Phase 2: Live Traffic Recording (Vertical Slice 1: `record`)

### Task 3: Sanitizer Pipeline (Headers & JSON Body Masking)
- **Description:** Implement `internal/sanitizer` to redact sensitive authentication headers (`Authorization`, `Cookie`, `X-Api-Key`, etc.) and recursively traverse JSON request/response bodies to mask sensitive keys (`password`, `secret`, `token`, `nik`, etc.) with `"[REDACTED]"`.
- **Acceptance criteria:**
  - [x] Header sanitizer drops or replaces auth tokens with `"[REDACTED]"`.
  - [x] Body masker recursively traverses nested JSON objects and arrays, masking configured sensitive keys regardless of case.
  - [x] Non-JSON bodies are preserved without mutation or crash.
  - [x] Table-driven tests verify complete redaction across various nested JSON fixtures.
- **Verification:**
  - [x] Tests pass: `go test -v ./internal/sanitizer/...` (87.7% coverage)
- **Dependencies:** Task 1
- **Files likely touched:**
  - `internal/sanitizer/headers.go`
  - `internal/sanitizer/body_mask.go`
  - `internal/sanitizer/sanitizer.go`
  - `internal/sanitizer/sanitizer_test.go`
- **Estimated scope:** M (4 files)

---

### Task 4: Reverse Proxy & Traffic Recording Interceptor
- **Description:** Implement `internal/proxy` utilizing `net/http/httputil.ReverseProxy` to forward incoming client requests to the target staging server, intercept responses, decompress gzip/deflate if present, measure round-trip latency, execute sanitizer, and persist stubs to FileStore.
- **Acceptance criteria:**
  - [x] Proxy directs requests accurately to the target host while rewriting the `Host` header.
  - [x] Transparently decompresses gzip responses so stored JSON remains human-readable.
  - [x] Measures response latency accurately in milliseconds (`latency_ms`).
  - [x] Dispatches sanitized `Stub` to `FileStore` asynchronously or synchronously without delaying client response.
  - [x] Returns `502 Bad Gateway` if target staging is unreachable.
- **Verification:**
  - [x] Integration tests pass using `httptest.Server`: `go test -v ./internal/proxy/...` (87.7% coverage)
- **Dependencies:** Task 2, Task 3
- **Files likely touched:**
  - `internal/proxy/recorder.go`
  - `internal/proxy/handler.go`
  - `internal/proxy/recorder_test.go`
- **Estimated scope:** M (3 files)

---

### Task 5: `driftmock record` CLI Command
- **Description:** Implement the `record` subcommand using Cobra, binding flags `--target` (`-t`), `--port` (`-p`), and `--out` (`-o`), starting the proxy server and outputting informative colored startup logs to the terminal.
- **Acceptance criteria:**
  - [ ] `driftmock record --target <url>` starts an HTTP proxy server on the specified port (default `8080`).
  - [ ] Validates target URL scheme (`http` or `https`).
  - [ ] Terminal displays clean startup banner showing listening port, forwarding target, and stubs directory.
  - [ ] Handles graceful shutdown on `SIGINT` / `SIGTERM`.
- **Verification:**
  - [ ] CLI help displays: `go run cmd/driftmock/main.go record --help`
  - [ ] Manual check: Start proxy against a mock HTTP server and verify generated stub in `.drift/stubs/`.
- **Dependencies:** Task 4
- **Files likely touched:**
  - `internal/cli/record.go`
  - `cmd/driftmock/main.go`
- **Estimated scope:** S (2 files)

---

### 🏁 Checkpoint 2: Record Mode Operational
- [ ] Running `driftmock record --target <test_server>` proxies real HTTP traffic.
- [ ] Generated stub in `.drift/stubs/` contains sanitized headers and redacted passwords.

---

## Phase 3: Deterministic Offline Mocking (Vertical Slice 2: `replay`)

### Task 6: Request Normalizer & Path Parameter Collapsing
- **Description:** Implement `internal/matcher` to normalize request paths, sort query parameters, and execute heuristic regex collapsing for dynamic REST paths (`/users/123` $\rightarrow$ `/users/:id`), calculating both exact and pattern match keys.
- **Acceptance criteria:**
  - [ ] Heuristic correctly identifies integers, UUIDs, and MongoDB ObjectIDs in path segments and collapses them to `:id`.
  - [ ] Query parameters are sorted deterministically regardless of URL parameter order.
  - [ ] Two-tier matching logic: prioritizes exact match, falls back to normalized pattern match.
- **Verification:**
  - [ ] Table-driven unit tests pass: `go test -v -race ./internal/matcher/...`
- **Dependencies:** Task 1
- **Files likely touched:**
  - `internal/matcher/normalizer.go`
  - `internal/matcher/path_collapse.go`
  - `internal/matcher/matcher_test.go`
- **Estimated scope:** M (3 files)

---

### Task 7: Replay Server Engine (Two-Tier Matching, CORS, Latency)
- **Description:** Implement the offline mock HTTP server in `internal/proxy` that loads stubs from `FileStore`, matches incoming requests via `Matcher`, handles CORS preflight (`OPTIONS`), and applies latency controls (default snappy `< 5ms`, `--simulate-delay`, or `--latency <ms>`).
- **Acceptance criteria:**
  - [ ] Responds with recorded status code, response headers, and response body.
  - [ ] Handles browser CORS `OPTIONS` requests automatically with permissive headers (`Access-Control-Allow-Origin: *`).
  - [ ] Replay returns within 5ms by default; simulates recorded delay if `--simulate-delay` is enabled; enforces fixed delay if `--latency` is provided.
  - [ ] Returns structured `404 Not Found` JSON with diagnostics when no stub matches.
- **Verification:**
  - [ ] Integration tests pass: `go test -v -race ./internal/proxy/ -run TestReplay`
- **Dependencies:** Task 2, Task 6
- **Files likely touched:**
  - `internal/proxy/replayer.go`
  - `internal/proxy/cors.go`
  - `internal/proxy/replayer_test.go`
- **Estimated scope:** M (3 files)

---

### Task 8: `driftmock replay` CLI Command
- **Description:** Implement the `replay` subcommand in Cobra with flags `--stubs` (`-s`), `--port` (`-p`), `--simulate-delay`, and `--latency`.
- **Acceptance criteria:**
  - [ ] `driftmock replay --stubs ./.drift` loads all stubs from disk and starts mock server on port 8080.
  - [ ] Prints registered routes summary in terminal upon startup.
  - [ ] Flags `--simulate-delay` and `--latency` correctly configure delay behavior.
  - [ ] Handles graceful shutdown on `SIGINT` / `SIGTERM`.
- **Verification:**
  - [ ] CLI help displays: `go run cmd/driftmock/main.go replay --help`
  - [ ] Test replay with `curl` or `net/http` client without internet connection.
- **Dependencies:** Task 7
- **Files likely touched:**
  - `internal/cli/replay.go`
  - `internal/cli/root.go`
- **Estimated scope:** S (2 files)

---

### 🏁 Checkpoint 3: Replay Mode Operational
- [ ] Offline replay server starts up and responds to both exact and parameterized routes (`/users/99` matches `:id`).
- [ ] Latency flags function as expected.

---

## Phase 4: Contract Drift Detection (Vertical Slice 3: `check`)

### Task 9: Semantic JSON Tree Diff & Severity Classification
- **Description:** Implement `internal/diff` to parse baseline stub JSON and live response JSON into generic ASTs, performing recursive traversal to detect missing fields, type changes, nullability shifts, and newly added fields.
- **Acceptance criteria:**
  - [ ] Detects missing keys and classifies them as 🔴 `BREAKING`.
  - [ ] Detects type changes (e.g. number to string, object to array) and classifies them as 🔴 `BREAKING`.
  - [ ] Detects new keys and classifies them as 🟢 `NON_BREAKING`.
  - [ ] Ignores JSON key ordering (zero false positives when keys are rearranged).
  - [ ] Exposes option to ignore volatile timestamp keys (`updated_at`, `timestamp`).
- **Verification:**
  - [ ] Table-driven tests pass with extensive diff scenarios: `go test -v -race ./internal/diff/...`
- **Dependencies:** Task 1
- **Files likely touched:**
  - `internal/diff/differ.go`
  - `internal/diff/severity.go`
  - `internal/diff/differ_test.go`
- **Estimated scope:** M (3 files)

---

### Task 10: Jest/Vitest-Style Colored Terminal Formatter
- **Description:** Implement `internal/diff/formatter.go` to render diff results in high-contrast ANSI colors with Jest/Vitest conventions (`-` red for removed/breaking, `~` yellow for modified/type mismatch, `+` green for additions).
- **Acceptance criteria:**
  - [ ] Produces clean, readable output matching the specification layout.
  - [ ] Displays route method, path, and file source header for each checked stub.
  - [ ] Summarizes total breaking changes, warnings, and non-breaking additions with final status badge.
- **Verification:**
  - [ ] Formatter tests pass: `go test -v -race ./internal/diff/ -run TestFormatter`
- **Dependencies:** Task 9
- **Files likely touched:**
  - `internal/diff/formatter.go`
  - `internal/diff/formatter_test.go`
- **Estimated scope:** S (2 files)

---

### Task 11: `driftmock check` CLI Command & HTTP Runner
- **Description:** Implement the `check` subcommand in Cobra with flags `--stubs` (`-s`), `--target` (`-t`), and `--strict`. Re-executes HTTP requests against live target using recorded concrete paths and runs semantic diff.
- **Acceptance criteria:**
  - [ ] Iterates through all recorded stubs, sending requests to the live target with stored headers and query parameters.
  - [ ] Compares live response against baseline stub using `internal/diff`.
  - [ ] Returns exit code `0` if all endpoints are compatible or only have non-breaking additions.
  - [ ] Returns exit code `1` if breaking changes are detected when `--strict` is enabled (CI/CD gate).
- **Verification:**
  - [ ] Integration tests pass: `go test -v -race ./internal/cli/ -run TestCheck`
- **Dependencies:** Task 10
- **Files likely touched:**
  - `internal/cli/check.go`
  - `internal/diff/runner.go`
  - `internal/cli/check_test.go`
- **Estimated scope:** M (3 files)

---

### 🏁 Checkpoint 4: Check Mode Operational
- [ ] Running `driftmock check` against a modified test server accurately detects breaking changes and exits with status 1.
- [ ] Running against an identical or backward-compatible server exits with status 0.

---

## Phase 5: CLI Polish & End-to-End Verification

### Task 12: Root CLI, Version Command, and Global Flags
- **Description:** Assemble the complete Cobra CLI application in `cmd/driftmock` and `internal/cli/root.go`, wire all subcommands (`record`, `replay`, `check`, `version`), support global `--verbose` flag, and implement `driftmock version` with build metadata.
- **Acceptance criteria:**
  - [ ] `driftmock --help` lists all subcommands and global options cleanly.
  - [ ] `driftmock version` prints version, commit hash, and Go runtime version.
  - [ ] Global flags (`--verbose`, `--help`) work consistently across all subcommands.
- **Verification:**
  - [ ] `go run cmd/driftmock/main.go --help` displays expected help menu.
  - [ ] `go run cmd/driftmock/main.go version` displays build info.
- **Dependencies:** Task 5, Task 8, Task 11
- **Files likely touched:**
  - `cmd/driftmock/main.go`
  - `internal/cli/root.go`
  - `internal/cli/version.go`
- **Estimated scope:** S (3 files)

---

### Task 13: End-to-End Integration Test Suite & Project Documentation
- **Description:** Create an automated end-to-end integration test suite under `tests/e2e/` testing the complete lifecycle (`record` $\rightarrow$ inspect stub $\rightarrow$ `replay` $\rightarrow$ mutate staging $\rightarrow$ `check`), and write a comprehensive `README.md`.
- **Acceptance criteria:**
  - [ ] E2E test runs full cycle against an ephemeral `httptest.Server` verifying that all three modes work together seamlessly.
  - [ ] Total package test coverage exceeds 80%.
  - [ ] `README.md` includes Quickstart guide, installation steps, command usage, and architecture overview.
- **Verification:**
  - [ ] Full test suite passes: `go test -v -race ./...`
  - [ ] Coverage verification: `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out`
  - [ ] Build standalone binary: `go build -o bin/driftmock cmd/driftmock/main.go`
- **Dependencies:** Task 12
- **Files likely touched:**
  - `tests/e2e/record_replay_test.go`
  - `tests/e2e/drift_check_test.go`
  - `README.md`
- **Estimated scope:** M (3 files)

---

### 🏁 Checkpoint 5: Complete & Launch Ready
- [ ] Single binary compiles cleanly: `go build -o bin/driftmock cmd/driftmock/main.go`.
- [ ] `go test -race ./...` passes 100% of unit and integration tests with >80% coverage.
- [ ] Binary size is under 25MB with zero external runtime dependencies.
