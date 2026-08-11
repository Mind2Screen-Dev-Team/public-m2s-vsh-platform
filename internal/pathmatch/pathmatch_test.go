package pathmatch

import "testing"

// TestOverlapMatrix memuat seluruh kasus pada
// docs/decisions/path-overlap-matrix.md §4. Nomor kasus di sini WAJIB sama
// dengan nomor pada dokumen tersebut — bila dokumen berubah, test ini berubah.
//
// Menutup R-03; acceptance AC-2.3…AC-2.6.
func TestOverlapMatrix(t *testing.T) {
	cases := []struct {
		id       int
		category string
		a, b     string
		want     bool
		why      string
	}{
		// §4.1 identik dan disjoint
		{1, "identik", "internal/payroll/**", "internal/payroll/**", true, "pola sama"},
		{2, "disjoint", "internal/payroll/**", "internal/attendance/**", false, "subtree terpisah"},
		{3, "disjoint", "go.mod", "go.sum", false, "berkas berbeda"},

		// §4.2 parent/child — kasus inti R-03
		{4, "parent/child", "internal/payroll/**", "internal/payroll/period/**", true, "B subtree dari A"},
		{5, "parent/child", "internal/**", "internal/payroll/period/close.go", true, "A mencakup B"},
		{6, "parent/child", "internal/payroll/**", "internal/payroll", true, "** mencakup direktori itu sendiri"},
		{7, "parent/child", "**", "go.mod", true, "A mencakup seluruh repo"},

		// §4.3 prefiks menyesatkan — menangkap strings.HasPrefix tanpa batas segmen
		{8, "prefiks", "internal/pay/**", "internal/payroll/**", false, "pay bukan segmen induk payroll"},
		{9, "prefiks", "internal/payroll/**", "internal/payroll-legacy/**", false, "segmen berbeda meski berprefiks sama"},
		{10, "prefiks", "a/b", "a/bc", false, "exact berbeda"},

		// §4.4 glob vs exact file
		{11, "glob/exact", "internal/payroll/**", "internal/payroll/enum.go", true, "exact di dalam glob"},
		{12, "glob/exact", "internal/payroll/*", "internal/payroll/enum.go", true, "satu segmen, cocok"},
		{13, "glob/exact", "internal/payroll/*", "internal/payroll/period/close.go", false, "* tidak melewati /"},
		{14, "glob/exact", "*.go", "main.go", true, "keduanya pada akar"},
		{15, "glob/exact", "*.go", "internal/main.go", false, "* tidak melewati /"},

		// §4.5 case-sensitivity
		{16, "case", "internal/payroll/**", "Internal/Payroll/**", true, "case-insensitive"},
		{17, "case", "go.mod", "GO.MOD", true, "berkas sama pada APFS/NTFS"},
		{18, "case", "internal/payroll/**", "INTERNAL/attendance/**", false, "segmen kedua tetap berbeda"},

		// §4.7 normalisasi
		{21, "normalisasi", "internal/payroll/", "internal/payroll", true, "trailing slash tidak bermakna"},
		{22, "normalisasi", "./internal/payroll/**", "internal/payroll/**", true, "./ dinormalisasi"},
		{23, "normalisasi", "internal//payroll/**", "internal/payroll/**", true, "pemisah ganda dinormalisasi"},
	}

	for _, c := range cases {
		got := Overlap(c.a, c.b)
		if got != c.want {
			t.Errorf("kasus #%d [%s]: Overlap(%q, %q) = %v, mau %v — %s",
				c.id, c.category, c.a, c.b, got, c.want, c.why)
		}
		// Overlap wajib simetris; asimetri adalah bug.
		if rev := Overlap(c.b, c.a); rev != got {
			t.Errorf("kasus #%d: TIDAK SIMETRIS — Overlap(a,b)=%v tetapi Overlap(b,a)=%v",
				c.id, got, rev)
		}
	}
}

