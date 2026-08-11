package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mind2Screen-Dev-Team/public-m2s-vsh-platform/internal/contract"
)

func newRegistry(t *testing.T) *Registry {
	t.Helper()
	schemaDir, err := filepath.Abs(filepath.Join("..", "..", "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	v, err := contract.NewValidator(schemaDir)
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	r, err := Open(t.TempDir(), v)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return r
}

// reservationDoc membuat dokumen reservasi valid minimal.
func reservationDoc(taskID string, paths []string) map[string]any {
	anyPaths := make([]any, len(paths))
	for i, p := range paths {
		anyPaths[i] = p
	}
	return map[string]any{
		"schema_version":  "1.0",
		"task_id":         taskID,
		"repository":      "m2s-vsh-project-backend",
		"branch":          "agent/" + taskID + "-slug",
		"worktree":        "/Users/x/.m2s/worktrees/m2s-vsh-project-backend/" + taskID,
		"allowed_paths":   anyPaths,
		"reserved_paths":  anyPaths,
		"forbidden_paths": []any{".claude/**", ".task/**"},
		"status":          StatusActive,
		"owner_role":      "backend-engineer",
		"created_at":      time.Now().Format(time.RFC3339),
	}
}

func TestPutGetList(t *testing.T) {
	r := newRegistry(t)

	if err := r.Put(reservationDoc("BE-101", []string{"internal/payroll/**"})); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := r.Put(reservationDoc("BE-102", []string{"internal/attendance/**"})); err != nil {
		t.Fatalf("Put: %v", err)
	}

	all, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List = %d reservasi, mau 2", len(all))
	}
	if all[0].TaskID() != "BE-101" {
		t.Errorf("List harus terurut, dapat %s lebih dulu", all[0].TaskID())
	}

	got, err := r.Get("BE-101")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status() != StatusActive || got.OwnerRole() != "backend-engineer" {
		t.Errorf("Get salah: status=%s role=%s", got.Status(), got.OwnerRole())
	}
}

// TestPutRejectsInvalid memastikan registry tidak menerima dokumen yang
// melanggar schema — registry rusak lebih berbahaya daripada operasi gagal.
func TestPutRejectsInvalid(t *testing.T) {
	r := newRegistry(t)

	doc := reservationDoc("BE-101", []string{"internal/payroll/**"})
	doc["worktree"] = "/Users/x/repo/.claude/worktrees/BE-101" // dilarang (A-01, Q8)
	if err := r.Put(doc); err == nil {
		t.Error("worktree di dalam .claude/** harus ditolak")
	}

	doc2 := reservationDoc("BE-102", []string{"internal/payroll/**"})
	doc2["owner_role"] = "code-reviewer" // read-only, bukan writerRole (A-03)
	if err := r.Put(doc2); err == nil {
		t.Error("code-reviewer tidak boleh memegang reservasi")
	}
}

// TestCheckConflictsOverlap menutup matriks §4.2 pada tingkat registry.
func TestCheckConflictsOverlap(t *testing.T) {
	r := newRegistry(t)
	if err := r.Put(reservationDoc("BE-101", []string{"internal/payroll/**"})); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		paths     []string
		wantCount int
	}{
		{"subtree dari reservasi aktif", []string{"internal/payroll/period/**"}, 1},
		{"exact file di dalam reservasi", []string{"internal/payroll/enum.go"}, 1},
		{"parent dari reservasi aktif", []string{"internal/**"}, 1},
		{"subtree terpisah", []string{"internal/attendance/**"}, 0},
		{"case berbeda tetap konflik", []string{"Internal/Payroll/**"}, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := r.CheckConflicts("BE-999", "m2s-vsh-project-backend", c.paths, nil)
			if err != nil {
				t.Fatalf("CheckConflicts: %v", err)
			}
			if len(got) != c.wantCount {
				t.Errorf("dapat %d konflik, mau %d: %v", len(got), c.wantCount, got)
			}
		})
	}
}

