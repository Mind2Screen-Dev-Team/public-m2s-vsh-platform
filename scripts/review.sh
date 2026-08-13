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
# Reviewer read-only menghasilkan handoff di stdout; runner tangkap + tulis
# ke .task/handoff.json (Q9: "runner yang menuliskannya"). Bila file sudah ada
# (jalur agent dengan Edit/Write), tidak ditimpa. Retry bila gagal.
SPAWN_RETRY=2
attempt=0
while [[ $attempt -lt $SPAWN_RETRY ]]; do
  attempt=$((attempt + 1))
  [[ $attempt -gt 1 ]] && echo "[review:$task] retry spawn code-reviewer (percobaan $attempt/$SPAWN_RETRY)"
  $dry_run || rm -f "$WT/.task/handoff.json"
  $dry_run || {
    REVIEW_OUT="$(cd "$WT" && printf '%s' \
      "Kamu adalah code-reviewer. Review diff PR task $task (read-only).
Baca .task/contract.json. Tulis review report ke .task/handoff.json
(role code-reviewer, wajib: decision, changed_files: [], tests, findings bila request-changes)." \
      | claude --print --model "$MODEL" --allowedTools "$TOOLS" 2>>"$WT/.task/audit.log" || true)"
    if [[ -n "$REVIEW_OUT" ]] && [[ ! -f "$WT/.task/handoff.json" ]]; then
      printf '%s' "$REVIEW_OUT" | awk '/^```json/{f=1;next} /^```/{if(f)exit} f{print}' \
        | sed '/^[[:space:]]*$/d' \
        | python3 -c "
import json,sys
try:
    print(json.dumps(json.load(sys.stdin),ensure_ascii=False))
except Exception:
    sys.exit(1)
" > "$WT/.task/handoff.json" 2>/dev/null || true
    fi
  }
  [[ -f "$WT/.task/handoff.json" ]] && break
done
$dry_run && echo "[dry-run] claude --print --model $MODEL --allowedTools $TOOLS"

# Normalisasi format handoff sebelum collect-review (perbaiki changed_files/tests/findings)
if [[ -f "$WT/.task/handoff.json" ]]; then
  python3 - <<'EOF' > "$WT/.task/handoff.json.tmp"
import json,sys
d = json.load(open("$WT/.task/handoff.json"))
d.setdefault("schema_version", "1.0")
d.setdefault("status", "implementation-complete")
d.setdefault("contract_deviations", [])
if isinstance(d.get("tests"), list):
    d["tests"] = {"executed": d["tests"]}
for t in d.get("tests", {}).get("executed", []):
    r = str(t.get("result","")).lower()
    if r in ("pass","ok"): t["result"]="passed"
    elif r == "fail": t["result"]="failed"
    elif r == "skip": t["result"]="skipped"
d["changed_files"] = []
for f in d.get("findings", []):
    f.setdefault("reason", f.get("summary","") or "Temuan review.")
    f.setdefault("recommended_action", "Perbaiki sesuai konteks finding.")
    if "location" not in f:
        f["location"] = {"path": f.pop("file", f.pop("path","")), "line": f.pop("line",0)}
json.dump(d, sys.stdout, indent=2, ensure_ascii=False)
EOF
  mv "$WT/.task/handoff.json.tmp" "$WT/.task/handoff.json"
fi

echo "[review:$task] collect-review"
$dry_run || "$M2S_BIN" collect-review \
  --handoff "$WT/.task/handoff.json" --control "$control"
$dry_run && echo "[dry-run] collect-review --handoff $WT/.task/handoff.json"

echo "[review:$task] selesai"
