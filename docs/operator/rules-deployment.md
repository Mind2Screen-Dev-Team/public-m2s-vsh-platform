# Patch human-only — deploy rules generik ke `.claude/rules/`

`.claude/**` ditolak `deny` di `.claude/settings.json`, sehingga agent tidak
dapat menyuntingnya — termasuk dari worktree, karena `deny` dievaluasi terhadap
path relatif repo. Rules kanonik karena itu berada di `templates/rules/`, dan
penyalinan ke `.claude/rules/` dilakukan manusia.

Pola sama dengan `templates/agents/` → `.claude/agents/` (Q10): sumber kanonik
version-controlled di `templates/`, salinan aktif per repository di `.claude/`.

## Rules yang tersedia

| Berkas kanonik | Sumber arsitektur | Isi |
|---|---|---|
| `templates/rules/architecture.md` | §12–§16, §44, §29 | struktur, boundary, branch strategy, path enforcement |
| `templates/rules/security.md` | §16.5, §42, R-12, R-20 | secret handling, config, install, boundary |
| `templates/rules/testing.md` | §22, §43, §66 | ownership test, quality gates, larangan |
| `templates/rules/universal-agent-rules.md` | §16 | tujuh blok larangan universal |
| `templates/rules/rule-precedence.md` | §15 | sembilan tingkat + penanganan konflik |

Setiap berkas memuat source note dan tanggal review (§11.2).

## Deploy

Jalankan dari akar repository yang membutuhkannya:

```bash
mkdir -p .claude/rules
for r in architecture security testing universal-agent-rules rule-precedence; do
  cp "templates/rules/$r.md" ".claude/rules/$r.md"
done
```

Pada repo aplikasi (backend/frontend), `templates/rules/` tidak ada — salin dari
control repo:

```bash
CONTROL=../public-m2s-vsh-platform
mkdir -p .claude/rules
for r in architecture security testing universal-agent-rules rule-precedence; do
  cp "$CONTROL/templates/rules/$r.md" ".claude/rules/$r.md"
done
```

Tambahkan juga `<stack>.md` per repo (golang.md, nextjs.md) sesuai §37 — itu
project-specific dan tidak punya template kanonik.

## Catatan

- Rules bersifat **soft** (§14.2): dimuat lewat konteks, bukan ditegakkan runtime.
  Yang menegakkan tetap `permissions.deny`, hook PreToolUse fail-closed, runner,
  Git protections, dan CI.
- Menyalin rules TIDAK mengubah enforcement. Ia menambah konteks yang membuat
  agent lebih sering benar sejak awal — bukan pengaman.
- Tidak ada test yang menuntut `.claude/rules/` terisi. Berbeda dari
  `.claude/agents/` yang dijaga `TestDeployedAgentsMatchTemplates`, rules
  bersifat opsional per repository.
