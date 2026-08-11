# Security Rules

**Sumber:** §16.5, §42, R-12, R-20 dokumen arsitektur M2S-VSH Lite v0.1.0
**Ditinjau:** 5 Agustus 2026
**Berlaku:** semua agent; path-scoped per repository

> TEMPLATE KANONIK. Salin ke `.claude/rules/security.md` pada repo yang
> membutuhkannya. `.claude/**` adalah human-only-write (component-inventory §7).
> Rules ini soft (§14.2) — enforcement lewat `permissions.deny`, hook
> fail-closed, dan CI. Jangan mengandalkan prosa sebagai pengaman.

## Secret handling (§16.5, §42.3)

Dilarang:

- membaca atau menampilkan `.env`, private keys, access tokens, credential files;
- menyimpan secret di source code atau log;
- menyalin secret keluar worktree — termasuk ke laporan uncommitted changes
  (§42.6, worktree-lifecycle).

Pola secret yang diblokir hook:

```
.env  .env.*  *.pem  *.key  credentials*.json  **/secrets/**
```

## Configuration dan Enforcement (§16.5)

Dilarang:

- menggunakan `bypassPermissions` sebagai default;
- mengubah CI protection, CODEOWNERS, atau security configuration tanpa
  dedicated task;
- mengubah `.claude/**` — definisi agent, hook, atau settings (R-12);
- mengubah `.github/**` — workflow yang menjadi required check (R-20);
- menjalankan command destructive tanpa explicit task dan human approval;
- mengakses production database;
- melakukan network exfiltration.

## Install dan Dependency (§9.3, §16.5, R-23)

Dilarang install otomatis:

- `npm install`, `yarn add`, `pnpm add`, `pip install`, `go get`, `prpm install`,
  `claude plugin`.

Instalasi capability eksternal wajib terdaftar `governance/capability-registry.yaml`
dan memenuhi §41 (source, license, reviewer, version pin, checksum).

## Boundary yang sering terlewat

- **Prompt bukan security boundary.** Menulis "jangan akses secret" di prosa
  tidak menggantikan `permissions.deny` + hook. Enforcement yang nyata ada di
  settings dan hook.
- **MCP write path.** Bila MCP dipakai, write dibatasi — mis. UI/UX hanya
  `design/**`, `prototypes/**`, `artifacts/**` (§8.3).
- **Secret di laporan.** Kutip error/status yang menyebut secret tetap dilarang.
