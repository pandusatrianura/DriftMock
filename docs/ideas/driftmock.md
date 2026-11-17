# DriftMock (Contract-Drift & Live Recording Proxy)

> **Tagline:** *Zero-effort live traffic recording, instant offline API mocking, and automated contract drift detection.*

---

## 1. Problem Statement (How Might We)

> **"How might we eliminate API contract drift and manual mock maintenance for frontend and mobile engineers by passively capturing live staging traffic and turning it into instant, Git-committable offline mock servers and type schemas?"**

---

## 2. Recommended Direction

Membangun tool CLI *single-binary* berkinerja tinggi (menggunakan **Go**) yang bertindak sebagai **Transparent Reverse Proxy** cerdas antara aplikasi client (Frontend web, mobile app, QA test runner) dan backend staging/development.

### Mode Operasi Utama:
1. **Mode Record (`driftmock record`):**
   - Developer mengarahkan base URL aplikasi lokal ke `http://localhost:8080`.
   - DriftMock meneruskan request ke target staging riil, merekam interaksi request & response secara pasif, menyensor data rahasia (*secrets/PII*), lalu menyimpannya sebagai file stubs JSON yang terbaca manusia di direktori `./.drift/stubs`.
2. **Mode Replay / Mock (`driftmock replay`):**
   - Berfungsi sebagai server mock offline instan tanpa koneksi internet atau saat staging server sedang *down*.
   - Menggunakan pencocokan deterministik (*smart deterministic matching*) berdasarkan HTTP Method, URL Path, Query Params, dan Header yang relevan.
3. **Mode Drift Check (`driftmock check`):**
   - Membandingkan response staging terkini dengan snapshot kontrak yang tersimpan sebelumnya untuk mendeteksi *breaking changes* secara otomatis (misal: field dihapus, tipe data berubah, atau format tanggal berganti).

---

## 3. Key Assumptions to Validate

- [ ] **Asumsi 1 (Adopsi Pengembang):** Developer lebih memilih mengganti base URL API mereka ke `localhost:8080` (atau menyetel env variable `API_URL`) daripada menggunakan sniffing network transparan level OS yang rumit dan membutuhkan akses `sudo`/admin.
- [ ] **Asumsi 2 (Path Parameter Collapsing):** Endpoint dinamis (seperti `GET /users/101` dan `GET /users/102`) dapat otomatis disederhanakan menjadi pola REST (`/users/:id`) menggunakan heuristik regex/integer-hash tanpa perlu konfigurasi manual dari awal.
- [ ] **Asumsi 3 (Security & Sanitization):** Fitur sanitasi otomatis (stripping `Authorization: Bearer ...`, `Cookie`, dan masking field sensitif seperti `password`) bekerja cukup andal sehingga developer merasa aman untuk melakukan `git commit` file stubs ke repository publik/privat.

---

## 4. MVP Scope (Target 2 Minggu Pertama)

### ✅ What's IN (MVP Scope)
1. **CLI Core:**
   - `driftmock record --target https://api.staging.example.com --port 8080 --out ./.drift`
   - `driftmock replay --stubs ./.drift --port 8080`
   - `driftmock check --stubs ./.drift --target https://api.staging.example.com`
2. **Deterministic Matching Engine:**
   - Pencocokan request berdasarkan: `HTTP Method` + `Normalized Path` + `Sorted Query Parameters`.
