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

# normalize_handoff — validasi dan normalisasi format handoff sebelum collect-*
# Perbaiki masalah umum: changed_files array string, tests array, findings inconsistent
normalize_handoff() {
  local handoff="$1"
  [[ -f "$handoff" ]] || return 0

  # Python script untuk normalisasi handoff
  python3 - <<EOF > "${handoff}.tmp" || cat "${handoff}.tmp" && return 1
import json,sys
d = json.load(open('$handoff'))

# 1. changed_files: pastikan array of objects
if 'changed_files' in d:
    if isinstance(d['changed_files'], list):
        fixed = []
        for item in d['changed_files']:
            if isinstance(item, str):
                fixed.append({"path": item, "purpose": "", "change_kind": "modified"})
            elif isinstance(item, dict):
                # Pastikan field wajib ada
                if 'path' not in item:
                    item['path'] = ''
                if 'purpose' not in item:
                    item['purpose'] = ''
                if 'change_kind' not in item:
                    item['change_kind'] = 'modified'
                fixed.append(item)
        d['changed_files'] = fixed
    else:
        d['changed_files'] = []

# 2. tests: pastikan object dengan field 'executed'
if 'tests' in d:
    if isinstance(d['tests'], list):
        d['tests'] = {"executed": d['tests']}
    elif isinstance(d['tests'], dict) and 'executed' not in d['tests']:
        d['tests'] = {"executed": [d['tests']]}
    # Pastikan setiap test punya result valid
    for t in d['tests'].get('executed', []):
        if 'result' in t:
            result = str(t['result']).lower()
            if result in ['pass', 'ok']:
                t['result'] = 'passed'
            elif result == 'fail':
                t['result'] = 'failed'
            elif result == 'skip':
                t['result'] = 'skipped'

# 3. findings: pastikan struktur konsisten
if 'findings' in d:
    fixed = []
    for f in d['findings']:
        # Pastikan field wajib ada
        if 'severity' not in f:
            f['severity'] = 'nit'
        if 'category' not in f:
            f['category'] = 'maintainability'
        if 'location' not in f:
            f['location'] = {
                'path': f.get('path', f.get('file', '')),
                'line': f.get('line', 0)
            }
        # Pastikan reason dan recommended_action ada
        if 'reason' not in f:
            f['reason'] = f.get('summary', f.get('message', 'Temuan review.'))
        if 'recommended_action' not in f:
            f['recommended_action'] = f.get('suggestion', f.get('fix', 'Perbaiki sesuai konteks finding.'))
        fixed.append(f)
    d['findings'] = fixed

# 4. Pastikan field wajib ada
required_fields = ['schema_version', 'task_id', 'role', 'status', 'summary']
for field in required_fields:
    if field not in d:
        if field == 'schema_version':
            d[field] = '1.0'
        elif field == 'task_id':
            d[field] = '$task'
        elif field == 'status':
            d[field] = 'implementation-complete'  # Default status

json.dump(d, sys.stdout, indent=2, ensure_ascii=False)
EOF

  # Ganti handoff asli dengan yang sudah dinormalisasi
  if [[ -s "${handoff}.tmp" ]]; then
    mv "${handoff}.tmp" "$handoff"
  else
    rm -f "${handoff}.tmp"
  fi
}

# step_log <msg> — cetak langkah pipeline ke stderr (agar mudah dibedakan output agent)
step_log() { echo "[pipeline:$task] $*" >&2; }

# step_dry <msg> — cetak langkah dry-run
step_dry() { echo "[dry-run:$task] $*"; }

