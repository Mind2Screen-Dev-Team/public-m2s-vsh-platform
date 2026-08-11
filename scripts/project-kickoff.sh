#!/usr/bin/env bash
#
# project-kickoff.sh — launcher pre-development untuk klien.
#
# Dijalankan SEKALI setelah setup control repo + repo aplikasi di org klien
# (docs/operator/client-setup.md). Ia menyiapkan skill project-document-builder,
# membaca brief project, lalu menghasilkan 8 dokumen pre-development
# (Discovery Notes → SDD) ke control/pre-dev/ sebagai acuan pengembangan.
#
# Setelah 8 dokumen selesai, klien diarahkan ke workflow normal m2s-vsh:
# task contract → `m2s launch-task`.
#
# Script ini client-safe: hanya berisi skill + role mapping + prompt.
# Tidak mereferensikan dokumen internal manapun (lihat client-setup Bagian 3).

set -euo pipefail

# ── Konstanta ─────────────────────────────────────────────────────────

M2S_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SKILL_SRC="$M2S_ROOT/templates/skills/project-document-builder"
SKILL_DIR="$M2S_ROOT/.claude/skills/project-document-builder"
OUT_DIR="$M2S_ROOT/control/pre-dev"
BRIEF_FILE="$OUT_DIR/BRIEF.md"

# Model default m2s-vsh-combo (kompatibel proxy 9Router / session). Override lewat
# env KICKOFF_MODEL. JANGAN default sonnet/opus — id Anthropic gagal via proxy.
MODEL="${KICKOFF_MODEL:-plan-d-full-free}"
TIMEOUT_S="${KICKOFF_TIMEOUT:-300}"
MAX_RETRY=3
# macOS tak punya `timeout` di PATH; pakai gtimeout (coreutils) bila ada.
if command -v timeout >/dev/null 2>&1; then
  TMO=timeout
elif command -v gtimeout >/dev/null 2>&1; then
  TMO=gtimeout
else
  TMO=
fi

# ── Helper ────────────────────────────────────────────────────────────

log() { printf '\n\033[1;36m==>\033[0m %s\n' "$*"; }
die() { printf '\n\033[1;31mGAGAL:\033[0m %s\n' "$*" >&2; exit 1; }
has_cmd() { command -v "$1" >/dev/null 2>&1; }

# ── 1. Siapkan skill project-document-builder ─────────────────────────

# Skill disalin dari templates/skills/ (kanonik, client-safe) ke
# .claude/skills/ agar `claude --print` mengaktifkannya otomatis saat dijalankan
# dari akar control repo.
setup_skill() {
  if [[ -d "$SKILL_DIR" ]]; then
    log "Skill sudah aktif: $SKILL_DIR (skip)"
    return
  fi
  [[ -d "$SKILL_SRC" ]] || die "Template skill tidak ditemukan di $SKILL_SRC"
  mkdir -p "$SKILL_DIR"
  cp -R "$SKILL_SRC/." "$SKILL_DIR/"
  log "Skill project-document-builder aktif: $SKILL_DIR"
}

# ── 2. Brief project ──────────────────────────────────────────────────

# Baca control/pre-dev/BRIEF.md jika sudah ada (untuk re-run). Jika belum,
# tanya 7 poin via stdin lalu simpan ke BRIEF_FILE.
read_brief() {
  if [[ -s "$BRIEF_FILE" ]]; then
    BRIEF="$(cat "$BRIEF_FILE")"
    log "Memakai brief: $BRIEF_FILE"
    return
  fi

  log "Belum ada brief project. Jawab 7 pertanyaan berikut (baris per jawaban,"
  log "enter untuk kosong). Jawaban disimpan ke $BRIEF_FILE"
  mkdir -p "$OUT_DIR"

  local name client problem goal features existing constraint
  printf '1. Nama project/produk: ';  read -r name
  printf '2. Jenis client / target user: '; read -r client
  printf '3. Masalah utama yang ingin diselesaikan: '; read -r problem
  printf '4. Tujuan bisnis: '; read -r goal
  printf '5. Gambaran fitur yang diinginkan: '; read -r features
  printf '6. Existing process (manual/sistem lama): '; read -r existing
  printf '7. Constraint (budget/timeline/teknologi): '; read -r constraint

  cat > "$BRIEF_FILE" <<EOF
# Project Brief

- **Nama project/produk:** ${name}
- **Jenis client / target user:** ${client}
- **Masalah utama:** ${problem}
- **Tujuan bisnis:** ${goal}
- **Gambaran fitur:** ${features}
- **Existing process:** ${existing}
- **Constraint:** ${constraint}
EOF
  BRIEF="$(cat "$BRIEF_FILE")"
  log "Brief tersimpan: $BRIEF_FILE"
}

