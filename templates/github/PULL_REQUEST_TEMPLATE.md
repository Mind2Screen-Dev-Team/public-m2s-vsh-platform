<!--
TEMPLATE KANONIK. Salin ke `.github/PULL_REQUEST_TEMPLATE.md` pada repo aplikasi.

Isi checklist diturunkan dari gate §43, larangan universal §16.5, dan negative
test §68. Checklist ini BUKAN penegakan — ia pengingat dan jejak audit. Yang
menegakkan: `permissions.deny`, hook PreToolUse, dan required check
`validate-changed-paths`. Checklist yang dicentang tetapi CI merah tidak
berarti apa pun; CI adalah otoritas final.

Baris yang tidak berlaku: tulis "n/a" beserta alasan. Jangan hapus barisnya —
baris yang hilang tidak dapat dibedakan dari baris yang terlewat.
-->

## Task

| | |
|---|---|
| **Task ID** | <!-- BE-101 — wajib, harus cocok dengan nama branch (§44) --> |
| **Branch** | <!-- agent/<task-id>-<slug> --> |
| **Base branch** | <!-- develop atau staging. `main` hanya manusia (ADR-001 #2) --> |
| **Role** | <!-- backend-engineer, frontend-engineer, qa-engineer, … --> |
| **Contract** | <!-- control/tasks/specifications/<task-id>.yaml --> |
| **Requirement** | <!-- rujukan requirement / issue --> |

## Perubahan

<!-- Apa yang berubah dan mengapa. Bukan daftar file — itu sudah ada di tab
     Files changed. Jelaskan keputusan yang tidak terbaca dari diff. -->

## Batas path

<!-- Salin dari contract. Required check `validate-changed-paths` memeriksa
     ini secara independen terhadap path yang benar-benar berubah; daftar di
     sini untuk reviewer manusia, bukan sebagai sumber kebenaran. -->

**allowed_paths:**

**forbidden_paths:**

- [ ] Seluruh file yang berubah berada di dalam `allowed_paths`
- [ ] Tidak satu pun file yang berubah berada di dalam `forbidden_paths`
- [ ] PR ini menyentuh **satu** repository saja (§29, R-15)

## Larangan universal §16.5

- [ ] Tidak menyentuh `.claude/**` — definisi agent, hook, atau `settings.json` (§26, R-12)
- [ ] Tidak menyentuh `.github/**` — CI workflow atau CODEOWNERS (R-20)
- [ ] Tidak mengubah branch protection atau security configuration
- [ ] Tidak ada secret, credential, token, atau `.env` di diff maupun di log
- [ ] Tidak ada dependency baru dipasang tanpa dedicated task (R-23)
- [ ] Tidak ada perubahan pada `Makefile` atau `cmd/m2s/**` (human-only write)

<!-- Bila salah satu kotak di atas TIDAK dapat dicentang: PR ini memerlukan
     dedicated task dan approval manusia. Jelaskan di bawah, jangan lanjutkan
     tanpa itu. -->

## Review

- [ ] Reviewer **bukan** implementer task ini (§29.7, §47)
- [ ] Handoff terlampir atau dirujuk: <!-- control/handoffs/<task-id>.yaml -->
- [ ] Test yang relevan ditambahkan atau diperbarui
- [ ] Perubahan yang terlihat pengguna sudah melalui QA, atau dinyatakan tidak ada

## Gate CI

- [ ] `path-enforcement / validate-changed-paths` hijau
- [ ] Test dan lint repo hijau

<!-- Merge ke `main` sepenuhnya milik manusia (ADR-001 #2). Tidak ada agent
     yang memiliki hak apa pun atas `main`. -->
