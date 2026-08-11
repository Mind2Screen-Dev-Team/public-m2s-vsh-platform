# Panduan Setup Klien — Memasang M2S-VSH di Organization GitHub Baru

**Versi:** v0.1.0 · **Bahasa:** Bahasa Indonesia
**Untuk:** manusia (operator/pemilik GitHub) yang akan memasang project ini
di organization klien.

Dokumen ini menjelaskan cara menyiapkan **control repository sampai repository
aplikasi** (1 atau lebih, bebas pilih teknologi) di organization GitHub milik
klien. Setelah mengikuti panduan ini, klien memiliki control repository dan
repository aplikasi yang berjalan dengan pipeline agent, perlindungan branch,
dan pemeriksaan otomatis `validate-changed-paths`.

---

## Bagian 1 — Gambaran Umum

### Struktur Project

| Repository | Isi | Peran |
|---|---|---|
| `public-m2s-vsh-platform` | repository pengatur — runner `m2s`, aturan task, template, tata kelola | sumber aturan dan konfigurasi |
| **Repository aplikasi (1+)** | tiap repository = satu jenis (backend, frontend, mobile, dan lainnya) | tempat kode produk dibuat |

**Repository aplikasi bisa apa saja — tidak harus backend + frontend.**
Sistem ini mendukung backend, frontend, mobile, fullstack (backend dan frontend
dalam satu repository), android, ios, atau campuran. Pilih peran pekerja sesuai
jenis repository (`backend-engineer`, `frontend-engineer`, `fullstack-engineer`,
`mobile-engineer`, `android-developer`, `ios-developer`). Minimal 1 repository
aplikasi; jumlah dan kombinasinya bebas.

### Yang Dibutuhkan Sebelum Mulai

- GitHub **organization** (disarankan pakai paket **Team**)
- Akun admin organization (izin `admin:org` + `repo`)
- Alat `gh` (sudah masuk akun), `git`, `make`, Go versi 1.26 atau lebih baru
- Akses model: API Anthropic langsung ATAU proxy/9Router milik klien sendiri —
  **jangan** menyalin `ANTHROPIC_BASE_URL` dari organization asal (lihat
  langkah 10)

### Aturan yang Berlaku

- Cabang (branch): `develop` / `staging` / `main`. Agent bekerja di `agent/*` →
  di-merge ke `develop` saja; merge ke `main` dilakukan manusia.
- Area yang hanya boleh diubah manusia (tidak boleh agent): `cmd/m2s/**`,
  `Makefile`, `.claude/**`, `.github/**`, `.mneme/**`, `governance/**`,
  `.task/**`, dan file rahasia.
- Template asli ada di `templates/` → salin ke lokasi aktif (dikerjakan manusia),
  jangan mengubah salinan aktif langsung.

---

## Bagian 2 — Langkah Pemasangan

> **Pembagian peran:** klien mengerjakan langkah yang berhubungan dengan repository miliknya (mengunduh, mengganti nama, membangun, menyinkronkan template, mengatur model, dan memulai pengembangan).
> Langkah yang membutuhkan identitas penjaga keamanan (GitHub App, aturan branch, perlindungan branch, verifikasi) dikerjakan **tim Mind2Screen** atas nama klien — klien menerima hasilnya, bukan rincian dokumen internalnya.

### 1. Mengunduh + mengganti nama ke organization klien

```bash
# ganti <KLIEN-ORGANIZATION>, <REPO-APLIKASI-1..N> sesuai kebutuhan klien
# clone dari lokasi repository asal yang dipakai (contoh di bawah: repo control Mind2Screen)
git clone https://github.com/Mind2Screen-Dev-Team/m2s-vsh-platform.git
# kalau yang dipakai adalah versi publik, clone dari situ:
# git clone https://github.com/Mind2Screen-Dev-Team/public-m2s-vsh-platform.git
# contoh: unduh 2 repository aplikasi (backend + frontend)
git clone https://github.com/Mind2Screen-Dev-Team/m2s-vsh-project-backend.git
git clone https://github.com/Mind2Screen-Dev-Team/m2s-vsh-project-frontend.git
# formasi lain: unduh/isi repository aplikasi sesuai teknologi (mobile, fullstack, dan lainnya)
```