// TestCheckConflictsIgnoresSelfAndOtherRepo memastikan operasi idempotent dan
// tidak lintas repository.
func TestCheckConflictsIgnoresSelfAndOtherRepo(t *testing.T) {
	r := newRegistry(t)
	if err := r.Put(reservationDoc("BE-101", []string{"internal/payroll/**"})); err != nil {
		t.Fatal(err)
	}

	// Reservasi milik sendiri tidak dihitung konflik.
	got, err := r.CheckConflicts("BE-101", "m2s-vsh-project-backend", []string{"internal/payroll/**"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("reservasi sendiri tidak boleh konflik, dapat %v", got)
	}

	// Path sama pada repository berbeda bukan konflik.
	got, err = r.CheckConflicts("FE-101", "m2s-vsh-project-frontend", []string{"internal/payroll/**"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("repository berbeda tidak boleh konflik, dapat %v", got)
	}
}

// TestCheckConflictsReleasedDoesNotBlock menegakkan Q12: hanya active dan
// reserved-pending-merge yang menahan path.
func TestCheckConflictsReleasedDoesNotBlock(t *testing.T) {
	r := newRegistry(t)
	doc := reservationDoc("BE-101", []string{"internal/payroll/**"})
	if err := r.Put(doc); err != nil {
		t.Fatal(err)
	}

	// Masih menahan saat pending-merge.
	if err := r.Transition("BE-101", StatusReservedPendingMerge, map[string]any{
		"pr_url": "https://github.com/Mind2Screen-Dev-Team/m2s-vsh-project-backend/pull/7",
	}); err != nil {
		t.Fatalf("Transition ke pending-merge: %v", err)
	}
	got, _ := r.CheckConflicts("BE-999", "m2s-vsh-project-backend", []string{"internal/payroll/**"}, nil)
	if len(got) != 1 {
		t.Errorf("reserved-pending-merge harus tetap menahan path (Q12), dapat %d konflik", len(got))
	}

	// Tidak lagi menahan setelah released.
	if err := r.Transition("BE-101", StatusReleased, map[string]any{"released_by": "runner"}); err != nil {
		t.Fatalf("Transition ke released: %v", err)
	}
	got, _ = r.CheckConflicts("BE-999", "m2s-vsh-project-backend", []string{"internal/payroll/**"}, nil)
	if len(got) != 0 {
		t.Errorf("released tidak boleh menahan path, dapat %d konflik", len(got))
	}
}

// TestSharedFileOwnerConflict menutup matriks §4.6 dan R-04.
func TestSharedFileOwnerConflict(t *testing.T) {
	r := newRegistry(t)

	doc := reservationDoc("BE-101", []string{"internal/payroll/**"})
	doc["shared_file_ownership"] = []any{
		map[string]any{
			"path":          "internal/shared/enum.go",
			"owner_task_id": "BE-101",
			"owner_role":    "backend-engineer",
		},
	}
	if err := r.Put(doc); err != nil {
		t.Fatal(err)
	}

	// Task lain mengklaim owner berbeda atas shared file yang sama.
	conflicts, err := r.CheckConflicts("BE-102", "m2s-vsh-project-backend",
		[]string{"internal/attendance/**"}, // reserved_paths TIDAK beririsan
		map[string]string{"internal/shared/enum.go": "BE-102"})
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("shared file dengan owner berbeda harus konflik meski path terpisah, dapat %d", len(conflicts))
	}
	if !strings.Contains(conflicts[0].Reason, "single owner") {
		t.Errorf("alasan harus menyebut single owner, dapat %q", conflicts[0].Reason)
	}

	// Owner yang sama bukan konflik.
	conflicts, _ = r.CheckConflicts("BE-102", "m2s-vsh-project-backend",
		[]string{"internal/attendance/**"},
		map[string]string{"internal/shared/enum.go": "BE-101"})
	if len(conflicts) != 0 {
		t.Errorf("owner sama tidak boleh konflik, dapat %v", conflicts)
	}
}

// TestTransitionRules menegakkan urutan status Q12 dan larangan §30.
func TestTransitionRules(t *testing.T) {
	prURL := "https://github.com/Mind2Screen-Dev-Team/m2s-vsh-project-backend/pull/7"

	t.Run("active langsung ke released ditolak", func(t *testing.T) {
		r := newRegistry(t)
		r.Put(reservationDoc("BE-101", []string{"a/**"}))
		err := r.Transition("BE-101", StatusReleased, map[string]any{"released_by": "runner"})
		if err == nil {
			t.Error("active → released harus ditolak; wajib lewat reserved-pending-merge (Q12)")
		}
	})

	t.Run("worker tidak boleh melepas", func(t *testing.T) {
		r := newRegistry(t)
		r.Put(reservationDoc("BE-101", []string{"a/**"}))
		r.Transition("BE-101", StatusReservedPendingMerge, map[string]any{"pr_url": prURL})
		err := r.Transition("BE-101", StatusReleased, map[string]any{"released_by": "worker"})
		if err == nil {
			t.Error("released_by=worker harus ditolak (§30)")
		}
	})

	t.Run("pending-merge tanpa pr_url ditolak schema", func(t *testing.T) {
		r := newRegistry(t)
		r.Put(reservationDoc("BE-101", []string{"a/**"}))
		err := r.Transition("BE-101", StatusReservedPendingMerge, nil)
		if err == nil {
			t.Error("pending-merge tanpa pr_url harus ditolak")
		}
	})

	t.Run("released terisi released_at otomatis", func(t *testing.T) {
		r := newRegistry(t)
		r.Put(reservationDoc("BE-101", []string{"a/**"}))
		r.Transition("BE-101", StatusReservedPendingMerge, map[string]any{"pr_url": prURL})
		if err := r.Transition("BE-101", StatusReleased, map[string]any{"released_by": "human"}); err != nil {
			t.Fatalf("Transition: %v", err)
		}
		res, _ := r.Get("BE-101")
		if res.str("released_at") == "" {
			t.Error("released_at harus terisi otomatis")
		}
	})

	t.Run("transisi dari terminal ditolak", func(t *testing.T) {
		r := newRegistry(t)
		r.Put(reservationDoc("BE-101", []string{"a/**"}))
		r.Transition("BE-101", StatusCancelled, map[string]any{"released_by": "runner"})
		if err := r.Transition("BE-101", StatusActive, nil); err == nil {
			t.Error("transisi dari cancelled harus ditolak")
		}
	})
}

// --- locking ---

func TestLockExclusive(t *testing.T) {
	r := newRegistry(t)

	l1, err := r.Acquire("uji", time.Second)
	if err != nil {
		t.Fatalf("Acquire pertama: %v", err)
	}

	// Percobaan kedua harus gagal selama kunci pertama masih dipegang.
	start := time.Now()
	if _, err := r.Acquire("uji-kedua", 200*time.Millisecond); err == nil {
		t.Error("Acquire kedua harus gagal selama kunci dipegang")
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Errorf("Acquire harus menunggu sampai timeout, hanya %v", elapsed)
	}

	if err := l1.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	l2, err := r.Acquire("setelah-release", time.Second)
	if err != nil {
		t.Fatalf("Acquire setelah Release harus berhasil: %v", err)
	}
	l2.Release()
}

func TestLockReleaseIdempotent(t *testing.T) {
	r := newRegistry(t)
	l, err := r.Acquire("uji", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("Release pertama: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Errorf("Release kedua harus aman, dapat: %v", err)
	}
}

// TestLockBreaksStale memastikan kunci milik proses mati tidak memblokir
// registry selamanya.
func TestLockBreaksStale(t *testing.T) {
	r := newRegistry(t)
	lockPath := filepath.Join(r.dir, LockName)

	// PID yang mustahil hidup.
	stale := "pid: 999999\nhost: mati\nacquired_at: " +
		time.Now().Add(-time.Hour).Format(time.RFC3339) + "\noperation: yatim\n"
	if err := os.WriteFile(lockPath, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	l, err := r.Acquire("setelah-yatim", time.Second)
	if err != nil {
		t.Fatalf("kunci yatim harus dapat direbut: %v", err)
	}
	l.Release()
}

// TestLockSerializesConcurrentReservation adalah inti alasan locking ada:
// tanpa kunci, dua runner dapat memeriksa konflik terhadap keadaan yang sama
// lalu sama-sama menulis reservasi yang beririsan.
func TestLockSerializesConcurrentReservation(t *testing.T) {
	r := newRegistry(t)

	const goroutines = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded := 0
	rejected := 0

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()

			lock, err := r.Acquire("reserve", 5*time.Second)
			if err != nil {
				t.Errorf("goroutine %d gagal mengambil kunci: %v", n, err)
				return
			}
			defer lock.Release()

			taskID := fmt.Sprintf("BE-%d", 101+n)
			paths := []string{"internal/payroll/**"} // seluruhnya beririsan

			conflicts, err := r.CheckConflicts(taskID, "m2s-vsh-project-backend", paths, nil)
			if err != nil {
				t.Errorf("goroutine %d CheckConflicts: %v", n, err)
				return
			}

			mu.Lock()
			defer mu.Unlock()
			if len(conflicts) > 0 {
				rejected++
				return
			}
			if err := r.Put(reservationDoc(taskID, paths)); err != nil {
				t.Errorf("goroutine %d Put: %v", n, err)
				return
			}
			succeeded++
		}(i)
	}
	wg.Wait()

	if succeeded != 1 {
		t.Errorf("tepat satu reservasi harus berhasil, dapat %d berhasil dan %d ditolak",
			succeeded, rejected)
	}
	if rejected != goroutines-1 {
		t.Errorf("sisanya harus ditolak karena konflik, dapat %d", rejected)
	}
}
