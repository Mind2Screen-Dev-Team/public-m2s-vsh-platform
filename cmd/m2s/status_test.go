package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// statusFixture menyiapkan control repository + status store.
func statusFixture(t *testing.T) string {
	t.Helper()
	return controlFixture(t)
}

// readStatusFile membaca isi berkas status untuk verifikasi.
func readStatusFile(t *testing.T, root, taskID string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "control", "tasks", "status", taskID+".yaml"))
	if err != nil {
		t.Fatalf("baca status %s: %v", taskID, err)
	}
	return string(b)
}

// --- update-status ---

func TestCmdUpdateStatusValid(t *testing.T) {
	root := statusFixture(t)
	silence(t)

	// Berkas belum ada: TL/SA tulis status awal technical-ready.
	if code := cmdUpdateStatus([]string{"-control", root,
		"-task", "BE-201", "-status", "technical-ready", "-by", "technical-lead-system-analyst"}); code != exitOK {
		t.Fatalf("update-status awal = exit %d, mau %d", code, exitOK)
	}

	body := readStatusFile(t, root, "BE-201")
	for _, want := range []string{"schema_version", "task_id: BE-201", "status: technical-ready", "by: technical-lead-system-analyst", "updated_at"} {
		if !strings.Contains(body, want) {
			t.Errorf("status file kurang %q:\n%s", want, body)
		}
	}

	// No-op: transisi ke status yang sama diizinkan, tak mengubah.
	if code := cmdUpdateStatus([]string{"-control", root,
		"-task", "BE-201", "-status", "technical-ready", "-by", "technical-lead-system-analyst"}); code != exitOK {
		t.Errorf("no-op = exit %d, mau %d", code, exitOK)
	}
}

func TestCmdUpdateStatusRejectsInvalid(t *testing.T) {
	root := statusFixture(t)
	silence(t)

	// Status di luar enum taskStatus — schema menolak.
	if code := cmdUpdateStatus([]string{"-control", root,
		"-task", "BE-201", "-status", "bukan-status", "-by", "technical-lead-system-analyst"}); code == exitOK {
		t.Error("status di luar enum harus ditolak")
	}

	// by di luar enum role — schema menolak.
	if code := cmdUpdateStatus([]string{"-control", root,
		"-task", "BE-201", "-status", "running", "-by", "runner"}); code == exitOK {
		t.Error("by=runner harus ditolak — by adalah enum role")
	}
}

func TestCmdUpdateStatusRejectsOwner(t *testing.T) {
	root := statusFixture(t)
	silence(t)

	// Technical Writer tak boleh menulis defect-found (tabel owner).
	if code := cmdUpdateStatus([]string{"-control", root,
		"-task", "BE-201", "-status", "defect-found", "-by", "technical-writer"}); code != exitViolation {
		t.Errorf("owner salah = exit %d, mau %d", code, exitViolation)
	}

	// Implementer tak boleh menulis status runner-owned.
	if code := cmdUpdateStatus([]string{"-control", root,
		"-task", "BE-201", "-status", "running", "-by", "backend-engineer"}); code != exitViolation {
		t.Errorf("implementer tulis running = exit %d, mau %d", code, exitViolation)
	}
}

func TestCmdUpdateStatusRejectsTransition(t *testing.T) {
	root := statusFixture(t)
	silence(t)

	// Setup technical-ready.
	if code := cmdUpdateStatus([]string{"-control", root,
		"-task", "BE-201", "-status", "technical-ready", "-by", "technical-lead-system-analyst"}); code != exitOK {
		t.Fatal("setup technical-ready gagal")
	}

	// Lompatan technical-ready → merge-ready ditolak state machine.
	if code := cmdUpdateStatus([]string{"-control", root,
		"-task", "BE-201", "-status", "merge-ready", "-by", "backend-engineer"}); code != exitViolation {
		t.Errorf("lompatan transisi = exit %d, mau %d", code, exitViolation)
	}

	// Transisi lompatan + owner salah: technical-ready → implementation-complete.
	if code := cmdUpdateStatus([]string{"-control", root,
		"-task", "BE-201", "-status", "implementation-complete", "-by", "technical-lead-system-analyst"}); code != exitViolation {
		t.Errorf("lompatan = exit %d, mau %d", code, exitViolation)
	}
}

