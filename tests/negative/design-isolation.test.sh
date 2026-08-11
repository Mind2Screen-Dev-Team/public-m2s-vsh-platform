#!/usr/bin/env bash
#
# design-isolation.test.sh — kriteria Done §62 (Phase 6).
#
# Bukti: sesi Open Design (cwd = design worktree di control repo) TIDAK dapat
# menulis ke application worktree / repo frontend. Enforce via validate-path-scope
# (task contract allowed paths) + block-dangerous-command (command cross-repo).
#
# Scenario:
#   - Contract task design: allowed hanya design/** (wilayah UI/UX §19.7).
#   - Coba Edit/Write ke ../m2s-vsh-project-frontend/src/**  → HARUS ditolak.
#   - Coba Edit/Write ke src/** (di dalam worktree, bukan wilayah design) → ditolak.
#   - Command git -C frontend (cross-repo) → ditolak.
#   - Write ke design/** (wilayah sah) → diizinkan (kontrol negatif).
#
# Dijalankan `make verify-hooks`. Berdiri sendiri; tidak bergantung pada worktree
# agent nyata.

set -uo pipefail

HOOKS_DIR="${CLAUDE_PROJECT_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}/.claude/hooks"
REPO_ROOT="${CLAUDE_PROJECT_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"

fails=0
pass=0

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

# --- Siapkan worktree + contract design + binary.
BIN="$(mktemp -d)/m2s"
if ! ( cd "$REPO_ROOT" && go build -o "$BIN" ./cmd/m2s ); then
  echo "  FAIL tidak dapat membangun bin/m2s untuk test"; exit 1
fi
WT="$(mktemp -d)"                     # = design worktree (control repo)
mkdir -p "$WT/.task"
cat >"$WT/.task/contract.json" <<'JSON'
{"schema_version":"1.0","task":{"id":"DESIGN-1","title":"System Status screen","type":"design"},"paths":{"allowed":["design/**"],"forbidden":["src/**","../**",".claude/**",".task/**"]}}
JSON
SCOPE_ENV=("M2S_BIN=$BIN" "CLAUDE_PROJECT_DIR=$WT")

echo "test negatif §62 design isolation — design tak boleh menulis app worktree:"

# Write lintas repo ke app frontend (application worktree) — HARUS ditolak.
expect_block "tulis frontend src" validate-path-scope.sh \
  '{"tool_input":{"file_path":"../m2s-vsh-project-frontend/src/components/StatusCard/StatusCard.tsx"}}' "${SCOPE_ENV[@]}"

# Write lintas repo ke design frontend (bukan milik sesi control) — ditolak.
expect_block "tulis frontend design" validate-path-scope.sh \
  '{"tool_input":{"file_path":"../m2s-vsh-project-frontend/design/DESIGN.md"}}' "${SCOPE_ENV[@]}"

# Write ke src/** di dalan worktree (bukan wilayah design) — ditolak.
expect_block "tulis src dalam worktree" validate-path-scope.sh \
  '{"tool_input":{"file_path":"src/app/status/page.tsx"}}' "${SCOPE_ENV[@]}"

# Write ke .claude (self-modification) — ditolak.
expect_block "tulis .claude" validate-path-scope.sh \
  '{"tool_input":{"file_path":".claude/agents/ui-ux-designer.md"}}' "${SCOPE_ENV[@]}"

# Command git -C frontend (cross-repo write) — ditolak block-dangerous.
expect_block "git -C frontend" block-dangerous-command.sh \
  '{"tool_input":{"command":"git -C ../m2s-vsh-project-frontend commit -am x"}}'

echo "kasus yang harus DIIZINKAN (design wilayah sah — kontrol negatif):"

# Write ke design/** (wilayah UI/UX §19.7) — izinkan.
expect_allow "tulis design handoff" validate-path-scope.sh \
  '{"tool_input":{"file_path":"design/handoff/DESIGN-1/handoff.md"}}' "${SCOPE_ENV[@]}"

rm -rf "$(dirname "$BIN")" "$WT"

if [ "$fails" -eq 0 ]; then
  echo "ok  design-isolation.test.sh: $pass kasus §62 lulus"
else
  echo "GAGAL design-isolation.test.sh"
  exit 1
fi