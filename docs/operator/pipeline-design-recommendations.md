# Rekomendasi Desain Pipeline — Akar Masalah & Arah Perbaikan

**Tanggal:** 2026-08-13
**Versi:** M2S-VSH Lite v0.1.0
**Status:** Rekomendasi desain (untuk keputusan manusia)
**Konteks:** hasil eksekusi pipeline fase-1 POS Penglaris yang menemukan banyak
bug beruntun. Dokumen ini merangkum akar masalah dan arah perbaikan, bukan
daftar patch.

## Ringkasan Masalah

Pipeline mengasumsikan agent AI **deterministik** (selalu patuhi schema, angka,
URL), padahal agent **stochastic** (LLM). Asumsi salah ini melahirkan deretan
bug: handoff format longgar, pr_url dihalusinasi, spawn intermitten, `set -e`
membunuh pipeline diam-diam.

Tiga akar:

## Akar 1 — Agent output non-deterministik

Agent LLM (`cmb-agent-coding`, `cmb-agent-review`) tidak dapat dijamin patuh
format ketat, angka, atau URL. Halusinasi adalah sifat model, bukan bug satu
kali. Gejala yang terlihat:
- `pr_url: github.com/pos-backend/pull/303` padahal PR nyata #6.
- `decision: request_changes` (underscore) padahal enum `request-changes`.
- severity `critical`/`low`, category `contract-violation` — bukan enum.
- spawn sesekali mengembalikan kosong (rc=0) tanpa output.

**Arah solusi:**

1. **Agent tidak boleh memproduksi data yang bisa di-derive dari sistem.**
   - `pr_url` → query `gh pr list --head $BRANCH` (sudah diterapkan).
   - `task_id`, `repository`, `branch` → dari reservation/contract, bukan agent.
   - Agent hanya mengisi yang tak bisa di-derive: `summary`, `reason`, konteks
     temuan.

2. **Structured output, bukan free-text JSON.** Instruksi "tulis blok ```json"
   rapuh. Gunakan mekanisme yang menegakkan schema di lapisan API/tool
   (structured output), bukan di prompt. Ini keputusan paling berdampak — perlu
   evaluasi apakah `claude --print` saat ini mendukungnya atau perlu ganti
   mekanisme spawn.

3. **Fallback deterministik**: validasi → retry → bila tetap gagal, tandai
   `blocked` + minta manusia. Jangan loop diam-diam 3x tanpa kemajuan.

## Akar 2 — Schema handoff terlalu ketat vs kemampuan agent

Enum 6 kategori, 4 severity, 3 decision, field nested. Agent sering meleset.
Normalisasi post-hoc (yang sekarang menumpuk) hanya menambal per field tanpa
ujung.

**Arah solusi (pilih satu, jangan setengah-setengah):**

- **(A) Enforce di input**: pakai structured-output (Akar 1 #2) sehingga agent
  dipaksa schema. Normalisasi jadi hampir tak perlu.
- **(B) Runner sebagai normalizer resmi**: satu normalizer tunggal yang dipahami
  sebagai bagian kontrak runner (bukan patch reaktif), menerima variasi input
  dan men-coerce ke canonical. Schema tetap ketat, tapi runner menyerap variasi.
- **(C) Longgarkan schema**: terima variasi (`request_changes` vs
  `request-changes`) di schema, canonical hanya di penyimpanan.

Saat ini sistem ada di posisi buruk: schema ketat + agent bebas + normalizer
nambal. Harus pilih salah satu arah.

## Akar 3 — Pipeline `set -e` fragile

`set -euo pipefail` global: satu command non-zero = exit seluruh pipeline
diam-diam. Tidak ada error boundary per tahap. Gejala: pipeline mati di
`collect-*` (karena `normalize_handoff` return 1 salah precedence) tanpa jejak.

**Arah solusi:**

1. **Error boundary per phase.** Tiap `spawn_agent` / `collect-*` dibungkus:
   tangkap exit code, log jelas, lanjut ke fallback (retry/manual/blocked).
2. **Hapus `set -e` untuk tahap yang boleh gagal.** Ganti `if ! cmd; then ...`
   eksplisit. `set -e` cocok untuk script pendek, bukan orchestrator multi-tahap.
3. **Idempotensi + resume.** Setiap tahap bisa di-run ulang dari state terkini
   (guard status sebagian sudah diterapkan). Failure jadi murah diperbaiki.

## Prioritas Rekomendasi

| Prioritas | Tindakan | Dampak | Keputusan |
|---|---|---|---|
| 1 | Evaluasi `claude --print` vs structured output | hilangkan Akar 1 + 2 sekaligus | manusia |
| 2 | Runner normalizer resmi (satu, bukan nambal) | kurangi variasi handoff | manusia |
| 3 | Error boundary + hapus `set -e` global | pipeline tak mati diam-diam | manusia |
| 4 | Pisahkan derive vs generate (pr_url dkk) | kurangi halusinasi | sebagian sudah jalan |

## Yang Sudah Diterapkan (sebagai dasar)

- `pr_url` resolve dari `gh pr list --repo owner/REPO` (bukan handoff).
- `normalize_handoff` (format changed_files/tests/findings/severity/category).
- `spawn_agent` retry 2x + stderr log.
- Transisi state machine launch-review/collect-result/registry idempoten.
- Prompt handoff ke stdout (konsisten reviewer).
- `cmdLaunchReview` advance `implementation-complete → reviewing`.

## Yang Perlu Diputuskan (bukan dieksekusi)

1. **Structured output** — apakah `claude --print` bisa enforce schema? Kalau
   tidak, apa mekanisme spawn pengganti?
2. **Arah normalisasi** — A (enforce input) vs B (runner normalizer) vs C
   (longgarkan schema)?
3. **Error boundary** — seberapa jauh refactor `set -e` di pipeline?
4. **Accept task yang sudah terlanjur** — BE-303 (implementasi di PR #6) diterima
   apa adanya oleh manusia, atau tetap perlu reviewer lulus?

## Kesimpulan

Masalah bukan pada satu field atau satu task. Masalah pada asumsi fundamental:
**pipeline memperlakukan output agent sebagai deterministic**. Solusi jangka
panjang = pisahkan derive-vs-generate, pakai structured output, dan buat
pipeline tahan gagal (error boundary + resume). Semua perubahan ini menyentuh
`cmd/m2s/**`, `scripts/**`, `schemas/**`, `.claude/agents/**` — human-only,
keputusan desain di tangan manusia.
