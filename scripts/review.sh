#!/usr/bin/env bash
#
# review.sh — spawn Code Reviewer untuk satu task (ADR-012).
#
# Dipakai standalone atau dipanggil pipeline.sh. Menggabungkan:
#   m2s launch-review (gate) → spawn claude code-reviewer → m2s collect-review
#
# Pemakaian:
#   ./scripts/review.sh --task <id> [--control <path>] [--dry-run]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
M2S_ROOT="${M2S_ROOT:-$(cd "$SCRIPT_DIR/.." && pwd)}"
M2S_BIN="${M2S_BIN:-$M2S_ROOT/bin/m2s}"

task="" control="$M2S_ROOT" dry_run=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --task)    task="$2";    shift 2 ;;
    --control) control="$2"; shift 2 ;;
    --dry-run) dry_run=true; shift   ;;
    *) echo "pemakaian: $0 --task <id> [--control <path>] [--dry-run]" >&2; exit 1 ;;
  esac
done
[[ -n "$task" ]] || { echo "review.sh: --task wajib diisi" >&2; exit 1; }

# Baca worktree dari reservasi
res="$control/control/reservations/$task.yaml"
WT=$(grep -m1 '^worktree:' "$res" 2>/dev/null | sed 's/^worktree: *//' | tr -d '"')
[[ -n "$WT" ]] || { echo "review.sh: reservasi $task tidak ditemukan" >&2; exit 1; }

# Model + tools code-reviewer
MODEL=$(grep -m1 '^model:' "$M2S_ROOT/.claude/agents/code-reviewer.md" 2>/dev/null | awk '{print $2}')
TOOLS=$(grep -m1 '^tools:' "$M2S_ROOT/.claude/agents/code-reviewer.md" 2>/dev/null \
  | sed 's/^tools: *\[//;s/\]//;s/[[:space:]]//g')

echo "[review:$task] launch-review (gate implementation-complete)"
$dry_run || "$M2S_BIN" launch-review --task "$task" --control "$control"
$dry_run && echo "[dry-run] launch-review --task $task"

echo "[review:$task] spawn code-reviewer model=$MODEL"
$dry_run || rm -f "$WT/.task/handoff.json"
$dry_run || (cd "$WT" && printf '%s' \
  "Kamu adalah code-reviewer. Review diff PR task $task (read-only).
Baca .task/contract.json. Tulis review report ke .task/handoff.json
(role code-reviewer, wajib: decision, changed_files: [], tests, findings bila request-changes)." \
  | claude --print --model "$MODEL" --allowedTools "$TOOLS" > /dev/null) || true
$dry_run && echo "[dry-run] claude --print --model $MODEL --allowedTools $TOOLS"

echo "[review:$task] collect-review"
$dry_run || "$M2S_BIN" collect-review \
  --handoff "$WT/.task/handoff.json" --control "$control"
$dry_run && echo "[dry-run] collect-review --handoff $WT/.task/handoff.json"

echo "[review:$task] selesai"
