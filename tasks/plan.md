# Implementation Plan: DriftMock (Contract-Drift & Live Recording Proxy)

> **Document Version:** 1.0.0  
> **Source Spec:** [docs/specification.md](file:///e:/github/pandusatrianura/DriftMock/docs/specification.md) | [docs/implementation.md](file:///e:/github/pandusatrianura/DriftMock/docs/implementation.md)  
> **Tracking Checklist:** [tasks/todo.md](file:///e:/github/pandusatrianura/DriftMock/tasks/todo.md)

---

## 1. Overview

DriftMock is a high-performance, single-binary Go CLI tool acting as a local-first transparent reverse proxy. It eliminates manual mock authoring and undetected API contract drift by:
1. Recording live staging HTTP interactions into human-readable, Git-committable JSON stubs in `.drift/stubs/` with automated PII/secret redaction.
2. Serving deterministic offline mocks using a two-tier matching engine (Exact Match + Normalized Path Fallback) with snappy default latency (< 5ms) and optional latency simulation.
3. Automatically detecting contract drift via a semantic JSON tree diff engine with Jest/Vitest-style terminal alerts and non-zero exit codes for CI/CD gates.

---

## 2. Architecture Decisions & Key Principles

- **Standard Library Centric:** Rely on Go's `net/http`, `net/http/httputil.ReverseProxy`, `encoding/json`, `crypto/sha256`, and `regexp` to minimize external dependencies and maintain a binary footprint under 25MB.
- **Vertical Feature Slicing:** Rather than building all storage, then all proxy, then all CLI, we implement each complete end-to-end command flow in order: `record` $\rightarrow$ `replay` $\rightarrow$ `check`.
- **Hybrid Storage for Path Parameters:** Persist the exact concrete request path (e.g. `/api/v1/users/42`) for fidelity and live `check` compatibility, while calculating and storing normalized route patterns (`/api/v1/users/:id`) for dynamic replay matching.
- **Sanitize-Before-Write Guarantee:** All sensitive headers and configured JSON body keys are sanitized prior to writing stubs to disk.
- **Strict Semantic Diff:** The diff engine compares JSON Abstract Syntax Trees (AST), ignoring key ordering differences while flagging missing keys and type mutations as breaking changes.

---

## 3. Dependency Graph

```text
               ┌───────────────────────────────┐
               │    Task 1: Go Mod & Models    │
               └───────────────┬───────────────┘
                               │
               ┌───────────────┴───────────────┐
               ▼                               ▼
  ┌─────────────────────────┐     ┌─────────────────────────┐
  │ Task 2: File Storage    │     │ Task 3: Sanitizer       │
  └────────────┬────────────┘     └────────────┬────────────┘
               │                               │
               └───────────────┬───────────────┘
                               ▼
               ┌───────────────────────────────┐
               │ Task 4: Proxy & Recorder      │
               └───────────────┬───────────────┘
                               │
               ┌───────────────▼───────────────┐
               │ Task 5: Record CLI Command    │
               └───────────────┬───────────────┘
                               ▼ [Checkpoint 1 & 2]
               ┌───────────────────────────────┐
               │ Task 6: Matcher & Collapser   │
               └───────────────┬───────────────┘
                               ▼
               ┌───────────────────────────────┐
               │ Task 7: Replay Server Engine  │
               └───────────────┬───────────────┘
                               ▼
               ┌───────────────────────────────┐
               │ Task 8: Replay CLI Command    │
               └───────────────┬───────────────┘
                               ▼ [Checkpoint 3]
               ┌───────────────────────────────┐
               │ Task 9: Semantic Diff Engine  │
               └───────────────┬───────────────┘
                               ▼
               ┌───────────────────────────────┐
               │ Task 10: Jest Diff Formatter  │
               └───────────────┬───────────────┘
                               ▼
               ┌───────────────────────────────┐
               │ Task 11: Check CLI Command    │
               └───────────────┬───────────────┘
                               ▼ [Checkpoint 4]
               ┌───────────────────────────────┐
               │ Task 12: Root CLI & Version   │
               └───────────────┬───────────────┘
                               ▼
               ┌───────────────────────────────┐
               │ Task 13: E2E Integration Suite│
               └───────────────────────────────┘
                               ▼ [Checkpoint 5: Launch Ready]
```

---

## 4. Phase Breakdown & Vertical Slices

### Phase 1: Foundation & Data Models
- **Task 1:** Initialize Go module, project layout, and domain entities (`model/stub.go`, `model/diff.go`).
- **Task 2:** Implement File Storage & Deterministic Naming Engine (`storage/file_store.go`, `storage/naming.go`).
- **Checkpoint 1:** Storage engine reads/writes stubs with 100% test coverage and deterministic JSON filenames.

### Phase 2: Live Traffic Recording (Vertical Slice 1: `driftmock record`)
- **Task 3:** Implement Sanitizer Pipeline (Header stripping & recursive JSON masking).
- **Task 4:** Implement Reverse Proxy & Recorder Engine (`proxy/recorder.go`, `proxy/handler.go`).
- **Task 5:** Implement `record` CLI Command with Cobra flags (`cmd/driftmock`, `internal/cli/record.go`).
- **Checkpoint 2:** Running `driftmock record` successfully intercepts live HTTP requests, forwards to target, redacts sensitive keys, and writes valid `.drift/stubs/*.json`.

### Phase 3: Deterministic Offline Mocking (Vertical Slice 2: `driftmock replay`)
- **Task 6:** Implement Request Normalizer & Path Parameter Collapsing Engine (`matcher/`).
- **Task 7:** Implement Replay Mock Server Engine with Two-Tier Matcher, CORS, and Latency Simulation.
- **Task 8:** Implement `replay` CLI Command with Cobra flags (`internal/cli/replay.go`).
- **Checkpoint 3:** Running `driftmock replay` deterministically returns recorded responses offline for both exact paths and parameterized routes with latency controls.

### Phase 4: Contract Drift Detection (Vertical Slice 3: `driftmock check`)
- **Task 9:** Implement Semantic JSON Tree Diff & Severity Classification Engine (`diff/differ.go`, `diff/severity.go`).
- **Task 10:** Implement Jest/Vitest-Style Colored Terminal Output Formatter (`diff/formatter.go`).
- **Task 11:** Implement `check` CLI Command with HTTP Re-request Runner (`internal/cli/check.go`).
- **Checkpoint 4:** Running `driftmock check` fetches live staging responses, performs semantic AST comparisons against saved stubs, prints colored diffs, and exits with code 1 upon breaking changes.

### Phase 5: CLI Polish & End-to-End Verification
- **Task 12:** Implement Root CLI Command, Version Command, and Global Flags (`internal/cli/root.go`, `version.go`).
- **Task 13:** Implement Comprehensive End-to-End Integration Tests (`tests/e2e/`).
- **Checkpoint 5:** Complete E2E cycle succeeds, `go test -race ./...` passes with >80% coverage, and binary compiles cleanly.

### Phase 6: Production Hardening (Phase 1 & Phase 2 Fixes)
- **Task 14:** Deterministic Map Key Sorting in Diff Engine & Test Stability (`internal/diff/differ.go`).
- **Task 15:** Sensitive Query Parameter Redaction in Sanitizer, Naming & Recorder (`internal/sanitizer`, `internal/storage`, `internal/proxy`).
- **Task 16:** Path Traversal Protection & Atomic File Persistence in Storage (`internal/storage/file_store.go`).
- **Task 17:** Request & Response Body Size Limits (DoS / Memory Exhaustion Protection) (`internal/proxy`).
- **Task 18:** Regression Verification & Complete Test Suite Validation (`tests/e2e/`, unit tests).
- **Checkpoint 6:** 100% of test suite passes reliably, all security, storage, and determinism hardening verified.

---

## 5. Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation Strategy |
|---|---|---|---|
| Sensitive data (passwords, tokens) leaked into committed stubs | High | Medium | Dual-layer sanitization (header redactor + recursive DFS key masker) executed unconditionally before writing to disk; tested with exhaustive test assertions. |
| Target server uses gzip/deflate compression, making recorded stubs unreadable binary | High | High | Custom `ModifyResponse` in `httputil.ReverseProxy` decompresses gzip/deflate streams before passing to storage, saving clean formatted JSON. |
| Replay mode blocked by browser CORS policy | Medium | High | Replay HTTP handler automatically responds to `OPTIONS` preflight with permissive CORS headers (`*`). |
| Dynamic path collision (e.g. `/users/new` falsely matched as `/users/:id`) | Medium | Low | Matcher prioritizes exact path matching (Tier 1) before falling back to parameterized routes (Tier 2). |
| False positive diff alerts due to dynamic timestamps (`updated_at`, `timestamp`) | Medium | Medium | Diff engine provides configurable volatile key ignoring so timestamp shifts do not trigger breaking drift warnings. |

---

## 6. Verification Protocol

Before declaring any phase or vertical slice complete:
1. **Compilation Check:** `go build ./...` produces zero errors.
2. **Unit & Race Tests:** `go test -v -race ./internal/...` passes with zero failures.
3. **Format & Lint:** `gofmt -s -w .` and `golangci-lint run` (or `go vet ./...`) pass clean.
4. **Integration Verification:** E2E tests in `tests/e2e/` verify real HTTP server interaction.
