#!/usr/bin/env bash
#
# validate-path-scope.sh — PreToolUse hook untuk Edit|Write, FAIL-CLOSED (§42.1).
#
# Memblokir tulis di luar allowed_paths task. Batas path BUKAN statis dan BUKAN
# per-role — ia ditetapkan task contract per-task (component-inventory.md §6:
# "allowed path spesifik task ... berubah per task — milik task contract").
# Hook karena itu TIDAK memuat daftar path; ia mendelegasikan keputusan ke
# `bin/m2s check-path`, satu-satunya otoritas overlap (pathmatch, 24 kasus R-03).
#
# Sesi tanpa contract (.task/contract.json tidak ada) berarti bukan sesi agent
# worker terisolasi — misalnya maintainer manusia. Hook lolos: enforcement path
# scope hanya berlaku pada worker ber-contract. Boundary bagi non-worker adalah
# permissions.deny + CI.
#
# exit 0 izinkan · exit 2 BLOKIR (exit 1 tidak memblokir — T-01, R-24).
#
# Self-test: `validate-path-scope.sh --selftest`.

set -euo pipefail

BLOCK=2

deny() {
  echo "validate-path-scope: DITOLAK — $1 (§42.1)" >&2
  exit "$BLOCK"
}

# locate_binary menemukan bin/m2s. CLAUDE_PROJECT_DIR menunjuk repo root pada
# sesi agent; cwd worktree agent juga memuat bin/ setelah bootstrap runner.
locate_binary() {
  for cand in \
    "${M2S_BIN:-}" \
    "${CLAUDE_PROJECT_DIR:-}/bin/m2s" \
    "$(pwd)/bin/m2s" \
    "bin/m2s"; do
    [ -n "$cand" ] && [ -x "$cand" ] && { echo "$cand"; return 0; }
  done
  return 1
}

# locate_contract menemukan snapshot .task/contract.json pada worktree agent.
locate_contract() {
  for cand in \
    "${CLAUDE_PROJECT_DIR:-}/.task/contract.json" \
    "$(pwd)/.task/contract.json" \
    ".task/contract.json"; do
    [ -n "$cand" ] && [ -f "$cand" ] && { echo "$cand"; return 0; }
  done
  return 1
}

check_payload() {
  local payload="$1"
  local p
  p=$(jq -r '.tool_input.file_path // .tool_input.notebook_path // ""' <<<"$payload" 2>/dev/null || true)
  [ -z "$p" ] && exit 0

  local contract
  if ! contract=$(locate_contract); then
    # Bukan sesi worker ber-contract; scope enforcement tidak berlaku di sini.
    exit 0
  fi

  local bin
  if ! bin=$(locate_binary); then
    # Contract ADA tapi binary tidak: ini sesi worker tanpa penegak. Fail-closed —
    # lebih baik memblokir daripada mengizinkan write tak-tervalidasi (R-24).
    deny "sesi ber-contract tetapi bin/m2s tidak ditemukan — jalankan make build"
  fi

  local wt
  wt=$(dirname "$(dirname "$contract")")

  local out rc=0
  out=$("$bin" check-path -contract "$contract" -worktree "$wt" -path "$p" 2>&1) || rc=$?
  case "$rc" in
    0) exit 0 ;;
    2) deny "$out" ;;
    *) deny "check-path gagal berjalan (exit $rc): $out" ;;
  esac
}

# Self-test membangun contract + binary sementara dan memverifikasi delegasi.
selftest() {
  command -v go >/dev/null 2>&1 || { echo "SKIP self-test: go tidak tersedia"; exit 0; }
  local repo_root tmp bin wt
  repo_root="${CLAUDE_PROJECT_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"
  tmp=$(mktemp -d)

  bin="$tmp/m2s"
  ( cd "$repo_root" && go build -o "$bin" ./cmd/m2s ) || { rm -rf "$tmp"; echo "FAIL build m2s"; exit 1; }
  export M2S_BIN="$bin"

  wt="$tmp/wt"
  mkdir -p "$wt/.task"
  cat >"$wt/.task/contract.json" <<'JSON'
{"schema_version":"1.0","paths":{"allowed":["internal/payroll/**"],"forbidden":["go.mod",".claude/**",".task/**"]}}
JSON
  export CLAUDE_PROJECT_DIR="$wt"

  local fails=0
  _expect() {
    local got=0
    ( check_payload "$3" >/dev/null 2>&1 ) || got=$?
    if [ "$got" -ne "$2" ]; then echo "FAIL $1: exit $got, mau $2"; fails=1; fi
  }
  _expect "izinkan allowed"  0 '{"tool_input":{"file_path":"internal/payroll/x.go"}}'
  _expect "blok di luar"     2 '{"tool_input":{"file_path":"internal/auth/token.go"}}'
  _expect "blok forbidden"   2 '{"tool_input":{"file_path":"go.mod"}}'
  _expect "blok .claude"     2 '{"tool_input":{"file_path":".claude/agents/x.md"}}'

  rm -rf "$tmp"
  if [ "$fails" -eq 0 ]; then echo "ok  validate-path-scope self-test lulus"; else exit 1; fi
}

command -v jq >/dev/null 2>&1 || deny "dependency jq tidak ditemukan"

if [ "${1:-}" = "--selftest" ]; then
  selftest
  exit 0
fi

check_payload "$(cat)"