Buat repository baru di organization klien, lalu ganti semua referensi
organization/repository lama ke organization klien. **Repository aplikasi
sebanyak kebutuhan klien (1+); untuk tiap repository aplikasi, kerjakan langkah
penggantian nama yang sama di bawah.**

> **Dua kemungkinan penggantian nama:**
> 1. **Ganti organization** (klien pakai org baru) — ganti `Mind2Screen-Dev-Team` → `<KLIEN-ORGANIZATION>` di semua referensi. Ini yang paling umum.
> 2. **Org tetap, ganti nama repository** (mis. republikasi: `m2s-vsh-platform` → `public-m2s-vsh-platform` di org yang sama) — ganti nama repository di semua referensi, org dibiarkan.
>
> Kerjakan **hanya satu** dari dua di atas sesuai situasi. Kalau ragu, lakukan
> yang mana pun yang membuat seluruh referensi menunjuk ke lokasi repository
> yang sebenarnya — tujuannya repository hasil harus **mandiri dan konsisten**:
> semua referensi menunjuk ke alamatnya sendiri.

**Control repository:**
- `go.mod` — ubah `module` → `github.com/<ORGANIZATION>/<NAMA-REPOSITORY>` (contoh `github.com/<KLIEN-ORGANIZATION>/m2s-vsh-platform`, atau `github.com/Mind2Screen-Dev-Team/public-m2s-vsh-platform`)
- `cmd/m2s/commands.go`, `cmd/m2s/pathcheck.go` — ubah import path mengikuti module
- `internal/registry/registry.go`, `registry_test.go` — ubah import path mengikuti module
- `templates/github/workflows/path-enforcement.yml` — ubah baris `repository:` → `<ORGANIZATION>/<NAMA-REPOSITORY>`
- `templates/github/CODEOWNERS`, `.github/CODEOWNERS` — ubah `@fajarcandraaa` → `@<PEMILIK-KLIEN>`
- `README.md` — perbarui referensi organization/repository lama di perintah `gh`
- `control/tasks/specifications/*.yaml`, `control/tasks/archive/*.yaml` — ubah field `repository:` → `<ORGANIZATION>/<NAMA-REPOSITORY>`
- `docs/operator/client-setup.md` — perbarui nama repository di semua contoh perintah (clone, module, workflow)
- `tests/negative/github-workflow.test.sh` — perbarui string grep yang menunjuk nama repository lama (baris ~263, ~267)
- Setelah mengganti nama, cek tidak ada sisa referensi lama: `grep -rn "<NAMA-REPOSITORY-LAMA>" . --exclude-dir=.git`

**Tiap repository aplikasi** (contoh di bawah pakai backend Go + frontend
Next.js; sesuaikan dengan teknologi repository klien):
- `go.mod` (backend Go) — ubah module path mengikuti organization/name baru
- `cmd/server/main.go` (backend Go) — ubah import path mengikuti module
- `.github/workflows/path-enforcement.yml` — ubah baris `repository:` (tempat
  mengambil control repository) → `<ORGANIZATION>/<NAMA-REPOSITORY>` control klien
- `.github/CODEOWNERS` — ubah `@fajarcandraaa` → `@<PEMILIK-KLIEN>`
- `README.md` — perbarui tautan control repository

### 2. Membangun + memeriksa control repository

```bash
cd <control-repository>
make build      # membangun bin/m2s
make verify     # memeriksa format, vet, test, aturan, agent, hook, tes negatif
```

`make verify` **harus lulus semua** sebelum lanjut. Pemeriksaan
`TestDeployedAgentsMatchTemplates` mengharuskan `.claude/agents/` sama persis
dengan `templates/agents/` — kalau kamu mengubah template, sinkronkan dulu
(langkah 3).

### 3. Menyinkronkan template

Template asli → salin ke lokasi aktif (dikerjakan manual, bukan agent):

```bash
# di control repository
for r in frontend-engineer project-manager technical-lead-system-analyst technical-writer; do
  cp "templates/agents/$r.md" ".claude/agents/$r.md"
done
cp templates/rules/*.md .claude/rules/          # lihat rules-deployment.md
cp templates/governance/capability-registry.yaml governance/   # lihat capability-registry-deploy.md
cp templates/github/CODEOWNERS .github/CODEOWNERS
cp templates/github/PULL_REQUEST_TEMPLATE.md .github/PULL_REQUEST_TEMPLATE.md
cp templates/github/workflows/path-enforcement.yml .github/workflows/path-enforcement.yml
# ulangi CODEOWNERS + template PR + workflow ke .github/ backend & frontend
```

