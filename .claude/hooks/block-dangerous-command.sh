#!/usr/bin/env bash
#
# block-dangerous-command.sh — PreToolUse hook untuk Bash, FAIL-CLOSED (§42.2).
#
# DEFENSE-IN-DEPTH, BUKAN BOUNDARY (R-08, A-09). Pencocokan teks perintah shell
# tidak dapat sempurna: `g=checkout; git $g` atau `rm -r -f` dapat lolos. Boundary
# sebenarnya adalah permissions.deny (settings.json) + CI changed-path validation
# + branch protection. Hook ini menambah lapisan cepat, bukan lapisan terakhir.
# Uji elakan dilacak sebagai V-04.
#
# exit 0 izinkan · exit 2 BLOKIR (exit 1 tidak memblokir — T-01, R-24).
#
# Self-test: `block-dangerous-command.sh --selftest`.

set -euo pipefail

BLOCK=2

# Pola §42.2 + installer §16.5 + git-escape R-15. Extended regex, dicocokkan
# terhadap perintah yang sudah dinormalisasi (spasi ganda → tunggal).
DANGER_PATTERNS=(
  'rm[[:space:]]+(-[a-z]*r[a-z]*[[:space:]]+)*-[a-z]*r'   # rm -rf, rm -r -f, dst
  '(^|[[:space:];&|])sudo([[:space:]]|$)'
  'chmod[[:space:]]+-[a-z]*R'
  'chown[[:space:]]+-[a-z]*R'
  'git[[:space:]]+reset[[:space:]]+--hard'
  'git[[:space:]]+clean[[:space:]]+-[a-z]*f'
  'git[[:space:]]+push[[:space:]]+.*--force'
  'git[[:space:]]+checkout([[:space:]]|$)'
  'git[[:space:]]+switch([[:space:]]|$)'
  'git[[:space:]]+worktree([[:space:]]|$)'
  'git[[:space:]]+-C([[:space:]]|$)'
  'docker[[:space:]]+system[[:space:]]+prune'
  'kubectl[[:space:]]+delete'
  'terraform[[:space:]]+apply'
  'npm[[:space:]]+(install|i)([[:space:]]|$)'
  'yarn[[:space:]]+add([[:space:]]|$)'
  'pnpm[[:space:]]+add([[:space:]]|$)'
  'pip[0-9]?[[:space:]]+install'
  'go[[:space:]]+get([[:space:]]|$)'
  'prpm[[:space:]]+install'
  'claude[[:space:]]+plugin'
)

deny() {
  echo "block-dangerous-command: DITOLAK — $1 (§42.2)" >&2
  echo "  hook ini defense-in-depth; boundary sebenarnya permissions.deny + CI (R-08)" >&2
  exit "$BLOCK"
}

check_payload() {
  local payload="$1"
  local cmd
  cmd=$(jq -r '.tool_input.command // ""' <<<"$payload" 2>/dev/null || true)
  [ -z "$cmd" ] && exit 0

  # Normalisasi spasi agar `rm  -rf` dan `rm -rf` diperlakukan sama.
  local norm
  norm=$(printf '%s' "$cmd" | tr '\t' ' ' | tr -s ' ')

  for pat in "${DANGER_PATTERNS[@]}"; do
    if printf '%s' "$norm" | grep -qE "$pat"; then
      deny "perintah cocok pola terlarang: $cmd"
    fi
  done
  exit 0
}

selftest() {
  local fails=0
  _expect() {
    local got=0
    ( check_payload "$3" >/dev/null 2>&1 ) || got=$?
    if [ "$got" -ne "$2" ]; then echo "FAIL $1: exit $got, mau $2"; fails=1; fi
  }
  _expect "rm -rf"          2 '{"tool_input":{"command":"rm -rf build"}}'
  _expect "rm -r -f split"  2 '{"tool_input":{"command":"rm -r -f build"}}'
  _expect "git checkout"    2 '{"tool_input":{"command":"git checkout main"}}'
  _expect "git switch"      2 '{"tool_input":{"command":"git switch develop"}}'
  _expect "git -C"          2 '{"tool_input":{"command":"git -C ../other status"}}'
  _expect "npm install"     2 '{"tool_input":{"command":"npm install lodash"}}'
  _expect "go get"          2 '{"tool_input":{"command":"go get github.com/x/y"}}'
  _expect "sudo"            2 '{"tool_input":{"command":"sudo rm x"}}'
  _expect "izinkan make"    0 '{"tool_input":{"command":"make test"}}'
  _expect "izinkan git st"  0 '{"tool_input":{"command":"git status"}}'
  _expect "izinkan go test" 0 '{"tool_input":{"command":"go test ./..."}}'
  if [ "$fails" -eq 0 ]; then echo "ok  block-dangerous-command self-test lulus"; else exit 1; fi
}

command -v jq >/dev/null 2>&1 || deny "dependency jq tidak ditemukan"

if [ "${1:-}" = "--selftest" ]; then
  selftest
  exit 0
fi

check_payload "$(cat)"
