package main

// Subcommand penegak batas path Phase 3 (§59).
//
// Keduanya membungkus internal/pathmatch — sumber kebenaran overlap yang sama
// dipakai reserve-paths, sudah teruji 24 kasus (R-03). Hook PreToolUse dan CI
// TIDAK menirukan logika glob; mereka memanggil binary ini agar tidak ada dua
// implementasi yang dapat menyimpang diam-diam.
//
//   check-path             memutuskan satu operasi tulis Edit/Write (dipakai hook)
//   validate-changed-paths memeriksa daftar changed file sebuah PR (dipakai CI)

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mind2Screen-Dev-Team/public-m2s-vsh-platform/internal/pathmatch"
	"gopkg.in/yaml.v3"
)

// loadContractPaths membaca contract dan mengembalikan allowed serta forbidden.
//
// Menerima dua bentuk tanpa memuat schema:
//   - .task/contract.json — snapshot read-only yang SUDAH divalidasi runner saat
//     materialisasi (Q15); dibaca hook check-path
//   - control/tasks/specifications/*.yaml — contract sumber; dibaca CI
//     validate-changed-paths
//
// Karena YAML adalah superset JSON, satu decoder yaml.v3 melayani keduanya.
// Pemeriksaan schema penuh bukan tugas subcommand ini: check-path membaca
// snapshot yang sudah sah, dan CI menjalankan validate-task terpisah.
func loadContractPaths(contractPath string) (allowed, forbidden []string, err error) {
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		return nil, nil, fmt.Errorf("membaca %s: %w", contractPath, err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("mem-parse %s: %w", contractPath, err)
	}
	if doc == nil {
		return nil, nil, fmt.Errorf("%s kosong", contractPath)
	}
	// Round-trip lewat JSON agar kunci non-string YAML tidak lolos ke strSlice
	// sebagai map[any]any yang tak terbaca.
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, nil, fmt.Errorf("normalisasi %s: %w", contractPath, err)
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, nil, fmt.Errorf("normalisasi %s: %w", contractPath, err)
	}
	allowed = strSlice(doc, "paths.allowed")
	forbidden = strSlice(doc, "paths.forbidden")
	if len(allowed) == 0 {
		return nil, nil, fmt.Errorf("%s tidak memuat paths.allowed", contractPath)
	}
	return allowed, forbidden, nil
}

// --- check-path ---

// cmdCheckPath memutuskan apakah satu operasi tulis diizinkan. Dipanggil hook
// validate-path-scope.sh untuk setiap Edit/Write.
//
// Tiga penolakan, seluruhnya exit 2 (kontrak ditolak, konvensi hook fail-closed):
//   - path teresolusi berada DI LUAR worktree (R-15: symlink/.. keluar repo)
//   - path tertutup forbidden (forbidden mengalahkan allowed, matriks §4.8)
//   - path tidak tercakup allowed
func cmdCheckPath(args []string) int {
	fs := newFlagSet("check-path")
	contractPath := fs.String("contract", "", "path .task/contract.json")
	target := fs.String("path", "", "path operasi tulis (absolut atau relatif worktree)")
	worktreeFlag := fs.String("worktree", "", "akar worktree; default diturunkan dari lokasi contract")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *contractPath == "" || *target == "" {
		return fail(exitError, "-contract dan -path wajib diisi")
	}

	allowed, forbidden, err := loadContractPaths(*contractPath)
	if err != nil {
		return fail(exitError, "%v", err)
	}

	// Worktree default: contract berada di <worktree>/.task/contract.json,
	// sehingga akar worktree adalah dua tingkat di atasnya.
	worktree := *worktreeFlag
	if worktree == "" {
		abs, err := filepath.Abs(*contractPath)
		if err != nil {
			return fail(exitError, "%v", err)
		}
		worktree = filepath.Dir(filepath.Dir(abs))
	}

	// Target relatif diselesaikan terhadap worktree — cwd agent adalah worktree.
	targetPath := *target
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(worktree, targetPath)
	}

	// Batas cross-repository (R-15): path teresolusi harus di dalam worktree.
	// isInside me-resolve symlink kedua sisi, sehingga symlink keluar worktree
	// tertangkap sebelum pencocokan glob.
	inside, err := isInside(targetPath, worktree)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	if !inside {
		reportViolations("operasi tulis ditolak", []string{
			fmt.Sprintf("%s berada di luar worktree %s — task hanya boleh menulis satu repository (R-15, §29.2)", *target, worktree),
		})
		return exitViolation
	}

	// Rel dihitung dari bentuk teresolusi agar symlink tidak dapat menyamarkan
	// lokasi sebenarnya terhadap pola glob.
	rt, err := resolveEventual(targetPath)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	rw, err := resolveEventual(worktree)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	rel, err := filepath.Rel(rw, rt)
	if err != nil {
		return fail(exitError, "menghitung path relatif: %v", err)
	}
	rel = filepath.ToSlash(rel)

	if f, blocked := pathmatch.IsForbidden(rel, forbidden); blocked {
		reportViolations("operasi tulis ditolak", []string{
			fmt.Sprintf("%s tertutup forbidden path %q", rel, f),
		})
		return exitViolation
	}
	if !pathmatch.IsAllowed(rel, allowed, forbidden) {
		reportViolations("operasi tulis ditolak", []string{
			fmt.Sprintf("%s tidak tercakup allowed_paths task", rel),
		})
		return exitViolation
	}

	fmt.Printf("ok  %s diizinkan\n", rel)
	return exitOK
}

// --- validate-changed-paths ---

// cmdValidateChangedPaths memeriksa seluruh changed file sebuah PR terhadap
// contract. Ini net kedua R-07: agent lokal dapat melewati hook lewat Bash,
// tetapi tidak dapat melewati CI yang mengulang pemeriksaan yang sama.
//
// Masukan changed adalah keluaran `git diff --name-only` — path relatif repo,
// satu per baris. "-" membaca dari stdin.
func cmdValidateChangedPaths(args []string) int {
	fs := newFlagSet("validate-changed-paths")
	contractPath := fs.String("contract", "", "path task contract (.yaml atau .json)")
	changedPath := fs.String("changed", "-", "berkas daftar changed file; \"-\" untuk stdin")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *contractPath == "" {
		return fail(exitError, "-contract wajib diisi")
	}

	allowed, forbidden, err := loadContractPaths(*contractPath)
	if err != nil {
		return fail(exitError, "%v", err)
	}

	var in *os.File
	if *changedPath == "-" {
		in = os.Stdin
	} else {
		f, err := os.Open(*changedPath)
		if err != nil {
			return fail(exitError, "membuka %s: %v", *changedPath, err)
		}
		defer f.Close()
		in = f
	}

	var violations []string
	count := 0
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		count++
		rel := filepath.ToSlash(line)
		if f, blocked := pathmatch.IsForbidden(rel, forbidden); blocked {
			violations = append(violations, fmt.Sprintf("%s tertutup forbidden path %q", rel, f))
			continue
		}
		if !pathmatch.IsAllowed(rel, allowed, forbidden) {
			violations = append(violations, fmt.Sprintf("%s tidak tercakup allowed_paths task", rel))
		}
	}
	if err := sc.Err(); err != nil {
		return fail(exitError, "membaca daftar changed file: %v", err)
	}

	if len(violations) > 0 {
		reportViolations("PR menyentuh path di luar scope task", violations)
		return exitViolation
	}
	fmt.Printf("ok  %d changed file seluruhnya dalam scope\n", count)
	return exitOK
}
