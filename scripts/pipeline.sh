#!/usr/bin/env bash
#
# pipeline.sh — orchestrator per-task ADR-011 + ADR-012.
#
# Alur otomatis satu task dari launch sampai merge-ready:
#   reserve-paths → launch-task → spawn implementer → collect-result
#   → launch-review → spawn reviewer → collect-review
#   → launch-qa → spawn QA → collect-qa → merge-ready
#
# Pemakaian:
#   ./scripts/pipeline.sh --task <id> [--repo <path>] [--control <path>] [--dry-run]
#
# --repo  : path filesystem repo aplikasi (override M2S_REPO_ROOT/<repository>)
# --dry-run : print rencana tiap tahap tanpa spawn atau tulis status
#
# Task paralel: panggil dua pipeline bersamaan (aman — registry anti-overlap):
#   ./scripts/pipeline.sh --task BE-301 &
#   ./scripts/pipeline.sh --task FE-301 &
#   wait

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
M2S_ROOT="${M2S_ROOT:-$(cd "$SCRIPT_DIR/.." && pwd)}"
M2S_BIN="${M2S_BIN:-$M2S_ROOT/bin/m2s}"
MAX_FIX_LOOP=3

task=""
repo_override=""
control="$M2S_ROOT"
dry_run=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --task)    task="$2";         shift 2 ;;
    --repo)    repo_override="$2"; shift 2 ;;
    --control) control="$2";      shift 2 ;;
    --dry-run) dry_run=true;      shift   ;;
    *) echo "pemakaian: $0 --task <id> [--repo <path>] [--control <path>] [--dry-run]" >&2; exit 1 ;;
  esac
done

[[ -n "$task" ]] || { echo "pipeline: --task wajib diisi" >&2; exit 1; }

# ── Helper ────────────────────────────────────────────────────────────────────

# agent_model <role> — baca model dari frontmatter .claude/agents/<role>.md
agent_model() {
  local role="$1"
  local f="$M2S_ROOT/.claude/agents/$role.md"
  [[ -f "$f" ]] || { echo "cmb-agent-coding"; return; }
  grep -m1 '^model:' "$f" | awk '{print $2}'
}

# agent_tools <role> — baca tools dari frontmatter, kembalikan comma-list
agent_tools() {
  local role="$1"
  local f="$M2S_ROOT/.claude/agents/$role.md"
  [[ -f "$f" ]] || { echo "Read,Glob,Grep,Edit,Write,Bash,Skill"; return; }
  grep -m1 '^tools:' "$f" | sed 's/^tools: *\[//;s/\]//;s/[[:space:]]//g'
}

# read_yaml_field <file> <key> — parse field sederhana dari YAML satu-level
read_yaml_field() {
  local file="$1" key="$2"
  grep -m1 "^${key}:" "$file" 2>/dev/null | sed "s/^${key}: *//" | tr -d '"'
}

# read_reservation — baca field reservasi task; set vars: WT, REPO, BRANCH, OWNER_ROLE
read_reservation() {
  local res="$control/control/reservations/$task.yaml"
  [[ -f "$res" ]] || { echo "pipeline: reservasi $task tidak ditemukan — jalankan reserve-paths dulu" >&2; return 1; }
  WT=$(read_yaml_field "$res" "worktree")
  REPO=$(read_yaml_field "$res" "repository")
  BRANCH=$(read_yaml_field "$res" "branch")
  OWNER_ROLE=$(read_yaml_field "$res" "owner_role")
}

# read_status — kembalikan status task saat ini (kosong bila belum ada)
read_status() {
  local sf="$control/control/tasks/status/$task.yaml"
  [[ -f "$sf" ]] || { echo ""; return; }
  read_yaml_field "$sf" "status"
}

# step_log <msg> — cetak langkah pipeline ke stderr (agar mudah dibedakan output agent)
step_log() { echo "[pipeline:$task] $*" >&2; }

