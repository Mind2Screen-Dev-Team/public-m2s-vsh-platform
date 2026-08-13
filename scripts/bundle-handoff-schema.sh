#!/usr/bin/env bash
#
# bundle-handoff-schema.sh — bundle handoff.schema.json menjadi satu file
# tanpa $ref lintas-file, agar bisa dipakai `claude --print --json-schema`.
#
# Latar: schemas/handoff.schema.json memakai $ref ke common.schema.json
# ("common.schema.json#/$defs/..."). `claude --json-schema` hanya menerima
# satu file dan menolak $ref lintas-file. Bundler ini menyalin $defs dari
# common.schema.json, me-rewrite $ref, dan menghapus $schema meta-ref.
#
# Idempotent: hasil ditulis ke $OUT (default schemas/.bundle/handoff.bundled.json).
# Dipakai pipeline.sh saat spawn_agent, bukan diedit manual.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
M2S_ROOT="${M2S_ROOT:-$(cd "$SCRIPT_DIR/.." && pwd)}"
OUT="${1:-$M2S_ROOT/schemas/.bundle/handoff.bundled.json}"

[[ -f "$M2S_ROOT/schemas/handoff.schema.json" ]] || { echo "bundle: handoff.schema.json tidak ditemukan" >&2; exit 1; }
[[ -f "$M2S_ROOT/schemas/common.schema.json" ]] || { echo "bundle: common.schema.json tidak ditemukan" >&2; exit 1; }

mkdir -p "$(dirname "$OUT")"

python3 - "$M2S_ROOT/schemas/handoff.schema.json" "$M2S_ROOT/schemas/common.schema.json" "$OUT" <<'PY'
import json, sys

handoff_path, common_path, out_path = sys.argv[1], sys.argv[2], sys.argv[3]
s = json.load(open(handoff_path))
common = json.load(open(common_path))

# Hapus $schema meta-ref — claude --json-schema menolaknya.
s.pop('$schema', None)

# Salin $defs dari common.schema.json agar $ref bisa di-resolve lokal.
defs = dict(common.get('$defs', {}))
defs.update(s.get('$defs', {}))
s['$defs'] = defs

# Rewrite $ref "common.schema.json#/$defs/X" → "#/$defs/X".
def rewrite(o):
    if isinstance(o, dict):
        for k, v in o.items():
            if k == '$ref' and isinstance(v, str) and v.startswith('common.schema.json#'):
                o[k] = v.replace('common.schema.json#', '#')
            else:
                rewrite(v)
    elif isinstance(o, list):
        for x in o:
            rewrite(x)
rewrite(s)

json.dump(s, open(out_path, 'w'), indent=2, ensure_ascii=False)
print(out_path)
PY
