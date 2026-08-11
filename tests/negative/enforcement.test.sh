#!/usr/bin/env bash
#
# enforcement.test.sh — test negatif §68, membuktikan Done §59.
#
# Sistem HARUS menolak setiap operasi di daftar §68. Setiap kasus di sini
# menyuntikkan payload tool pelanggar ke hook dan menuntut exit 2 (BLOKIR).
# exit 0 atau exit 1 adalah kegagalan — exit 1 khususnya tidak memblokir apa
# pun (T-01, R-24), jadi hook yang exit 1 dianggap gagal sama seperti exit 0.
#
# Dijalankan `make verify-hooks`. Berdiri sendiri: membangun bin/m2s + contract
# sementara, tidak bergantung pada worktree agent nyata.

set -uo pipefail

HOOKS_DIR="${CLAUDE_PROJECT_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}/.claude/hooks"
REPO_ROOT="${CLAUDE_PROJECT_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"

fails=0
pass=0

# expect_block menuntut hook memblokir (exit 2) payload yang diberikan.
# argumen: nama-kasus, hook, payload-json, [env-KV...]
expect_block() {
  local name="$1" hook="$2" payload="$3"; shift 3
  local got=0
  ( echo "$payload" | env "$@" bash "$HOOKS_DIR/$hook" >/dev/null 2>&1 ) || got=$?
  if [ "$got" -eq 2 ]; then
    pass=$((pass + 1))
  else
    echo "  FAIL [$name] $hook: exit $got, mau 2 (BLOKIR)"; fails=1
  fi
}

# expect_allow menuntut hook mengizinkan (exit 0) payload yang diberikan.
expect_allow() {
  local name="$1" hook="$2" payload="$3"; shift 3
  local got=0
  ( echo "$payload" | env "$@" bash "$HOOKS_DIR/$hook" >/dev/null 2>&1 ) || got=$?
  if [ "$got" -eq 0 ]; then
    pass=$((pass + 1))
  else
    echo "  FAIL [$name] $hook: exit $got, mau 0 (IZINKAN)"; fails=1
  fi
}

# --- Siapkan worktree + contract + binary agar validate-path-scope dapat menilai.
BIN="$(mktemp -d)/m2s"
if ! ( cd "$REPO_ROOT" && go build -o "$BIN" ./cmd/m2s ) 2>/dev/null; then
  echo "  FAIL tidak dapat membangun bin/m2s untuk test"; exit 1
fi
WT="$(mktemp -d)"
mkdir -p "$WT/.task"
cat >"$WT/.task/contract.json" <<'JSON'
{"schema_version":"1.0","paths":{"allowed":["internal/payroll/**","docs/user/**"],"forbidden":["go.mod",".claude/**",".task/**","internal/auth/**"]}}
JSON
SCOPE_ENV=("M2S_BIN=$BIN" "CLAUDE_PROJECT_DIR=$WT")

echo "test negatif §68 — setiap kasus harus DITOLAK:"

# §68: Backend mengedit path di luar scope (frontend/lain).
expect_block "edit di luar allowed" validate-path-scope.sh \
  '{"tool_input":{"file_path":"internal/auth/token.go"}}' "${SCOPE_ENV[@]}"

# §68: Edit forbidden path eksplisit.
expect_block "edit forbidden go.mod" validate-path-scope.sh \
  '{"tool_input":{"file_path":"go.mod"}}' "${SCOPE_ENV[@]}"

# §68: Agent mengubah .claude/agents (self-modification, R-12).
expect_block "edit .claude/agents" validate-path-scope.sh \
  '{"tool_input":{"file_path":".claude/agents/backend-engineer.md"}}' "${SCOPE_ENV[@]}"

# §68: Agent menjalankan git switch.
expect_block "git switch" block-dangerous-command.sh \
  '{"tool_input":{"command":"git switch main"}}'

# §68: Agent menjalankan git checkout.
expect_block "git checkout" block-dangerous-command.sh \
  '{"tool_input":{"command":"git checkout -- ."}}'

# §68: Agent memasang PRPM package.
expect_block "prpm install" block-dangerous-command.sh \
  '{"tool_input":{"command":"prpm install some-pkg"}}'

# §68: Agent memasang npm package (memutakhirkan lockfile, R-19).
expect_block "npm install" block-dangerous-command.sh \
  '{"tool_input":{"command":"npm install left-pad"}}'

# Bash write-effect keluar scope lewat rm -rf (destruktif §42.2).
expect_block "rm -rf" block-dangerous-command.sh \
  '{"tool_input":{"command":"rm -rf internal"}}'

# git -C menulis repo lain (cross-repo R-15).
expect_block "git -C repo lain" block-dangerous-command.sh \
  '{"tool_input":{"command":"git -C ../other commit -am x"}}'

# §68: Agent membaca .env.
expect_block "baca .env" block-secret-paths.sh \
  '{"tool_input":{"file_path":".env"}}'

# Secret .pem.
expect_block "baca .pem" block-secret-paths.sh \
  '{"tool_input":{"file_path":"deploy/tls/server.pem"}}'

# secrets/ dir.
expect_block "tulis secrets/" block-secret-paths.sh \
  '{"tool_input":{"file_path":"config/secrets/db.txt"}}'

echo "kasus yang harus DIIZINKAN (kontrol negatif — penjaga tak boleh menolak segalanya):"

# Path dalam allowed harus lolos.
expect_allow "edit allowed" validate-path-scope.sh \
  '{"tool_input":{"file_path":"internal/payroll/period.go"}}' "${SCOPE_ENV[@]}"

# Perintah aman harus lolos.
expect_allow "make test" block-dangerous-command.sh \
  '{"tool_input":{"command":"make test"}}'

# File biasa bukan secret.
expect_allow "baca file biasa" block-secret-paths.sh \
  '{"tool_input":{"file_path":"internal/env/load.go"}}'

rm -rf "$(dirname "$BIN")" "$WT"

if [ "$fails" -eq 0 ]; then
  echo "ok  enforcement.test.sh: $pass kasus §68 lulus"
else
  echo "GAGAL enforcement.test.sh"
  exit 1
fi
