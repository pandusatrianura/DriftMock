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
  - [x] `driftmock record --target <url>` starts an HTTP proxy server on the specified port (default `8080`).
  - [x] Validates target URL scheme (`http` or `https`).
  - [x] Terminal displays clean startup banner showing listening port, forwarding target, and stubs directory.
  - [x] Handles graceful shutdown on `SIGINT` / `SIGTERM`.
- **Verification:**
  - [x] CLI help displays: `go run cmd/driftmock/main.go record --help`
  - [x] End-to-end integration test passes: `go test -v ./internal/cli/...`
  - [x] Standalone binary compiled: `go build -o bin/driftmock.exe cmd/driftmock/main.go`
- **Dependencies:** Task 4
- **Files likely touched:**
  - `internal/cli/root.go`
  - `internal/cli/record.go`
  - `internal/cli/record_test.go`
  - `cmd/driftmock/main.go`
- **Estimated scope:** S (4 files)

---

### 🏁 Checkpoint 2: Record Mode Operational
- [x] Running `driftmock record --target <test_server>` proxies real HTTP traffic.
- [x] Generated stub in `.drift/stubs/` contains sanitized headers and redacted passwords.

---

## Phase 3: Deterministic Offline Mocking (Vertical Slice 2: `replay`)

### Task 6: Request Normalizer & Path Parameter Collapsing
- **Description:** Implement `internal/matcher` to normalize request paths, sort query parameters, and execute heuristic regex collapsing for dynamic REST paths (`/users/123` $\rightarrow$ `/users/:id`), calculating both exact and pattern match keys.
- **Acceptance criteria:**
  - [x] Heuristic correctly identifies integers, UUIDs, and MongoDB ObjectIDs in path segments and collapses them to `:id`.
  - [x] Query parameters are sorted deterministically regardless of URL parameter order.
  - [x] Two-tier matching logic: prioritizes exact match, falls back to normalized pattern match.
- **Verification:**
  - [x] Table-driven unit tests pass: `go test -v ./internal/matcher/...` (90.0% coverage)
- **Dependencies:** Task 1
- **Files likely touched:**
  - `internal/matcher/path_collapse.go`
  - `internal/matcher/normalizer.go`
  - `internal/matcher/matcher.go`
  - `internal/matcher/matcher_test.go`
- **Estimated scope:** M (4 files)

---

### Task 7: Replay Server Engine (Two-Tier Matching, CORS, Latency)
- **Description:** Implement the offline mock HTTP server in `internal/proxy` that loads stubs from `FileStore`, matches incoming requests via `Matcher`, handles CORS preflight (`OPTIONS`), and applies latency controls (default snappy `< 5ms`, `--simulate-delay`, or `--latency <ms>`).
- **Acceptance criteria:**
  - [x] Responds with recorded status code, response headers, and response body.
  - [x] Handles browser CORS `OPTIONS` requests automatically with permissive headers (`Access-Control-Allow-Origin: *`).
  - [x] Replay returns within 5ms by default; simulates recorded delay if `--simulate-delay` is enabled; enforces fixed delay if `--latency` is provided.
  - [x] Returns structured `404 Not Found` JSON with diagnostics when no stub matches.
- **Verification:**
  - [x] Integration tests pass: `go test -v ./internal/proxy/ -run TestReplay` (86.0% coverage)
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
  - [x] `driftmock replay --stubs ./.drift` loads all stubs from disk and starts mock server on port 8080.
  - [x] Prints registered routes summary in terminal upon startup.
  - [x] Flags `--simulate-delay` and `--latency` correctly configure delay behavior.
  - [x] Handles graceful shutdown on `SIGINT` / `SIGTERM`.
- **Verification:**
  - [x] CLI help displays: `go run cmd/driftmock/main.go replay --help`
  - [x] Integration test passes: `go test -v ./internal/cli/ -run TestRunReplay_EndToEnd`
  - [x] Standalone binary updated: `go build -o bin/driftmock.exe cmd/driftmock/main.go`
- **Dependencies:** Task 7
- **Files likely touched:**
  - `internal/cli/replay.go`
  - `internal/cli/replay_test.go`
- **Estimated scope:** S (2 files)

---

### 🏁 Checkpoint 3: Replay Mode Operational
- [x] Offline replay server starts up and responds to both exact and parameterized routes (`/users/99` matches `:id`).
- [x] Latency flags function as expected.

---

## Phase 4: Contract Drift Detection (Vertical Slice 3: `check`)