# step_dry <msg> — cetak langkah dry-run
step_dry() { echo "[dry-run:$task] $*"; }

# spawn_agent <role> <worktree> <prompt>
# Spawn claude sebagai agent <role> di <worktree>. Cetak role + model sebelum spawn.
# Agent menulis .task/handoff.json di worktree; pipeline baca setelah selesai.
spawn_agent() {
  local role="$1" wt="$2" prompt="$3"
  local model tools
  model=$(agent_model "$role")
  tools=$(agent_tools "$role")
  step_log "spawn role=$role model=$model cwd=$wt"
  if $dry_run; then
    step_dry "  claude --model $model --allowedTools $tools"
    step_dry "  prompt: ${prompt:0:80}..."
    return 0
  fi
  # Hapus handoff lama agar tidak terbaca ulang
  rm -f "$wt/.task/handoff.json"
  # Spawn claude di worktree; agent kerja + tulis .task/handoff.json
  (cd "$wt" && printf '%s' "$prompt" \
    | claude --print \
        --model "$model" \
        --allowedTools "$tools" \
    > /dev/null) || true
  # Verifikasi handoff terbentuk
  [[ -f "$wt/.task/handoff.json" ]] || {
    step_log "WARN: $role tidak menulis .task/handoff.json — handoff kosong"
    return 1
  }
}

# ── Phase 0: setup & verifikasi binary ────────────────────────────────────────

[[ -x "$M2S_BIN" ]] || {
  echo "pipeline: bin/m2s tidak ditemukan di $M2S_BIN — jalankan: make build" >&2
  exit 1
}

# Baca spec task utk dapat role + repo
SPEC=$(ls "$control/control/tasks/specifications/$task.yaml" 2>/dev/null) || {
  echo "pipeline: spec task $task tidak ditemukan" >&2; exit 1
}
SPEC_ROLE=$(read_yaml_field "$SPEC" "  role")
SPEC_REPO=$(read_yaml_field "$SPEC" "  repository")
REPO_PATH="${repo_override:-${M2S_REPO_ROOT:-$M2S_ROOT}/$SPEC_REPO}"

step_log "mulai pipeline task=$task role=$SPEC_ROLE repo=$REPO_PATH"

# ── Phase 1: reserve-paths ─────────────────────────────────────────────────

CURRENT_STATUS=$(read_status)
if [[ "$CURRENT_STATUS" == "" || "$CURRENT_STATUS" == "technical-ready" ]]; then
  step_log "phase 1: reserve-paths"
  $dry_run && step_dry "m2s reserve-paths --task $SPEC --control $control"
  $dry_run || "$M2S_BIN" reserve-paths --task "$SPEC" --control "$control"
fi

# ── Phase 2: launch-task ──────────────────────────────────────────────────

CURRENT_STATUS=$(read_status)
if [[ "$CURRENT_STATUS" == "reserved" || "$CURRENT_STATUS" == "technical-ready" || "$CURRENT_STATUS" == "" ]]; then
  step_log "phase 2: launch-task"
  $dry_run && step_dry "m2s launch-task --task $SPEC --repo $REPO_PATH --control $control"
  $dry_run || "$M2S_BIN" launch-task --task "$SPEC" --repo "$REPO_PATH" --control "$control"
fi

# Baca worktree dari reservasi (tersedia setelah launch-task)
WT="" REPO="" BRANCH="" OWNER_ROLE=""
if $dry_run; then
  # Dry-run: reservasi belum ada — pakai placeholder utk print rencana tahap
  WT="/dry-run/worktrees/$SPEC_REPO/$task"
  REPO="$SPEC_REPO"
  BRANCH="agent/$task-dry"
  OWNER_ROLE="$SPEC_ROLE"
else
  read_reservation
fi

