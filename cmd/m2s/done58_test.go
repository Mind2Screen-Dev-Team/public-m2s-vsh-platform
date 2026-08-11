package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRepo membuat repository git nyata dengan branch develop dan berkas awal.
func gitRepo(t *testing.T, dir string, subdirs ...string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, sd := range subdirs {
		if err := os.MkdirAll(filepath.Join(dir, sd), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, sd, "seed.txt"), []byte("awal\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	steps := [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "add", "-A"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init"},
		{"branch", "develop"},
	}
	for _, args := range steps {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestDoneCriteriaPhase58 menegakkan kriteria Done §58:
//
//	"dua task pada repository berbeda berjalan tanpa shared cwd"
//
// Ini adalah gerbang penutup fase, karena itu diuji sebagai perilaku nyata
// dengan dua repository git sungguhan — bukan hanya memeriksa nilai kembalian.
func TestDoneCriteriaPhase58(t *testing.T) {
	root := controlFixture(t)
	specs := t.TempDir()
	base := t.TempDir()
	silence(t)

	repoBE := gitRepo(t, filepath.Join(base, "repo-be"), "internal/payroll")
	repoFE := gitRepo(t, filepath.Join(base, "repo-fe"), "src/features")

	wtRoot := filepath.Join(base, "worktrees")
	t.Setenv("M2S_WORKTREE_ROOT", wtRoot)

	be := writeTask(t, specs, taskOpts{
		id: "BE-101", repo: "repo-be", role: "backend-engineer",
		taskType: "backend-implementation", allowed: []string{"internal/payroll/**"},
	})
	fe := writeTask(t, specs, taskOpts{
		id: "FE-101", repo: "repo-fe", role: "frontend-engineer",
		taskType: "frontend-implementation", allowed: []string{"src/features/**"},
	})

	// Kedua task melalui alur penuh: validasi → reservasi → launch.
	for _, tc := range []struct {
		name, task, repo string
	}{
		{"BE-101", be, repoBE},
		{"FE-101", fe, repoFE},
	} {
		if code := cmdValidateTask([]string{"-control", root, "-task", tc.task}); code != exitOK {
			t.Fatalf("%s validate = exit %d", tc.name, code)
		}
		if code := cmdReservePaths([]string{"-control", root, "-task", tc.task}); code != exitOK {
			t.Fatalf("%s reserve = exit %d", tc.name, code)
		}
		code := cmdLaunchTask([]string{"-control", root, "-task", tc.task, "-repo", tc.repo, "-dry-run"})
		if code != exitOK {
			t.Fatalf("%s launch = exit %d", tc.name, code)
		}
	}

	wtBE := filepath.Join(wtRoot, "repo-be", "BE-101")
	wtFE := filepath.Join(wtRoot, "repo-fe", "FE-101")

	// Kriteria inti: cwd tidak dibagi.
	if wtBE == wtFE {
		t.Fatal("kedua task memakai cwd yang sama — melanggar Done §58")
	}
	for _, wt := range []string{wtBE, wtFE} {
		if fi, err := os.Stat(wt); err != nil || !fi.IsDir() {
			t.Fatalf("worktree %s tidak terbentuk: %v", wt, err)
		}
	}

	// Worktree wajib di luar repository masing-masing (Q8, A-01).
	for _, pair := range [][2]string{{wtBE, repoBE}, {wtFE, repoFE}} {
		inside, err := isInside(pair[0], pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if inside {
			t.Errorf("worktree %s berada di dalam repo %s", pair[0], pair[1])
		}
	}

	// Branch terpisah per task.
	if b := gitOut(t, wtBE, "branch", "--show-current"); b != "agent/BE-101-uji" {
		t.Errorf("branch BE = %q", b)
	}
	if b := gitOut(t, wtFE, "branch", "--show-current"); b != "agent/FE-101-uji" {
		t.Errorf("branch FE = %q", b)
	}

	// Snapshot contract ada di masing-masing worktree, read-only (Q15).
	for _, wt := range []string{wtBE, wtFE} {
		snap := filepath.Join(wt, ".task", "contract.json")
		fi, err := os.Stat(snap)
		if err != nil {
			t.Fatalf("snapshot %s tidak ada: %v", snap, err)
		}
		if perm := fi.Mode().Perm(); perm != 0o444 {
			t.Errorf("%s berizin %o, mau 444 (Q15)", snap, perm)
		}
	}

	// Isolasi nyata: menulis di satu worktree tidak menyentuh yang lain
	// maupun repo induk.
	if err := os.WriteFile(filepath.Join(wtBE, "internal/payroll/close.go"), []byte("package payroll\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, wtBE, "-c", "user.name=be", "-c", "user.email=be@t", "add", "-A")
	gitOut(t, wtBE, "-c", "user.name=be", "-c", "user.email=be@t", "commit", "-q", "-m", "BE-101")

	if st := gitOut(t, wtFE, "status", "--short"); st != "" {
		t.Errorf("worktree FE terpengaruh penulisan BE:\n%s", st)
	}

	// .task/ wajib terabaikan git. Snapshot berada di dalam worktree, sehingga
	// `git add -A` milik agent akan ikut men-stage-nya — bertentangan dengan
	// .task/** yang justru ada pada forbidden_paths (Q15).
	for _, wt := range []string{wtBE, wtFE} {
		if st := gitOut(t, wt, "status", "--short", "--untracked-files=all"); strings.Contains(st, ".task") {
			t.Errorf("%s: .task/ terlihat git dan dapat ikut ter-commit agent:\n%s", wt, st)
		}
	}
	if st := gitOut(t, repoBE, "status", "--short"); st != "" {
		t.Errorf("repo induk BE terpengaruh:\n%s", st)
	}
	if head := gitOut(t, repoBE, "rev-parse", "--abbrev-ref", "HEAD"); head != "main" {
		t.Errorf("HEAD repo induk berpindah ke %q — runner tidak boleh memindahkannya", head)
	}
}

// TestNonOverlapAcceptance menguji bagian §67 yang berada dalam lingkup Phase 1:
// reservasi menolak task baru pada path yang sudah dipegang, dan mengizinkan
// path yang terpisah pada repository yang sama.
func TestNonOverlapAcceptance(t *testing.T) {
	root := controlFixture(t)
	specs := t.TempDir()
	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(t.TempDir(), "wt"))
	silence(t)

	first := writeTask(t, specs, taskOpts{
		id: "BE-101", repo: "repo-be", allowed: []string{"backend/payroll/**"},
	})
	if code := cmdReservePaths([]string{"-control", root, "-task", first}); code != exitOK {
		t.Fatalf("BE-101 = exit %d", code)
	}

	cases := []struct {
		name    string
		opts    taskOpts
		wantErr bool
	}{
		{
			name:    "path identik ditolak",
			opts:    taskOpts{id: "BE-199", repo: "repo-be", allowed: []string{"backend/payroll/**"}},
			wantErr: true,
		},
		{
			name:    "subtree ditolak",
			opts:    taskOpts{id: "BE-198", repo: "repo-be", allowed: []string{"backend/payroll/period/**"}},
			wantErr: true,
		},
		{
			name:    "path terpisah pada repo sama diizinkan",
			opts:    taskOpts{id: "BE-102", repo: "repo-be", allowed: []string{"backend/attendance/**"}},
			wantErr: false,
		},
		{
			name: "repo berbeda dengan path serupa diizinkan",
			opts: taskOpts{
				id: "FE-101", repo: "repo-fe", role: "frontend-engineer",
				taskType: "frontend-implementation", allowed: []string{"frontend/payroll/**"},
			},
			wantErr: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			task := writeTask(t, specs, c.opts)
			code := cmdReservePaths([]string{"-control", root, "-task", task})
			if c.wantErr && code != exitViolation {
				t.Errorf("harus ditolak, dapat exit %d", code)
			}
			if !c.wantErr && code != exitOK {
				t.Errorf("harus diizinkan, dapat exit %d", code)
			}
		})
	}
}

// TestForbiddenPathStaysForbidden memastikan go.mod tetap terlarang meski
// berada di bawah pohon yang diizinkan — §67 menuntut BE-101 dan BE-102 tidak
// mengubah go.mod.
func TestForbiddenPathStaysForbidden(t *testing.T) {
	root := controlFixture(t)
	specs := t.TempDir()
	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(t.TempDir(), "wt"))
	silence(t)

	// allowed mencakup seluruh repo, tetapi go.mod ada pada forbidden.
	task := writeTask(t, specs, taskOpts{id: "BE-101", allowed: []string{"**"}})
	if code := cmdReservePaths([]string{"-control", root, "-task", task}); code != exitOK {
		t.Fatalf("reservasi = exit %d", code)
	}

	res := filepath.Join(root, "control", "reservations", "BE-101.yaml")
	b, err := os.ReadFile(res)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)
	for _, want := range []string{"go.mod", ".claude/**", ".task/**"} {
		if !strings.Contains(content, want) {
			t.Errorf("reservasi harus meneruskan forbidden %q dari task contract", want)
		}
	}
}
