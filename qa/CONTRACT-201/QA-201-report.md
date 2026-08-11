# QA-201 — Acceptance Test Report CONTRACT-201

**Tanggal:** 2026-08-05
**Task:** QA-201 (integration)
**Contract:** CONTRACT-201 — Multi-Component Status API

## Ringkasan

**Hasil: PASS (6/6 test)**

Backend (`GET /api/v1/status` multi-component) memenuhi CONTRACT-201.
Backward-compat terpelihara: field lama tetap ada. Database + auth = probe
sintetik (tanpa dep nyata), sesuai convention contract.

## Test Results

### TC-1: GET /api/v1/status → HTTP 200, 3 services ✅

**Command:**
```bash
curl -s -w "\nHTTP_CODE:%{http_code}\nCT:%{content_type}\n" http://localhost:8080/api/v1/status
```

**Result:**
```json
{"status":"ok","version":"0.1.0","uptime_seconds":20.17,
 "services":[
   {"id":"backend-api","status":"ok","version":"0.1.0","uptime":20.17,"last_checked":"2026-08-05T12:29:01+07:00","latency_ms":0,"message":""},
   {"id":"database","status":"ok","version":"","uptime":0,"last_checked":"2026-08-05T12:29:01+07:00","latency_ms":2,"message":"synthetic ping ok"},
   {"id":"auth","status":"ok","version":"0.1.0","uptime":20.17,"last_checked":"2026-08-05T12:29:01+07:00","latency_ms":1,"message":"synthetic ok"}
 ]}
HTTP_CODE: 200
CT: application/json
```

| Check | Expected | Actual |
|---|---|---|
| HTTP code | 200 | 200 ✅ |
| Content-Type | application/json | application/json ✅ |
| Jumlah services | 3 | 3 ✅ |
| Urutan id | backend-api, database, auth | sama ✅ |

### TC-2: Field per komponen ✅

| Field | Expected | backend-api | database | auth |
|---|---|---|---|---|
| `status` | ok\|degraded\|error\|unknown | ok ✅ | ok ✅ | ok ✅ |
| `version` | string | "0.1.0" ✅ | "" ✅ | "0.1.0" ✅ |
| `uptime` | number ≥ 0 | 20.17 ✅ | 0 ✅ | 20.17 ✅ |
| `last_checked` | RFC 3339 | `2026-08-05T12:29:01+07:00` ✅ | sama ✅ | sama ✅ |
| `latency_ms` | int ≥ 0 | 0 ✅ | 2 ✅ | 1 ✅ |
| `message` | string | "" ✅ | "synthetic ping ok" ✅ | "synthetic ok" ✅ |

### TC-3: Backward-compat — field lama tetap ada ✅

`status`, `version`, `uptime_seconds` masih di top-level. StatusCard lama
(membaca field ini) tidak patah. Unit test `TestStatusHandlerBackwardCompat`
+ 5 test lama tetap pass.

### TC-4: Aggregate status = worst-of ✅

| Komponen | Aggregat harapan |
|---|---|
| semua ok | `ok` ✅ |
| satu degraded | `degraded` |
| satu error | `error` |
| semua unknown | `unknown` |

Diverifikasi unit test `TestStatusHandlerAggregateStatus` (4 kasus tabel).

### TC-5: Negative test — server mati ✅

Kill server di port 8080, curl ulang:
```
HTTP_CODE: 000 (connection failed)
```
Response gagal sesuai harapan. Frontend (`StatusDashboard`) menampilkan
error state + tombol "Coba lagi".

### TC-6: Frontend integration ✅

`src/components/StatusDashboard/StatusDashboard.tsx` (branch `agent/FE-202-multi-status-dashboard`):
- Fetch `http://localhost:8080/api/v1/status` — benar
- Polling auto 30s (`setInterval` 30_000, clear on unmount) — ada
- Manual refresh — button "↻ Refresh", reset interval — ada
- State: loading (skeleton pulse 3 kartu) / empty ("Tidak ada komponen status") / success (grid 3 kartu) / error (+Coba lagi) — semua ada
- Responsive grid `grid-cols-1 md:grid-cols-2 lg:grid-cols-3` — ada
- A11y: `aria-live="polite"` region, button `aria-label="Perbarui status"` — ada
- Vitest 3 test PASS (loading→success, error+refresh, empty) — ✅
- `npm run build` sukses — route `/status` 1.43 kB static — ✅

## Acceptance

Semua 6 TC PASS. CONTRACT-201 terpenuhi. Backward-compat terpelihara,
polling + state FE lengkap, a11y + responsive sesuai DESIGN-2 handoff.
