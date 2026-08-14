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

# gh_repo <repo> — resolve "<owner>/<repo>" dari remote origin control repo
gh_repo() {
  local r="$1" owner
  owner="$(git remote get-url origin 2>/dev/null | sed -E 's#https://github.com/([^/]+)/.*#\1#')"
  owner="${owner:-Mind2Screen-Dev-Team}"
  echo "${owner}/${r}"
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
# Agent menghasilkan handoff sebagai structured output di stdout; runner menuliskan
# .task/handoff.json (Q9: "runner yang menuliskannya"). Implementer yang punya
# Edit/Write juga menulis file langsung — dua jalur aman untuk path yang sama: bila
# file sudah terbentuk di worktree, output stdout tidak menggantikannya.
#
# Structured output (--json-schema) dipakai agar agent DIPAKSA mematuhi schema di
# lapisan API, bukan instruksi prompt. Ini menegakkan Akar 1 + 2 (agent stochastic):
# enum severity/category/decision salah dikoreksi sendiri oleh model, bukan dinormalisasi
# post-hoc. Schema harus dibundle (tanpa $ref lintas-file) via bundle-handoff-schema.sh
# karena `claude --json-schema` menolak $ref ke file lain.
#
# Retry maks SPAWN_RETRY bila exit code non-zero atau handoff tidak terbentuk.
SPAWN_RETRY=2
spawn_agent() {
  local role="$1" wt="$2" prompt="$3"
  local model tools schema
  model=$(agent_model "$role")
  tools=$(agent_tools "$role")
  # Bundle schema handoff (idempotent) agar --json-schema bisa di-resolve lokal.
  # `--json-schema` menerima JSON INLINE, bukan path — jadi isi file dibaca di
  # sini. `|| true` mencegah set -e mematikan pipeline bila bundler gagal; guard
  # [[ -s ]] di bawah menangkapnya.
  local schema_file schema
  schema_file="$("$SCRIPT_DIR/bundle-handoff-schema.sh" 2>/dev/null || true)"
  step_log "spawn role=$role model=$model cwd=$wt schema=$schema_file"
  if $dry_run; then
    step_dry "  claude --model $model --allowedTools $tools --json-schema <bundled>"
    step_dry "  prompt: ${prompt:0:80}..."
    return 0
  fi
  [[ -s "$schema_file" ]] || { step_log "ERROR: schema handoff tidak terbentuk — jalankan scripts/bundle-handoff-schema.sh" >&2; return 1; }
  schema="$(cat "$schema_file")"
  local attempt
  for ((attempt=1; attempt<=SPAWN_RETRY; attempt++)); do
    [[ $attempt -gt 1 ]] && step_log "retry spawn $role (percobaan $attempt/$SPAWN_RETRY)"
    # Hapus handoff lama agar tidak terbaca ulang
    rm -f "$wt/.task/handoff.json"
    # Spawn claude di worktree. `--json-schema` memaksa output valid JSON schema di
    # lapisan API; `--output-format text` (default) menghasilkan satu objek JSON
    # murni di stdout. stderr diarahkan ke log agar kegagalan model terlihat (bug 4).
    local out rc
    out="$(cd "$wt" && printf '%s' "$prompt" \
      | claude --print \
          --model "$model" \
          --allowedTools "$tools" \
          --json-schema "$schema" \
          --output-format text \
          2>>"$wt/.task/audit.log" || true)"
    rc=$?
    # stdout seharusnya sudah objek JSON murni (structured output). Verifikasi
    # singkat: parse JSON, tulis ke handoff bila valid. Tidak ada ekstraksi blok
    # ```json manual — schema sudah menegakkan bentuknya.
    local extracted=""
    if [[ -n "$out" ]]; then
      extracted="$(printf '%s' "$out" | python3 -c "
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

read_findings() {
  local hf="$1"
  [[ -f "$hf" ]] || { echo ""; return 0; }
  python3 -c '
import json,sys
try:
    d=json.load(open(sys.argv[1]))
except Exception:
    sys.exit(0)
fs=d.get("findings") or []
if not fs:
    sys.exit(0)
out=[]
for i,f in enumerate(fs,1):
    loc=f.get("location") or {}
    path=loc.get("path","")
    line=loc.get("line","")
    where=path + (":" + str(line) if line else "")
    out.append("%d. [%s/%s] %s\n   reason: %s\n   aksi: %s" % (
        i, f.get("severity","?"), f.get("category","?"), where,
        f.get("reason",""), f.get("recommended_action","")))
print("\n".join(out))
' "$hf" 2>/dev/null || true
}

# build_fix_prompt <findings_text> — prompt implementer re-spawn dengan findings
# tersuntik. Bila tak ada findings, kembalikan PROMPT_IMPL apa adanya.
build_fix_prompt() {
  local findings="$1" p="$PROMPT_IMPL"
  if [[ -n "$findings" ]]; then
    p="${p}

=== TEMUAN REVIEW/QA YANG HARUS DIPERBAIKI (jangan ulangi yang sama) ===
${findings}
Perbaiki setiap temuan di atas, jalankan ulang quality_gates, commit + push, lalu tulis handoff implementation-complete."
  fi
  printf '%s' "$p"
}

# read_quality_gates <contract_json> — cetak daftar gate (satu per baris).
# contract.json materialize saat launch-task di <worktree>/.task/contract.json.
read_quality_gates() {
  local cj="$1"
  [[ -f "$cj" ]] || return 0
  python3 -c '
import json,sys
try:
    d=json.load(open(sys.argv[1]))
except Exception:
    sys.exit(0)
for x in (d.get("quality_gates") or []):
    print(x)
' "$cj" 2>/dev/null
}

# run_quality_gates <worktree> <contract_json> — jalankan tiap gate di worktree.
# Set global GATE_OUTPUT (output gate yang gagal). Return 0 bila semua pass/skip,
# 1 bila ada yang fail. Gate yang toolchain-nya tak tersedia di-skip (warn),
# bukan dianggap gagal — env mobile/web (Flutter/Xcode) sering absen di runner.
GATE_OUTPUT=""
run_quality_gates() {
  local wt="$1" cj="$2" gates gate out rc any_fail=0
  GATE_OUTPUT=""
  gates="$(read_quality_gates "$cj")"
  [[ -z "$gates" ]] && return 0
  while IFS= read -r gate; do
    [[ -z "$gate" ]] && continue
    step_log "quality-gate: $gate"
    # Toolchain check pakai kata pertama command ("go", "make", "flutter", ...).
    if ! command -v "${gate%% *}" >/dev/null 2>&1; then
      step_log "quality-gate: SKIP $gate — toolchain '${gate%% *}' tidak tersedia di runner"
      continue
    fi
    out="$(cd "$wt" && bash -c "$gate" 2>&1)"; rc=$?
    if [[ $rc -ne 0 ]]; then
      any_fail=1
      # Batasi output agar prompt fix tidak meledak — ambil 40 baris terakhir.
      GATE_OUTPUT="${GATE_OUTPUT}
=== QUALITY GATE GAGAL: ${gate} (exit ${rc}) ===
$(printf '%s' "$out" | tail -40)"
    else
      step_log "quality-gate: pass — $gate"
    fi
  done <<< "$gates"
  return "$any_fail"
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
  $dry_run || {
    if ! "$M2S_BIN" reserve-paths --task "$SPEC" --control "$control"; then
      step_log "ERROR: reserve-paths gagal — periksa output di atas" >&2
      exit 1
    fi
  }
fi

# ── Phase 2: launch-task ──────────────────────────────────────────────────

CURRENT_STATUS=$(read_status)
if [[ "$CURRENT_STATUS" == "reserved" || "$CURRENT_STATUS" == "technical-ready" || "$CURRENT_STATUS" == "" ]]; then
  step_log "phase 2: launch-task"
  $dry_run && step_dry "m2s launch-task --task $SPEC --repo $REPO_PATH --control $control"
  $dry_run || {
    if ! "$M2S_BIN" launch-task --task "$SPEC" --repo "$REPO_PATH" --control "$control"; then
      step_log "ERROR: launch-task gagal — periksa output di atas" >&2
      exit 1
    fi
  }
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
4. Keluarkan handoff sebagai structured output (JSON) di STDOUT. Runner menuliskan
   .task/handoff.json — JANGAN mencoba menulis file .task/handoff.json sendiri
   (path .task/** deny untuk agent). Schema output dipaksakan otomatis; isi field
   berikut secara akurat (referensi: schemas/examples/handoff-BE-101.valid.yaml):
   - schema_version: \"1.0\", task_id: \"$task\", role: \"$SPEC_ROLE\"
   - status: \"implementation-complete\"
   - summary: string
   - changed_files: ARRAY OF OBJECT {path, purpose, change_kind} — BUKAN array string.
   - tests: OBJECT {executed: [{command, result, output_excerpt?}]} — result passed/failed/skipped.
   - contract_deviations: array (kosong [] bila tidak ada)
   - pr_url: string URL PR yang dibuat
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
  # Resolve PR dari branch (bukan handoff — agent sering halusinasi pr_url).
  # gh pr list --head $BRANCH mengembalikan PR nyata, tidak bergantung pada
  # angka yang ditulis agent di handoff.
  PR_URL=""
  if [[ -n "$BRANCH" && -n "$REPO" ]]; then
    PR_URL=$(gh pr list --repo "$(gh_repo "$REPO")" --head "$BRANCH" --state all --json url --jq '.[0].url' 2>/dev/null || true)
  fi
  # Gate quality_gates SEBELUM collect-result. Implementer yang klaim
  # implementation-complete padahal build/test gagal dihentikan di sini —
  # status tidak maju, output gate disuntik ke prompt fix berikutnya.
  if ! $dry_run && ! run_quality_gates "$WT" "$WT/.task/contract.json"; then
    step_log "quality-gate: GAGAL — tahan implementation-complete, re-spawn implementer dengan output gate"
    if [[ $fix_iter -ge $((MAX_FIX_LOOP - 1)) ]]; then
      step_log "BATAS FIX LOOP ($MAX_FIX_LOOP) tercapai — berhenti (quality gate gagal)"
      exit 1
    fi
    GATE_PROMPT="$(build_fix_prompt "$GATE_OUTPUT")"
    fix_iter=$((fix_iter + 1))
    spawn_agent "$SPEC_ROLE" "$WT" "$GATE_PROMPT"
    continue
  fi
  step_log "phase 3: collect-result (pr_url=$PR_URL)"
  PR_FLAG=""
  [[ -n "$PR_URL" ]] && PR_FLAG="--pr $PR_URL"
  $dry_run && step_dry "m2s collect-result --handoff $WT/.task/handoff.json $PR_FLAG --control $control"
  $dry_run || {
    if ! "$M2S_BIN" collect-result \
      --handoff "$WT/.task/handoff.json" \
      $PR_FLAG \
      --control "$control"; then
      step_log "ERROR: collect-result gagal — handoff tak valid (schema) atau status ditolak. Lihat output di atas."
    fi
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
Keluarkan review report sebagai structured output (JSON) di STDOUT. Runner menuliskan
.task/handoff.json. Schema output dipaksakan otomatis; isi field berikut secara
akurat (referensi: schemas/examples/handoff-review-BE-101.valid.yaml):
- schema_version: \"1.0\", task_id: \"$task\", role: \"code-reviewer\"
- status: \"implementation-complete\" atau \"changes-requested\"
- decision: approve / approve-with-nonblocking-notes / request-changes
- changed_files: [] (wajib KOSONG — reviewer read-only)
- summary: string
- tests: OBJECT {executed: [{command, result, output_excerpt?}]} — result passed/failed/skipped.
- contract_deviations: []
- findings: tiap elemen {severity (blocker/major/minor/nit), category (correctness/security/maintainability/test/scope/performance), location: {path, line}, reason, recommended_action}.
JANGAN edit atau tulis berkas application code apa pun."

review_iter=0
while true; do
  CURRENT_STATUS=$(read_status)
   # Resume: bila sudah qa-testing/merge-ready, lewati review. `reviewing` TIDAK
  # termasuk di sini — ia ambigu (launch-review advance ATAU collect-review approve).
  # Bila reviewer belum selesai, pipeline wajib jalankan launch-review (idempoten)
  # + spawn reviewer, bukan lompat QA (Bug 5 lanjutan).
  case "$CURRENT_STATUS" in
    qa-testing|merge-ready|ci-passed|merged) step_log "review: sudah $CURRENT_STATUS — lanjut"; break ;;
  esac
  step_log "phase 4: launch-review"
  $dry_run && step_dry "m2s launch-review --task $task --control $control"
  $dry_run || {
    if ! "$M2S_BIN" launch-review --task "$task" --control "$control"; then
      step_log "ERROR: launch-review gagal — periksa output di atas" >&2
      exit 1
    fi
  }

  step_log "phase 4: spawn code-reviewer model=$(agent_model code-reviewer)"
  spawn_agent "code-reviewer" "$WT" "$PROMPT_REVIEW"

  step_log "phase 4: collect-review"
  $dry_run && step_dry "m2s collect-review --handoff $WT/.task/handoff.json --control $control"
  $dry_run || {
    if ! "$M2S_BIN" collect-review \
      --handoff "$WT/.task/handoff.json" \
      --control "$control"; then
      step_log "ERROR: collect-review gagal — handoff tak valid (schema) atau status ditolak. Lihat output di atas."
    fi
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
    # Tangkap findings reviewer SEBELUM spawn_agent menghapus handoff.json.
    REVIEW_FINDINGS="$(read_findings "$WT/.task/handoff.json")"
    PROMPT_IMPL_FIX="$(build_fix_prompt "$REVIEW_FINDINGS")"
    spawn_agent "$SPEC_ROLE" "$WT" "$PROMPT_IMPL_FIX"
    # Gate sebelum collect-result: implementer yang klaim selesai tanpa build/
    # test hijau di-spawn ulang dengan output gate sampai hijau (bounded).
    $dry_run || {
      local gate_try=0
      while ! run_quality_gates "$WT" "$WT/.task/contract.json"; do
        gate_try=$((gate_try + 1))
        if [[ $gate_try -ge $MAX_FIX_LOOP ]]; then
          step_log "quality-gate: GAGAL $gate_try× setelah fix review — berhenti"
          exit 1
        fi
        step_log "quality-gate: GAGAL — re-spawn implementer dengan output gate (iter $gate_try/$MAX_FIX_LOOP)"
        PROMPT_IMPL_FIX="$(build_fix_prompt "$GATE_OUTPUT")"
        spawn_agent "$SPEC_ROLE" "$WT" "$PROMPT_IMPL_FIX"
      done
      if ! "$M2S_BIN" collect-result \
        --handoff "$WT/.task/handoff.json" \
        --control "$control"; then
        step_log "ERROR: collect-result gagal saat re-spawn implementer — handoff tak valid atau status ditolak."
      fi
    }
  else
    step_log "status tak terduga setelah collect-review: $CURRENT_STATUS"
    exit 1
  fi
done

# ── Phase 5: QA loop ──────────────────────────────────────────────────────

PROMPT_QA="Kamu adalah qa-engineer. Verifikasi implementasi task $task.
Baca .task/contract.json: jalankan quality_gates dan verifikasi acceptance_criteria.
Keluarkan handoff sebagai structured output (JSON) di STDOUT. Runner menuliskan
.task/handoff.json. Schema output dipaksakan otomatis; isi field berikut secara
akurat (referensi: schemas/examples/handoff-BE-101.valid.yaml):
- schema_version: \"1.0\", task_id: \"$task\", role: \"qa-engineer\"
- status: \"implementation-complete\" bila lulus, \"defect-found\" bila ada defect
- summary: string
- changed_files: ARRAY OF OBJECT {path, purpose, change_kind} — BUKAN array string
- tests: OBJECT {executed: [{command, result, output_excerpt?}]} — result passed/failed/skipped.
- contract_deviations: []
- findings: tiap elemen {severity (blocker/major/minor/nit), category, location: {path, line}, reason, recommended_action} — hanya bila defect-found."

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
  $dry_run || {
    if ! "$M2S_BIN" launch-qa --task "$task" --control "$control"; then
      step_log "ERROR: launch-qa gagal — periksa output di atas" >&2
      exit 1
    fi
  }

  step_log "phase 5: spawn qa-engineer model=$(agent_model qa-engineer)"
  spawn_agent "qa-engineer" "$WT" "$PROMPT_QA"

  step_log "phase 5: collect-qa"
  $dry_run && step_dry "m2s collect-qa --handoff $WT/.task/handoff.json --control $control"
  $dry_run || {
    if ! "$M2S_BIN" collect-qa \
      --handoff "$WT/.task/handoff.json" \
      --control "$control"; then
      step_log "ERROR: collect-qa gagal — handoff tak valid (schema) atau status ditolak. Lihat output di atas."
    fi
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
    QA_FINDINGS="$(read_findings "$WT/.task/handoff.json")"
    PROMPT_IMPL_FIX="$(build_fix_prompt "$QA_FINDINGS")"
    spawn_agent "$SPEC_ROLE" "$WT" "$PROMPT_IMPL_FIX"
    $dry_run || {
      local gate_try=0
      while ! run_quality_gates "$WT" "$WT/.task/contract.json"; do
        gate_try=$((gate_try + 1))
        if [[ $gate_try -ge $MAX_FIX_LOOP ]]; then
          step_log "quality-gate: GAGAL $gate_try× setelah fix QA — berhenti"
          exit 1
        fi
        step_log "quality-gate: GAGAL — re-spawn implementer dengan output gate (iter $gate_try/$MAX_FIX_LOOP)"
        PROMPT_IMPL_FIX="$(build_fix_prompt "$GATE_OUTPUT")"
        spawn_agent "$SPEC_ROLE" "$WT" "$PROMPT_IMPL_FIX"
      done
      if ! "$M2S_BIN" collect-result \
        --handoff "$WT/.task/handoff.json" \
        --control "$control"; then
        step_log "ERROR: collect-result gagal saat re-spawn implementer — handoff tak valid atau status ditolak."
      fi
    }
  else
    step_log "status tak terduga setelah collect-qa: $CURRENT_STATUS"
    exit 1
  fi
done

# ── Selesai ───────────────────────────────────────────────────────────────

PR_FINAL=""
if [[ -n "$BRANCH" && -n "$REPO" ]]; then
  PR_FINAL=$(gh pr list --repo "$(gh_repo "$REPO")" --head "$BRANCH" --state all --json url --jq '.[0].url' 2>/dev/null || true)
fi

echo ""
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║  pipeline selesai: $task"
echo "║  status  : merge-ready"
[[ -n "$PR_FINAL" ]] && echo "║  PR      : $PR_FINAL"
echo "║  Langkah berikut: merge ke main dilakukan MANUSIA"
echo "╚══════════════════════════════════════════════════════════════╝"