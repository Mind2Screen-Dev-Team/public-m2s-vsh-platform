#!/usr/bin/env bash
#
# Bagian bersama seluruh wrapper runner.
#
# Wrapper WAJIB tetap tipis (ADR-004 #2 dan #5): ia hanya menemukan binary,
# meneruskan argumen, dan meneruskan exit code. Tidak ada logika keputusan di
# sini — seluruh keputusan berada pada bin/m2s, yang dapat diuji.
#
# Berkas ini sengaja diawali garis bawah agar tidak menyerupai pola
# `scripts/<runner>.sh` pada Q11: ia bukan entry point yang boleh dipanggil
# Project Manager.

set -euo pipefail

# Akar control repository = direktori induk scripts/.
M2S_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
M2S_BIN="${M2S_BIN:-$M2S_ROOT/bin/m2s}"

# run <subcommand> [argumen...]
run() {
  if [[ ! -x "$M2S_BIN" ]]; then
    cat >&2 <<EOF
m2s: binary runner tidak ditemukan di $M2S_BIN

Binary dibangun lokal dan sengaja tidak di-commit (ADR-004 #5), karena ia
adalah penegak batas path yang tidak dapat direview sebagai blob biner.

Bangun lebih dulu:

  make -C "$M2S_ROOT" build
EOF
    # exit 1 = runner gagal berjalan, bukan kontrak ditolak (exit 2).
    exit 1
  fi

  # exec menggantikan proses shell, sehingga exit code dan sinyal diteruskan
  # apa adanya tanpa lapisan tambahan.
  exec "$M2S_BIN" "$@"
}
