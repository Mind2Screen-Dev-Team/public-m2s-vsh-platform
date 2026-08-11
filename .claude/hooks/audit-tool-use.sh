#!/usr/bin/env bash
#
# audit-tool-use.sh — PostToolUse hook, logged pasif (§42.5).
#
# Mencatat setiap tool use (Edit, Write, Bash) ke audit log dengan timestamp,
# agent, tool, path/command, dan exit code. Berjalan setelah tool, tidak memblokir.
# Exit code hook ini TIDAK mempengaruhi tool yang sudah selesai (T-06).
#
# Log ke $CLAUDE_PROJECT_DIR/.task/audit.log pada sesi agent worker; ke /dev/null
# atau console jika tidak ada contract (sesi maintainer manusia).
#
# Self-test: `audit-tool-use.sh --selftest`.

set -euo pipefail

# locate_log menemukan tempat tulis audit. Worker ber-contract pakai .task/audit.log;
# non-worker tidak punya .task — tulis ke stderr atau discard (sesuai env var).
locate_log() {
  local contract
  for cand in \
    "${CLAUDE_PROJECT_DIR:-}/.task/contract.json" \
    "$(pwd)/.task/contract.json" \
    ".task/contract.json"; do
    if [ -n "$cand" ] && [ -f "$cand" ]; then
      contract="$cand"
      break
    fi
  done

  if [ -n "${contract:-}" ]; then
    local logdir
    logdir=$(dirname "$contract")
    echo "$logdir/audit.log"
  else
    # Bukan sesi worker — audit tidak diperlukan; sink ke /dev/null.
    echo "/dev/null"
  fi
}

write_audit() {
  local payload="$1"
  local result="$2"

  local ts agent tool_name file_path command exit_code
  ts=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
  agent="${CLAUDE_AGENT_NAME:-main}"
  tool_name=$(jq -r '.tool_name // "unknown"' <<<"$payload" 2>/dev/null || echo "unknown")
  file_path=$(jq -r '.tool_input.file_path // .tool_input.notebook_path // ""' <<<"$payload" 2>/dev/null || true)
  command=$(jq -r '.tool_input.command // ""' <<<"$payload" 2>/dev/null || true)
  exit_code=$(jq -r '.exit_code // 0' <<<"$result" 2>/dev/null || echo "0")
  input_tokens=$(jq -r '.tool_input.usage.input_tokens // ""' <<<"$payload" 2>/dev/null || true)
  output_tokens=$(jq -r '.tool_input.usage.output_tokens // ""' <<<"$payload" 2>/dev/null || true)

  local logfile
  logfile=$(locate_log)

  # Format: timestamp|agent|tool|path-or-cmd|exit
  if [ -n "$file_path" ]; then
    echo "$ts|$agent|$tool_name|$file_path|$exit_code|$input_tokens|$output_tokens" >> "$logfile"
  elif [ -n "$command" ]; then
    local cmd_short
    cmd_short=$(printf '%.80s' "$command")
    echo "$ts|$agent|$tool_name|$cmd_short|$exit_code|$input_tokens|$output_tokens" >> "$logfile"
  else
    echo "$ts|$agent|$tool_name|(no-path)|$exit_code|$input_tokens|$output_tokens" >> "$logfile"
  fi

  exit 0
}

selftest() {
  command -v jq >/dev/null 2>&1 || { echo "SKIP self-test: jq tidak tersedia"; exit 0; }
  local tmp
  tmp=$(mktemp -d)

  export CLAUDE_PROJECT_DIR="$tmp"
  mkdir -p "$CLAUDE_PROJECT_DIR/.task"
  touch "$CLAUDE_PROJECT_DIR/.task/contract.json"
  export CLAUDE_AGENT_NAME="test-agent"

  local payload='{"tool_name":"Write","tool_input":{"file_path":"x.go"}}'
  local result='{"exit_code":0}'
  ( write_audit "$payload" "$result" >/dev/null ) || true

  local logfile="$CLAUDE_PROJECT_DIR/.task/audit.log"
  if ! grep -q "test-agent|Write|x.go|0||" "$logfile"; then
    rm -rf "$tmp"
    echo "FAIL audit tidak tercatat"
    exit 1
  fi

  rm -rf "$tmp"
  echo "ok  audit-tool-use self-test lulus"
}

command -v jq >/dev/null 2>&1 || {
  # Kalau jq tidak ada, audit tidak bisa parsing — tidak fatal, sink saja.
  exit 0
}

if [ "${1:-}" = "--selftest" ]; then
  selftest
  exit 0
fi

# Payload tool datang sebagai arg pertama; result sebagai arg kedua (jika ada).
# Kalau hanya satu arg, anggap itu payload dan result kosong.
PAYLOAD="${1:-$(cat)}"
RESULT="${2:-{}}"

write_audit "$PAYLOAD" "$RESULT"
