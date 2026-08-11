package status

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mind2Screen-Dev-Team/public-m2s-vsh-platform/internal/contract"
)

// newStore menyiapkan store pada direktori sementara dengan schema asli.
func newStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	src, err := filepath.Abs(filepath.Join("..", "..", "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(root, "schemas")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	v, err := contract.NewValidator(dst)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(root, "control", "tasks", "status"), v)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestWriteReadRoundtrip(t *testing.T) {
	s := newStore(t)
	if err := s.Write("BE-201", "running", "backend-engineer", "launch", nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	st, err := s.Read("BE-201")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if st.TaskID() != "BE-201" || st.Status() != "running" || st.By() != "backend-engineer" {
		t.Errorf("roundtrip salah: %+v", st.Doc)
	}
	if st.Doc["updated_at"] == "" {
		t.Error("updated_at harus terisi")
	}
	if st.Doc["reason"] != "launch" {
		t.Error("reason harus tersimpan")
	}
}

func TestWriteRejectsInvalidStatus(t *testing.T) {
	s := newStore(t)
	// "bukan-status" bukan anggota enum taskStatus — schema menolak.
	if err := s.Write("BE-201", "bukan-status", "backend-engineer", "", nil); err == nil {
		t.Fatal("status di luar enum harus ditolak schema")
	}
	// by "runner" bukan anggota enum role — schema menolak.
	if err := s.Write("BE-201", "running", "runner", "", nil); err == nil {
		t.Fatal("by=runner harus ditolak — by adalah enum role")
	}
}

func TestReadMissing(t *testing.T) {
	s := newStore(t)
	_, err := s.Read("BE-999")
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Errorf("berkas belum ada harus membungkus ErrNotExist, dapat %v", err)
	}
}

func TestWritePreservesExtras(t *testing.T) {
	s := newStore(t)
	extra := map[string]any{"pr_url": "https://github.com/Mind2Screen-Dev-Team/m2s-vsh-project-backend/pull/13"}
	if err := s.Write("BE-201", "implementation-complete", "backend-engineer", "", extra); err != nil {
		t.Fatalf("Write: %v", err)
	}
	st, err := s.Read("BE-201")
	if err != nil {
		t.Fatal(err)
	}
	if st.Doc["pr_url"] != extra["pr_url"] {
		t.Error("pr_url harus tersimpan sebagai properti tambahan")
	}
}

func TestTransitionAllowed(t *testing.T) {
	// Rantai §33 linier — satu langkah maju.
	allowed := [][2]string{
		{"draft", "needs-business-clarification"},
		{"technical-ready", "reserved"},
		{"reserved", "running"},
		{"running", "implementation-complete"},
		{"implementation-complete", "reviewing"},
		{"reviewing", "qa-testing"},
		{"qa-testing", "ci-passed"},
		{"ci-passed", "merge-ready"},
		{"merge-ready", "merged"},
		{"documented", "staging-verified"},
		{"staging-verified", "released"},
	}
	for _, tr := range allowed {
		if !TransitionAllowed(tr[0], tr[1]) {
			t.Errorf("%s → %s harus diizinkan", tr[0], tr[1])
		}
	}

	// Cabang fix loop.
	loop := [][2]string{
		{"reviewing", "changes-requested"},
		{"changes-requested", "running"},
		{"qa-testing", "defect-found"},
		{"defect-found", "running"},
	}
	for _, tr := range loop {
		if !TransitionAllowed(tr[0], tr[1]) {
			t.Errorf("%s → %s harus diizinkan (fix loop)", tr[0], tr[1])
		}
	}

	// Terminal berhenti dari status hidup mana pun.
	for _, terminal := range []string{"cancelled", "failed", "superseded", "blocked"} {
		if !TransitionAllowed("running", terminal) {
			t.Errorf("running → %s harus diizinkan", terminal)
		}
	}

	// Idempotent.
	if !TransitionAllowed("running", "running") {
		t.Error("running → running harus diizinkan (no-op)")
	}

	// Ditolak.
	denied := [][2]string{
		{"running", "merge-ready"}, // lompatan
		{"reserved", "implementation-complete"},
		{"running", "released"},  // release hanya pasca-merge
		{"released", "running"},  // terminal sukses
		{"cancelled", "running"}, // terminal berhenti
		{"implementation-complete", "defect-found"},
		{"reviewing", "running"}, // harus lewat changes-requested
	}
	for _, tr := range denied {
		if TransitionAllowed(tr[0], tr[1]) {
			t.Errorf("%s → %s harus ditolak", tr[0], tr[1])
		}
	}

	// released dari fase pasca-merge (Q12: release-reservation setelah merge).
	for _, from := range []string{"reserved", "running", "implementation-complete", "reviewing", "qa-testing", "ci-passed"} {
		if TransitionAllowed(from, "released") {
			t.Errorf("%s → released harus ditolak — belum pasca-merge", from)
		}
	}
	for _, from := range []string{"merge-ready", "merged", "documented", "staging-verified"} {
		if !TransitionAllowed(from, "released") {
			t.Errorf("%s → released harus diizinkan (pasca-merge)", from)
		}
	}
}

func TestCanWriteOwnerTable(t *testing.T) {
	// TL/SA menulis technical-ready.
	if !CanWrite("technical-lead-system-analyst", "technical-ready") {
		t.Error("TL/SA harus boleh menulis technical-ready")
	}
	if CanWrite("backend-engineer", "technical-ready") {
		t.Error("implementer tidak boleh menulis technical-ready")
	}

	// Implementer menulis implementation-complete.
	if !CanWrite("backend-engineer", "implementation-complete") {
		t.Error("implementer harus boleh menulis implementation-complete")
	}
	if CanWrite("code-reviewer", "implementation-complete") {
		t.Error("code-reviewer read-only tidak boleh menulis implementation-complete")
	}

	// Code Reviewer menulis changes-requested.
	if !CanWrite("code-reviewer", "changes-requested") {
		t.Error("code-reviewer harus boleh menulis changes-requested")
	}
	if CanWrite("backend-engineer", "changes-requested") {
		t.Error("implementer tidak boleh menulis changes-requested")
	}

	// QA menulis defect-found + qa-testing + merge-ready.
	if !CanWrite("qa-engineer", "defect-found") {
		t.Error("QA harus boleh menulis defect-found")
	}
	if !CanWrite("qa-engineer", "qa-testing") {
		t.Error("QA harus boleh menulis qa-testing")
	}
	if CanWrite("backend-engineer", "qa-testing") {
		t.Error("implementer tidak boleh menulis qa-testing (prinsip #6)")
	}

	// Status runner-owned tidak dapat ditulis agen (prinsip #4).
	for _, s := range []string{"reserved", "running", "reviewing", "ci-passed", "merged", "released"} {
		for _, by := range []string{"backend-engineer", "project-manager", "qa-engineer", "technical-lead-system-analyst"} {
			if CanWrite(by, s) {
				t.Errorf("%s tidak boleh menulis %s — runner-owned", by, s)
			}
		}
	}

	// PM menulis merge-ready + cancelled.
	if !CanWrite("project-manager", "merge-ready") {
		t.Error("PM harus boleh menulis merge-ready")
	}
	if !CanWrite("project-manager", "cancelled") {
		t.Error("PM harus boleh menulis cancelled")
	}

	// TW + DevOps.
	if !CanWrite("technical-writer", "documented") {
		t.Error("TW harus boleh menulis documented")
	}
	if !CanWrite("devops-release", "staging-verified") {
		t.Error("DevOps harus boleh menulis staging-verified")
	}
}

func TestFromReservationStatus(t *testing.T) {
	cases := map[string]string{
		"active":                 "reserved",
		"reserved-pending-merge": "merge-ready",
		"released":               "released",
		"cancelled":              "cancelled",
	}
	for res, want := range cases {
		if got := FromReservationStatus(res); got != want {
			t.Errorf("FromReservationStatus(%s) = %q, ingin %q", res, got, want)
		}
	}
	if got := FromReservationStatus("tidak-dikenal"); got != "" {
		t.Errorf("status tak dikenal harus '', dapat %q", got)
	}
}
