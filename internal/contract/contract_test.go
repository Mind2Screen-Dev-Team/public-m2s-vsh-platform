package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// schemaDir menunjuk schemas/ dari lokasi paket ini.
func schemaDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.Abs(filepath.Join("..", "..", "schemas"))
	if err != nil {
		t.Fatalf("resolve schemas/: %v", err)
	}
	return d
}

func newV(t *testing.T) *Validator {
	t.Helper()
	v, err := NewValidator(schemaDir(t))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	return v
}

// TestCompileAllSchemas memastikan ketiga schema dapat dikompilasi. Kegagalan
// di sini berarti schema rusak, bukan dokumen yang salah.
func TestCompileAllSchemas(t *testing.T) {
	newV(t)
}

// TestLoadExamples memvalidasi seluruh contoh *.valid.yaml terhadap schema-nya.
// Contoh yang gagal berarti schema dan contoh sudah menyimpang.
func TestLoadExamples(t *testing.T) {
	v := newV(t)
	dir := filepath.Join(schemaDir(t), "examples")

	cases := map[string]Kind{
		"task-BE-101.valid.yaml":           KindTask,
		"reservation-BE-101.valid.yaml":    KindReservation,
		"handoff-BE-101.valid.yaml":        KindHandoff,
		"handoff-review-BE-101.valid.yaml": KindHandoff,
	}

	for name, kind := range cases {
		path := filepath.Join(dir, name)
		doc, err := v.Load(path, kind)
		if err != nil {
			t.Errorf("%s harus valid, dapat: %v", name, err)
			if ve, ok := err.(*ValidationError); ok {
				for _, s := range ve.Violations {
					t.Errorf("    %s", s)
				}
			}
			continue
		}
		if doc["schema_version"] != "1.0" {
			t.Errorf("%s: schema_version = %v", name, doc["schema_version"])
		}
	}
}

// TestRejectInvalid memastikan dokumen yang melanggar schema ditolak beserta
// alasannya, bukan lolos diam-diam.
func TestRejectInvalid(t *testing.T) {
	v := newV(t)

	cases := []struct {
		name string
		kind Kind
		yaml string
		want string // potongan yang harus muncul pada pelanggaran
	}{
		{
			name: "task tanpa .task/** pada forbidden",
			kind: KindTask,
			yaml: minimalTask("  forbidden:\n    - .claude/**\n"),
			want: "task",
		},
		{
			name: "reviewer melaporkan perubahan file",
			kind: KindHandoff,
			yaml: `
schema_version: "1.0"
task_id: BE-101
role: code-reviewer
status: implementation-complete
summary: ringkasan
decision: approve
changed_files:
  - path: a.go
    purpose: p
tests:
  executed: []
  not_executed_reason: read-only
contract_deviations: []
`,
			want: "changed_files",
		},
		{
			name: "handoff blocked tanpa blocked_reason",
			kind: KindHandoff,
			yaml: `
schema_version: "1.0"
task_id: BE-101
role: backend-engineer
status: blocked
summary: ringkasan
changed_files: []
tests:
  executed: []
  not_executed_reason: terhenti
contract_deviations: []
`,
			want: "blocked_reason",
		},
		{
			name: "reservasi pending-merge tanpa pr_url",
			kind: KindReservation,
			yaml: `
schema_version: "1.0"
task_id: BE-101
repository: m2s-vsh-project-backend
branch: agent/BE-101-close-payroll
worktree: /Users/x/.m2s/worktrees/m2s-vsh-project-backend/BE-101
allowed_paths: ["internal/payroll/**"]
reserved_paths: ["internal/payroll/**"]
forbidden_paths: [".claude/**"]
status: reserved-pending-merge
owner_role: backend-engineer
created_at: "2026-07-30T09:15:00+07:00"
`,
			want: "pr_url",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "doc.yaml")
			if err := os.WriteFile(path, []byte(c.yaml), 0o644); err != nil {
				t.Fatalf("menulis fixture: %v", err)
			}
			_, err := v.Load(path, c.kind)
			if err == nil {
				t.Fatal("harus ditolak, tetapi diterima")
			}
			ve, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("harus ValidationError, dapat %T: %v", err, err)
			}
			if len(ve.Violations) == 0 {
				t.Error("pelanggaran harus dilaporkan, bukan daftar kosong")
			}
			joined := strings.Join(ve.Violations, " | ")
			if !strings.Contains(joined, c.want) {
				t.Errorf("pelanggaran harus menyebut %q, dapat: %s", c.want, joined)
			}
		})
	}
}

// TestMaterializeJSON memastikan snapshot Q15 ditulis read-only dan setia
// terhadap sumber YAML.
func TestMaterializeJSON(t *testing.T) {
	v := newV(t)
	src := filepath.Join(schemaDir(t), "examples", "task-BE-101.valid.yaml")
	doc, err := v.Load(src, KindTask)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	dest := filepath.Join(t.TempDir(), ".task", "contract.json")
	if err := MaterializeJSON(doc, dest); err != nil {
		t.Fatalf("MaterializeJSON: %v", err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o444 {
		t.Errorf("contract.json harus read-only 0444, dapat %o", perm)
	}

	// Snapshot hasil materialisasi wajib tetap valid terhadap schema yang sama.
	if _, err := v.Load(dest, KindTask); err != nil {
		t.Errorf("hasil materialisasi harus tetap valid: %v", err)
	}

	if _, err := os.Stat(dest + ".tmp"); !os.IsNotExist(err) {
		t.Error("berkas sementara harus dibersihkan")
	}
}

func TestDecodeRejectsEmpty(t *testing.T) {
	v := newV(t)
	path := filepath.Join(t.TempDir(), "kosong.yaml")
	if err := os.WriteFile(path, []byte("# hanya komentar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Load(path, KindTask); err == nil {
		t.Error("dokumen kosong harus ditolak")
	}
}

// --- helper fixture ---

func minimalTask(forbidden string) string {
	return `schema_version: "1.0"
task:
  id: BE-101
  title: t
  type: backend-implementation
  project: p
  status: technical-ready
ownership:
  role: backend-engineer
  repository: m2s-vsh-project-backend
  base_branch: develop
  branch: agent/BE-101-close-payroll
execution:
  isolation: worktree
  max_turns: 30
  timeout_minutes: 45
paths:
  allowed:
    - internal/payroll/**
` + forbidden + `acceptance_criteria:
  - a
quality_gates:
  - make test
outputs:
  - code
stop_conditions:
  - contract change required
`
}