# spawn_agent <role> <worktree> <prompt>
# Spawn claude sebagai agent <role> di <worktree>. Cetak role + model sebelum spawn.
# Agent menulis .task/handoff.json di worktree; pipeline baca setelah selesai.
# Retry maks SPAWN_RETRY bila exit code non-zero atau handoff tidak terbentuk.
SPAWN_RETRY=2
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
  local attempt
  for ((attempt=1; attempt<=SPAWN_RETRY; attempt++)); do
    [[ $attempt -gt 1 ]] && step_log "retry spawn $role (percobaan $attempt/$SPAWN_RETRY)"
    # Hapus handoff lama agar tidak terbaca ulang
    rm -f "$wt/.task/handoff.json"
    # Spawn claude di worktree. Agent read-only (mis. code-reviewer) menghasilkan
    # structured output (handoff) di stdout, bukan menulis file. Runner tangkap
    # stdout, ekstrak blok JSON/YAML handoff, dan tuliskan ke .task/handoff.json
    # (Q9: "runner yang menuliskannya"). Implementer yang punya Edit/Write juga
    # menulis file langsung — dua jalur aman untuk path yang sama: bila file sudah
    # terbentuk di worktree, output stdout tidak menggantikannya.
    # stderr diarahkan ke log agar kegagalan model terlihat (bug 4).
    local out rc
    out="$(cd "$wt" && printf '%s' "$prompt" \
      | claude --print \
          --model "$model" \
          --allowedTools "$tools" \
          2>>"$wt/.task/audit.log" || true)"
    rc=$?
    # Ekstrak handoff: prioritas blok ```json/```yaml, lalu blok JSON {} valid tunggal.
    local extracted=""
    if [[ -n "$out" ]]; then
      extracted="$(printf '%s' "$out" | awk '/^```json/{f=1;next} /^```/{if(f)exit} f{print}' | sed '/^[[:space:]]*$/d' | python3 -c "
import json,sys
t=sys.stdin.read()
try:
    d=json.loads(t)
    print(json.dumps(d,ensure_ascii=False))
except Exception:
    sys.exit(1)
