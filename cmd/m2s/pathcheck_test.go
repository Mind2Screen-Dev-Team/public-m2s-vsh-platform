package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeContractJSON menulis snapshot .task/contract.json ke <worktree>/.task/,
// meniru materialisasi runner (Q15). Mengembalikan path contract.
func writeContractJSON(t *testing.T, worktree string, allowed, forbidden []string) string {
	t.Helper()
	dir := filepath.Join(worktree, ".task")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b []byte
	b = append(b, []byte(`{"schema_version":"1.0","paths":{"allowed":[`)...)
	for i, p := range allowed {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, []byte(`"`+p+`"`)...)
	}
	b = append(b, []byte(`],"forbidden":[`)...)
	for i, p := range forbidden {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, []byte(`"`+p+`"`)...)
	}
	b = append(b, []byte(`]}}`)...)

	path := filepath.Join(dir, "contract.json")
	if err := os.WriteFile(path, b, 0o444); err != nil {
		t.Fatal(err)
	}
	return path
}

// --- check-path ---

func TestCmdCheckPath(t *testing.T) {
	silence(t)
	wt := t.TempDir()
	contract := writeContractJSON(t,
		wt,
		[]string{"internal/payroll/**"},
		[]string{"go.mod", ".claude/**", ".task/**"},
	)

	cases := []struct {
		name string
		path string
		want int
	}{
		{"di dalam allowed", "internal/payroll/period.go", exitOK},
		{"allowed nested", "internal/payroll/deep/x.go", exitOK},
		{"di luar allowed", "internal/auth/token.go", exitViolation},
		{"forbidden mengalahkan", "go.mod", exitViolation},
		{"forbidden .claude", ".claude/agents/x.md", exitViolation},
		{"forbidden .task", ".task/contract.json", exitViolation},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code := cmdCheckPath([]string{"-contract", contract, "-worktree", wt, "-path", c.path})
			if code != c.want {
				t.Errorf("%s: exit %d, mau %d", c.path, code, c.want)
			}
		})
	}
}

// TestCmdCheckPathRejectsEscape menegakkan R-15: path yang teresolusi keluar
// worktree ditolak, baik lewat `..` maupun symlink.
func TestCmdCheckPathRejectsEscape(t *testing.T) {
	silence(t)
	wt := t.TempDir()
	outside := t.TempDir()
	contract := writeContractJSON(t, wt, []string{"**"}, []string{".task/**"})

	// `..` traversal keluar worktree — meski allowed "**", batas worktree
	// diperiksa lebih dulu.
	if code := cmdCheckPath([]string{"-contract", contract, "-worktree", wt, "-path", "../keluar.go"}); code != exitViolation {
		t.Errorf(".. traversal = exit %d, mau %d (R-15)", code, exitViolation)
	}

	// Symlink di dalam worktree menunjuk keluar. isInside me-resolve symlink,
	// sehingga tujuan sebenarnya yang dinilai.
	link := filepath.Join(wt, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink tidak didukung: %v", err)
	}
	if code := cmdCheckPath([]string{"-contract", contract, "-worktree", wt, "-path", "link/rahasia.go"}); code != exitViolation {
		t.Errorf("symlink keluar = exit %d, mau %d (R-15)", code, exitViolation)
	}
}

// TestCmdCheckPathDerivesWorktree memastikan worktree dapat diturunkan dari
// lokasi contract bila -worktree tidak diberikan (contract di <wt>/.task/).
func TestCmdCheckPathDerivesWorktree(t *testing.T) {
	silence(t)
	wt := t.TempDir()
	contract := writeContractJSON(t, wt, []string{"internal/**"}, []string{".task/**"})

	if code := cmdCheckPath([]string{"-contract", contract, "-path", "internal/x.go"}); code != exitOK {
		t.Errorf("turunan worktree, allowed = exit %d, mau %d", code, exitOK)
	}
	if code := cmdCheckPath([]string{"-contract", contract, "-path", "docs/x.md"}); code != exitViolation {
		t.Errorf("turunan worktree, di luar allowed = exit %d, mau %d", code, exitViolation)
	}
}

func TestCmdCheckPathFlags(t *testing.T) {
	silence(t)
	if code := cmdCheckPath([]string{"-path", "x.go"}); code != exitError {
		t.Errorf("tanpa -contract = exit %d, mau %d", code, exitError)
	}
	if code := cmdCheckPath([]string{"-contract", "/tidak/ada.json", "-path", "x.go"}); code != exitError {
		t.Errorf("contract hilang = exit %d, mau %d", code, exitError)
	}
}

// --- validate-changed-paths ---

