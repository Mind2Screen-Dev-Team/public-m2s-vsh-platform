# Testing Rules

**Sumber:** §22, §43, §66 dokumen arsitektur M2S-VSH Lite v0.1.0
**Ditinjau:** 5 Agustus 2026
**Berlaku:** semua agent yang menulis atau menjalankan test; path-scoped per repo

> TEMPLATE KANONIK. Salin ke `.claude/rules/testing.md` pada repo yang
> membutuhkannya. `.claude/**` adalah human-only-write (component-inventory §7).

## Test Ownership (§22.7)

- Implementer memiliki unit/component tests yang tightly coupled dengan
  implementation-nya.
- QA memiliki integration, system, acceptance, dan E2E tests.
- File test yang sudah dimiliki satu task tidak boleh diedit task lain secara
  paralel.

## Quality Gates (§43, Q4)

- Gate command ditetapkan per task di `quality_gates` contract — bukan di-hardcode
  runner. Untuk repo Go umumnya memanggil target make.
- Gate wajib nyata, bukan nama abstrak.

## Aturan eksekusi (§16.3)

- **Jangan mengklaim test lulus bila tidak dijalankan.**
- Sebutkan command test yang benar-benar dijalankan, dan hasilnya.
- Handoff tanpa test evidence dianggap incomplete (§35); handoff.json divalidasi
  `validate-handoff.sh` (fail-closed exit 2).
- Kegagalan test tanpa `output_excerpt` tidak dapat ditindak (handoff schema).

## Larangan (§22.5, §16.4)

Dilarang:

- memperbaiki implementation code secara langsung saat berperan QA;
- mengurangi expected behaviour agar test lulus;
- mengedit unit test milik implementer tanpa handoff;
- mengubah business rule atau API contract demi test;
- menyembunyikan test yang gagal lewat skip diam-diam.

## Coverage expectation

- Perubahan logika non-trivial meninggalkan setidaknya satu runnable check
  (assert-based self-check atau satu file test kecil; tanpa framework).
- Menerima perubahan tanpa test bila perubahan trivial dan jelas tidak
  membutuhkannya — test yang ditulis tanpa nilai sama buruknya dengan yang
  kurang.
