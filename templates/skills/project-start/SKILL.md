---
name: project-start
description: Titik masuk pengembangan baru. User berikan brief project, deskripsi fitur, atau link design — skill mendeteksi jalur (project baru vs fitur spesifik), mengorkestrasi PM dan/atau TL/SA sebagai subagent, menghasilkan requirements + task contract yang siap dijalankan m2s launch-task. User tidak perlu tahu urutan agent atau format contract.
---

# Project Start

Skill ini adalah **titik masuk pengembangan** di M2S-VSH. Ia menerima input dari user
(brief project, deskripsi fitur, link design, atau requirement awal) lalu mengorkestrasi
agent PM dan TL/SA untuk menghasilkan task contract yang siap dieksekusi via
`m2s launch-task`.

User hanya perlu memberikan **apa yang ingin dibangun**. Semua keputusan teknis
(paths, acceptance criteria, quality gate, contract format) dikerjakan agent, bukan user.

## Kapan Skill Ini Digunakan

Gunakan skill ini ketika user:
- Ingin memulai project baru ("saya mau bangun aplikasi X")
- Ingin menambah fitur baru ke project yang sudah ada
- Memberikan link design dan minta diterjemahkan ke task
- Memberikan dokumen pre-dev (`control/pre-dev/`) hasil `project-kickoff.sh`
- Bertanya "dari mana mulai?" atau "bagaimana cara mulai pengembangan?"

## Intake dari User (minimal)

Kumpulkan hanya informasi berikut — jangan tanya hal teknis ke user:

1. **Deskripsi**: apa yang ingin dibangun? (bisa berupa teks, link design, atau dokumen)
2. **Stack** (opsional): frontend, backend, mobile, fullstack — jika belum disebut,
   PM akan menggali ini lewat structured interview
3. **Scope kasar** (opsional): apakah ini project baru dari nol, atau penambahan fitur
   ke project yang sudah ada?

Jika user sudah memberi cukup konteks: langsung tentukan jalur dan lanjut. Jangan
tanya ulang informasi yang sudah ada.

Jika user memberikan link design: fetch dan analisa kontennya dulu — ekstrak
komponen, flow, dan entitas yang terlihat, gunakan sebagai konteks ke PM/TL/SA.

Jika `control/pre-dev/` sudah berisi dokumen dari `project-kickoff.sh`: baca
dokumen relevan (PRD, SRS, TRD) sebagai input ke TL/SA — skip PM phase karena
requirements sudah ada.

## Deteksi Jalur

Tentukan jalur sebelum spawn agent:

**Jalur PROJECT** — gunakan bila:
- Brief luas, belum ada requirements sama sekali
- Project baru dari nol
- User belum tahu fitur apa yang perlu dibuat
- Ada banyak role yang akan terlibat (BE + FE + mobile, dst)

**Jalur FITUR** — gunakan bila:
- Scope sudah spesifik (satu fitur, satu endpoint, satu screen)
- Requirements sudah ada (dari pre-dev docs atau user sudah detail)
- Hanya menyentuh satu repo dan satu role engineer

Jika ragu: tanyakan satu pertanyaan — "Ini project baru dari nol atau penambahan
fitur ke sistem yang sudah ada?"

---

## Jalur PROJECT — Alur Lengkap

### Langkah 1: Spawn PM Subagent

Spawn `project-manager` sebagai subagent dengan prompt berisi:
- Brief/deskripsi project dari user (verbatim)
- Konten design jika ada
- Konten dokumen pre-dev yang relevan jika ada
- Instruksi:

```
Kamu adalah Project Manager. Kerjakan hal berikut berdasarkan brief di atas:

1. Lakukan analisa requirement — identifikasi wants vs needs, stakeholder,
   masalah utama, dan tujuan bisnis.
2. Tulis dokumen requirement ke control/requirements/REQ-<YYYYMM>-<slug>.md
   (format markdown, requirement ID per baris, scope + out-of-scope eksplisit).
3. Tulis backlog awal ke control/backlog/<project-slug>.md — daftar item kerja
   yang diperlukan, dikelompokkan per komponen/domain, belum perlu urutan prioritas.
4. Buat usulan pecahan task — untuk setiap item backlog, tentukan:
   - Jenis task (backend-implementation, frontend-implementation, dll)
   - Role engineer yang mengerjakan
   - Repo target
   - Dependency antar task (jika ada)
   Tulis ke control/projects/<project-slug>-task-breakdown.md
5. Catat open question bisnis yang masih perlu dijawab user sebelum lanjut.

Output akhir: path dokumen yang sudah ditulis + ringkasan task breakdown + open question.
```