" 2>/dev/null || true)"
    fi
    if [[ -n "$extracted" ]] && [[ ! -f "$wt/.task/handoff.json" ]]; then
      printf '%s\n' "$extracted" > "$wt/.task/handoff.json"
    fi
    # Verifikasi handoff terbentuk
    [[ -f "$wt/.task/handoff.json" ]] && { return 0; }
    step_log "WARN: $role tidak menulis .task/handoff.json (rc=$rc) — handoff kosong"
  done
  return 1
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
4. Keluarkan handoff sebagai blok JSON tunggal (\`\`\`json ... \`\`\`) di STDOUT — JANGAN
   mencoba menulis file .task/handoff.json (path .task/** deny untuk agent).
   Runner yang menangkap stdout dan menuliskan .task/handoff.json.
   WAJIB bentuk field berikut (lihat schemas/examples/handoff-BE-101.valid.yaml):
   - schema_version: \"1.0\"
   - task_id: \"$task\"
   - role: \"$SPEC_ROLE\"
   - status: \"implementation-complete\"
   - summary: string
   - changed_files: ARRAY OF OBJECT, tiap elemen {path, purpose, change_kind} — BUKAN array string.
   - tests: OBJECT {executed: [{command, result, output_excerpt?}]} — result wajib salah satu dari passed/failed/skipped. BUKAN array langsung.
   - contract_deviations: array (kosong [] bila tidak ada)
   - pr_url: string URL PR yang dibuat
   Contoh tests:
     \"tests\": {\"executed\": [{\"command\": \"go test ./...\", \"result\": \"passed\"}]}
   Contoh changed_files:
     \"changed_files\": [{\"path\": \"internal/handler/x.go\", \"purpose\": \"handler POST\", \"change_kind\": \"added\"}]
   JANGAN tulis changed_files sebagai array string, JANGAN tulis tests sebagai array langsung.
Ikuti acceptance_criteria dan quality_gates di contract."

# ── Phase 3: loop implementer (maks MAX_FIX_LOOP iterasi) ────────────────

fix_iter=0
while true; do
  CURRENT_STATUS=$(read_status)
  # Keluar loop implementer bila sudah implementation-complete ATAU sudah lewat
  # implementer (reviewing/qa-testing/merge-ready) — resume tidak boleh re-spawn.
  case "$CURRENT_STATUS" in
    implementation-complete|reviewing|qa-testing|merge-ready|ci-passed|merged) break ;;
  esac
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
  $dry_run || {
    normalize_handoff "$WT/.task/handoff.json"
    "$M2S_BIN" collect-result \
      --handoff "$WT/.task/handoff.json" \
      $PR_FLAG \
      --control "$control"
  }
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
WAJIB bentuk field (lihat schemas/examples/handoff-review-BE-101.valid.yaml):
- schema_version: \"1.0\", task_id: \"$task\", role: \"code-reviewer\", status: \"implementation-complete\" atau \"changes-requested\"
- decision: salah satu dari approve / approve-with-nonblocking-notes / request-changes (BUKAN \"approved\")
- changed_files: [] (wajib KOSONG — reviewer read-only)
- summary: string
- tests: OBJECT {executed: [{command, result, output_excerpt?}]} — result passed/failed/skipped. BUKAN array.
- contract_deviations: []
- findings: tiap elemen WAJIB {severity (nit/minor/major/blocker), category (correctness/security/maintainability/test/scope/performance), location: {path, line}, reason, recommended_action}. BUKAN pakai file/line/message.
JANGAN edit atau tulis berkas application code apa pun."

review_iter=0
while true; do
  CURRENT_STATUS=$(read_status)
  # Resume: bila sudah reviewing/qa-testing/merge-ready, lewati review.
  case "$CURRENT_STATUS" in
    reviewing|qa-testing|merge-ready|ci-passed|merged) step_log "review: sudah $CURRENT_STATUS — lanjut"; break ;;
  esac
  step_log "phase 4: launch-review"
  $dry_run && step_dry "m2s launch-review --task $task --control $control"
  $dry_run || "$M2S_BIN" launch-review --task "$task" --control "$control"

  # Advance status implementation-complete → reviewing (runner) SEBELUM spawn
  # reviewer, agar collect-review memulai transisi dari reviewing. State machine
  # §33 menolak lompatan implementation-complete → changes-requested.
  step_log "phase 4: set status reviewing (runner)"
  $dry_run && step_dry "m2s update-status -task $task -status reviewing -by runner"
  $dry_run || "$M2S_BIN" update-status -task "$task" -status reviewing -by runner

  step_log "phase 4: spawn code-reviewer model=$(agent_model code-reviewer)"
  spawn_agent "code-reviewer" "$WT" "$PROMPT_REVIEW"

  step_log "phase 4: collect-review"
  $dry_run && step_dry "m2s collect-review --handoff $WT/.task/handoff.json --control $control"
  $dry_run || {
    normalize_handoff "$WT/.task/handoff.json"
    "$M2S_BIN" collect-review \
      --handoff "$WT/.task/handoff.json" \
      --control "$control"
  }

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
Keluarkan handoff sebagai blok JSON tunggal (\`\`\`json ... \`\`\`) di STDOUT — JANGAN
mencoba menulis file .task/handoff.json (path .task/** deny untuk agent).
Runner yang menangkap stdout dan menuliskan .task/handoff.json.
WAJIB bentuk field:
- schema_version: \"1.0\", task_id: \"$task\", role: \"qa-engineer\"
- status: \"implementation-complete\" bila lulus, \"defect-found\" bila ada defect
- summary: string
- changed_files: ARRAY OF OBJECT {path, purpose, change_kind} — BUKAN array string
- tests: OBJECT {executed: [{command, result, output_excerpt?}]} — result passed/failed/skipped. BUKAN array langsung.
- contract_deviations: []
- findings: tiap elemen {severity, category, location: {path, line}, reason, recommended_action} — hanya bila defect-found."

qa_iter=0
while true; do
  CURRENT_STATUS=$(read_status)
  # Resume: bila sudah merge-ready, selesai — jangan re-run QA.
  [[ "$CURRENT_STATUS" == "merge-ready" || "$CURRENT_STATUS" == "ci-passed" || "$CURRENT_STATUS" == "merged" ]] && {
    step_log "QA: sudah $CURRENT_STATUS — selesai"
    break
  }
  step_log "phase 5: launch-qa"
  $dry_run && step_dry "m2s launch-qa --task $task --control $control"
  $dry_run || "$M2S_BIN" launch-qa --task "$task" --control "$control"

  step_log "phase 5: spawn qa-engineer model=$(agent_model qa-engineer)"
  spawn_agent "qa-engineer" "$WT" "$PROMPT_QA"

  step_log "phase 5: collect-qa"
  $dry_run && step_dry "m2s collect-qa --handoff $WT/.task/handoff.json --control $control"
  $dry_run || {
    normalize_handoff "$WT/.task/handoff.json"
    "$M2S_BIN" collect-qa \
      --handoff "$WT/.task/handoff.json" \
      --control "$control"
  }

  CURRENT_STATUS=$(read_status)
  if [[ "$CURRENT_STATUS" == "merge-ready" || "$dry_run" == "true" ]]; then
    step_log "QA: lulus — status merge-ready"
    break
  elif [[ "$CURRENT_STATUS" == "running" || "$CURRENT_STATUS" == "defect-found" ]]; then
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