func TestCmdUpdateStatusMissingArgs(t *testing.T) {
	root := statusFixture(t)
	silence(t)

	if code := cmdUpdateStatus([]string{"-control", root, "-task", "BE-201", "-status", "running"}); code != exitError {
		t.Errorf("tanpa -by = exit %d, mau %d", code, exitError)
	}
}

// --- deterministic write: reserve-paths → reserved ---

func TestCmdReservePathsWritesStatus(t *testing.T) {
	root := statusFixture(t)
	dir := t.TempDir()
	silence(t)

	taskPath := writeTask(t, dir, taskOpts{id: "BE-201", role: "backend-engineer"})
	if code := cmdReservePaths([]string{"-control", root, "-task", taskPath}); code != exitOK {
		t.Fatalf("reserve-paths = exit %d, mau %d", code, exitOK)
	}

	body := readStatusFile(t, root, "BE-201")
	if !strings.Contains(body, "status: reserved") {
		t.Errorf("reserve-paths harus menulis status reserved:\n%s", body)
	}
	if !strings.Contains(body, "by: backend-engineer") {
		t.Errorf("by harus role pemilik task:\n%s", body)
	}
}

// --- deterministic write: release-reservation → cancelled ---

func TestCmdReleaseReservationWritesStatus(t *testing.T) {
	root := statusFixture(t)
	dir := t.TempDir()
	silence(t)

	taskPath := writeTask(t, dir, taskOpts{id: "BE-201", role: "backend-engineer"})
	if code := cmdReservePaths([]string{"-control", root, "-task", taskPath}); code != exitOK {
		t.Fatalf("reserve-paths = exit %d", code)
	}
	if code := cmdReleaseReservation([]string{"-control", root,
		"-task-id", "BE-201", "-by", "runner", "-cancel"}); code != exitOK {
		t.Fatalf("release-reservation cancel = exit %d", code)
	}
	body := readStatusFile(t, root, "BE-201")
	if !strings.Contains(body, "status: cancelled") {
		t.Errorf("release -cancel harus menulis status cancelled (sinkron reservasi):\n%s", body)
	}
}

// --- deterministic write: collect-result → status handoff ---

func TestCmdCollectResultWritesHandoffStatus(t *testing.T) {
	root := statusFixture(t)
	dir := t.TempDir()
	repo := t.TempDir()
	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(t.TempDir(), "wt"))
	silence(t)

	initGitRepo(t, repo, "develop")
	taskPath := writeTask(t, dir, taskOpts{id: "BE-201", role: "backend-engineer"})
	if code := cmdReservePaths([]string{"-control", root, "-task", taskPath}); code != exitOK {
		t.Fatalf("reserve-paths = exit %d", code)
	}
	// Alur nyata: launch-task tulis running sebelum agent jalan.
	if code := cmdLaunchTask([]string{"-control", root, "-task", taskPath, "-repo", repo, "-dry-run"}); code != exitOK {
		t.Fatalf("launch-task = exit %d, mau %d", code, exitOK)
	}

	handoff := writeHandoff(t, dir, "BE-201")
	if code := cmdCollectResult([]string{"-control", root, "-handoff", handoff}); code != exitOK {
		t.Fatalf("collect-result = exit %d, mau %d", code, exitOK)
	}

	body := readStatusFile(t, root, "BE-201")
	if !strings.Contains(body, "status: implementation-complete") {
		t.Errorf("collect-result harus menulis status handoff:\n%s", body)
	}
	if !strings.Contains(body, "by: backend-engineer") {
		t.Errorf("by harus role dari handoff:\n%s", body)
	}
}