### Langkah 2: Gate Manusia (Post-PM)

Setelah PM selesai:
- Tampilkan ringkasan: requirements yang ditulis, backlog, usulan task breakdown
- Tampilkan open question dari PM (jika ada) — minta jawaban user
- Tanyakan: ada yang perlu diubah sebelum TL/SA menganalisa?

Jangan lanjut ke TL/SA sampai user memberi konfirmasi atau menjawab open question
yang blocking.

### Langkah 3: Spawn TL/SA Subagent (per task atau batch)

Untuk setiap task dalam breakdown (atau seluruhnya sebagai batch), spawn
`technical-lead-system-analyst` dengan prompt berisi:
- Dokumen requirement dari PM (`control/requirements/`)
- Task breakdown dari PM
- Jawaban user atas open question PM
- Instruksi:

```
Kamu adalah Technical Lead & System Analyst. Kerjakan hal berikut:

1. Baca requirements dari PM dan task breakdown yang diberikan.
2. Untuk setiap task, tulis task contract YAML ke
   control/tasks/specifications/<TASK-ID>.yaml yang valid terhadap
   schemas/task.schema.json. Aturan:
   - TASK-ID: format [A-Z][A-Z0-9]*-[0-9]+ (contoh BE-301, FE-301)
     Prefix: BE backend, FE frontend, PM planning, CONTRACT contract-change,
     DESIGN design, QA quality assurance
     Nomor: lanjutkan dari ID tertinggi yang ada di control/tasks/specifications/
   - status: draft (bukan ready — user yang approve)
   - base_branch: develop (wajib — bukan main)
   - branch: agent/<TASK-ID>-<slug-lowercase>
   - paths.forbidden wajib memuat .claude/** dan .task/**
   - paths.allowed: tentukan berdasarkan jenis task dan repo target
   - acceptance_criteria: minimal 2, konkret, verifiable
   - quality_gates: sesuai stack (make verify untuk Go, npm test untuk Node, dst)
3. Untuk task BE + FE yang independen setelah contract: tandai keduanya agar
   bisa dijalankan paralel (catat di task breakdown).
4. Catat open question teknis jika ada dependency atau ambiguitas yang blocking.

Output: path setiap contract yang ditulis + ringkasan per task.
```

### Langkah 4: Gate Manusia (Post-TL/SA)

Setelah TL/SA selesai:
- Tampilkan daftar contract yang dibuat (task-id, title, role, repo)
- Tampilkan open question teknis dari TL/SA (jika ada)
- Tanyakan: ada yang perlu direvisi?

### Langkah 4b: Pemeriksaan Repo Aplikasi (hard-gate)

Sebelum pipeline, pastikan setiap repo aplikasi yang dirujuk contract sudah
berada dan berisi boilerplate. Repo aplikasi **tidak** dibikin oleh skill —
inisiasi (buat repo, struktur project, kustomisasi) adalah kerja manusia
klien/project (lihat `docs/operator/client-setup.md` Bagian 3).

Untuk tiap task dalam contract yang disetujui:
- Ambil `ownership.repository` dari contract.
- Cek repo ada + tidak kosong:
  - Lokal: folder di `M2S_REPO_ROOT/<repository>` (`scripts/pipeline.sh`
    resolve path ini) — direktori ada dan berisi file selain `.git`?
  - Remote: `git ls-remote <owner>/<repository>` mengembalikan ref? (`gh repo
    view <owner>/<repo>` sebagai alternatif.)
- Gunakan salah satu yang tersedia; jangan mengarang.

