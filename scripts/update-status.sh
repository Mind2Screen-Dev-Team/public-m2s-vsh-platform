#!/usr/bin/env bash
#
# Wrapper `m2s update-status` — lihat scripts/_runner.sh.
#
# Entry point yang dipanggil agent/PM untuk menulis status task §33 (ADR-011).
# Nama berkas mengikuti pola `scripts/<runner>.sh` pada Q11 dan §36. Agent
# memanggil lewat wrapper ini (bukan bin/m2s langsung), dengan identitas
# role-nya — validasi owner tetap di kode runner, bukan prompt.

source "$(dirname "${BASH_SOURCE[0]}")/_runner.sh"
run update-status "$@"