# Prompt implementer — minta agent kerjakan task + tulis handoff + buat PR
PROMPT_IMPL="Kamu adalah $SPEC_ROLE. Kerjakan task sesuai kontrak di .task/contract.json.
Setelah selesai:
1. Commit semua perubahan ke branch $BRANCH.
2. Push branch: git push origin $BRANCH.
3. Buat PR ke develop: gh pr create --base develop --title \"[task $task]\" --body \"Implementasi $task\".
4. Tulis handoff ke .task/handoff.json sesuai schemas/handoff.schema.json.
   Wajib: schema_version, task_id ($task), role ($SPEC_ROLE), status (implementation-complete),
   summary, changed_files, tests, contract_deviations. Sertakan pr_url dari PR yang dibuat.
Ikuti acceptance_criteria dan quality_gates di contract."

# ── Phase 3: loop implementer (maks MAX_FIX_LOOP iterasi) ────────────────

fix_iter=0
while true; do
  CURRENT_STATUS=$(read_status)
  # Keluar loop implementer bila sudah implementation-complete
  [[ "$CURRENT_STATUS" == "implementation-complete" ]] && break
  # Keluar bila melebihi batas iterasi
  if [[ $fix_iter -ge $MAX_FIX_LOOP ]]; then
    step_log "BATAS FIX LOOP ($MAX_FIX_LOOP) tercapai — berhenti"
    exit 1
  fi
  [[ $fix_iter -gt 0 ]] && step_log "fix loop iterasi $fix_iter/$MAX_FIX_LOOP"
  step_log "phase 3: spawn implementer ($SPEC_ROLE)"
  spawn_agent "$SPEC_ROLE" "$WT" "$PROMPT_IMPL"
  # Baca pr_url dari handoff (bila ada)
  PR_URL=""
  if [[ -f "$WT/.task/handoff.json" ]]; then
    PR_URL=$(python3 -c "import json,sys; d=json.load(open('$WT/.task/handoff.json')); print(d.get('pr_url',''))" 2>/dev/null || true)
  fi
  step_log "phase 3: collect-result (pr_url=$PR_URL)"
  PR_FLAG=""
  [[ -n "$PR_URL" ]] && PR_FLAG="--pr $PR_URL"
  $dry_run && step_dry "m2s collect-result --handoff $WT/.task/handoff.json $PR_FLAG --control $control"
  $dry_run || "$M2S_BIN" collect-result \
    --handoff "$WT/.task/handoff.json" \
    $PR_FLAG \
    --control "$control"
  CURRENT_STATUS=$(read_status)
  [[ "$CURRENT_STATUS" == "implementation-complete" ]] && break
  # Dry-run: satu iterasi cukup untuk print rencana — tidak ada status yang advance
  $dry_run && break
  fix_iter=$((fix_iter + 1))
done

# ── Phase 4: review loop ───────────────────────────────────────────────────

PROMPT_REVIEW="Kamu adalah code-reviewer. Lakukan review read-only terhadap diff PR task $task.
Baca .task/contract.json untuk acceptance criteria dan quality gates.
Tulis review report ke .task/handoff.json (schema: schemas/handoff.schema.json, role code-reviewer).
Wajib: decision (approve / approve-with-nonblocking-notes / request-changes),
findings (bila request-changes: min 1 finding dengan severity, category, location, reason, recommended_action),
changed_files: [], tests (bukti static analysis / read-only test run).
JANGAN edit atau tulis berkas application code apa pun."