**Hard-gate:** kalau ada repo yang tidak ada ATAU kosong → **berhenti**. Jangan
lanjut ke pipeline. Tampilkan:

```
Repo aplikasi menunggu inisiasi:
  BE-XXX → backend-api   (belum ada / kosong)
  FE-XXX → web-dashboard (belum ada / kosong)

Buat + isi repo aplikasi tsb dulu (boilerplate, struktur, kustomisasi —
owner manusia). Beri tahu aku begitu repo berisi, aku lanjut ke pipeline.
```

Jangan lanjut ke Langkah 5 sampai user konfirmasi repo sudah berisi.

### Langkah 5: Launch Pipeline

Setelah user setuju (atau jawab "jalankan semua" / "jalankan BE-XXX"):

Tampilkan daftar task yang siap jalan (status `technical-ready`):
```
Task siap dijalankan:
  BE-XXX — <judul>  (role: backend-engineer)
  FE-XXX — <judul>  (role: frontend-engineer)
```

Tanya user: "Mau jalankan task mana? (bisa sebut ID spesifik, atau 'semua' / 'bersamaan')"

Berdasarkan jawaban user, eksekusi pipeline lewat Bash:

**Satu task:**
```bash
./scripts/pipeline.sh --task BE-XXX
```

**Beberapa task paralel ("semua" / "bersamaan" / sebut beberapa ID):**
```bash
./scripts/pipeline.sh --task BE-XXX &
./scripts/pipeline.sh --task FE-XXX &
wait && echo "semua pipeline selesai"
```

Pipeline otomatis rantai: `reserve-paths → launch-task → spawn implementer
→ collect-result → spawn reviewer → collect-review → spawn QA → collect-qa → merge-ready`.
Tiap spawn menampilkan role dan model yang digunakan.

Fix loop otomatis (maks 3×): bila reviewer minta changes atau QA temukan defect,
implementer di-spawn ulang di worktree yang sama tanpa membuat worktree baru.

Ingatkan: merge akhir ke `main` tetap dilakukan manusia setelah `merge-ready`.

---

## Jalur FITUR — Fast Path

### Langkah 1: Spawn TL/SA Langsung

Skip PM. Spawn `technical-lead-system-analyst` dengan:
- Deskripsi fitur dari user
- Konten design / dokumen pre-dev jika ada
- Instruksi serupa Langkah 3 Jalur PROJECT di atas, tapi untuk satu task saja

### Langkah 2: Gate + Repo Check + Launch

Sama seperti Langkah 4, 4b, dan 5 Jalur PROJECT.

---

## Aturan Wajib untuk Skill Ini

- **Jangan minta user mengisi paths, acceptance criteria, atau quality gate.**
  Itu domain TL/SA — tanyakan ke user hanya jika TL/SA benar-benar tidak bisa
  menyimpulkan dari konteks yang ada.
- **Jangan tulis contract sendiri** kecuali TL/SA gagal spawn (error model/tool).
  Jika gagal: laporkan error, minta user jalankan `m2s launch-task` manual setelah
  contract ada, atau retry spawn.
- **Jangan set status: ready atau base_branch: main** di contract manapun.
- **Satu task = satu repo.** Jika fitur menyentuh BE + FE, itu dua task terpisah
  dengan dua contract. TL/SA yang memecahnya.
- **Jika `control/pre-dev/` sudah berisi PRD/SRS/TRD:** baca dokumen itu dan
  lewatkan PM phase — requirements sudah ada. Mulai dari TL/SA.

## Batas yang Didukung M2S-VSH Saat Ini

- **PM** dapat ditulis ke: `control/requirements/**`, `control/backlog/**`,
  `control/projects/**`, `control/tasks/status/**`
- **TL/SA** dapat ditulis ke: `control/tasks/specifications/**`, `contracts/**`,
  `docs/system-analysis/**`
- **Merge** ke `main` selalu dilakukan manusia — agent hanya sampai PR
- **Task paralel** (BE + FE) didukung via dua `launch-task` terpisah
- **PM tidak bisa spawn TL/SA langsung** — orchestration dilakukan skill ini
  (main session) yang spawn keduanya secara berurutan