### Task 9: Semantic JSON Tree Diff & Severity Classification
- **Description:** Implement `internal/diff` to parse baseline stub JSON and live response JSON into generic ASTs, performing recursive traversal to detect missing fields, type changes, nullability shifts, and newly added fields.
- **Acceptance criteria:**
  - [x] Detects missing keys and classifies them as 🔴 `BREAKING`.
  - [x] Detects type changes (e.g. number to string, object to array) and classifies them as 🔴 `BREAKING`.
  - [x] Detects new keys and classifies them as 🟢 `NON_BREAKING`.
  - [x] Ignores JSON key ordering (zero false positives when keys are rearranged).
  - [x] Exposes option to ignore volatile timestamp keys (`updated_at`, `timestamp`).
- **Verification:**
  - [x] Table-driven tests pass with extensive diff scenarios: `go test -v ./internal/diff/...` (83.5% coverage)
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
  - [x] Produces clean, readable output matching the specification layout.
  - [x] Displays route method, path, and file source header for each checked stub.
  - [x] Summarizes total breaking changes, warnings, and non-breaking additions with final status badge.
- **Verification:**
  - [x] Formatter tests pass: `go test -v ./internal/diff/ -run TestFormatter` (85.6% coverage)
- **Dependencies:** Task 9
- **Files likely touched:**
  - `internal/diff/formatter.go`
  - `internal/diff/formatter_test.go`
- **Estimated scope:** S (2 files)

---

### Task 11: `driftmock check` CLI Command & HTTP Runner
- **Description:** Implement the `check` subcommand in Cobra with flags `--stubs` (`-s`), `--target` (`-t`), and `--strict`. Re-executes HTTP requests against live target using recorded concrete paths and runs semantic diff.
- **Acceptance criteria:**
  - [x] Iterates through all recorded stubs, sending requests to the live target with stored headers and query parameters.
  - [x] Compares live response against baseline stub using `internal/diff`.
  - [x] Returns exit code `0` if all endpoints are compatible or only have non-breaking additions.
  - [x] Returns exit code `1` if breaking changes are detected when `--strict` is enabled (CI/CD gate).
- **Verification:**
  - [x] Integration tests pass: `go test -v ./internal/cli/ -run TestRunCheck`
  - [x] Standalone binary updated: `go build -o bin/driftmock.exe cmd/driftmock/main.go`
- **Dependencies:** Task 10
- **Files likely touched:**
  - `internal/diff/runner.go`
  - `internal/cli/check.go`
  - `internal/cli/check_test.go`
- **Estimated scope:** M (3 files)

---

### 🏁 Checkpoint 4: Check Mode Operational
- [x] Running `driftmock check` against a modified test server accurately detects breaking changes and exits with status 1.
- [x] Running against an identical or backward-compatible server exits with status 0.

---

## Phase 5: CLI Polish & End-to-End Verification

### Task 12: Root CLI, Version Command, and Global Flags
- **Description:** Assemble the complete Cobra CLI application in `cmd/driftmock` and `internal/cli/root.go`, wire all subcommands (`record`, `replay`, `check`, `version`), support global `--verbose` flag, and implement `driftmock version` with build metadata.
- **Acceptance criteria:**
  - [x] `driftmock --help` lists all subcommands and global options cleanly.
  - [x] `driftmock version` prints version, commit hash, and Go runtime version.
  - [x] Global flags (`--verbose`, `--help`) work consistently across all subcommands.
- **Verification:**
  - [x] `go run cmd/driftmock/main.go --help` displays expected help menu.
  - [x] `go run cmd/driftmock/main.go version` displays build info (plain text & JSON).
  - [x] `go test -v ./internal/cli/ -run TestVersion` passes.
- **Dependencies:** Task 5, Task 8, Task 11
- **Files likely touched:**
  - `cmd/driftmock/main.go`
  - `internal/cli/root.go`
  - `internal/cli/version.go`
  - `internal/cli/version_test.go`
- **Estimated scope:** S (4 files)

---

### Task 13: End-to-End Integration Test Suite & Project Documentation
- **Description:** Create an automated end-to-end integration test suite under `tests/e2e/` testing the complete lifecycle (`record` $\rightarrow$ inspect stub $\rightarrow$ `replay` $\rightarrow$ mutate staging $\rightarrow$ `check`), and write a comprehensive `README.md`.
- **Acceptance criteria:**
  - [x] E2E test runs full cycle against an ephemeral `httptest.Server` verifying that all three modes work together seamlessly.
  - [x] Total package test coverage exceeds 80% (achieved 83.0%).
  - [x] `README.md` includes Quickstart guide, installation steps, command usage, and architecture overview.
- **Verification:**
  - [x] Full test suite passes: `go test -v ./...`
  - [x] Coverage verification: `go test -coverprofile=coverage.out ./internal/... && go tool cover -func=coverage.out` (83.0%)
  - [x] Build standalone binary: `go build -o bin/driftmock.exe cmd/driftmock/main.go` (11.18 MB)
- **Dependencies:** Task 12
- **Files likely touched:**
  - `tests/e2e/lifecycle_test.go`
  - `README.md`
- **Estimated scope:** M (3 files)

