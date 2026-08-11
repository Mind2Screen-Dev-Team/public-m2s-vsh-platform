# Universal Agent Rules

**Sumber:** §16 dokumen arsitektur M2S-VSH Lite v0.1.0
**Ditinjau:** 5 Agustus 2026
**Berlaku:** semua agent, semua repository

> TEMPLATE KANONIK. Salin ke `.claude/rules/universal-agent-rules.md` pada repo
> yang membutuhkannya. `.claude/**` adalah human-only-write (component-inventory §7).

## Scope dan Ownership (§16.1)

Wajib:

- bekerja hanya pada task ID yang diberikan;
- membaca task contract sebelum bekerja;
- memastikan repository dan branch benar;
- hanya menulis pada `allowed_paths`;
- memperlakukan `forbidden_paths` sebagai read-only atau inaccessible;
- tidak memperluas scope;
- tidak mengambil task agent lain;
- tidak mengedit shared file tanpa ownership eksplisit;
- tidak mengedit generated files;
- tidak mengubah project rules atau agent definitions.

## Repository dan Workspace (§16.2)

Semua write-capable agents wajib:

- menggunakan dedicated worktree;
- tidak menjalankan `git checkout`, `git switch`, atau mengubah branch;
- tidak mengubah `GIT_DIR` atau `GIT_WORK_TREE`;
- tidak melakukan `git -C` ke repository lain;
- tidak menggunakan symlink untuk menulis ke luar worktree;
- tidak mengedit main checkout;
- tidak menjalankan task pada shared shell workspace.

## Communication (§16.3)

Wajib:

- berkomunikasi dalam Bahasa Indonesia;
- mempertahankan identifier, code, API field, dan terminology teknis
  sebagaimana source of truth;
- membedakan Fact, Assumption, Open Question, Risk, dan Decision;
- tidak menyembunyikan asumsi;
- tidak mengklaim test lulus bila tidak dijalankan;
- menyebutkan command test yang benar-benar dijalankan;
- menyebutkan file yang berubah;
- menyebutkan unresolved issue.

## Implementation (§16.4)

Wajib:

- membaca flow end-to-end sebelum mengubah code;
- mengikuti existing architecture dan pattern;
- menggunakan minimum correct change;
- tidak menambah dependency tanpa approval;
- tidak membuat abstraction yang belum dibutuhkan;
- tidak melakukan unrelated refactor;
- tidak memperbaiki issue di luar task contract;
- menulis test sesuai ownership;
- mempertahankan backward compatibility kecuali breaking change disetujui;
- melakukan error handling pada trust boundary;
- tidak mengurangi security, accessibility, observability, atau data safety
  demi diff kecil.

## Security (§16.5)

Dilarang:

- membaca atau menampilkan `.env`, private keys, access tokens, atau credential files;
- menyimpan secret di source code atau log;
- menggunakan `bypassPermissions` sebagai default;
- menjalankan command destructive tanpa explicit task dan human approval;
- mengubah CI protection, CODEOWNERS, atau security configuration tanpa
  dedicated task;
- mengakses production database;
- melakukan network exfiltration;
- menginstal package, plugin, extension, atau MCP secara otomatis.

## Git (§16.6)

Wajib:

- menggunakan branch pattern `agent/<task-id>-<slug>`;
- membuat atomic commit;
- menggunakan commit message yang mengandung task ID;
- tidak force push;
- tidak rebase shared branch;
- tidak merge sendiri;
- tidak menandai review sendiri sebagai approved;
- tidak menghapus branch agent lain.

## Stop Conditions (§16.7)

Berhenti dan kembalikan status `blocked` bila:

- task contract tidak lengkap;
- file yang perlu diubah berada di luar allowed paths;
- contract belum disetujui;
- dependency task belum selesai;
- requirement dan ADR konflik;
- test environment tidak tersedia;
- diperlukan secret;
- diperlukan dependency baru;
- ditemukan data-loss risk;
- ditemukan breaking change yang belum disetujui;
- ada active path reservation oleh task lain.