func TestCmdValidateChangedPaths(t *testing.T) {
	silence(t)
	dir := t.TempDir()
	contract := writeContractJSON(t, dir, // dir apa saja; tidak dipakai sebagai worktree di sini
		[]string{"internal/payroll/**", "docs/user/**"},
		[]string{"go.mod", ".claude/**", ".task/**"},
	)

	write := func(name string, lines string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(lines), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	clean := write("clean.txt", "internal/payroll/x.go\ndocs/user/guide.md\n")
	if code := cmdValidateChangedPaths([]string{"-contract", contract, "-changed", clean}); code != exitOK {
		t.Errorf("PR bersih = exit %d, mau %d", code, exitOK)
	}

	dirty := write("dirty.txt", "internal/payroll/x.go\ngo.mod\n")
	if code := cmdValidateChangedPaths([]string{"-contract", contract, "-changed", dirty}); code != exitViolation {
		t.Errorf("PR sentuh go.mod = exit %d, mau %d (R-07)", code, exitViolation)
	}

	claude := write("claude.txt", ".claude/settings.json\n")
	if code := cmdValidateChangedPaths([]string{"-contract", contract, "-changed", claude}); code != exitViolation {
		t.Errorf("PR sentuh .claude = exit %d, mau %d (R-12)", code, exitViolation)
	}

	outside := write("outside.txt", "internal/auth/token.go\n")
	if code := cmdValidateChangedPaths([]string{"-contract", contract, "-changed", outside}); code != exitViolation {
		t.Errorf("PR di luar allowed = exit %d, mau %d", code, exitViolation)
	}

	empty := write("empty.txt", "\n  \n")
	if code := cmdValidateChangedPaths([]string{"-contract", contract, "-changed", empty}); code != exitOK {
		t.Errorf("PR tanpa perubahan = exit %d, mau %d", code, exitOK)
	}
}

// TestCmdValidateChangedPathsReadsYAML memastikan CI dapat memberi contract
// sumber YAML, bukan hanya snapshot JSON.
func TestCmdValidateChangedPathsReadsYAML(t *testing.T) {
	silence(t)
	root := controlFixture(t)
	dir := t.TempDir()
	yamlContract := writeTask(t, dir, taskOpts{id: "BE-101", allowed: []string{"internal/payroll/**"}})
	_ = root

	changed := filepath.Join(dir, "changed.txt")
	if err := os.WriteFile(changed, []byte("internal/payroll/x.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdValidateChangedPaths([]string{"-contract", yamlContract, "-changed", changed}); code != exitOK {
		t.Errorf("contract YAML, path allowed = exit %d, mau %d", code, exitOK)
	}

	bad := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(bad, []byte("go.mod\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdValidateChangedPaths([]string{"-contract", yamlContract, "-changed", bad}); code != exitViolation {
		t.Errorf("contract YAML, go.mod = exit %d, mau %d", code, exitViolation)
	}
}

// --- role ⇄ platform (ADR-006, ditutup §59) ---

// TestLaunchTaskIosImpliesDarwin membuktikan pertanyaan terbuka ADR-006
// tertutup: role ios-developer menurunkan platform darwin secara otomatis.
func TestLaunchTaskIosImpliesDarwin(t *testing.T) {
	silence(t)
	root := controlFixture(t)
	dir := t.TempDir()
	repo := t.TempDir()

	iosTask := func(id, platform string) string {
		return writeTask(t, dir, taskOpts{
			id: id, role: "ios-developer", taskType: "mobile-implementation",
			repo: "proyek-mobile", allowed: []string{"ios/**"}, platform: platform,
		})
	}

	// ios-developer dengan platform linux: bertentangan, ditolak apa pun GOOS.
	t.Run("linux ditolak", func(t *testing.T) {
		t.Setenv("M2S_WORKTREE_ROOT", t.TempDir())
		task := iosTask("IOS-101", "linux")
		if code := cmdReservePaths([]string{"-control", root, "-task", task}); code != exitOK {
			t.Fatalf("reservasi = exit %d", code)
		}
		if code := cmdLaunchTask([]string{"-control", root, "-task", task, "-repo", repo, "-dry-run"}); code != exitViolation {
			t.Errorf("ios+linux = exit %d, mau %d (ADR-006 §59)", code, exitViolation)
		}
	})

	// ios-developer tanpa field platform: diturunkan darwin. Pada runner non-darwin
	// karena itu ditolak sebagai mismatch — bukti penurunan benar-benar terjadi.
	if runtime.GOOS != "darwin" {
		t.Run("tanpa field diturunkan darwin lalu mismatch di non-darwin", func(t *testing.T) {
			t.Setenv("M2S_WORKTREE_ROOT", t.TempDir())
			task := iosTask("IOS-102", "")
			if code := cmdReservePaths([]string{"-control", root, "-task", task}); code != exitOK {
				t.Fatalf("reservasi = exit %d", code)
			}
			if code := cmdLaunchTask([]string{"-control", root, "-task", task, "-repo", repo, "-dry-run"}); code != exitViolation {
				t.Errorf("ios tanpa platform pada %s = exit %d, mau %d — darwin harus diturunkan",
					runtime.GOOS, code, exitViolation)
			}
		})
	}
}