---

### 🏁 Checkpoint 5: Complete & Launch Ready
- [x] Single binary compiles cleanly: `go build -o bin/driftmock.exe cmd/driftmock/main.go`.
- [x] `go test ./...` passes 100% of unit and integration tests with >80% coverage (83.0%).
- [x] Binary size is under 25MB (11.18 MB) with zero external runtime dependencies.

---

## Phase 6: Production Hardening (Phase 1 & Phase 2 Fixes)

### Task 14: Deterministic Map Key Sorting in Diff Engine
- **Description:** Fix non-deterministic map iteration in `internal/diff/differ.go`. Sort object keys alphabetically when inspecting expected and received JSON objects so that diffs are evaluated in deterministic order across runs, eliminating test flakiness in `TestDiffer_TypeMismatchIsBreaking`.
- **Acceptance criteria:**
  - [x] `diffRecursive` extracts and sorts map keys alphabetically before iterating for missing fields and additions.
  - [x] Diff items are returned in consistent, deterministic alphabetical order.
  - [x] `go test -v ./internal/diff/...` passes consistently on repeated runs without failure.
- **Verification:**
  - [x] Unit tests pass repeatedly: `go test -v -count=5 ./internal/diff/...`
- **Dependencies:** None
- **Files likely touched:**
  - `internal/diff/differ.go`
  - `internal/diff/differ_test.go`
- **Estimated scope:** S (2 files)

---

### Task 15: Sensitive Query Parameter Redaction in Sanitizer, Naming & Recorder
- **Description:** Prevent leaking secrets in URL query parameters. Implement `SanitizeQueryParams` in `internal/sanitizer`, integrate it into `Pipeline.SanitizeRequest` and `Pipeline.SanitizeStub`, and ensure `storage.HashQueryParams` or `storage.GenerateStubID` hashes sanitized parameters rather than raw secrets.
- **Acceptance criteria:**
  - [x] `internal/sanitizer` redacts sensitive query keys (`token`, `api_key`, `secret`, `password`, `key`, `client_secret`, etc.) with `"[REDACTED]"`.
  - [x] `stub.Request.Query` values for sensitive keys are sanitized in stored stubs.
  - [x] Naming engine hashes sanitized query parameters, avoiding secret leaks in filenames.
  - [x] Unit tests verify query parameter redaction for sensitive keys while preserving non-sensitive parameters.
- **Verification:**
  - [x] Unit tests pass: `go test -v ./internal/sanitizer/... ./internal/storage/...`
- **Dependencies:** None
- **Files likely touched:**
  - `internal/sanitizer/query.go`
  - `internal/sanitizer/sanitizer.go`
  - `internal/sanitizer/sanitizer_test.go`
  - `internal/proxy/recorder.go`
  - `internal/proxy/recorder_test.go`
- **Estimated scope:** M (3-4 files)

---

### Task 16: Path Traversal Protection & Atomic File Persistence in Storage
- **Description:** Harden `internal/storage/file_store.go` against path traversal attacks in `Load(idOrFilename)` and `Save(stub)`, and implement atomic file writes using a temporary file and atomic rename (`os.Rename`) to guarantee zero file corruption on unexpected termination.
- **Acceptance criteria:**
  - [x] `FileStore.Load` sanitizes inputs and blocks directory traversal attempts (e.g., `../../etc/passwd`), returning an explicit error.
  - [x] `FileStore.Save` strips directory components from `stub.ID` and writes via atomic temporary file + rename.
  - [x] Unit tests verify path traversal rejection and atomic write behavior.
- **Verification:**
  - [x] Unit tests pass: `go test -v ./internal/storage/...`
- **Dependencies:** None
- **Files likely touched:**
  - `internal/storage/file_store.go`
  - `internal/storage/file_store_test.go`
- **Estimated scope:** S (2 files)

---

### Task 17: Request & Response Body Size Limits (DoS / Memory Exhaustion Protection)
- **Description:** Guard reverse proxy recorder and replayer against memory exhaustion (OOM) by enforcing safe maximum payload size limits (e.g. 10MB) via `io.LimitReader` when reading request and response bodies.
- **Acceptance criteria:**
  - [x] `RecorderProxy` limits request body reading and upstream response body reading to `DefaultMaxBodyBytes` (10MB).
  - [x] `Replayer` limits incoming client request body reading to `DefaultMaxBodyBytes`.
  - [x] Payloads exceeding limits are safely handled without server crash or unbounded memory allocation.
  - [x] Unit tests verify boundary behavior with oversized payloads.
- **Verification:**
  - [x] Unit tests pass: `go test -v ./internal/proxy/...`
- **Dependencies:** None
- **Files likely touched:**
  - `internal/proxy/recorder.go`
  - `internal/proxy/replayer.go`
  - `internal/proxy/recorder_test.go`
