#!/usr/bin/env bash
#
# validate-handoff.sh — SubagentStop hook, fail-closed (§42.5, T-01).
#
# Memvalidasi .task/handoff.json terhadap schemas/handoff.schema.json sebelum
# agent worker diserahkan. Handoff tanpa test evidence atau changed-file inventory
# dianggap incomplete (§35). Blokir SubagentStop kalau handoff tidak valid.
#
# Non-worker (tidak ada contract): bypass tanpa error.
#
# Exit 2 = blokir SubagentStop; exit 0/1 = lanjut (R-24).
#
# Self-test: `validate-handoff.sh --selftest`.

set -euo pipefail

locate_contract() {
  local contract=""
  for cand in \
    "${CLAUDE_PROJECT_DIR:-}/.task/contract.json" \
    "$(pwd)/.task/contract.json" \
    ".task/contract.json"; do
    if [ -n "$cand" ] && [ -f "$cand" ]; then
      contract="$cand"
      break
    fi
  done
  echo "$contract"
}

validate_handoff() {
  local payload="$1"

  local contract
  contract=$(locate_contract)

  if [ -z "$contract" ]; then
    # Bukan sesi worker, bypass.
    exit 0
  fi

  local contract_dir handoff_path schema_path
  contract_dir=$(dirname "$contract")
  handoff_path="$contract_dir/handoff.json"

  # Cari schema: mulai dari project root, naik sampai ketemu.
  schema_path=""
  local search_dir="$contract_dir"
  while [ "$search_dir" != "/" ]; do
    if [ -f "$search_dir/schemas/handoff.schema.json" ]; then
      schema_path="$search_dir/schemas/handoff.schema.json"
      break
    fi
    search_dir=$(dirname "$search_dir")
  done

  if [ -z "$schema_path" ]; then
    echo "BLOCKED: schemas/handoff.schema.json tidak ditemukan" >&2
    exit 2
  fi

  if [ ! -f "$handoff_path" ]; then
    echo "BLOCKED: handoff.json tidak ditemukan di $contract_dir" >&2
    exit 2
  fi

  # Validasi dengan ajv-cli atau python jsonschema — cek ketersediaan.
  local validator=""
  if command -v ajv >/dev/null 2>&1; then
    validator="ajv"
  elif command -v python3 >/dev/null 2>&1 && python3 -c "import jsonschema" 2>/dev/null; then
    validator="python"
  else
    echo "BLOCKED: ajv-cli atau python jsonschema tidak tersedia untuk validasi handoff" >&2
    exit 2
  fi

  local validation_error
  case "$validator" in
    ajv)
      if ! validation_error=$(ajv validate -s "$schema_path" -d "$handoff_path" 2>&1); then
        echo "BLOCKED: handoff.json tidak valid:" >&2
        echo "$validation_error" >&2
        exit 2
      fi
      ;;
    python)
      if ! validation_error=$(python3 -c "
import json, sys
from jsonschema import validate, ValidationError, RefResolver
from pathlib import Path

schema_path = Path('$schema_path')
handoff_path = Path('$handoff_path')
schemas_dir = schema_path.parent

with open(schema_path) as f:
    schema = json.load(f)

# Setup resolver untuk \$ref ke common.schema.json.
resolver = RefResolver(
    base_uri=schema_path.as_uri(),
    referrer=schema,
    store={
        'https://m2s-vsh.mindtoscreen.dev/schemas/common.schema.json':
            json.load(open(schemas_dir / 'common.schema.json'))
    }
)

with open(handoff_path) as f:
    handoff = json.load(f)

try:
    validate(instance=handoff, schema=schema, resolver=resolver)
except ValidationError as e:
    print(f'{e.message} at {list(e.path)}', file=sys.stderr)
    sys.exit(1)
" 2>&1); then
        echo "BLOCKED: handoff.json tidak valid:" >&2
        echo "$validation_error" >&2
        exit 2
      fi
      ;;
  esac

  # Handoff valid.
  exit 0
}

selftest() {
  local tmp
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  export CLAUDE_PROJECT_DIR="$tmp"
  mkdir -p "$tmp/.task"
  mkdir -p "$tmp/schemas"

  # Minimal schema untuk test.
  cat > "$tmp/schemas/common.schema.json" <<'EOF'
{
  "$defs": {
    "schemaVersion": {"type": "string"},
    "taskId": {"type": "string"},
    "role": {"type": "string"},
    "handoffStatus": {"type": "string"},
    "nonEmptyString": {"type": "string", "minLength": 1},
    "pathGlob": {"type": "string"}
  }
}
EOF

  cat > "$tmp/schemas/handoff.schema.json" <<'EOF'
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["schema_version", "task_id", "role", "status", "summary", "changed_files", "tests", "contract_deviations"],
  "properties": {
    "schema_version": {"$ref": "common.schema.json#/$defs/schemaVersion"},
    "task_id": {"$ref": "common.schema.json#/$defs/taskId"},
    "role": {"$ref": "common.schema.json#/$defs/role"},
    "status": {"$ref": "common.schema.json#/$defs/handoffStatus"},
    "summary": {"$ref": "common.schema.json#/$defs/nonEmptyString"},
    "changed_files": {"type": "array"},
    "tests": {"type": "object", "required": ["executed"], "properties": {"executed": {"type": "array"}}},
    "contract_deviations": {"type": "array"}
  }
}
EOF

  echo '{}' > "$tmp/.task/contract.json"

  # validate_handoff memakai `exit` inline; dipanggil dalam subshell agar
  # penghentiannya tidak mengakhiri selftest.
  local got
  # Test 1: handoff tidak ada → blokir.
  got=0; ( validate_handoff '{}' >/dev/null 2>&1 ) || got=$?
  if [ "$got" -ne 2 ]; then
    echo "FAIL test 1: tanpa handoff.json exit $got, mau 2"; exit 1
  fi

  # Test 2: handoff rusak → blokir.
  echo '{"bad json' > "$tmp/.task/handoff.json"
  got=0; ( validate_handoff '{}' >/dev/null 2>&1 ) || got=$?
  if [ "$got" -ne 2 ]; then
    echo "FAIL test 2: handoff rusak exit $got, mau 2"; exit 1
  fi

  # Test 3: handoff valid → lanjut.
  cat > "$tmp/.task/handoff.json" <<'EOF'
{
  "schema_version": "1.0",
  "task_id": "T-001",
  "role": "backend-engineer",
  "status": "implementation-complete",
  "summary": "Selesai implementasi endpoint /users",
  "changed_files": [{"path": "api/users.go", "purpose": "implement endpoint"}],
  "tests": {"executed": [{"command": "go test ./...", "result": "passed"}]},
  "contract_deviations": []
}
EOF

  # Skip kalau validator tidak ada.
  if ! command -v ajv >/dev/null 2>&1 && \
     ! (command -v python3 >/dev/null 2>&1 && python3 -c "import jsonschema" 2>/dev/null); then
    echo "SKIP self-test: ajv atau python jsonschema tidak tersedia"
    exit 0
  fi

  got=0; ( validate_handoff '{}' >/dev/null 2>&1 ) || got=$?
  if [ "$got" -ne 0 ]; then
    echo "FAIL test 3: handoff valid exit $got, mau 0"; exit 1
  fi

  echo "ok  validate-handoff self-test lulus"
}

if [ "${1:-}" = "--selftest" ]; then
  selftest
  exit 0
fi

PAYLOAD="${1:-$(cat)}"
validate_handoff "$PAYLOAD"