// TestSharedFileOwnership menutup matriks §4.6 (kasus 19-20) dan R-04.
// Kasus ini butuh konteks reservasi, bukan sepasang pola.
func TestSharedFileOwnership(t *testing.T) {
	type claim struct {
		taskID string
		path   string
	}

	// Kasus 19: dua task mengklaim exact file yang sama.
	a := claim{"BE-101", "internal/payroll/enum.go"}
	b := claim{"BE-102", "internal/payroll/enum.go"}
	if !Overlap(a.path, b.path) {
		t.Errorf("kasus #19: dua task pada %q harus konflik (§29.6 single owner)", a.path)
	}
	if a.taskID == b.taskID {
		t.Fatal("prasyarat test salah: task harus berbeda")
	}

	// Kasus 20: glob milik satu task mencakup exact file yang diklaim task lain.
	glob := "internal/payroll/**"
	exact := "internal/payroll/enum.go"
	if !Overlap(glob, exact) {
		t.Errorf("kasus #20: %q harus konflik dengan %q — owner berbeda dari pengklaim",
			glob, exact)
	}
}

// TestForbiddenBeatsAllowed menutup matriks §4.8 (kasus 24).
func TestForbiddenBeatsAllowed(t *testing.T) {
	allowed := []string{"internal/**"}
	forbidden := []string{"internal/auth/**"}

	if IsAllowed("internal/auth/token.go", allowed, forbidden) {
		t.Error("kasus #24: internal/auth/token.go harus DITOLAK — forbidden diperiksa lebih dulu")
	}
	if !IsAllowed("internal/payroll/close.go", allowed, forbidden) {
		t.Error("kasus #24: internal/payroll/close.go harus DIIZINKAN")
	}
	if f, blocked := IsForbidden("internal/auth/token.go", forbidden); !blocked || f != "internal/auth/**" {
		t.Errorf("IsForbidden harus melaporkan pola pemblokir, dapat %q blocked=%v", f, blocked)
	}
}

func TestAnyOverlap(t *testing.T) {
	setA := []string{"internal/payroll/**", "docs/**"}
	setB := []string{"internal/attendance/**", "internal/payroll/period/close.go"}

	a, b, found := AnyOverlap(setA, setB)
	if !found {
		t.Fatal("AnyOverlap harus menemukan konflik")
	}
	if a != "internal/payroll/**" || b != "internal/payroll/period/close.go" {
		t.Errorf("pasangan konflik salah: %q vs %q", a, b)
	}

	if _, _, found := AnyOverlap([]string{"docs/**"}, []string{"internal/**"}); found {
		t.Error("himpunan disjoint tidak boleh dilaporkan konflik")
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"internal/payroll/":     "internal/payroll",
		"./internal/payroll":    "internal/payroll",
		"internal//payroll":     "internal/payroll",
		"Internal/Payroll":      "internal/payroll",
		"internal\\payroll":     "internal/payroll",
		"internal/payroll/**":   "internal/payroll/**",
		".//./internal/payroll": "internal/payroll",
		"/":                     "/",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, mau %q", in, got, want)
		}
	}
}

// TestMatchesConcretePath memastikan Matches memperlakukan argumen kedua
// sebagai berkas nyata, bukan pola.
func TestMatchesConcretePath(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"internal/**", "internal/payroll/close.go", true},
		{"internal/**", "internal", true},
		{"internal/*", "internal/payroll", true},
		{"internal/*", "internal/payroll/close.go", false},
		{"*.go", "main.go", true},
		{"*.go", "internal/main.go", false},
		{".claude/**", ".claude/settings.json", true},
		{".task/**", ".task/contract.json", true},
		{"go.mod", "go.sum", false},
	}
	for _, c := range cases {
		if got := Matches(c.pattern, c.path); got != c.want {
			t.Errorf("Matches(%q, %q) = %v, mau %v", c.pattern, c.path, got, c.want)
		}
	}
}
