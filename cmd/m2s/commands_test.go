package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// controlFixture menyiapkan control repository sementara: schemas/ disalin dari
// repo nyata, control/reservations/ kosong.
//
// Menyalin schema, bukan menirunya, agar test menguji schema yang sebenarnya
// dipakai produksi.
func controlFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	srcSchemas, err := filepath.Abs(filepath.Join("..", "..", "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	dstSchemas := filepath.Join(root, "schemas")
	if err := os.MkdirAll(dstSchemas, 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(srcSchemas)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(srcSchemas, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dstSchemas, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "control", "reservations"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

type taskOpts struct {
	id       string
	repo     string
	role     string
	taskType string
	base     string
	allowed  []string
	shared   string // path shared file; kosong berarti tidak ada
	platform string // execution.platform; kosong berarti field tidak ditulis
	status   string // task.status; kosong berarti technical-ready
	scaffold string // execution.scaffold; kosong berarti field tidak ditulis
	contract string // satu contract_ids; kosong berarti field tidak ditulis
}

func writeTask(t *testing.T, dir string, o taskOpts) string {
	t.Helper()
	if o.repo == "" {
		o.repo = "proyek-backend"
	}
	if o.role == "" {
		o.role = "backend-engineer"
	}
	if o.taskType == "" {
		o.taskType = "backend-implementation"
	}
	if o.base == "" {
		o.base = "develop"
	}
	if o.status == "" {
		o.status = "technical-ready"
	}
	if len(o.allowed) == 0 {
		o.allowed = []string{"internal/payroll/**"}
	}

	var b strings.Builder
	b.WriteString("schema_version: \"1.0\"\ntask:\n")
	b.WriteString("  id: " + o.id + "\n  title: uji\n")
	b.WriteString("  type: " + o.taskType + "\n  project: uji\n  status: " + o.status + "\n")
	if o.contract != "" {
		b.WriteString("  contract_ids: [" + o.contract + "]\n")
	}
	b.WriteString("ownership:\n  role: " + o.role + "\n  repository: " + o.repo + "\n")
	b.WriteString("  base_branch: " + o.base + "\n")
	b.WriteString("  branch: agent/" + o.id + "-uji\n")
	b.WriteString("execution:\n  isolation: worktree\n")
	if o.platform != "" {
		b.WriteString("  platform: " + o.platform + "\n")
	}
	if o.scaffold != "" {
		b.WriteString("  scaffold: " + o.scaffold + "\n")
	}
	b.WriteString("  max_turns: 30\n  timeout_minutes: 45\n")
	b.WriteString("paths:\n  allowed:\n")
	for _, p := range o.allowed {
		// Path dikutip: nilai yang diawali '*' adalah alias YAML dan akan
		// gagal di-parse. Pola seperti "**" karena itu wajib berkutip.
		b.WriteString("    - \"" + p + "\"\n")
	}
	b.WriteString("  forbidden:\n    - \"go.mod\"\n    - \".claude/**\"\n    - \".task/**\"\n")
	if o.shared != "" {
		b.WriteString("shared_file_ownership:\n  - path: \"" + o.shared + "\"\n")
		b.WriteString("    owner_task_id: " + o.id + "\n    owner_role: " + o.role + "\n")
	}
	b.WriteString("acceptance_criteria:\n  - uji\nquality_gates:\n  - make test\n")
	b.WriteString("outputs:\n  - code\nstop_conditions:\n  - contract change required\n")

	path := filepath.Join(dir, o.id+".yaml")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// initGitRepo menyiapkan repository git nyata dengan satu commit pada base.
//
// Preflight H-05 memeriksa `git show-ref refs/heads/<base>` sebelum worktree
// dibuat, jadi test launch yang berharap lolos preflight wajib repo dengan
// branch base yang benar-benar ada. Repo non-git kini gagal di preflight,
// bukan di `git worktree add`.
func initGitRepo(t *testing.T, dir, base string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-b", base},
		{"-c", "user.email=uji@m2s.test", "-c", "user.name=uji", "commit",
			"--allow-empty", "-m", "awal"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func writeHandoff(t *testing.T, dir, taskID string) string {
	t.Helper()
	y := `schema_version: "1.0"
task_id: ` + taskID + `
role: backend-engineer
status: implementation-complete
summary: uji
changed_files:
  - path: internal/payroll/x.go
    purpose: uji
tests:
  executed:
    - command: make test
      result: passed
contract_deviations: []
`
	path := filepath.Join(dir, "handoff-"+taskID+".yaml")
	if err := os.WriteFile(path, []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// silence membungkam stdout/stderr selama subcommand berjalan, agar keluaran
// test tidak tertimbun pesan runner.
func silence(t *testing.T) {
	t.Helper()
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = devnull, devnull
	t.Cleanup(func() {
		os.Stdout, os.Stderr = origOut, origErr
		devnull.Close()
	})
}

// --- validate-task ---

func TestCmdValidateTask(t *testing.T) {
	root := controlFixture(t)
	dir := t.TempDir()
	silence(t)

	valid := writeTask(t, dir, taskOpts{id: "BE-101"})
	if code := cmdValidateTask([]string{"-control", root, "-task", valid}); code != exitOK {
		t.Errorf("contract valid = exit %d, mau %d", code, exitOK)
	}

	// base_branch main hanya dapat ditolak runner, bukan schema (ADR-001 #2).
	mainBase := writeTask(t, dir, taskOpts{id: "BE-102", base: "main"})
	if code := cmdValidateTask([]string{"-control", root, "-task", mainBase}); code != exitViolation {
		t.Errorf("base_branch main = exit %d, mau %d (ADR-001 #2)", code, exitViolation)
	}

	// Pelanggaran schema.
	bad := filepath.Join(dir, "bad.yaml")
	os.WriteFile(bad, []byte("schema_version: \"1.0\"\n"), 0o644)
	if code := cmdValidateTask([]string{"-control", root, "-task", bad}); code != exitViolation {
		t.Errorf("contract tidak lengkap = exit %d, mau %d", code, exitViolation)
	}

	// Berkas tidak ada = runner gagal, BUKAN kontrak ditolak.
	if code := cmdValidateTask([]string{"-control", root, "-task", "/tidak/ada.yaml"}); code != exitError {
		t.Errorf("berkas hilang = exit %d, mau %d", code, exitError)
	}

	// Flag wajib.
	if code := cmdValidateTask([]string{"-control", root}); code != exitError {
		t.Errorf("tanpa -task = exit %d, mau %d", code, exitError)
	}
}

// --- reserve-paths ---

func TestCmdReservePaths(t *testing.T) {
	root := controlFixture(t)
	dir := t.TempDir()
	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(t.TempDir(), "wt"))
	silence(t)

	first := writeTask(t, dir, taskOpts{id: "BE-101", allowed: []string{"internal/payroll/**"}})
	if code := cmdReservePaths([]string{"-control", root, "-task", first}); code != exitOK {
		t.Fatalf("reservasi pertama = exit %d", code)
	}

	// Idempotent.
	if code := cmdReservePaths([]string{"-control", root, "-task", first}); code != exitOK {
		t.Errorf("pengulangan = exit %d, harus idempotent", code)
	}

	// Subtree bertabrakan — inti R-03.
	sub := writeTask(t, dir, taskOpts{id: "BE-102", allowed: []string{"internal/payroll/period/**"}})
	if code := cmdReservePaths([]string{"-control", root, "-task", sub}); code != exitViolation {
		t.Errorf("subtree bertabrakan = exit %d, mau %d", code, exitViolation)
	}

	// Parent juga bertabrakan.
	parent := writeTask(t, dir, taskOpts{id: "BE-103", allowed: []string{"internal/**"}})
	if code := cmdReservePaths([]string{"-control", root, "-task", parent}); code != exitViolation {
		t.Errorf("parent bertabrakan = exit %d, mau %d", code, exitViolation)
	}

	// Repository berbeda tidak bertabrakan meski path sama.
	other := writeTask(t, dir, taskOpts{
		id: "FE-101", repo: "proyek-frontend", role: "frontend-engineer",
		taskType: "frontend-implementation", allowed: []string{"internal/payroll/**"},
	})
	if code := cmdReservePaths([]string{"-control", root, "-task", other}); code != exitOK {
		t.Errorf("repo berbeda = exit %d, mau %d", code, exitOK)
	}

	// Subtree terpisah pada repo yang sama boleh — dasar repo fullstack.
	disjoint := writeTask(t, dir, taskOpts{id: "BE-104", allowed: []string{"internal/attendance/**"}})
	if code := cmdReservePaths([]string{"-control", root, "-task", disjoint}); code != exitOK {
		t.Errorf("path terpisah pada repo sama = exit %d, mau %d", code, exitOK)
	}
}

// TestCmdReservePathsSharedFile menutup R-04 pada tingkat CLI.
func TestCmdReservePathsSharedFile(t *testing.T) {
	root := controlFixture(t)
	dir := t.TempDir()
	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(t.TempDir(), "wt"))
	silence(t)

	a := writeTask(t, dir, taskOpts{
		id: "BE-201", allowed: []string{"internal/a/**"}, shared: "internal/shared/enum.go",
	})
	if code := cmdReservePaths([]string{"-control", root, "-task", a}); code != exitOK {
		t.Fatalf("reservasi pertama = exit %d", code)
	}

	// Path terpisah, tetapi mengklaim shared file yang sama dengan owner beda.
	b := writeTask(t, dir, taskOpts{
		id: "BE-202", allowed: []string{"internal/b/**"}, shared: "internal/shared/enum.go",
	})
	if code := cmdReservePaths([]string{"-control", root, "-task", b}); code != exitViolation {
		t.Errorf("shared file owner berbeda = exit %d, mau %d (§29.6)", code, exitViolation)
	}
}

// --- launch-task ---

func TestCmdLaunchTaskRequiresReservation(t *testing.T) {
	root := controlFixture(t)
	dir := t.TempDir()
	repo := t.TempDir()
	silence(t)

	task := writeTask(t, dir, taskOpts{id: "BE-101"})
	// Urutan Q13: reservasi mendahului worktree.
	code := cmdLaunchTask([]string{"-control", root, "-task", task, "-repo", repo, "-dry-run"})
	if code != exitViolation {
		t.Errorf("tanpa reservasi = exit %d, mau %d (Q13)", code, exitViolation)
	}
}

// TestCmdLaunchTaskRejectsWorktreeInsideRepo menegakkan Q8/A-01.
//
// Penjagaan ini pernah gagal-BUKA karena asimetri resolusi symlink: parent yang
// sudah ada teresolusi (`/var` → `/private/var`) sementara target yang belum ada
// tidak, sehingga keduanya dianggap berbeda pohon.
func TestCmdLaunchTaskRejectsWorktreeInsideRepo(t *testing.T) {
	root := controlFixture(t)
	dir := t.TempDir()
	repo := t.TempDir()
	silence(t)

	// Worktree diarahkan ke dalam repository, tetapi BUKAN ke
	// .claude/worktrees — pola itu sudah ditolak schema sebagai anti-pattern
	// §30. Memakai subfolder biasa memastikan yang teruji di sini adalah
	// penjagaan isInside pada runner, bukan pola schema.
	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(repo, "wt"))

	task := writeTask(t, dir, taskOpts{id: "BE-101"})
	if code := cmdReservePaths([]string{"-control", root, "-task", task}); code != exitOK {
		t.Fatalf("reservasi = exit %d", code)
	}
	code := cmdLaunchTask([]string{"-control", root, "-task", task, "-repo", repo, "-dry-run"})
	if code != exitViolation {
		t.Errorf("worktree di dalam repo = exit %d, mau %d (Q8, A-01)", code, exitViolation)
	}
}

// TestCmdLaunchTaskRejectsPlatformMismatch menegakkan ADR-006 #3.
//
// Prasyarat platform diperiksa sebelum worktree dibuat, sehingga penolakan
// tidak meninggalkan worktree yatim. Yang diuji di sini adalah keduanya:
// exit code, dan tidak adanya sisa worktree.
func TestCmdLaunchTaskRejectsPlatformMismatch(t *testing.T) {
	// Nilai yang pasti tidak sama dengan runtime.GOOS mesin penguji.
	other := "linux"
	if runtime.GOOS == "linux" {
		other = "darwin"
	}

	root := controlFixture(t)
	dir := t.TempDir()
	repo := t.TempDir()
	wtRoot := t.TempDir()
	silence(t)
	t.Setenv("M2S_WORKTREE_ROOT", wtRoot)

	task := writeTask(t, dir, taskOpts{id: "IOS-101", platform: other})
	if code := cmdReservePaths([]string{"-control", root, "-task", task}); code != exitOK {
		t.Fatalf("reservasi = exit %d", code)
	}

	code := cmdLaunchTask([]string{"-control", root, "-task", task, "-repo", repo, "-dry-run"})
	if code != exitViolation {
		t.Errorf("platform %s pada runner %s = exit %d, mau %d (ADR-006 #3)",
			other, runtime.GOOS, code, exitViolation)
	}

	// Penolakan mendahului `git worktree add`; tidak boleh ada sisa.
	entries, err := os.ReadDir(wtRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("penolakan platform meninggalkan %d entri pada worktree root — "+
			"pemeriksaan harus mendahului pembuatan worktree", len(entries))
	}
}

// TestCmdLaunchTaskAcceptsMatchingPlatform memastikan pemeriksaan tidak
// fail-closed terhadap nilai yang benar. Penjagaan yang menolak segalanya sama
// tidak bergunanya dengan yang tidak menolak apa pun.
func TestCmdLaunchTaskAcceptsMatchingPlatform(t *testing.T) {
	for _, platform := range []string{"", "any", runtime.GOOS} {
		name := platform
		if name == "" {
			name = "tanpa-field"
		}
		t.Run(name, func(t *testing.T) {
			// runtime.GOOS di luar enum schema (misal windows) tidak dapat
			// diuji: dokumennya akan ditolak validasi lebih dulu.
			if platform == runtime.GOOS && platform != "darwin" && platform != "linux" {
				t.Skipf("runtime.GOOS %q di luar enum schema", runtime.GOOS)
			}

			root := controlFixture(t)
			dir := t.TempDir()
			repo := t.TempDir()
			initGitRepo(t, repo, "develop")
			silence(t)
			t.Setenv("M2S_WORKTREE_ROOT", t.TempDir())

			task := writeTask(t, dir, taskOpts{id: "BE-101", platform: platform})
			if code := cmdReservePaths([]string{"-control", root, "-task", task}); code != exitOK {
				t.Fatalf("reservasi = exit %d", code)
			}

			// repo bukan repository git, sehingga `git worktree add` gagal dan
			// mengembalikan exitError. Yang penting: BUKAN exitViolation, yang
			// berarti pemeriksaan platform sudah dilewati.
			if code := cmdLaunchTask([]string{"-control", root, "-task", task, "-repo", repo, "-dry-run"}); code == exitViolation {
				t.Errorf("platform %q pada runner %s ditolak, seharusnya diterima", platform, runtime.GOOS)
			}
		})
	}
}

// TestSchemaRejectsClaudeWorktreePath menegakkan lapisan pertahanan kedua:
// schema menolak pola `.claude/worktrees` yang merupakan anti-pattern §30,
// bahkan sebelum runner memeriksa apakah worktree berada di dalam repo.
//
// Kedua lapisan diperlukan karena masing-masing menangkap hal berbeda: schema
// menangkap pola yang salah di mana pun lokasinya, runner menangkap lokasi yang
// salah apa pun polanya.
func TestSchemaRejectsClaudeWorktreePath(t *testing.T) {
	root := controlFixture(t)
	dir := t.TempDir()
	silence(t)

	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(t.TempDir(), ".claude", "worktrees"))
	task := writeTask(t, dir, taskOpts{id: "BE-101"})

	if code := cmdReservePaths([]string{"-control", root, "-task", task}); code != exitViolation {
		t.Errorf(".claude/worktrees = exit %d, mau %d (anti-pattern §30)", code, exitViolation)
	}
}

// --- collect-result ---

func TestCmdCollectResult(t *testing.T) {
	root := controlFixture(t)
	dir := t.TempDir()
	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(t.TempDir(), "wt"))
	silence(t)

	task := writeTask(t, dir, taskOpts{id: "BE-101"})
	if code := cmdReservePaths([]string{"-control", root, "-task", task}); code != exitOK {
		t.Fatalf("reservasi = exit %d", code)
	}

	// Alur nyata: launch-task menulis running sebelum agent jalan. collect-result
	// kini menulis status dari handoff (ADR-011), yang menuntut transisi
	// reserved → running → implementation-complete.
	repo := t.TempDir()
	initGitRepo(t, repo, "develop")
	if code := cmdLaunchTask([]string{"-control", root, "-task", task, "-repo", repo, "-dry-run"}); code != exitOK {
		t.Fatalf("launch = exit %d", code)
	}

	handoff := writeHandoff(t, dir, "BE-101")

	// Tanpa -pr: hanya memvalidasi, tidak menyentuh reservasi.
	if code := cmdCollectResult([]string{"-control", root, "-handoff", handoff}); code != exitOK {
		t.Errorf("handoff valid = exit %d", code)
	}

	// Dengan -pr: berpindah ke reserved-pending-merge, TIDAK dilepas (Q12).
	pr := "https://github.com/x/y/pull/7"
	if code := cmdCollectResult([]string{"-control", root, "-handoff", handoff, "-pr", pr}); code != exitOK {
		t.Fatalf("collect dengan -pr = exit %d", code)
	}

	// Bukti A-05 tertutup: path masih tertahan setelah PR dibuat.
	other := writeTask(t, dir, taskOpts{id: "BE-102", allowed: []string{"internal/payroll/period/**"}})
	if code := cmdReservePaths([]string{"-control", root, "-task", other}); code != exitViolation {
		t.Errorf("path harus masih tertahan saat pending-merge = exit %d, mau %d (A-05, Q12)",
			code, exitViolation)
	}

	// Handoff tidak valid ditolak.
	bad := filepath.Join(dir, "bad-handoff.yaml")
	os.WriteFile(bad, []byte("schema_version: \"1.0\"\ntask_id: BE-101\n"), 0o644)
	if code := cmdCollectResult([]string{"-control", root, "-handoff", bad}); code != exitViolation {
		t.Errorf("handoff tidak lengkap = exit %d, mau %d", code, exitViolation)
	}
}

// --- release-reservation ---

func TestCmdReleaseReservation(t *testing.T) {
	root := controlFixture(t)
	dir := t.TempDir()
	t.Setenv("M2S_WORKTREE_ROOT", filepath.Join(t.TempDir(), "wt"))
	silence(t)

	task := writeTask(t, dir, taskOpts{id: "BE-101"})
	if code := cmdReservePaths([]string{"-control", root, "-task", task}); code != exitOK {
		t.Fatalf("reservasi = exit %d", code)
	}

	repo := t.TempDir()
	initGitRepo(t, repo, "develop")
	if code := cmdLaunchTask([]string{"-control", root, "-task", task, "-repo", repo, "-dry-run"}); code != exitOK {
		t.Fatalf("launch = exit %d", code)
	}

	// Worker tidak boleh melepas (§30).
	if code := cmdReleaseReservation([]string{"-control", root, "-task-id", "BE-101", "-by", "worker"}); code != exitViolation {
		t.Errorf("-by worker = exit %d, mau %d (§30)", code, exitViolation)
	}

	// active langsung ke released ditolak; wajib lewat pending-merge (Q12).
	if code := cmdReleaseReservation([]string{"-control", root, "-task-id", "BE-101", "-by", "runner"}); code != exitViolation {
		t.Errorf("active → released = exit %d, mau %d (Q12)", code, exitViolation)
	}

	// Lewat jalur yang benar.
	handoff := writeHandoff(t, dir, "BE-101")
	if code := cmdCollectResult([]string{"-control", root, "-handoff", handoff, "-pr", "https://github.com/x/y/pull/7"}); code != exitOK {
		t.Fatalf("collect = exit %d", code)
	}
	if code := cmdReleaseReservation([]string{"-control", root, "-task-id", "BE-101", "-by", "runner"}); code != exitOK {
		t.Errorf("release setelah pending-merge = exit %d, mau %d", code, exitOK)
	}

	// Idempotent.
	if code := cmdReleaseReservation([]string{"-control", root, "-task-id", "BE-101", "-by", "runner"}); code != exitOK {
		t.Errorf("pengulangan release = exit %d, harus idempotent", code)
	}

	// Setelah dilepas, task lain boleh memakai path itu.
	other := writeTask(t, dir, taskOpts{id: "BE-102", allowed: []string{"internal/payroll/**"}})
	if code := cmdReservePaths([]string{"-control", root, "-task", other}); code != exitOK {
		t.Errorf("path harus bebas setelah released = exit %d", code)
	}

	// Task yang tidak ada.
	if code := cmdReleaseReservation([]string{"-control", root, "-task-id", "BE-999", "-by", "runner"}); code != exitError {
		t.Errorf("task tidak ada = exit %d, mau %d", code, exitError)
	}
}

// --- Phase 8 hardening ---

// TestCmdValidateTaskRejectsScaffoldForbidden menegakkan H-03.
//
// Contract Phase 7 melarang go.mod dan src/app/layout.tsx padahal scaffolding
// wajib membuatnya. Ditolak sebelum agent mulai, bukan setelah CI gagal.
func TestCmdValidateTaskRejectsScaffoldForbidden(t *testing.T) {
	root := controlFixture(t)
	dir := t.TempDir()
	silence(t)

	// writeTask selalu menulis go.mod pada forbidden, sehingga stack go
	// otomatis melanggar.
	goTask := writeTask(t, dir, taskOpts{id: "BE-201", scaffold: "go"})
	if code := cmdValidateTask([]string{"-control", root, "-task", goTask}); code != exitViolation {
		t.Errorf("scaffold go dengan go.mod forbidden = exit %d, mau %d (H-03)", code, exitViolation)
	}

	nextTask := writeTask(t, dir, taskOpts{
		id: "FE-201", repo: "proyek-frontend", role: "frontend-engineer",
		taskType: "frontend-implementation", scaffold: "nextjs",
		allowed: []string{"src/components/**"},
	})
	if code := cmdValidateTask([]string{"-control", root, "-task", nextTask}); code != exitViolation {
		t.Errorf("scaffold nextjs tanpa layout.tsx = exit %d, mau %d (H-03)", code, exitViolation)
	}
}

// TestCmdValidateTaskScaffoldCoveredByGlob adalah kontrol negatif H-03.
//
// Penjaga yang menolak segalanya sama tidak bergunanya dengan yang tidak
// menolak apa pun. Pola glob yang mencakup berkas scaffolding harus diterima,
// dan task tanpa field scaffold tidak boleh diperiksa sama sekali.
func TestCmdValidateTaskScaffoldCoveredByGlob(t *testing.T) {
	root := controlFixture(t)
	dir := t.TempDir()
	silence(t)

	// src/app/** mencakup layout.tsx dan globals.css.
	covered := writeTask(t, dir, taskOpts{
		id: "FE-202", repo: "proyek-frontend", role: "frontend-engineer",
		taskType: "frontend-implementation", scaffold: "nextjs",
		allowed: []string{"src/app/**"},
	})
	if code := cmdValidateTask([]string{"-control", root, "-task", covered}); code != exitOK {
		t.Errorf("scaffold nextjs dengan src/app/** = exit %d, mau %d", code, exitOK)
	}

	// Tanpa field scaffold: task pada repo yang sudah berdiri boleh melarang
	// go.mod. Ini yang membuat H-03 opt-in, bukan universal.
	optOut := writeTask(t, dir, taskOpts{id: "BE-202"})
	if code := cmdValidateTask([]string{"-control", root, "-task", optOut}); code != exitOK {
		t.Errorf("task tanpa scaffold = exit %d, mau %d — H-03 harus opt-in", code, exitOK)
	}
}

// TestCmdValidateTaskRejectsMissingContractID menegakkan H-05/H-06.
func TestCmdValidateTaskRejectsMissingContractID(t *testing.T) {
	root := controlFixture(t)
	dir := t.TempDir()
	silence(t)

	task := writeTask(t, dir, taskOpts{id: "BE-203", contract: "CONTRACT-999"})
	if code := cmdValidateTask([]string{"-control", root, "-task", task}); code != exitViolation {
		t.Errorf("contract_ids menunjuk berkas hilang = exit %d, mau %d (H-06)", code, exitViolation)
	}

	// Kontrol negatif: contract yang ada harus lolos.
	specs := filepath.Join(root, "control", "tasks", "specifications")
	if err := os.MkdirAll(specs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specs, "CONTRACT-102.yaml"), []byte("# uji\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok := writeTask(t, dir, taskOpts{id: "BE-204", contract: "CONTRACT-102"})
	if code := cmdValidateTask([]string{"-control", root, "-task", ok}); code != exitOK {
		t.Errorf("contract_ids yang ada = exit %d, mau %d", code, exitOK)
	}
}

// TestCmdLaunchTaskRejectsStatusNotTechnicalReady menegakkan H-07.
//
// Gate TL/SA berada di launch, bukan di validate-handoff: handoff berjalan pada
// SubagentStop, yaitu setelah kerja habis, dan payload-nya tidak memuat task
// spec.
func TestCmdLaunchTaskRejectsStatusNotTechnicalReady(t *testing.T) {
	root := controlFixture(t)
	dir := t.TempDir()
	repo := t.TempDir()
	wtRoot := t.TempDir()
	silence(t)
	t.Setenv("M2S_WORKTREE_ROOT", wtRoot)

	task := writeTask(t, dir, taskOpts{id: "BE-205", status: "draft"})
	if code := cmdReservePaths([]string{"-control", root, "-task", task}); code != exitOK {
		t.Fatalf("reservasi = exit %d", code)
	}

	code := cmdLaunchTask([]string{"-control", root, "-task", task, "-repo", repo, "-dry-run"})
	if code != exitViolation {
		t.Errorf("status draft = exit %d, mau %d (H-07)", code, exitViolation)
	}

	// Penolakan mendahului pembuatan worktree.
	entries, err := os.ReadDir(wtRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("penolakan H-07 meninggalkan %d entri pada worktree root", len(entries))
	}
}

// TestCmdLaunchTaskRejectsBaseBranchMissing menegakkan H-05.
//
// `git worktree add ... <base>` gagal dengan exitError bila base tidak ada,
// yang terbaca sebagai runner rusak. H-05 mengubahnya menjadi kontrak ditolak
// (exitViolation) dan memindahkannya ke sebelum worktree disentuh.
func TestCmdLaunchTaskRejectsBaseBranchMissing(t *testing.T) {
	root := controlFixture(t)
	dir := t.TempDir()
	repo := t.TempDir()
	initGitRepo(t, repo, "main")
	silence(t)
	t.Setenv("M2S_WORKTREE_ROOT", t.TempDir())

	// Repository nyata pada main; `develop` tidak ada — H-05 harus menolak.
	task := writeTask(t, dir, taskOpts{id: "BE-206", base: "develop"})
	if code := cmdReservePaths([]string{"-control", root, "-task", task}); code != exitOK {
		t.Fatalf("reservasi = exit %d", code)
	}

	code := cmdLaunchTask([]string{"-control", root, "-task", task, "-repo", repo, "-dry-run"})
	if code != exitViolation {
		t.Errorf("base_branch develop tidak ada = exit %d, mau %d (H-05)", code, exitViolation)
	}
}
