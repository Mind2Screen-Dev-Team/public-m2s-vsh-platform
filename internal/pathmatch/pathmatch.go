// Package pathmatch mengimplementasikan semantik glob dan deteksi konflik
// reservasi path sesuai docs/decisions/path-overlap-matrix.md.
//
// Paket ini menutup R-03. Seluruh 24 kasus pada matriks tersebut wajib hadir
// sebagai test pada pathmatch_test.go.
//
// Aturan yang ditegakkan (matriks §1-§3):
//   - pemisah selalu '/', path relatif terhadap akar repository
//   - "a/**" mencakup seluruh isi a/ secara rekursif, termasuk "a" sendiri
//   - "a/*" hanya satu segmen langsung di bawah a/
//   - '*' tidak melewati '/', hanya "**" yang melewatinya
//   - pencocokan case-insensitive (matriks §3)
package pathmatch

import "strings"

// Normalize menyeragamkan pola sebelum pembandingan (matriks §4.7).
//
// Menangani: pemisah backslash, prefiks "./", pemisah ganda, trailing slash,
// dan case. Tidak menyentuh segmen ".." — penolakannya urusan schema.
func Normalize(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")

	// Kolaps pemisah ganda dan prefiks "./" diulang sampai stabil.
	// Urutannya penting: membuang "./" lebih dulu pada ".//./a" menyisakan
	// "/./a" yang tidak lagi memuat "//" untuk diciutkan.
	for {
		before := p
		for strings.Contains(p, "//") {
			p = strings.ReplaceAll(p, "//", "/")
		}
		for strings.HasPrefix(p, "./") {
			p = p[2:]
		}
		if p == before {
			break
		}
	}

	// "a/" dan "a" menunjuk hal yang sama; "a/**" dipertahankan utuh.
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimSuffix(p, "/")
	}
	return strings.ToLower(p)
}

func segments(p string) []string {
	p = Normalize(p)
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// matchSegment mencocokkan satu segmen pola terhadap satu segmen literal.
// Mendukung '*' sebagai wildcard dalam-segmen; tidak melewati '/'.
func matchSegment(pattern, s string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == s
	}
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	rest := s[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(rest, parts[i])
		if idx < 0 {
			return false
		}
		rest = rest[idx+len(parts[i]):]
	}
	last := parts[len(parts)-1]
	return last == "" || strings.HasSuffix(rest, last)
}

// segmentsCompatible menentukan apakah dua segmen dapat menunjuk nama yang sama.
// Salah satu boleh mengandung wildcard, atau keduanya.
func segmentsCompatible(a, b string) bool {
	return matchSegment(a, b) || matchSegment(b, a)
}

// Overlap menentukan apakah dua pola dapat mencakup berkas yang sama.
//
// Ini BUKAN kesamaan string. "internal/payroll/**" dan
// "internal/payroll/period/**" adalah overlap meski string keduanya berbeda —
// kasus yang R-03 nyatakan akan diloloskan implementasi naif.
//
// Fungsi ini simetris: Overlap(a, b) == Overlap(b, a).
func Overlap(a, b string) bool {
	return walk(segments(a), segments(b))
}

func walk(a, b []string) bool {
	switch {
	case len(a) == 0 && len(b) == 0:
		return true
	// Satu pola habis: overlap hanya bila sisi lain tersisa tepat "**",
	// karena "a/**" mencakup "a" itu sendiri (matriks kasus 6).
	case len(a) == 0:
		return len(b) == 1 && b[0] == "**"
	case len(b) == 0:
		return len(a) == 1 && a[0] == "**"
	}

	if a[0] == "**" {
		if len(a) == 1 {
			return true // "**" di posisi akhir menyerap seluruh sisa
		}
		// Coba setiap kemungkinan jumlah segmen yang diserap "**".
		for i := 0; i <= len(b); i++ {
			if walk(a[1:], b[i:]) {
				return true
			}
		}
		return false
	}
	if b[0] == "**" {
		return walk(b, a)
	}

	if !segmentsCompatible(a[0], b[0]) {
		return false
	}
	return walk(a[1:], b[1:])
}

// Matches menentukan apakah sebuah path konkret dicakup satu pola.
//
// Berbeda dari Overlap: di sini path diperlakukan sebagai berkas nyata, bukan
// pola. Dipakai PreToolUse hook Phase 3 (§59) untuk memutuskan satu operasi
// tulis.
func Matches(pattern, path string) bool {
	return walk(segments(pattern), segments(path))
}

// AnyOverlap melaporkan pasangan pertama yang konflik antara dua himpunan pola.
// Mengembalikan string kosong bila tidak ada konflik.
func AnyOverlap(setA, setB []string) (string, string, bool) {
	for _, a := range setA {
		for _, b := range setB {
			if Overlap(a, b) {
				return a, b, true
			}
		}
	}
	return "", "", false
}

// IsForbidden menentukan apakah path tertutup salah satu pola forbidden.
//
// Matriks §4.8: forbidden diperiksa LEBIH DULU daripada allowed. Pemanggil
// wajib memakai fungsi ini sebelum memeriksa allowed.
func IsForbidden(path string, forbidden []string) (string, bool) {
	for _, f := range forbidden {
		if Matches(f, path) {
			return f, true
		}
	}
	return "", false
}

// IsAllowed menerapkan urutan yang benar: forbidden mengalahkan allowed.
func IsAllowed(path string, allowed, forbidden []string) bool {
	if _, blocked := IsForbidden(path, forbidden); blocked {
		return false
	}
	for _, a := range allowed {
		if Matches(a, path) {
			return true
		}
	}
	return false
}