# ── 3. Pipeline dokumen ───────────────────────────────────────────────

# Indeks paralel: idx 0-7. REFS berisi indeks dokumen referensi yang
# dibawa (full text) sebagai konteks ke dokumen berikutnya.
FILES=( 01-discovery.md 02-brd.md 03-sow.md 04-prd.md
        05-uiux.md 06-srs.md 07-trd.md 08-sdd.md )
TITLES=( "Discovery Notes" "BRD" "SOW" "PRD"
         "UI/UX Flow" "SRS" "TRD" "SDD" )
ROLES=(  project-manager project-manager project-manager project-manager
         ui-ux-designer technical-lead-system-analyst technical-lead-system-analyst
         technical-lead-system-analyst )
REFS=(  ""            # 0: titik awal (hanya brief)
        "0"           # 1: discovery
        "0 1"         # 2: discovery, brd
        "0 1 2"       # 3: discovery, brd, sow
        "1 2 3"       # 4: brd, sow, prd
        "3 4"         # 5: prd, uiux
        "3 4 5"       # 6: prd, uiux, srs
        "3 4 5 6" )   # 7: prd, uiux, srs, trd

# Isi tiap dokumen disimpan penuh untuk context carry-forward antar dokumen.
declare -a CONTENTS

# ── 4. Prompt builder ─────────────────────────────────────────────────

build_prompt() {
  local i="$1" refs="$2"
  local out
  out="PROJECT BRIEF:\n\n$BRIEF\n\n"
  out+="ROLE PENULIS (persona m2s-vsh): ${ROLES[$i]}\n\n"
  if [[ -n "$refs" ]]; then
    out+="DOKUMEN REFERENSI (bawa konteks, jaga konsistensi istilah/scope):\n"
    local r
    for r in $refs; do
      out+="\n## ${TITLES[$r]} (${FILES[$r]})\n${CONTENTS[$r]}\n"
    done
    out+="\n"
  fi
  out+="INSTRUKSI:\n"
  out+="Gunakan skill project-document-builder yang aktif di project ini.\n"
  out+="Buat dokumen ${TITLES[$i]} sebagai persona ${ROLES[$i]}.\n"
  out+="Ikuti format output 10-bagian dari skill dan tutup dengan Quality Gate Review.\n"
  out+="Output HANYA isi dokumen Markdown lengkap. Setelah Quality Gate Review, STOP — jangan lanjut ke dokumen berikutnya.\n"
  if [[ -n "${FEEDBACK:-}" ]]; then
    out+="\nREVISI DARI USER (terapkan, naikkan versi dokumen, ulangi Quality Gate):\n$FEEDBACK\n"
  fi
  printf '%b' "$out"
}

# ── 5. Generate dokumen via claude --print ────────────────────────────

