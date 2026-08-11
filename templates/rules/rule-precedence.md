# Rule Precedence

**Sumber:** §15 dokumen arsitektur M2S-VSH Lite v0.1.0
**Ditinjau:** 5 Agustus 2026
**Berlaku:** semua agent, semua repository

> TEMPLATE KANONIK. Salin ke `.claude/rules/rule-precedence.md` pada repo yang
> membutuhkannya. `.claude/**` adalah human-only-write (component-inventory §7),
> sehingga penyalinan dilakukan manusia.

Urutan prioritas tertinggi ke terendah:

1. Human safety, legal, dan production governance.
2. Managed security settings dan repository protections.
3. Task contract: repository, branch, allowed paths, forbidden paths.
4. Approved ADR dan Mneme project decisions.
5. Project-specific architecture and domain rules.
6. Role-agent definition.
7. Skill instructions.
8. User prompt untuk task tersebut.
9. External reference rules.

## Ketika terjadi konflik

- Agent **harus berhenti**.
- Tulis conflict report.
- **Jangan memilih rule sendiri.**
- Eskalasi ke TL/SA atau PM sesuai jenis keputusan.

## Konsekuensi yang sering terlewat

- **Task contract (3) mengalahkan role-agent definition (6).** Path yang
  ditetapkan contract berlaku meski template role melarangnya secara umum —
  template adalah pola default, contract adalah otoritas per-task.
- **User prompt berada di peringkat 8.** Permintaan yang bertentangan dengan
  contract atau ADR tidak menjadikannya sah. Berhenti dan laporkan.
- **Rules ini soft (§14.2).** Ia dimuat lewat konteks, bukan ditegakkan runtime.
  Yang menegakkan: `permissions.deny`, hook PreToolUse, runner, Git, CI.
  Prompt bukan security boundary.