- **Estimated scope:** S (3 files)

---

### Task 18: Regression Verification & Complete Test Suite Validation
- **Description:** Run full test suite across all packages (`./...`), verify 100% pass rate, confirm test coverage meets or exceeds 80%, and verify end-to-end integration lifecycle.
- **Acceptance criteria:**
  - [x] All unit and integration tests in all packages pass: `go test -v ./...`.
  - [x] End-to-end integration test passes: `go test -v ./tests/e2e/...`.
  - [x] Total statement test coverage exceeds 80%.
  - [x] `go vet ./...` reports zero issues.
- **Verification:**
  - [x] `go test -v ./...`
  - [x] `go vet ./...`
- **Dependencies:** Task 14, Task 15, Task 16, Task 17
- **Files likely touched:**
  - `tasks/todo.md`
- **Estimated scope:** S (1 file)

---

### 🏁 Checkpoint 6: Production Hardened
- [x] Deterministic diffing eliminates test flakiness.
- [x] Query parameter secrets are redacted before persistence.
- [x] Storage is protected against path traversal and atomic persistence prevents file corruption.
- [x] Proxy is protected against memory exhaustion from oversized payloads.
- [x] 100% of tests pass across all packages.

---

## Phase 7: Enterprise Operations & CI/CD Automation (Production Grade 10/10)

### Task 19: Multi-Platform GitHub Actions CI Workflow
- **Description:** Implement `.github/workflows/ci.yml` that triggers on pushes and PRs to `main`, running dependency verification, `go vet`, multi-OS matrix testing (`ubuntu-latest`, `macos-latest`, `windows-latest`), test coverage measurement, and binary build verification.
- **Acceptance criteria:**
  - [x] `.github/workflows/ci.yml` created with OS matrix for Ubuntu, macOS, and Windows.
  - [x] `.gitignore` updated to track `.github/workflows/ci.yml` while continuing to ignore untracked local editor/agent directories.
  - [x] Workflow verifies clean build and executes `--version` check.
- **Verification:**
  - [x] YAML syntax valid, git tracks workflow.
- **Dependencies:** None
- **Files likely touched:**
  - `.github/workflows/ci.yml`
  - `.gitignore`
- **Estimated scope:** S (2 files)

---

### Task 20: Container Healthcheck Endpoint
- **Description:** Implement `GET /healthz` and `GET /__driftmock/healthz` in `internal/proxy/replayer.go` returning `200 OK` with JSON status and loaded stubs count, enabling Docker and Kubernetes liveness/readiness probes.
- **Acceptance criteria:**
  - [x] `GET /healthz` returns `200 OK` with `{"status":"ok","mode":"replay","stubs_count":N}` and `X-DriftMock-Health: ok`.
  - [x] `GET /__driftmock/healthz` always serves healthcheck probe even if custom stubs exist.
  - [x] Exact recorded mocks for `/healthz` take precedence if explicitly recorded.
  - [x] Unit test `TestReplay_Healthcheck` validates probe responses.
- **Verification:**
  - [x] Unit tests pass: `go test -v ./internal/proxy/... -run TestReplay_Healthcheck`
- **Dependencies:** None
- **Files likely touched:**
  - `internal/matcher/matcher.go`
  - `internal/proxy/replayer.go`
  - `internal/proxy/replayer_test.go`
- **Estimated scope:** S (3 files)

---

### Task 21: Structured Cloud Logging with log/slog
- **Description:** Add `internal/logger` package wrapping `log/slog` with `--log-format` flag (`text` or `json`). In `json` mode, format proxy recording, replay hits/misses, and drift check results as single-line JSON entries suitable for Datadog, AWS CloudWatch, and Grafana Loki.
- **Acceptance criteria:**
  - [x] `--log-format` flag exposed globally on root CLI command.
  - [x] `internal/logger` produces valid JSON lines when `format == "json"`.
  - [x] Proxy recorder, replayer, and check commands emit structured log entries without ASCII/ANSI noise in JSON mode.
  - [x] Unit tests verify JSON log output formatting.
- **Verification:**
  - [x] Unit tests pass: `go test -v ./internal/logger/... ./internal/cli/...`
- **Dependencies:** None
- **Files likely touched:**
  - `internal/logger/logger.go`
  - `internal/logger/logger_test.go`
  - `internal/cli/root.go`
  - `internal/cli/record.go`
  - `internal/cli/replay.go`
  - `internal/cli/check.go`
- **Estimated scope:** M (6 files)

---

### 🏁 Checkpoint 7: Full Enterprise Grade (10/10)
- [x] GitHub Actions automated multi-platform CI is live.
- [x] Kubernetes/Docker container healthcheck endpoints are operational.
- [x] Structured JSON cloud logging (`log/slog`) is operational for cloud observability.
- [x] All unit, integration, and E2E tests pass across 100% of packages.