Dokumen penjelas: `docs/operator/rules-deployment.md`, `capability-registry-deploy.md`.

### 4–6. Pengaturan keamanan (GitHub App, aturan branch, perlindungan branch)

**Dikerjakan oleh tim Mind2Screen atas nama klien.** Hasil yang klien terima:

- **GitHub App** `m2s-worker` (membuat PR) dan `m2s-approver` (meninjau +
  menggabungkan), terpasang di control repository + semua repository aplikasi
  klien. Kunci pribadi dienkripsi dan disimpan aman oleh tim Mind2Screen.
- **Aturan / perlindungan branch** di `develop` + `staging` (mengunci
  perubahan ref di luar PR) dan `main` (wajib pemeriksaan `validate-changed-paths`,
  tanpa paksa-dorong, tanpa hapus).
- **Pemeriksaan wajib CI**: `validate-changed-paths` aktif di control + tiap
  repository aplikasi, menunjuk control repository klien.

Klien tidak perlu dan **tidak menerima** rincian cara pemasangan ini. Jika
klien perlu mengubah hal terkait keamanan, koordinasikan dengan tim Mind2Screen.

### 7. Hubungan antara Control Repository dan Repository Aplikasi

Keduanya terhubung lewat beberapa titik (klien yang menyesuaikan):

| # | Titik hubung | File / lokasi | Yang diubah klien |
|---|---|---|---|
| 1 | **Ambil control di CI aplikasi** | `.github/workflows/path-enforcement.yml` di tiap repository aplikasi | baris `repository:` → `<KLIEN-ORGANIZATION>/public-m2s-vsh-platform` |
| 2 | **CODEOWNERS** | `.github/CODEOWNERS` di control + tiap aplikasi | `@fajarcandraaa` → `@<PEMILIK-KLIEN>` |
| 3 | **Cakupan GitHub App** | pengaturan install App di UI GitHub | batasi ke control + semua repository aplikasi |
| 4 | **Daftar repository runner** | field `ownership.repository` di tiap kontrak task | nama repository → nama repository klien |

**Alur CI (control ↔ aplikasi):** saat PR agent dibuka di repository aplikasi,
CI aplikasi:
1. mengambil repository aplikasi (isi PR)
2. mengambil **control repository** ke folder `.control` (lewat baris `repository:`)
3. membangun binary `m2s` dari `.control/go.mod`
4. membaca **kontrak tugas** dari `.control/control/tasks/specifications/${TASK_ID}.yaml`
5. menjalankan `validate-changed-paths` — memeriksa path yang berubah sesuai kontrak

Jadi repository aplikasi **mengambil** kontrak + runner dari control repository
setiap kali CI berjalan. Tanpa titik hubung #1, CI aplikasi tidak bisa menemukan
kontrak tugasnya.

### 8. Hubungan antar Repository Aplikasi

Repository aplikasi (backend, frontend, mobile, dan lainnya) **berdiri sendiri —
tidak terhubung langsung satu sama lain**. Ada dua bentuk hubungan:

**A. Terhubung lewat kontrak di control repository (cara utama):**
- TL/SA menulis **kontrak API** di control repository (contoh `contracts/CONTRACT-201.yaml`)
- **Backend** membuat endpoint sesuai kontrak
- **Frontend** membaca kontrak untuk tahu bentuk respons, lalu menulis kode pemakaian
- Kecocokan dijamin oleh kontrak bersama — bukan kode yang saling menunjuk

```
        control repository (kontrak)
       /                            \
  backend (membuat)           frontend (memakai)
       \                            /
          saat jalan: HTTP + CORS
```

**B. Terhubung saat berjalan (HTTP):**
- Frontend memanggil backend lewat alamat (`http://localhost:8080/api/v1/status`)
- Backend mengizinkan akses lintas situs (CORS `*`)
- Klien dengan pengaturan berbeda: sesuaikan alamat di kode frontend + pastikan
  CORS backend mengizinkan asal frontend

### 9. Memeriksa Hasil Pemasangan

