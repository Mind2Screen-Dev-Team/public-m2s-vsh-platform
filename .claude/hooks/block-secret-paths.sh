#!/usr/bin/env bash
#
# block-secret-paths.sh — PreToolUse hook, FAIL-CLOSED (§42.3).
#
# Memblokir read/write pada path secret. Berjalan LEBIH DULU dari audit hook
# (component-inventory.md §6, R-25) agar nilai secret tidak pernah tercatat.
#
# Kontrak hook Claude Code:
#   - Payload tool datang sebagai JSON di stdin.
#   - exit 0  → izinkan.
#   - exit 2  → BLOKIR. exit 1 TIDAK memblokir apa pun (T-01, R-24) — karena itu
#               setiap jalur kegagalan di sini memakai exit 2, bukan exit 1.
#
# Self-test: `block-secret-paths.sh --selftest`.

set -euo pipefail

BLOCK=2

# Pola secret §42.3. Dicocokkan terhadap basename maupun path penuh.
SECRET_PATTERNS=(
  '(^|/)\.env$'
  '(^|/)\.env\.'
  '/secrets/'
  '\.pem$'
  '\.key$'
  '(^|/)credentials[^/]*\.json$'
)

# deny memblokir dengan pesan ke stderr dan exit 2.
deny() {
  echo "block-secret-paths: DITOLAK — $1 (§42.3)" >&2
  exit "$BLOCK"
}

# extract_paths mengambil seluruh nilai path yang mungkin dari payload tool.
# Edit/Write memakai file_path; Read memakai file_path; sebagian tool memakai path.
extract_paths() {
  local payload="$1"
  jq -r '
    .tool_input // {} |
    [ .file_path, .path, .notebook_path ] |
    map(select(. != null and . != "")) | .[]
  ' <<<"$payload" 2>/dev/null || true
}

check_payload() {
  local payload="$1"
  local p
  while IFS= read -r p; do
    [ -z "$p" ] && continue
    for pat in "${SECRET_PATTERNS[@]}"; do
      if printf '%s' "$p" | grep -qiE "$pat"; then
        deny "path secret: $p"
      fi
    done
  done < <(extract_paths "$payload")
  exit 0
}

selftest() {
  local fails=0
  _expect() { # nama, exit-kode-harapan, payload
    local got=0
    ( check_payload "$3" >/dev/null 2>&1 ) || got=$?
    if [ "$got" -ne "$2" ]; then
      echo "FAIL $1: exit $got, mau $2"; fails=1
    fi
  }
  _expect "blok .env"        2 '{"tool_input":{"file_path":".env"}}'
  _expect "blok .env.prod"   2 '{"tool_input":{"file_path":"config/.env.production"}}'
  _expect "blok secrets dir" 2 '{"tool_input":{"file_path":"infra/secrets/db.txt"}}'
  _expect "blok .pem"        2 '{"tool_input":{"file_path":"certs/server.pem"}}'
  _expect "blok credentials" 2 '{"tool_input":{"file_path":"config/credentials.json"}}'
  _expect "blok credentials-prod" 2 '{"tool_input":{"file_path":"credentials-prod.json"}}'
  _expect "izinkan biasa"    0 '{"tool_input":{"file_path":"internal/payroll/x.go"}}'
  _expect "izinkan env.go"   0 '{"tool_input":{"file_path":"internal/env/load.go"}}'
  if [ "$fails" -eq 0 ]; then echo "ok  block-secret-paths self-test lulus"; else exit 1; fi
}

command -v jq >/dev/null 2>&1 || deny "dependency jq tidak ditemukan"

if [ "${1:-}" = "--selftest" ]; then
  selftest
  exit 0
fi

check_payload "$(cat)"
