package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTaskTo menuju ke status target melalui alur nyata: reserve → launch →
// update-status. Menyiapkan task BE-201 pada status tertentu.
func setupTaskTo(t *testing.T, root, dir, repo, status string) string {
	t.Helper()
	taskPath := writeTask(t, dir, taskOpts{id: "BE-201", role: "backend-engineer"})
	if code := cmdReservePaths([]string{"-control", root, "-task", taskPath}); code != exitOK {
		t.Fatalf("reserve = exit %d", code)
	}
	if code := cmdLaunchTask([]string{"-control", root, "-task", taskPath, "-repo", repo, "-dry-run"}); code != exitOK {
		t.Fatalf("launch = exit %d", code)
	}
	// running → implementation-complete (implementer).
	if code := cmdUpdateStatus([]string{"-control", root,
		"-task", "BE-201", "-status", "implementation-complete", "-by", "backend-engineer"}); code != exitOK {
		t.Fatalf("update-status implementation-complete = exit %d", code)
	}
	if status == "implementation-complete" {
		return taskPath
	}
	// implementation-complete → reviewing (runner via collect-review approve).
	reviewApprove := writeReviewHandoff(t, dir, "BE-201", "approve")
	if code := cmdCollectReview([]string{"-control", root, "-handoff", reviewApprove}); code != exitOK {
		t.Fatalf("collect-review = exit %d", code)
	}
	return taskPath
}

// writeReviewHandoff membuat handoff Code Reviewer.
func writeReviewHandoff(t *testing.T, dir, taskID, decision string) string {
	t.Helper()
	y := `schema_version: "1.0"
task_id: ` + taskID + `
role: code-reviewer
status: implementation-complete
summary: uji review
findings:
  - severity: minor
    category: maintainability
    location:
      path: internal/payroll/x.go
      line: 1
    reason: uji
    recommended_action: perbaiki
decision: ` + decision + `
changed_files: []
tests:
  executed:
    - command: go vet
      result: passed
contract_deviations: []
`
	path := filepath.Join(dir, "review-"+taskID+".yaml")
	if err := os.WriteFile(path, []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeQAHandoff membuat handoff QA.
func writeQAHandoff(t *testing.T, dir, taskID, qaStatus string) string {
	t.Helper()
	y := `schema_version: "1.0"
task_id: ` + taskID + `
role: qa-engineer
status: ` + qaStatus + `
summary: uji QA
findings:
  - severity: blocker
    category: correctness
    location:
      path: internal/payroll/x.go
      line: 1
    reason: uji
    recommended_action: perbaiki
changed_files:
  - path: qa/evidence/test-evidence.md
    purpose: uji QA
tests:
  executed:
    - command: make test
      result: passed
contract_deviations: []
`
	path := filepath.Join(dir, "qa-"+taskID+".yaml")
	if err := os.WriteFile(path, []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// --- launch-review gate ---

func TestCmdLaunchReviewGate(t *testing.T) {
	root := statusFixture(t)
	dir := t.TempDir()
	repo := t.TempDir()
	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(t.TempDir(), "wt"))
	silence(t)
	initGitRepo(t, repo, "develop")

	// Status running → launch-review ditolak (harus implementation-complete).
	taskPath := writeTask(t, dir, taskOpts{id: "BE-201", role: "backend-engineer"})
	if code := cmdReservePaths([]string{"-control", root, "-task", taskPath}); code != exitOK {
		t.Fatal("reserve")
	}
	if code := cmdLaunchTask([]string{"-control", root, "-task", taskPath, "-repo", repo, "-dry-run"}); code != exitOK {
		t.Fatal("launch")
	}
	if code := cmdLaunchReview([]string{"-control", root, "-task", "BE-201"}); code != exitViolation {
		t.Errorf("launch-review saat running = exit %d, mau %d", code, exitViolation)
	}

	// implementation-complete → launch-review OK.
	if code := cmdUpdateStatus([]string{"-control", root,
		"-task", "BE-201", "-status", "implementation-complete", "-by", "backend-engineer"}); code != exitOK {
		t.Fatal("update implementation-complete")
	}
	if code := cmdLaunchReview([]string{"-control", root, "-task", "BE-201"}); code != exitOK {
		t.Errorf("launch-review saat implementation-complete = exit %d, mau %d", code, exitOK)
	}
}

// --- collect-review transitions ---

func TestCmdCollectReviewApprove(t *testing.T) {
	root := statusFixture(t)
	dir := t.TempDir()
	repo := t.TempDir()
	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(t.TempDir(), "wt"))
	silence(t)
	initGitRepo(t, repo, "develop")

	setupTaskTo(t, root, dir, repo, "implementation-complete")
	handoff := writeReviewHandoff(t, dir, "BE-201", "approve")
	if code := cmdCollectReview([]string{"-control", root, "-handoff", handoff}); code != exitOK {
		t.Fatalf("collect-review approve = exit %d, mau %d", code, exitOK)
	}
	body := readStatusFile(t, root, "BE-201")
	if !strings.Contains(body, "status: reviewing") {
		t.Errorf("approve harus menulis reviewing:\n%s", body)
	}
	if !strings.Contains(body, "by: code-reviewer") {
		t.Errorf("by harus code-reviewer:\n%s", body)
	}
}

func TestCmdCollectReviewRequestChanges(t *testing.T) {
	root := statusFixture(t)
	dir := t.TempDir()
	repo := t.TempDir()
	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(t.TempDir(), "wt"))
	silence(t)
	initGitRepo(t, repo, "develop")

	// request-changes dari reviewing (reviewer menolak implementer saat review).
	setupTaskTo(t, root, dir, repo, "reviewing")
	handoff := writeReviewHandoff(t, dir, "BE-201", "request-changes")
	if code := cmdCollectReview([]string{"-control", root, "-handoff", handoff}); code != exitOK {
		t.Fatalf("collect-review request-changes = exit %d, mau %d", code, exitOK)
	}
	body := readStatusFile(t, root, "BE-201")
	if !strings.Contains(body, "status: changes-requested") {
		t.Errorf("request-changes harus menulis changes-requested:\n%s", body)
	}
}

func TestCmdCollectReviewWrongRole(t *testing.T) {
	root := statusFixture(t)
	dir := t.TempDir()
	silence(t)

	// Handoff backend-engineer, bukan code-reviewer → ditolak.
	handoff := writeHandoff(t, dir, "BE-201")
	if code := cmdCollectReview([]string{"-control", root, "-handoff", handoff}); code != exitViolation {
		t.Errorf("collect-review role salah = exit %d, mau %d", code, exitViolation)
	}
}

// --- launch-qa gate ---

func TestCmdLaunchQAGate(t *testing.T) {
	root := statusFixture(t)
	dir := t.TempDir()
	repo := t.TempDir()
	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(t.TempDir(), "wt"))
	silence(t)
	initGitRepo(t, repo, "develop")

	// implementation-complete → launch-qa ditolak.
	setupTaskTo(t, root, dir, repo, "implementation-complete")
	if code := cmdLaunchQA([]string{"-control", root, "-task", "BE-201"}); code != exitViolation {
		t.Errorf("launch-qa saat implementation-complete = exit %d, mau %d", code, exitViolation)
	}

	// reviewing → launch-qa OK.
	reviewApprove := writeReviewHandoff(t, dir, "BE-201", "approve")
	if code := cmdCollectReview([]string{"-control", root, "-handoff", reviewApprove}); code != exitOK {
		t.Fatal("collect-review")
	}
	if code := cmdLaunchQA([]string{"-control", root, "-task", "BE-201"}); code != exitOK {
		t.Errorf("launch-qa saat reviewing = exit %d, mau %d", code, exitOK)
	}
}

