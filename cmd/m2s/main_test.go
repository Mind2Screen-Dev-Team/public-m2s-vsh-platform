package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStr(t *testing.T) {
	doc := map[string]any{
		"task": map[string]any{"id": "BE-101", "n": 5},
		"top":  "nilai",
	}
	cases := map[string]string{
		"top":            "nilai",
		"task.id":        "BE-101",
		"task.n":         "", // bukan string
		"task.tidak.ada": "",
		"hilang":         "",
		"top.lanjut":     "", // menembus non-map
	}
	for path, want := range cases {
		if got := str(doc, path); got != want {
			t.Errorf("str(%q) = %q, mau %q", path, got, want)
		}
	}
}

func TestStrSlice(t *testing.T) {
	doc := map[string]any{
		"paths": map[string]any{
			"allowed": []any{"a/**", "b/**", 42},
			"kosong":  []any{},
		},
	}
	got := strSlice(doc, "paths.allowed")
	if len(got) != 2 || got[0] != "a/**" || got[1] != "b/**" {
		t.Errorf("strSlice = %v, mau [a/** b/**] — nilai non-string harus dilewati", got)
	}
	if got := strSlice(doc, "paths.kosong"); len(got) != 0 {
		t.Errorf("array kosong = %v", got)
	}
	if got := strSlice(doc, "tidak.ada"); got != nil {
		t.Errorf("path hilang = %v, mau nil", got)
	}
}

func TestToAny(t *testing.T) {
	got := toAny([]string{"x", "y"})
	if len(got) != 2 || got[0] != "x" {
		t.Errorf("toAny = %v", got)
	}
	if got := toAny(nil); len(got) != 0 {
		t.Errorf("toAny(nil) = %v", got)
	}
}

// TestIsInside menegakkan penjagaan Q8/A-01: worktree harus di luar repository.
//
// Jaminan ini tidak dapat diungkapkan schema karena schema tidak mengetahui
// lokasi repository.
func TestIsInside(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "worktrees", "BE-101")

	cases := []struct {
		name   string
		target string
		want   bool
	}{
		{"di dalam repo", filepath.Join(repo, "sub", "wt"), true},
		{"langsung di bawah repo", filepath.Join(repo, "wt"), true},
		{"di dalam .claude repo", filepath.Join(repo, ".claude", "worktrees", "BE-101"), true},
		{"di luar repo", outside, false},
		{"sibling berprefiks sama", base + "/repo-lain/wt", false},
		{"repo itu sendiri", repo, false},
		{"induk repo", base, false},
	}
	for _, c := range cases {
		got, err := isInside(c.target, repo)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: isInside(%q, repo) = %v, mau %v", c.name, c.target, got, c.want)
		}
	}
}

// TestIsInsideResolvesSymlink memastikan symlink tidak dapat dipakai
// menyelundupkan worktree ke dalam repository.
func TestIsInsideResolvesSymlink(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "nyata"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "tautan")
	if err := os.Symlink(filepath.Join(repo, "nyata"), link); err != nil {
		t.Skipf("symlink tidak didukung: %v", err)
	}

	inside, err := isInside(link, repo)
	if err != nil {
		t.Fatal(err)
	}
	if !inside {
		t.Error("symlink yang menunjuk ke dalam repo harus terdeteksi di dalam")
	}
}

func TestControlRoot(t *testing.T) {
	dir := t.TempDir()

	// Flag menang atas env.
	t.Setenv("M2S_CONTROL_ROOT", filepath.Join(dir, "dari-env"))
	got, err := controlRoot(filepath.Join(dir, "dari-flag"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "dari-flag" {
		t.Errorf("flag harus menang, dapat %q", got)
	}

	// Tanpa flag, env dipakai.
	got, err = controlRoot("")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "dari-env" {
		t.Errorf("env harus dipakai, dapat %q", got)
	}

	// Hasil selalu absolut.
	if !filepath.IsAbs(got) {
		t.Errorf("controlRoot harus absolut, dapat %q", got)
	}
}

// TestWorktreeRootDefault menegakkan Q8: default di luar repo, dan BUKAN
// .claude/worktrees seperti contoh §30.
func TestWorktreeRootDefault(t *testing.T) {
	t.Setenv("M2S_WORKTREE_ROOT", "")
	got, err := worktreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "worktrees" || filepath.Base(filepath.Dir(got)) != ".m2s" {
		t.Errorf("default harus $HOME/.m2s/worktrees, dapat %q", got)
	}

	custom := t.TempDir()
	t.Setenv("M2S_WORKTREE_ROOT", custom)
	got, err = worktreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != custom {
		t.Errorf("override = %q, mau %q", got, custom)
	}
}