review_iter=0
while true; do
  step_log "phase 4: launch-review"
  $dry_run && step_dry "m2s launch-review --task $task --control $control"
  $dry_run || "$M2S_BIN" launch-review --task "$task" --control "$control"

  step_log "phase 4: spawn code-reviewer model=$(agent_model code-reviewer)"
  spawn_agent "code-reviewer" "$WT" "$PROMPT_REVIEW"

  step_log "phase 4: collect-review"
  $dry_run && step_dry "m2s collect-review --handoff $WT/.task/handoff.json --control $control"
  $dry_run || "$M2S_BIN" collect-review \
    --handoff "$WT/.task/handoff.json" \
    --control "$control"

  CURRENT_STATUS=$(read_status)
  if [[ "$CURRENT_STATUS" == "reviewing" || "$dry_run" == "true" ]]; then
    step_log "review: approve — lanjut QA"
    break
  elif [[ "$CURRENT_STATUS" == "changes-requested" ]]; then
    review_iter=$((review_iter + 1))
    if [[ $review_iter -ge $MAX_FIX_LOOP ]]; then
      step_log "BATAS REVIEW LOOP ($MAX_FIX_LOOP) — berhenti"
      exit 1
    fi
    step_log "review: changes-requested — re-spawn implementer (iter $review_iter/$MAX_FIX_LOOP)"
    spawn_agent "$SPEC_ROLE" "$WT" "$PROMPT_IMPL"
    $dry_run || "$M2S_BIN" collect-result \
      --handoff "$WT/.task/handoff.json" \
      --control "$control"
  else
    step_log "status tak terduga setelah collect-review: $CURRENT_STATUS"
    exit 1
  fi
done

# ── Phase 5: QA loop ──────────────────────────────────────────────────────

PROMPT_QA="Kamu adalah qa-engineer. Verifikasi implementasi task $task.
Baca .task/contract.json: jalankan quality_gates dan verifikasi acceptance_criteria.
Tulis handoff ke .task/handoff.json (schema: schemas/handoff.schema.json, role qa-engineer).
Wajib: status (implementation-complete bila lulus, defect-found bila ada defect),
findings (bila defect-found: min 1 finding), changed_files (QA artifacts bila ada),
tests (bukti eksekusi quality gates)."

qa_iter=0
while true; do
  step_log "phase 5: launch-qa"
  $dry_run && step_dry "m2s launch-qa --task $task --control $control"
  $dry_run || "$M2S_BIN" launch-qa --task "$task" --control "$control"

  step_log "phase 5: spawn qa-engineer model=$(agent_model qa-engineer)"
  spawn_agent "qa-engineer" "$WT" "$PROMPT_QA"

  step_log "phase 5: collect-qa"
  $dry_run && step_dry "m2s collect-qa --handoff $WT/.task/handoff.json --control $control"
  $dry_run || "$M2S_BIN" collect-qa \
    --handoff "$WT/.task/handoff.json" \
    --control "$control"

  CURRENT_STATUS=$(read_status)
  if [[ "$CURRENT_STATUS" == "merge-ready" || "$dry_run" == "true" ]]; then
    step_log "QA: lulus — status merge-ready"
    break
  elif [[ "$CURRENT_STATUS" == "running" ]]; then
    qa_iter=$((qa_iter + 1))
    if [[ $qa_iter -ge $MAX_FIX_LOOP ]]; then
      step_log "BATAS QA LOOP ($MAX_FIX_LOOP) — berhenti"
      exit 1
    fi
    step_log "QA: defect-found → re-spawn implementer di worktree sama (iter $qa_iter/$MAX_FIX_LOOP)"
    spawn_agent "$SPEC_ROLE" "$WT" "$PROMPT_IMPL"
    $dry_run || "$M2S_BIN" collect-result \
      --handoff "$WT/.task/handoff.json" \
      --control "$control"
  else
    step_log "status tak terduga setelah collect-qa: $CURRENT_STATUS"
    exit 1
  fi
done

# ── Selesai ───────────────────────────────────────────────────────────────

PR_FINAL=""
if [[ -f "$WT/.task/handoff.json" ]]; then
  PR_FINAL=$(python3 -c \
    "import json; d=json.load(open('$WT/.task/handoff.json')); print(d.get('pr_url',''))" \
    2>/dev/null || true)
fi

echo ""
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║  pipeline selesai: $task"
echo "║  status  : merge-ready"
[[ -n "$PR_FINAL" ]] && echo "║  PR      : $PR_FINAL"
echo "║  Langkah berikut: merge ke main dilakukan MANUSIA"
echo "╚══════════════════════════════════════════════════════════════╝"
