#!/usr/bin/env bash
#
# qa.sh — spawn QA Engineer untuk satu task (ADR-012).
#
# Dipakai standalone atau dipanggil pipeline.sh. Menggabungkan:
#   m2s launch-qa (gate) → spawn claude qa-engineer → m2s collect-qa
#
# Pemakaian:
#   ./scripts/qa.sh --task <id> [--control <path>] [--dry-run]

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
[[ -n "$task" ]] || { echo "qa.sh: --task wajib diisi" >&2; exit 1; }

res="$control/control/reservations/$task.yaml"
WT=$(grep -m1 '^worktree:' "$res" 2>/dev/null | sed 's/^worktree: *//' | tr -d '"')
[[ -n "$WT" ]] || { echo "qa.sh: reservasi $task tidak ditemukan" >&2; exit 1; }

MODEL=$(grep -m1 '^model:' "$M2S_ROOT/.claude/agents/qa-engineer.md" 2>/dev/null | awk '{print $2}')
TOOLS=$(grep -m1 '^tools:' "$M2S_ROOT/.claude/agents/qa-engineer.md" 2>/dev/null \
  | sed 's/^tools: *\[//;s/\]//;s/[[:space:]]//g')

echo "[qa:$task] launch-qa (gate reviewing)"
$dry_run || "$M2S_BIN" launch-qa --task "$task" --control "$control"
$dry_run && echo "[dry-run] launch-qa --task $task"

echo "[qa:$task] spawn qa-engineer model=$MODEL"
$dry_run || rm -f "$WT/.task/handoff.json"
$dry_run || (cd "$WT" && printf '%s' \
  "Kamu adalah qa-engineer. Verifikasi implementasi task $task.
Baca .task/contract.json, jalankan quality_gates, verifikasi acceptance_criteria.
Tulis handoff ke .task/handoff.json (role qa-engineer, status implementation-complete
bila lulus, defect-found bila ada defect, wajib: findings bila defect-found, tests)." \
  | claude --print --model "$MODEL" --allowedTools "$TOOLS" > /dev/null) || true
$dry_run && echo "[dry-run] claude --print --model $MODEL --allowedTools $TOOLS"

echo "[qa:$task] collect-qa"
$dry_run || "$M2S_BIN" collect-qa \
  --handoff "$WT/.task/handoff.json" --control "$control"
$dry_run && echo "[dry-run] collect-qa --handoff $WT/.task/handoff.json"

echo "[qa:$task] selesai"