```bash
# control repository
make verify    # harus lulus

# bukti pipeline agent: PR agent/* → develop, CI validate-changed-paths berjalan
# bukti main terlindungi: dorong agent/* → main DITOLAK (405 aturan dilanggar)
# bukti alur gabung: PR worker → approver SETUJU → approver gabung
```

### 10. Mengatur Model (Anthropic / 9Router)

`ANTHROPIC_BASE_URL` organization asal adalah **khusus Mind2Screen**, JANGAN
disalin. Klien memakai:

- API Anthropic langsung (standar, tanpa `ANTHROPIC_BASE_URL`), ATAU
- Proxy/9Router sendiri, set `ANTHROPIC_BASE_URL` + `ANTHROPIC_AUTH_TOKEN` +
  `ANTHROPIC_MODEL` sesuai pengaturan klien.

Template agent memakai `model: m2s-vsh-combo` (nama combo 9Router). Kalau klien
memakai Anthropic langsung, ganti `model:` di `templates/agents/*.md` ke model
Anthropic yang valid (`sonnet` / `opus` / `haiku`), lalu sinkronkan ke
`.claude/agents/` (langkah 3).

### 11. Memulai Pengembangan — Membuat Dokumen Awal

Setelah pemasangan selesai (langkah 1–10), jalankan launcher (satu kali, dari
akar control repository):

```bash
./scripts/project-kickoff.sh
```

Launcher membuat **8 dokumen awal pengembangan** ke `control/pre-dev/`:

| File | Dokumen | Penulis (peran) |
|---|---|---|
| `01-discovery.md` | Catatan Awal | project-manager |
| `02-brd.md` | Kebutuhan Bisnis | project-manager |
| `03-sow.md` | Ruang Lingkup Pekerjaan | project-manager |
| `04-prd.md` | Kebutuhan Produk | project-manager |
| `05-uiux.md` | Alur Tampilan | ui-ux-designer |
| `06-srs.md` | Kebutuhan Perangkat Lunak | technical-lead-system-analyst |
| `07-trd.md` | Kebutuhan Teknis | technical-lead-system-analyst |
| `08-sdd.md` | Desain Sistem | technical-lead-system-analyst |

Cara kerja launcher:
- Menyiapkan skill `project-document-builder` (menyalin dari
  `templates/skills/project-document-builder/` → `.claude/skills/`) — klien
  tidak perlu memasang skill manual.
- Membaca ringkasan dari `control/pre-dev/BRIEF.md`; jika belum ada, menanyakan
  7 poin (nama project, jenis klien, masalah, tujuan, fitur, proses yang ada,
  batasan) lalu menyimpannya.
- Satu dokumen per panggilan, **ada persetujuan manusia di antara**: `y` setujui
  dan lanjut, `r` revisi (masukan → buat ulang dokumen yang sama), `n` berhenti,
  atau tulis jawaban bebas sebagai jawaban pertanyaan terbuka (ditambahkan ke
  dokumen).
- Model dasar `plan-d-full-free`; bisa diganti dengan `KICKOFF_MODEL`.

Setelah 8 dokumen disetujui, lanjut ke alur normal: buat **kontrak task** per
pekerjaan (`control/tasks/specifications/`), lalu jalankan pipeline otomatis per
task: `./scripts/pipeline.sh --task <TASK_ID>`.
Pipeline mengerjakan implementasi, peninjauan, dan pemeriksaan kualitas secara
otomatis sampai status `merge-ready`.
Untuk menjalankan beberapa task sekaligus (misal backend + frontend), jalankan
di latar belakang:
`./scripts/pipeline.sh --task BE-XXX & ./scripts/pipeline.sh --task FE-XXX & wait`.
Pastikan status task di kontrak diubah ke `technical-ready` sebelum pipeline
dijalankan.

**Cara cepat:** jika tidak menjalankan `project-kickoff.sh` (misal hanya
menganalisa desain/tautan), gunakan skill **`project-start`** untuk memulai
pengembangan langsung. Skill ini menerima ringkasan/deskripsi/tautan desain
dari user, lalu mengatur alur menuju kontrak task:

- **Alur project** — ringkasan luas: panggil agent PM → tulis kebutuhan +
  backlog + rincian task → persetujuan manusia → panggil TL/SA → tulis kontrak
  task → persetujuan manusia → `launch-task`.
- **Alur fitur** — lingkup spesifik: lewati PM, langsung panggil TL/SA → tulis
  kontrak → persetujuan → `launch-task`.

