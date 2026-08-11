#!/usr/bin/env bash
#
# worktree-lifecycle.sh — WorktreeCreate/WorktreeRemove hook, fail-closed (§42.6).
#
# WorktreeCreate membatalkan pembuatan pada SETIAP exit non-zero (R-26) — cocok
# sebagai guard secret. Tugas per event:
#
#   WorktreeCreate:
#     - menolak bila worktree akan memuat berkas secret (guard, R-26/§42.6)
#   WorktreeRemove:
#     - menyimpan uncommitted changes report sebelum cleanup (§45, R-26)
#     - TIDAK PERNAH menyalin secret keluar (§42.6)
#
# Catatan: manajemen worktree utama dilakukan RUNNER di luar sesi agent (Q13/A-08:
# launch-task menjalankan `git worktree add`). Hook ini adalah lapisan tambahan
# pada event worktree bawaan Claude Code, bukan penggantinya.
#
# exit 0 izinkan · exit non-zero membatalkan operasi.
#
# Self-test: `worktree-lifecycle.sh --selftest`.

set -euo pipefail

BLOCK=2

# Pola secret §42.3 — sama dengan block-secret-paths agar keduanya konsisten.
SECRET_GLOBS=(
  '.env'
  '.env.*'
  '*.pem'
  '*.key'
  'credentials*.json'
)

deny() {
  echo "worktree-lifecycle: DITOLAK — $1 (§42.6)" >&2
  exit "$BLOCK"
}

# find_secrets memindai sebuah direktori untuk berkas yang cocok pola secret.
# Mengembalikan 0 (dan mencetak daftar) bila ada, 1 bila bersih.
find_secrets() {
  local dir="$1"
  [ -d "$dir" ] || return 1
  local found=1
  for glob in "${SECRET_GLOBS[@]}"; do
    while IFS= read -r -d '' f; do
      echo "$f"
      found=0
    done < <(find "$dir" -type f -name "$glob" ! -path '*/secrets/*' -print0 2>/dev/null || true)
    # /secrets/ dicek terpisah agar seluruh isinya tertangkap.
  done
  while IFS= read -r -d '' f; do
    echo "$f"
    found=0
  done < <(find "$dir" -type d -name secrets -print0 2>/dev/null || true)
  return $found
}

on_create() {
  local worktree="$1"
  [ -z "$worktree" ] && exit 0

  # Buat worktree dulu — EnterWorktree butuh direktori sudah ada.
  local branch="agent/${NAME:-worktree}"
  local ref="${GIT_BASE_REF:-main}"
  if [ -n "${CWD:-}" ] && [ -n "${NAME:-}" ]; then
    git -C "$CWD" worktree add -b "$branch" "$worktree" "$ref" 2>/dev/null || true
  fi

  if [ -d "$worktree" ]; then
    if secrets=$(find_secrets "$worktree"); then
      git -C "${CWD:-.}" worktree remove --force "$worktree" 2>/dev/null || true
      deny "worktree $worktree memuat berkas secret; pembuatan dibatalkan: $(echo "$secrets" | head -3 | tr '\n' ' ')"
    fi
  fi
  echo "$worktree"
  exit 0
}

on_remove() {
  local worktree="$1"
  [ -z "$worktree" ] && exit 0
  [ -d "$worktree" ] || exit 0

  # Simpan laporan uncommitted changes sebelum cleanup (§45, R-26). Laporan
  # ditulis ke lokasi di LUAR worktree agar tidak ikut terhapus.
  local report_dir="${M2S_WORKTREE_REPORTS:-$HOME/.m2s/worktree-reports}"
  mkdir -p "$report_dir" 2>/dev/null || true

  local ts wtname report
  ts=$(date -u '+%Y%m%dT%H%M%SZ')
  wtname=$(basename "$worktree")
  report="$report_dir/$wtname-$ts.txt"

  if command -v git >/dev/null 2>&1 && git -C "$worktree" rev-parse --git-dir >/dev/null 2>&1; then
    {
      echo "# uncommitted changes pada $worktree sebelum cleanup ($ts)"
      git -C "$worktree" status --short 2>/dev/null || echo "(status tidak dapat dibaca)"
    } > "$report" 2>/dev/null || true
  fi

  # Secret TIDAK PERNAH disalin keluar (§42.6): laporan di atas hanya `git status`
  # (nama berkas), bukan isinya. Tidak ada penyalinan berkas dari worktree.
  exit 0
}

