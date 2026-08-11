// Command m2s adalah deterministic task runner untuk M2S-VSH Lite.
//
// Runner BUKAN agent (§13.4): ia tidak membuat keputusan teknis, dan seluruh
// perilakunya ditentukan task contract. Setiap subcommand wajib idempotent —
// menjalankannya dua kali tidak boleh menghasilkan keadaan berbeda.
//
// Binary ini dipanggil lewat wrapper scripts/<runner>.sh sehingga pola tool
// Bash Project Manager pada Q11 tetap berlaku tanpa perubahan (ADR-004 #2).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Exit code yang dipakai seluruh subcommand.
//
// exitViolation dibedakan dari exitError agar pemanggil dapat membedakan
// "kontrak ditolak" dari "runner gagal berjalan" — keduanya menuntut tindakan
// berbeda.
const (
	exitOK        = 0
	exitError     = 1
	exitViolation = 2
)

type command struct {
	name    string
	summary string
	run     func(args []string) int
}

func main() {
	commands := []command{
		{"validate-task", "Validasi task contract terhadap schema", cmdValidateTask},
		{"reserve-paths", "Buat reservasi path setelah memeriksa konflik", cmdReservePaths},
		{"launch-task", "Siapkan worktree dan materialisasi contract", cmdLaunchTask},
		{"collect-result", "Kumpulkan dan validasi handoff", cmdCollectResult},
		{"release-reservation", "Lepas reservasi setelah merge", cmdReleaseReservation},
		{"update-status", "Tulis status task dengan validasi transisi + owner", cmdUpdateStatus},
		{"launch-review", "Siapkan sesi Code Reviewer (gate implementation-complete)", cmdLaunchReview},
		{"collect-review", "Tulis hasil review dari handoff (reviewing/changes-requested)", cmdCollectReview},
		{"launch-qa", "Siapkan sesi QA Engineer (gate reviewing)", cmdLaunchQA},
		{"collect-qa", "Tulis hasil QA dari handoff (merge-ready/defect-found)", cmdCollectQA},
		{"check-path", "Putuskan satu operasi tulis terhadap contract (dipakai hook)", cmdCheckPath},
		{"validate-changed-paths", "Periksa daftar changed file PR terhadap contract (dipakai CI)", cmdValidateChangedPaths},
	}

	if len(os.Args) < 2 {
		usage(commands)
		os.Exit(exitError)
	}

	sub := os.Args[1]
	for _, c := range commands {
		if c.name == sub {
			os.Exit(c.run(os.Args[2:]))
		}
	}

	fmt.Fprintf(os.Stderr, "subcommand tidak dikenal: %s\n\n", sub)
	usage(commands)
	os.Exit(exitError)
}

func usage(commands []command) {
	fmt.Fprintf(os.Stderr, "m2s — deterministic task runner M2S-VSH Lite\n\n")
	fmt.Fprintf(os.Stderr, "Penggunaan:\n  m2s <subcommand> [flag]\n\nSubcommand:\n")
	for _, c := range commands {
		fmt.Fprintf(os.Stderr, "  %-20s %s\n", c.name, c.summary)
	}
	fmt.Fprintf(os.Stderr, "\nExit code:\n")
	fmt.Fprintf(os.Stderr, "  %d  berhasil\n", exitOK)
	fmt.Fprintf(os.Stderr, "  %d  runner gagal berjalan\n", exitError)
	fmt.Fprintf(os.Stderr, "  %d  kontrak atau reservasi ditolak\n", exitViolation)
}

// controlRoot menentukan akar control repository.
//
// Urutan: flag -control, lalu env M2S_CONTROL_ROOT, lalu direktori kerja.
func controlRoot(flagVal string) (string, error) {
	if flagVal != "" {
		return filepath.Abs(flagVal)
	}
	if env := os.Getenv("M2S_CONTROL_ROOT"); env != "" {
		return filepath.Abs(env)
	}
	return os.Getwd()
}

// worktreeRoot menentukan akar worktree.
//
// Default $HOME/.m2s/worktrees, dapat di-override M2S_WORKTREE_ROOT (Q8).
// Sengaja BUKAN .claude/worktrees seperti contoh §30, karena .claude/**
// adalah forbidden path (A-01).
func worktreeRoot() (string, error) {
	if env := os.Getenv("M2S_WORKTREE_ROOT"); env != "" {
		return filepath.Abs(env)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("menentukan home directory: %w", err)
	}
	return filepath.Join(home, ".m2s", "worktrees"), nil
}

// fail mencetak kesalahan ke stderr dan mengembalikan exit code.
func fail(code int, format string, args ...any) int {
	fmt.Fprintf(os.Stderr, "m2s: "+format+"\n", args...)
	return code
}

// reportViolations mencetak daftar pelanggaran dengan bentuk yang seragam.
func reportViolations(header string, items []string) {
	fmt.Fprintf(os.Stderr, "m2s: %s\n", header)
	for _, s := range items {
		fmt.Fprintf(os.Stderr, "  - %s\n", s)
	}
}

// newFlagSet membuat FlagSet yang mencetak bantuan ke stderr.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

// str membaca field string dari dokumen berjenjang, misal "ownership.role".
func str(doc map[string]any, path string) string {
	parts := strings.Split(path, ".")
	var cur any = doc
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[p]
	}
	s, _ := cur.(string)
	return s
}

// strSlice membaca array string dari dokumen berjenjang.
func strSlice(doc map[string]any, path string) []string {
	parts := strings.Split(path, ".")
	var cur any = doc
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[p]
	}
	raw, _ := cur.([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
