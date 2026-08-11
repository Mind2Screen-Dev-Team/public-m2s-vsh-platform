# Architecture Rules

**Sumber:** §12–§16, §44, §29 dokumen arsitektur M2S-VSH Lite v0.1.0
**Ditinjau:** 5 Agustus 2026
**Berlaku:** semua agent; path-scoped per repository

> TEMPLATE KANONIK. Salin ke `.claude/rules/architecture.md` pada repo yang
> membutuhkannya. `.claude/**` adalah human-only-write (component-inventory §7).
> Rules ini soft (§14.2) — enforcement lewat permissions/hooks/runner/CI.

## Struktur dan Boundaries

- Control repo tidak menyimpan application source code (§36).
- Satu task = satu repository = satu branch = satu worktree = satu writer per path.
- One Active Writer per Path (§29.4): setiap writable path hanya satu writer aktif.
- Shared file (§29.6): route registry, `go.mod`/`go.sum`, `package.json`/lockfile,
  migration registry, global enum, `DESIGN.md`, CI workflow, `.claude` config,
  `.mneme/project_memory.json` — masing-masing punya **satu owner** yang ditunjuk
  per task.

## Branch Strategy (§44)

- Branch task: `agent/<task-id>-<slug>`.
- Branch planning/dokumentasi: `worktree-*`.
- Merge flow hanya naik satu level: `develop → staging → main`. Tidak boleh
  lompati.
- Agent PR wajib target `develop` (H-01). Promosi `staging`/`main` via manusia.

## Path Enforcement (§14, §42.1)

- `allowed_paths` ditetapkan task contract, bukan template role.
- `forbidden_paths` dianggap read-only atau inaccessible.
- Batas path ditegakkan `validate-path-scope.sh` (PreToolUse, fail-closed exit 2)
  dan CI `validate-changed-paths`.

## Konsekuensi yang sering terlewat

- **Batas path BUKAN per-role statis.** Contract menetapkan per-task. Jangan
  menebak dari template role.
- **Rules ini tidak menggantikan enforcement.** Prompt bukan security boundary —
  jangan menambah aturan ke prosa sebagai pengganti `permissions.deny`/hook.
- **Worktree di luar repo** (Q8/A-01). Jangan buat worktree di dalam repo.