Skill aktif setelah dipasang ke `.claude/skills/` (langkah 3). Jalankan dengan
`/project-start` atau berikan ringkasan/deskripsi project ke sesi Claude.

### 12. Alur Lengkap Initiate Setup Repo Aplikasi

Ringkasan seluruh alur menyiapkan repo aplikasi, dari nol sampai siap
dikerjakan agent. **Kunci: repo aplikasi diinisiasi oleh manusia — bukan
agent.** Agent dan skill `/project-start` baru masuk setelah repo aplikasi
sudah ada dan berisi boilerplate.

```
1. Manusia buat repo aplikasi di org klien
   → <KLIEN-ORGANIZATION>/<nama-repo-aplikasi>
   (backend, frontend, mobile, fullstack, android, ios — bebas sesuai formasi)

2. Manusia inisiasi boilerplate + struktur project
   → clone dari control repository, ganti referensi ke org klien
   (go.mod, import path, workflow repository:, CODEOWNERS, README)
   → isi struktur sesuai teknologi stack yang dipakai

3. Sinkronkan template (langkah 3)
   → cp templates/github/{CODEOWNERS,PULL_REQUEST_TEMPLATE.md,
     workflows/path-enforcement.yml} ke .github/ repo aplikasi
   → workflow `repository:` menunjuk control repository klien
     (titik hubung CI aplikasi ↔ control)

4. Tim Mind2Screen setup identitas penegakan (langkah 4-6)
   → GitHub App m2s-worker / m2s-approver di-install ke repo aplikasi
   → ruleset / perlindungan branch develop, staging, main

5. Skill /project-start cek repo aplikasi (hard-gate)
   → repo ada + berisi? → lolos → lanjut ke pipeline
   → repo kosong / belum ada → berhenti, minta manusia inisiasi dulu

6. Pipeline jalan
   → agent engineer masuk, WAJIB baca struktur repo aktual dulu
     (aturan baru di def role engineer — boilerplate bisa dikustom manusia,
     bukan asumsi template)
```

**Ringkas:** manusia buat repo → manusia isi boilerplate → sinkron template →
tim M2S setup App + aturan branch → skill `/project-start` cek (lolos) →
pipeline.

---

## Bagian 3 — Ringkasan Cara Kerja untuk Klien

Pipeline M2S-VSH bekerja seperti ini (ringkasan konsep, tanpa rincian keamanan
internal):

- **Pipeline berbasis kontrak** — kontrak API/bersama disetujui dulu, baru
  pengerjaan paralel per repository aplikasi.
- **Peran terpisah** — PM (daftar kerja/lingkup), TL/SA (kontrak + desain
  teknis), UI/UX (arah tampilan), engineering per teknologi (backend/frontend/
  mobile/fullstack), QA (penerimaan), penulis teknis (dokumentasi).
- **Kontrak task** — tiap pekerjaan agent punya ketentuan: repository, cabang,
  path yang boleh diubah, kriteria selesai, batas kualitas.
- **Pengerjaan terisolasi** — tiap task berjalan di worktree sendiri; tidak ada
  dua task menulis path yang sama.
- **Alur cabang** — `agent/*` → `develop` → `staging` → `main`; merge ke `main`
  dilakukan manusia.
- **Persetujuan manusia** — kontrak disetujui, desain disetujui, dan merge akhir
  dipegang manusia, bukan agent.

Dokumen internal (arsitektur, ADR, daftar risiko, rincian keamanan) **tidak
dikirim ke klien**. Pemasangan yang membutuhkan dokumen internal (GitHub App,
aturan branch, perlindungan branch) dikerjakan tim Mind2Screen. Klien menerima
hasil pemasangan + panduan ini + template.

---

## Bagian 4 — Catatan

- **Bahasa dokumen:** Bahasa Indonesia.
- **Versi dasar:** v0.1.0.
- **Model:** `templates/agents/*.md` memakai `model: m2s-vsh-combo` (combo 9Router
  organization asal). Klien yang memakai Anthropic langsung harus mengganti ke
  `sonnet`/`opus` dan menyinkronkan `.claude/agents/`.
- **Antrian penggabungan** ditunda ke v0.2.0 — belum ada di pemasangan ini.
- **Multi-project** (field `project` di runner) ke v0.2.0.
