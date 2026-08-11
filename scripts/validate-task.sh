#!/usr/bin/env bash
#
# Wrapper `m2s validate-task` — lihat scripts/_runner.sh.
#
# Entry point yang dipanggil Project Manager. Nama berkas mengikuti pola
# `scripts/<runner>.sh` pada Q11 dan §36; jangan diubah tanpa meninjau
# pembatasan tool Bash PM.

source "$(dirname "${BASH_SOURCE[0]}")/_runner.sh"
run validate-task "$@"