// --- collect-qa transitions ---

func TestCmdCollectQAPass(t *testing.T) {
	root := statusFixture(t)
	dir := t.TempDir()
	repo := t.TempDir()
	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(t.TempDir(), "wt"))
	silence(t)
	initGitRepo(t, repo, "develop")

	setupTaskTo(t, root, dir, repo, "reviewing")
	if code := cmdLaunchQA([]string{"-control", root, "-task", "BE-201"}); code != exitOK {
		t.Fatalf("launch-qa = exit %d", code)
	}
	handoff := writeQAHandoff(t, dir, "BE-201", "implementation-complete")
	if code := cmdCollectQA([]string{"-control", root, "-handoff", handoff}); code != exitOK {
		t.Fatalf("collect-qa pass = exit %d, mau %d", code, exitOK)
	}
	body := readStatusFile(t, root, "BE-201")
	if !strings.Contains(body, "status: merge-ready") {
		t.Errorf("QA pass harus berakhir merge-ready:\n%s", body)
	}
}

func TestCmdCollectQADefect(t *testing.T) {
	root := statusFixture(t)
	dir := t.TempDir()
	repo := t.TempDir()
	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(t.TempDir(), "wt"))
	silence(t)
	initGitRepo(t, repo, "develop")

	setupTaskTo(t, root, dir, repo, "reviewing")
	if code := cmdLaunchQA([]string{"-control", root, "-task", "BE-201"}); code != exitOK {
		t.Fatalf("launch-qa = exit %d", code)
	}
	handoff := writeQAHandoff(t, dir, "BE-201", "defect-found")
	if code := cmdCollectQA([]string{"-control", root, "-handoff", handoff}); code != exitOK {
		t.Fatalf("collect-qa defect = exit %d, mau %d", code, exitOK)
	}
	// Fix loop ADR-012: defect → running.
	body := readStatusFile(t, root, "BE-201")
	if !strings.Contains(body, "status: running") {
		t.Errorf("QA defect harus berakhir running (fix loop):\n%s", body)
	}
}

func TestCmdCollectQAWrongRole(t *testing.T) {
	root := statusFixture(t)
	dir := t.TempDir()
	silence(t)

	handoff := writeHandoff(t, dir, "BE-201") // backend-engineer
	if code := cmdCollectQA([]string{"-control", root, "-handoff", handoff}); code != exitViolation {
		t.Errorf("collect-qa role salah = exit %d, mau %d", code, exitViolation)
	}
}