selftest() {
  local fails=0
  local tmp
  tmp=$(mktemp -d)

  # Create: worktree bersih diizinkan.
  local clean="$tmp/clean"
  mkdir -p "$clean/internal"
  echo "package x" > "$clean/internal/x.go"
  ( on_create "$clean" >/dev/null 2>&1 ) || { echo "FAIL create-clean: seharusnya izinkan"; fails=1; }

  # Create: worktree dengan .env ditolak.
  local dirty="$tmp/dirty"
  mkdir -p "$dirty"
  echo "SECRET=1" > "$dirty/.env"
  local got=0
  ( on_create "$dirty" >/dev/null 2>&1 ) || got=$?
  [ "$got" -eq "$BLOCK" ] || { echo "FAIL create-secret: exit $got, mau $BLOCK"; fails=1; }

  # Create: worktree dengan *.pem ditolak.
  local dirty2="$tmp/dirty2"
  mkdir -p "$dirty2/certs"
  echo "----" > "$dirty2/certs/server.pem"
  got=0
  ( on_create "$dirty2" >/dev/null 2>&1 ) || got=$?
  [ "$got" -eq "$BLOCK" ] || { echo "FAIL create-pem: exit $got, mau $BLOCK"; fails=1; }

  # Remove: worktree tidak ada tetap exit 0 (idempotent).
  ( on_remove "$tmp/tidak-ada" >/dev/null 2>&1 ) || { echo "FAIL remove-missing: seharusnya exit 0"; fails=1; }

  rm -rf "$tmp"
  if [ "$fails" -eq 0 ]; then echo "ok  worktree-lifecycle self-test lulus"; else exit 1; fi
}

if [ "${1:-}" = "--selftest" ]; then
  selftest
  exit 0
fi

# Event dan path worktree datang dari payload JSON stdin bila tersedia, atau
# dari env var yang disuntikkan Claude Code.
PAYLOAD="$(cat 2>/dev/null || true)"
EVENT=""
WORKTREE=""
NAME=""
if [ -n "$PAYLOAD" ] && command -v jq >/dev/null 2>&1; then
  EVENT=$(jq -r '.hook_event_name // ""' <<<"$PAYLOAD" 2>/dev/null || true)
  WORKTREE=$(jq -r '.worktree_path // ""' <<<"$PAYLOAD" 2>/dev/null || true)
  NAME=$(jq -r '.name // ""' <<<"$PAYLOAD" 2>/dev/null || true)
  CWD=$(jq -r '.cwd // ""' <<<"$PAYLOAD" 2>/dev/null || true)
fi
# WorktreeCreate: path belum ada, konstruksi dari cwd + .claude/worktrees/ + name
if [ "$EVENT" = "WorktreeCreate" ] && [ -z "$WORKTREE" ] && [ -n "$NAME" ] && [ -n "$CWD" ]; then
  WORKTREE="$CWD/.claude/worktrees/$NAME"
fi
EVENT="${EVENT:-${CLAUDE_HOOK_EVENT:-}}"
WORKTREE="${WORKTREE:-${CLAUDE_WORKTREE_PATH:-}}"

case "$EVENT" in
  WorktreeCreate) on_create "$WORKTREE" ;;
  WorktreeRemove) on_remove "$WORKTREE" ;;
  *)
    # Event tak dikenal: jangan blokir operasi worktree yang sah.
    exit 0
    ;;
esac