# Prompt dikirim via stdin (--input-format text) untuk menghindari batas
# panjang argumen command line pada dokumen akhir yang bisa besar.
gen_doc() {
  local i="$1" outfile="$2" attempt=0
  local prompt; prompt="$(build_prompt "$i" "${REFS[$i]}")"
  while (( attempt < MAX_RETRY )); do
    (( attempt++ ))
    log "Generate [${FILES[$i]}] (role: ${ROLES[$i]}, percobaan $attempt/$MAX_RETRY)"
    if printf '%s' "$prompt" | ${TMO:+$TMO "$TIMEOUT_S"} claude --print --model "$MODEL" \
        --input-format text > "$outfile" 2> "$outfile.err"; then
      if [[ -s "$outfile" ]]; then
        return 0
      fi
      log "Keluaran kosong — stderr: $(tail -c 300 "$outfile.err" 2>/dev/null)"
    else
      local rc=$?
      log "Panggilan gagal (exit $rc) — stderr: $(tail -c 300 "$outfile.err" 2>/dev/null)"
    fi
    sleep 3
  done
  die "Gagal generate ${FILES[$i]} setelah $MAX_RETRY percobaan (log: $outfile.err)"
}

# ── 6. Gate interaktif ────────────────────────────────────────────────

# y  → setujui & lanjut ke dokumen berikutnya
# r  → minta feedback, regenerate dokumen yang sama
# n  → berhenti (dokumen yang sudah dibuat tetap tersimpan)
# <teks bebas> → jawaban open question, di-append ke dokumen, lalu lanjut
gate_interactive() {
  local i="$1" file="$2"
  while true; do
    printf '\n\033[1;33m[Gate %d/8 — %s]\033[0m\n' "$((i+1))" "${FILES[$i]}"
    printf '  [y] setujui & lanjut    [r] revisi    [n] berhenti\n'
    printf '  (ketik teks lain = jawaban open question dokumen)\n'
    local ans
    read -r -p '> ' ans || return 0
    case "$ans" in
      [yY]|ya|Ya|approve|Approve|setuju|Setuju|lanjut|Lanjut)
        return 0 ;;
      [rR]|revisi|Revisi|revis|Revis)
        local fb
        read -r -p '  Feedback revisi: ' fb
        FEEDBACK="$fb"
        return 1 ;;
      [nN]|[qQ]|stop|Stop|berhenti|Berhenti|keluar|Keluar)
        die "Dihentikan user. Dokumen yang sudah dibuat tersimpan di $OUT_DIR" ;;
      *)
        # teks bebas = jawaban open question → append ke dokumen
        printf '\n## Jawaban atas Pertanyaan Terbuka\n\n%s\n' "$ans" >> "$file"
        log "Jawaban ditambahkan ke $file"
        return 0 ;;
    esac
  done
}

# ── 7. main ───────────────────────────────────────────────────────────

trap 'die "Dibatalkan. Dokumen yang sudah dibuat tidak hilang."' INT TERM

main() {
  has_cmd claude || die "claude CLI tidak ditemukan di PATH"

  setup_skill
  read_brief

  log "Membuat 8 dokumen pre-development di $OUT_DIR"
  mkdir -p "$OUT_DIR"

  local i
  for (( i=0; i<8; i++ )); do
    local file="$OUT_DIR/${FILES[$i]}"
    FEEDBACK=""
    gen_doc "$i" "$file"
    CONTENTS[$i]="$(cat "$file")"
    local rc=0
    while true; do
      gate_interactive "$i" "$file" || rc=$?
      # rc=0 → approve/jawaban, lanjut. rc=1 → revisi, regenerate dengan FEEDBACK.
      if [[ $rc -eq 0 ]]; then
        break
      fi
      gen_doc "$i" "$file"
      CONTENTS[$i]="$(cat "$file")"
    done
  done

  printf '\n\033[1;32mSelesai — 8 dokumen pre-development di %s.\033[0m\n' "$OUT_DIR"
  printf 'Daftar:\n'
  for f in "${FILES[@]}"; do printf '  - %s\n' "$OUT_DIR/$f"; done
  cat <<'FINAL'

Langkah berikutnya (workflow normal m2s-vsh):
  1. Periksa dan setujui dokumen sebagai acuan pengembangan.
  2. Buat task contract per item kerja (control/tasks/specifications/).
  3. Jalankan:  m2s launch-task <TASK_ID>

Referensi: docs/operator/client-setup.md Bagian 7 (integrasi runner).
FINAL
}

main "$@"
