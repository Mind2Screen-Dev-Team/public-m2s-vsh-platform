# Arsip Task Spec

Berkas di direktori ini adalah task spec **Phase 7** yang **TIDAK VALID
terhadap `schemas/task.schema.json` saat ini**. Disimpan sebagai catatan sejarah,
bukan contract aktif.

## Kenapa diarsip

Kelima spec ini ditulis dan dieksekusi sebelum Phase 8 menstandarkan guard.
Ketiganya melanggar schema dengan cara yang kini dilarang:

| Berkas | Pelanggaran |
|---|---|
| `BE-102.yaml` | `role: backend-developer` (enum: `backend-engineer`), `status: completed` (bukan nilai `taskStatus`), `base_branch: main` |
| `BE-102-fix.yaml` | sama dengan BE-102 |
| `FE-102.yaml` | `status: completed`, `base_branch: main` |
| `QA-102.yaml` | `role: quality-assurance` (enum: `qa-engineer`), `status: completed`, `base_branch: main` |
| `CONTRACT-102.yaml` | `status: approved` (bukan nilai `taskStatus`), `base_branch: main` |

Semuanya juga punya `base_branch: main` — ditolak `cmdValidateTask`
(ADR-001 #2: agent tidak boleh menargetkan main). **Itu catatan sejarah yang
jujur**: Phase 7 memang merge langsung ke main, dan itulah kesalahan yang H-01
(Phase 8) ada untuk mencegah. Berkas-berkas ini sengaja TIDAK diperbaiki supaya
riwayatnya tidak terdistorsi.

## Konsekuensi

- `m2s validate-task` akan menolak (exit 2) berkas mana pun di sini. Itu
  diharapkan, bukan bug.
- Runner membaca spec berdasarkan task-id
  (`control/tasks/specifications/${TASK_ID}.yaml`), bukan dengan memindai
  direktori. Berkas di archive tidak akan pernah di-resolve sebagai contract
  aktif.

## Contoh spec yang VALID

`schemas/examples/task-BE-101.valid.yaml` — contoh resmi yang lolos schema dan
runner.
