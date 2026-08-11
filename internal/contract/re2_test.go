package contract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSchemaPatternsAreRE2Compatible menegakkan konsekuensi ADR-004 #2.
//
// Go memakai RE2 yang tidak mendukung lookahead/lookbehind, sementara JSON
// Schema mengacu ECMA-262 yang mendukungnya. Pola yang memakai (?!...) lolos
// validator JavaScript tetapi membuat validator Go GAGAL SAAT KOMPILASI —
// artinya cmd/m2s tidak dapat start sama sekali.
//
// Terverifikasi empiris:
//
//	'^(?!/).+$' is not valid regex: invalid or unsupported Perl syntax: `(?!`
//
// Test ini menangkap regresi bila seseorang menambahkan pola bergaya ECMA-262
// ke schemas/*.schema.json.
func TestSchemaPatternsAreRE2Compatible(t *testing.T) {
	dir := schemaDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("membaca %s: %v", dir, err)
	}

	patternKey := regexp.MustCompile(`"pattern"\s*:\s*"((?:[^"\\]|\\.)*)"`)
	checked := 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".schema.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("membaca %s: %v", e.Name(), err)
		}

		for _, m := range patternKey.FindAllStringSubmatch(string(raw), -1) {
			// Unescape rangkaian JSON agar pola diuji sebagaimana dilihat
			// validator, bukan sebagaimana tertulis di berkas.
			pat := strings.NewReplacer(`\\`, `\`, `\"`, `"`, `\/`, `/`).Replace(m[1])
			checked++

			if _, err := regexp.Compile(pat); err != nil {
				t.Errorf("%s: pola %q tidak dapat dikompilasi RE2: %v\n"+
					"    Tulis batasan negatif sebagai `not` + pola sederhana, bukan lookahead.\n"+
					"    Lihat ADR-004 § Batasan yang ditimbulkan pilihan Go.",
					e.Name(), pat, err)
			}
			for _, forbidden := range []string{"(?!", "(?=", "(?<"} {
				if strings.Contains(pat, forbidden) {
					t.Errorf("%s: pola %q memuat %q — tidak didukung RE2",
						e.Name(), pat, forbidden)
				}
			}
		}
	}

	if checked == 0 {
		t.Error("tidak ada pola yang diperiksa — regex ekstraksi mungkin rusak")
	}
	t.Logf("%d pola diperiksa, seluruhnya kompatibel RE2", checked)
}

// TestSchemaFilesAreRegistered memastikan setiap *.schema.json pada schemas/
// benar-benar didaftarkan NewValidator. Berkas yang terlupa akan menyebabkan
// kegagalan $ref yang membingungkan saat runtime.
func TestSchemaFilesAreRegistered(t *testing.T) {
	entries, err := os.ReadDir(schemaDir(t))
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string]bool{
		"common.schema.json":      true,
		"task.schema.json":        true,
		"reservation.schema.json": true,
		"handoff.schema.json":     true,
		// Dokumen registry/referensi: didaftarkan sebagai resource agar $ref
		// ter-resolve, tetapi bukan Kind — belum ada subcommand runner yang
		// memvalidasinya. Lihat komentar pada NewValidator. Pengecualian:
		// task-status menjadi Kind (ADR-011), divalidasi runner update-status.
		"failure.schema.json":       true,
		"review-report.schema.json": true,
		"capability.schema.json":    true,
		"task-state.schema.json":    true,
		"task-status.schema.json":   true,
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".schema.json") {
			continue
		}
		if !registered[e.Name()] {
			t.Errorf("%s ada di schemas/ tetapi tidak didaftarkan NewValidator", e.Name())
		}
	}
}