3. **Built-in Sanitizer Pipeline:**
   - Otomatis me-redact header sensitif (`Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, `X-Auth-Token`).
   - Regex masking untuk key JSON umum: `password`, `token`, `secret`, `credit_card`.
4. **Human-Readable Storage:**
   - Stub disimpan dalam format file `.json` bersih per-endpoint (contoh: `./.drift/stubs/GET_api_v1_users.json`) yang mudah diedit manual atau di-review via GitHub Pull Request.
5. **Basic Drift Alert:**
   - CLI terminal log berwarna saat me-record ulang: memberi peringatan jika ada perubahan field response dibanding snapshot sebelumnya.

---

## 5. Not Doing (and Why) — *Trade-offs & Boundaries*

- ❌ **TIDAK membuat Desktop GUI / Web Dashboard di v0.1:**
  *Alasan:* GUI memakan 70% waktu awal dan memperlambat integrasi CI/CD. CLI dengan log terminal berwarna (*clean TUI*) jauh lebih disukai oleh engineer.
- ❌ **TIDAK menggunakan Transparent OS Network Sniffing / TUN Interface:**
  *Alasan:* Sniffing transparan membutuhkan hak akses root/administrator, sering konflik dengan antivirus/VPN korporat, dan sulit stabil di lintas OS (Windows vs macOS vs Linux). Reverse proxy port-forwarding 100% aman dan portable.
- ❌ **TIDAK mendukung gRPC / WebSocket / Protobuf di v0.1:**
  *Alasan:* 90% penderitaan *contract drift* dialami pada REST/JSON API. Selesaikan satu masalah utama sebelum ekspansi protokol.
- ❌ **TIDAK membuat Backend Dependency Mocking (ala Keploy):**
  *Alasan:* Keploy memfokuskan diri pada database & internal backend microservices. DriftMock fokus spesifik pada **Frontend/Client $\rightarrow$ API Gateway boundary**.
- ❌ **TIDAK ada Cloud Backend / Login Akun / Telemetri Wajib:**
  *Alasan:* 100% *local-first*, *offline-first*, dan ramah privasi data.

---

## 6. Open Questions & Rekomendasi Solusi Riset

### Q1: Bagaimana menangani mutasi stateful (`POST`, `PUT`, `DELETE`) saat replay mode?
> **Pertanyaan:** *Apakah cukup mengembalikan response sukses statis yang terekam, atau perlu in-memory cache sederhana?*

#### 💡 Rekomendasi Solusi: **"Idempotent Replay with Request Echo" (Two-Tier Approach)**

1. **Untuk MVP v0.1: Pure Idempotent Replay (Statis yang Terekam)**
   - **Alasan:** Dalam 95% use case frontend development (Storybook, Playwright E2E UI testing, offline layout development), frontend **tidak membutuhkan database engine sungguhan**. Frontend hanya butuh kepastian bahwa:
     - Endpoint mengembalikan status code yang sesuai (`201 Created` atau `200 OK`).
     - Response body memiliki struktur schema yang valid agar UI tidak crash.
     - Ada simulasi latency yang realistis (misal 150ms).
   - **Risiko In-Memory Cache di Awal:** Jika kita mencoba membuat in-memory stateful store (misal: `POST /items` menambah item ke array in-memory), kita akan terjebak dalam *rabbit hole* mensimulasikan logika bisnis backend (validasi unik, relasi foreign key, cascading delete). Ini membuat tool menjadi rapuh dan tidak konsisten.
2. **Pengembangan Lanjutan (v0.2+ Opt-in Flag):**
   - Tambahkan flag opsional `--echo-payload`: Mengambil data yang dikirim di request body dan menimpanya ke dalam template response recorded (misal: client mengirim `{"name": "Budi"}`, response mock mengembalikan `{"id": 123, "name": "Budi", "status": "created"}`).
   - Tambahkan opsi plugin `--stateful-crud` untuk endpoint standar REST `/resource/:id` jika user secara eksplisit menginginkannya.

---

### Q2: Format snapshot diff apa yang paling intuitif bagi frontend engineer?
> **Pertanyaan:** *Mirip Jest snapshot diff atau standard git diff?*

#### 💡 Rekomendasi Solusi: **"Semantic JSON Tree Diff" (Gaya Jest / Vitest Snapshot)**

#### Mengapa BUKAN Standard Git Diff?
Standard `git diff` bekerja pada level *line-by-line text*. Pada JSON, jika backend hanya mengubah urutan key:
```json
// Snapshot Lama:
{"age": 25, "name": "Pandu"}
// Response Baru:
{"name": "Pandu", "age": 25}
```
Standard `git diff` akan mendeteksi ini sebagai 2 baris perubahan (false positive). Selain itu, `git diff` tidak memahami hierarki tipe data.

#### Keunggulan Format Semantik ala Jest/Vitest:
Frontend engineer sudah terbiasa dengan Jest/Vitest snapshot testing. Format ini memahami struktur JSON dan mengkategorikan dampak perubahan ke dalam 3 level keparahan:

1. **🔴 BREAKING CHANGE (Merah):**
   - Field dihapus: `- data.user.email: string (MISSING)`
   - Tipe data berubah: `~ data.user.id: expected number, received string`
   - Field nullable menjadi non-nullable atau sebaliknya.
2. **🟡 WARNING / POTENTIAL DRIFT (Kuning):**
   - Format enum berubah: `~ status: expected "ACTIVE"|"INACTIVE", received "PENDING"`
3. **🟢 NON-BREAKING ADDITION (Hijau):**
   - Field baru ditambahkan: `+ data.user.avatar_url: string`

#### Contoh Tampilan CLI Terminal Output yang Diinginkan:
```text
[DriftMock] ⚠️ CONTRACT DRIFT DETECTED: GET /api/v1/profile

- data.user.avatar_url: string (REMOVED - Breaking Change!)
~ data.user.age: number -> string (TYPE MISMATCH - Breaking Change!)
+ data.user.badge: string (Added - Non-breaking)

Status: ❌ 2 breaking changes found compared to .drift/stubs/GET_api_v1_profile.json
```

---

## 7. Arsitektur Modular (Magnet Kontributor Komunitas)

Dirancang dengan pola plugin/adapter sederhana agar ramah kontributor pemula (*Good First Issue*):

```text
┌────────────────┐      ┌──────────────────────────┐      ┌─────────────────┐
│ Incoming HTTP  │ ───► │   DriftCore Proxy Engine │ ───► │ Target Staging  │
└────────────────┘      └────────────┬─────────────┘      └─────────────────┘
                                     │
                      ┌──────────────┴──────────────┐
                      ▼                             ▼
           ┌──────────────────────┐      ┌──────────────────────┐
           │   Sanitizer Pipeline │      │  Storage & Matcher   │
           └──────────────────────┘      └──────────┬───────────┘
                                                    │
                                                    ▼
                                         [ Plugin / Exporters ]
                                         ├── TypeScript / Zod Schemas
                                         ├── OpenAPI 3.1 Spec Generator
                                         └── Postman / Bruno Collections
```

### Ide "Good First Issue" untuk Menarik Bintang & PR Pertama:
- `feat(exporter): Generate TypeScript interfaces & Zod schemas from recorded stubs`
- `feat(exporter): Export recorded requests to Postman Collection / Bruno`
- `feat(sanitizer): Add custom regex rules for Indonesian PII (NIK, Nomor HP)`
- `feat(matcher): Support fuzzy matching for query parameter order`
