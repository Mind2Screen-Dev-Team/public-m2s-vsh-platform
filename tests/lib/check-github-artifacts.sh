#!/usr/bin/env bash
# check-github-artifacts.sh — penegak aturan bentuk artefak GitHub (§60, ADR-007).
#
# Satu sumber kebenaran untuk aturan bentuk. `make verify-github` memakainya
# atas berkas nyata; `tests/negative/github-workflow.test.sh` memakainya atas
# fixture yang sengaja salah. Keduanya memanggil kode yang SAMA — kalau aturan
# ini hanya disalin ke dua tempat, test negatif akan menguji salinannya, bukan
# penegaknya.
#
# Pemakaian:
#   check-github-artifacts.sh workflow   <berkas>
#   check-github-artifacts.sh codeowners <berkas>
#   check-github-artifacts.sh prtemplate <berkas>
#
# Exit: 0 = lulus, 1 = ada pelanggaran (dicetak satu baris per pelanggaran),
#       3 = salah pakai.

set -uo pipefail

fails=0

fail() {
  echo "FAIL $1"
  fails=1
}

# ── workflow ─────────────────────────────────────────────────────────────
#
# Dua aturan yang menentukan apakah workflow AMAN dijadikan required check.
check_workflow() {
  local f="$1"

  # 1. Tidak ada `if:` pada level job.
  #
  # Bentuk YAML: `jobs:` kolom 0, nama job indentasi 2, kunci job indentasi 4,
  # `- name:` step indentasi 6, kunci step indentasi 8. Jadi `if:` berindentasi
  # tepat 4 spasi adalah `if:` level job — dan itulah yang membuat GitHub
  # men-skip job tanpa melaporkan status apa pun. Sebagai required check, job
  # semacam itu memblokir setiap PR yang tidak cocok, permanen (ADR-001:
  # "status check yang tidak pernah dilaporkan akan memblokir seluruh merge
  # tanpa jalan keluar"). `if:` pada level STEP (indentasi 8) justru yang
  # benar dan tidak dilarang.
  if grep -nE '^ {4}if:' "$f" >/dev/null 2>&1; then
    fail "$f memuat \`if:\` pada level job — job yang di-skip tidak melaporkan status, jadi ia tidak dapat menjadi required check (ADR-007 #2). Pindahkan pemilahan ke dalam step."
  fi

  # 2. Trigger merge_group wajib ada.
  #
  # Docs GitHub: "you need to update the workflows to include the merge_group
  # event as an additional trigger. Otherwise, status checks will not be
  # triggered when you add a pull request to a merge queue. The merge will
  # fail as the required status check will not be reported."
  if ! grep -qE '^ {2}merge_group:' "$f"; then
    fail "$f tidak memuat trigger \`merge_group:\` — required check tidak akan melapor di merge queue dan merge gagal selamanya (ADR-007 #3)"
  fi

  # 3. Nama job adalah nama required check; mengubahnya memutus branch protection.
  # Nama job berada pada indentasi 2 (di bawah `jobs:` pada kolom 0).
  if ! grep -qE '^ {2}validate-changed-paths:$' "$f"; then
    fail "$f tidak memuat job bernama \`validate-changed-paths\` — nama itu ADALAH nama required check yang terpasang di branch protection"
  fi

  # 4. Nama branch tidak boleh diinterpolasi langsung ke dalam shell.
  #
  # Nama branch adalah input tak terpercaya dan dapat memuat metakarakter
  # shell. `${{ github.head_ref }}` hanya boleh muncul sebagai nilai variabel
  # env, tidak di dalam blok `run:`.
  local bad
  bad=$(grep -n '\${{ *github\.head_ref *}}' "$f" 2>/dev/null \
    | grep -vE ':[[:space:]]*[A-Z_]+:[[:space:]]*\$\{\{' || true)
  if [ -n "$bad" ]; then
    fail "$f menginterpolasi \${{ github.head_ref }} di luar blok env — nama branch adalah input tak terpercaya, lewatkan sebagai variabel env. Baris: $(printf '%s' "$bad" | cut -d: -f1 | tr '\n' ' ')"
  fi

  # 5. Read-only. Workflow ini memvalidasi, tidak menulis.
  if ! grep -qE '^permissions:' "$f"; then
    fail "$f tanpa blok \`permissions:\` — workflow harus menyatakan hak minimum secara eksplisit"
  fi

  # 6. H-02: branch agent/* wajib dipecah task-id lewat sed, bukan dibiarkan
  #    lolos sampai contract dicari jauh kemudian. Workflow tanpa langkah ini
  #    gagal-BUKA: branch planning `agent/planning-xyz` lolos, lalu CI baru
  #    patah saat contract tak ditemukan (phase-8-hardening.md H-02).
  if ! grep -qF 's#^agent/' "$f"; then
    fail "$f tanpa langkah ekstraksi task-id agent/ (H-02) — branch agent/* tanpa task-id akan lolos scope lalu gagal jauh saat contract dicari"
  fi
}

# ── CODEOWNERS ───────────────────────────────────────────────────────────
check_codeowners() {
  local f="$1"

  # Cakupan yang dituntut mitigasi R-12 (vektor PR) dan R-20.
  local pat
  for pat in '/\.claude/' '/\.github/' '/\.github/CODEOWNERS' '/cmd/m2s/' '/Makefile'; do
    if ! grep -qE "^${pat}" "$f"; then
      fail "$f tidak memuat aturan untuk ${pat//\\/} — vektor PR R-12/R-20 tidak tertahan"
    fi
  done

  # Setiap baris aturan wajib punya owner, jika tidak ia justru MENGHAPUS
  # kepemilikan dari pola yang lebih umum di atasnya.
  local ownerless
  ownerless=$(grep -nE '^[^#[:space:]]+[[:space:]]*$' "$f" || true)
  if [ -n "$ownerless" ]; then
    fail "$f memuat pola tanpa owner — pola tanpa owner MENGHAPUS kepemilikan, bukan menambahkannya. Baris: $(printf '%s' "$ownerless" | cut -d: -f1 | tr '\n' ' ')"
  fi
}

# ── PR template ──────────────────────────────────────────────────────────
check_prtemplate() {
  local f="$1"

  grep -qi 'task id' "$f" \
    || fail "$f tidak meminta Task ID — §44 menuntut task-id pada tiap PR, dan §68 menolak PR tanpa task ID"
  grep -q '16\.5' "$f" \
    || fail "$f tidak merujuk larangan universal §16.5"
  grep -q '\.claude/' "$f" \
    || fail "$f tidak memuat pernyataan bahwa PR tidak menyentuh .claude/** (R-12)"
  grep -qi 'forbidden_paths' "$f" \
    || fail "$f tidak meminta forbidden_paths dari contract"
  grep -qiE 'bukan[^[:alnum:]]{0,4}implementer' "$f" \
    || fail "$f tidak memuat penegasan reviewer bukan implementer (§29.7, §47)"
}

# ── Dispatch ─────────────────────────────────────────────────────────────
if [ "$#" -ne 2 ]; then
  echo "pakai: $(basename "$0") workflow|codeowners|prtemplate <berkas>" >&2
  exit 3
fi

kind="$1"
file="$2"

if [ ! -f "$file" ]; then
  echo "FAIL $file tidak ada"
  exit 1
fi

case "$kind" in
  workflow)   check_workflow   "$file" ;;
  codeowners) check_codeowners "$file" ;;
  prtemplate) check_prtemplate "$file" ;;
  *)
    echo "jenis tidak dikenal: $kind" >&2
    exit 3
    ;;
esac

exit "$fails"
