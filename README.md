# DriftMock

> **Zero-effort live traffic recording, instant offline API mocking, and automated contract drift detection.**

[![Go Report Card](https://goreportcard.com/badge/github.com/pandusatrianura/driftmock)](https://goreportcard.com/report/github.com/pandusatrianura/driftmock)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Coverage](https://img.shields.io/badge/coverage-83%25-brightgreen)](coverage.out)
[![Binary Size](https://img.shields.io/badge/binary-<15MB-success)](bin/)

DriftMock is a high-performance, single-binary CLI tool written in **Go** that acts as an intelligent, transparent reverse proxy between client applications (frontend web, mobile apps, QA automation test runners) and backend staging/development environments.

---

## 🌟 Key Features

- 🎥 **Zero-Effort Recording (`driftmock record`):**
  Passively captures live HTTP traffic, transparently decompresses gzip payloads, strips authorization tokens/cookies, redacts sensitive keys (passwords, tokens, NIK), and stores clean human-readable JSON stubs in `./.drift/stubs/`.

- ⚡ **Deterministic Offline Mocking (`driftmock replay`):**
  Instantly serves recorded stubs offline with **< 5ms** internal proxy overhead. Features a smart two-tier matching hierarchy:
  1. **Tier 1 (Exact):** Matches Method + Path + Sorted Query Parameters.
  2. **Tier 2 (Pattern Fallback):** Automatically collapses dynamic REST parameters (`/users/123` $\rightarrow$ `/users/:id`) so unrecorded IDs return valid template mocks.
  Includes automated CORS preflight (`OPTIONS`) handling and simulated latency options.

- 🔍 **Automated Contract Drift Detection (`driftmock check`):**
  Re-requests staging endpoints and performs semantic JSON tree diffs (ignoring key order), categorizing changes with Jest/Vitest-style terminal alerts:
  - 🔴 **Breaking Change:** Missing fields, type mismatches (e.g. number $\rightarrow$ string), nullability shifts.
  - 🟡 **Warning:** State/Enum format shifts.
  - 🟢 **Non-Breaking Addition:** New optional fields.
  Exits with code `1` in strict mode to guard CI/CD pipelines against undetected breaking changes.

- 🚀 **100% Local-First & Single Binary:**
  Compiled size < 15MB. No Docker daemon, no background database, no OS packet sniffing, and zero telemetry.

---

## 🏗️ Architecture

```text
┌────────────────┐      ┌──────────────────────────┐      ┌─────────────────┐
│ Incoming HTTP  │ ───► │   DriftMock Proxy Engine │ ───► │ Target Staging  │
└────────────────┘      └────────────┬─────────────┘      └─────────────────┘
                                     │
                      ┌──────────────┴──────────────┐
                      ▼                             ▼
           ┌──────────────────────┐      ┌──────────────────────┐
           │   Sanitizer Pipeline │      │  Storage & Matcher   │
           │ (Headers & JSON DFS) │      │ (Exact & REST :id)   │
           └──────────────────────┘      └──────────┬───────────┘
                                                    │
                                                    ▼
                                         [.drift/stubs/*.json]
                                                    │
                                                    ▼
                                         ┌──────────────────────┐
                                         │ Semantic Diff Engine │
                                         │ (Jest/Vitest Style)  │
                                         └──────────────────────┘
```

---

## 🚀 Quickstart

### 1. Build or Install

```bash
# Clone the repository
git clone https://github.com/pandusatrianura/DriftMock.git
cd DriftMock

# Build standalone binary
go build -o bin/driftmock cmd/driftmock/main.go

# Verify installation
./bin/driftmock version
```

### 2. Record Live Traffic

Point your frontend app's API base URL (e.g., `VITE_API_URL` or `NEXT_PUBLIC_API_URL`) to `http://localhost:8080`:

```bash
driftmock record --target https://api.staging.example.com --port 8080 --out ./.drift
```

As you navigate your app, DriftMock outputs real-time logs and saves sanitized stubs into `.drift/stubs/`:
```text
[RECORDED] GET    /api/v1/users/42               200 -> GET_api_v1_users_42.json (84ms)
[RECORDED] POST   /api/v1/auth/login             200 -> POST_api_v1_auth_login.json (112ms)
```

### 3. Serve Offline Mocks

Develop offline without internet access or when staging is degraded:

```bash
# Snappy mock server (< 5ms response time)
driftmock replay --stubs ./.drift --port 8080

# Or simulate recorded staging latency
driftmock replay --simulate-delay

# Or simulate network throttling (e.g. 250ms)
driftmock replay --latency 250
```

### 4. Detect Contract Drift in CI/CD

Verify whether current backend staging responses still match your frontend's recorded expectations:

```bash
driftmock check --stubs ./.drift --target https://api.staging.example.com
```

If breaking changes are detected, DriftMock highlights the exact diff and exits with code `1`:

```text
[DriftMock] ⚠️ CONTRACT DRIFT DETECTED: GET /api/v1/users/42

  - data.user.email: field 'data.user.email' is missing (expected string)
  ~ data.user.id: type mismatch: expected number, received string
  + data.user.avatar_url: new field 'data.user.avatar_url' added (string)

  Status: ❌ 2 breaking changes found compared to stub 'GET_api_v1_users_42'

════════════════════════════════════════════════════════════════
                     Contract Drift Summary
════════════════════════════════════════════════════════════════
  Endpoints Checked: 5
  Passed:            4
  Failed (Breaking): 1

  Total Differences:
    🔴 2 Breaking changes
    🟡 0 Warnings
    🟢 1 Additions (Non-breaking)

  Final Verdict: ❌ FAILED (breaking contract drift detected)
════════════════════════════════════════════════════════════════
```

---

## 📋 Command Reference

| Command | Flags | Description |
|---|---|---|
| `driftmock record` | `-t, --target` *(required)*<br>`-p, --port` *(default: 8080)*<br>`-o, --out` *(default: ./.drift)* | Intercepts HTTP traffic, forwards to target, redacts secrets, and saves stubs to disk. |
| `driftmock replay` | `-s, --stubs` *(default: ./.drift)*<br>`-p, --port` *(default: 8080)*<br>`--simulate-delay`<br>`--latency <ms>` | Serves recorded stubs offline with deterministic matching and latency simulation. |
| `driftmock check` | `-t, --target` *(required)*<br>`-s, --stubs` *(default: ./.drift)*<br>`--strict` *(default: true)*<br>`--ignore-volatile` *(default: true)*<br>`--timeout <sec>` *(default: 10)* | Compares live staging responses against recorded stubs using semantic tree diffing. |
| `driftmock version`| `--json` | Displays build version, git commit, Go runtime, and platform architecture. |

---

## 🔒 Security & Sanitization

DriftMock guarantees that sensitive information is never persisted to disk:
1. **Header Redaction:** `Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, and `X-Auth-Token` are automatically replaced with `[REDACTED]`.
2. **Recursive JSON Body Masking:** Deeply traverses JSON request and response payloads, masking keys like `password`, `secret`, `token`, `credit_card`, `cvv`, `ssn`, and Indonesian `nik`.

Stubs in `.drift/stubs/` are safe to commit to Git and share across your frontend and QA teams.

---

## 🧪 Testing & Quality Assurance

```bash
# Run all unit, integration, and E2E tests
go test -v ./...

# Run test suite with coverage profiling
go test -coverprofile=coverage.out ./internal/...
go tool cover -func=coverage.out
```

All 7 core packages maintain **>80% test coverage** with comprehensive E2E lifecycle tests in `tests/e2e/`.

---

## 📄 License

MIT License © 2026 Pandu Satria & Contributors.
