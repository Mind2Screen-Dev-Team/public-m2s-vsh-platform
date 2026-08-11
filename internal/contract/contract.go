// Package contract memuat, mengonversi, dan memvalidasi task contract,
// reservation, dan handoff.
//
// Bentuk normatif adalah schemas/*.schema.json (ADR-004 #3). Paket ini tidak
// menduplikasi aturan schema dalam kode Go — ia membaca schema tersebut dan
// menyerahkan validasi kepadanya, sehingga tidak ada dua sumber kebenaran.
//
// Alur format mengikuti ADR-004 #1:
//
//	YAML (ditulis manusia)  →  JSON (transport ke .task/contract.json)
package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// Kind menandai schema mana yang dipakai memvalidasi sebuah dokumen.
type Kind string

const (
	KindTask        Kind = "task"
	KindReservation Kind = "reservation"
	KindHandoff     Kind = "handoff"
	KindTaskStatus  Kind = "task-status"
)

func (k Kind) schemaFile() string { return string(k) + ".schema.json" }

// schemaBaseURI wajib sama dengan prefiks "$id" pada schemas/*.schema.json.
// Sumber $ref antar-schema ditulis relatif ("common.schema.json#/$defs/..."),
// sehingga di-resolve terhadap $id — bukan terhadap nama berkas lokal. Resource
// karena itu didaftarkan dengan URI absolut ini, bukan nama berkas.
const schemaBaseURI = "https://m2s-vsh.mindtoscreen.dev/schemas/"

// Validator memvalidasi dokumen terhadap schema pada satu direktori.
type Validator struct {
	schemaDir string
	compiled  map[Kind]*jsonschema.Schema
}

// NewValidator memuat dan mengompilasi seluruh schema dari schemaDir.
//
// Kompilasi dilakukan sekali di awal agar kegagalan schema terdeteksi saat
// start-up, bukan saat validasi dokumen pertama.
func NewValidator(schemaDir string) (*Validator, error) {
	c := jsonschema.NewCompiler()

	// common.schema.json hanya dirujuk lewat $ref, tidak divalidasi langsung.
	// Ia harus didaftarkan dengan nama relatif yang sama seperti pada $ref.
	//
	// Lima schema terakhir (failure, review-report, capability, task-state,
	// task-status) adalah dokumen registry/referensi: task-status menjadi Kind
	// di bawah; sisanya belum ada subcommand runner yang memvalidasinya. Semua
	// tetap didaftarkan karena review-report.schema.json mem-$ref handoff.schema.json
	// dan task-status.schema.json mem-$ref common.schema.json — resolusi $ref
	// menuntut resource-nya ada — dan karena TestSchemaFilesAreRegistered
	// menuntut setiap *.schema.json terdaftar.
	for _, name := range []string{
		"common.schema.json",
		"task.schema.json",
		"reservation.schema.json",
		"handoff.schema.json",
		"failure.schema.json",
		"review-report.schema.json",
		"capability.schema.json",
		"task-state.schema.json",
		"task-status.schema.json",
	} {
		path := filepath.Join(schemaDir, name)
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("membuka schema %s: %w", name, err)
		}
		doc, err := jsonschema.UnmarshalJSON(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("membaca schema %s: %w", name, err)
		}
		if err := c.AddResource(schemaBaseURI+name, doc); err != nil {
			return nil, fmt.Errorf("mendaftarkan schema %s: %w", name, err)
		}
	}

	v := &Validator{schemaDir: schemaDir, compiled: map[Kind]*jsonschema.Schema{}}
	for _, k := range []Kind{KindTask, KindReservation, KindHandoff, KindTaskStatus} {
		s, err := c.Compile(schemaBaseURI + k.schemaFile())
		if err != nil {
			return nil, fmt.Errorf("mengompilasi %s: %w", k.schemaFile(), err)
		}
		v.compiled[k] = s
	}
	return v, nil
}

// ValidationError memuat seluruh pelanggaran pada satu dokumen.
type ValidationError struct {
	Path       string
	Kind       Kind
	Violations []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s tidak memenuhi %s.schema.json: %d pelanggaran",
		e.Path, e.Kind, len(e.Violations))
}

// Load membaca dokumen YAML atau JSON, memvalidasinya terhadap schema, dan
// mengembalikan bentuk JSON-nya.
//
// Nilai kembalian adalah map hasil konversi — siap ditulis sebagai
// .task/contract.json tanpa konversi ulang.
func (v *Validator) Load(path string, k Kind) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("membaca %s: %w", path, err)
	}

	doc, err := decode(raw, path)
	if err != nil {
		return nil, err
	}

	if err := v.Validate(doc, k, path); err != nil {
		return nil, err
	}
	return doc, nil
}

// Validate memvalidasi dokumen yang sudah ter-decode.
func (v *Validator) Validate(doc any, k Kind, sourcePath string) error {
	s, ok := v.compiled[k]
	if !ok {
		return fmt.Errorf("kind tidak dikenal: %s", k)
	}
	if err := s.Validate(doc); err != nil {
		return &ValidationError{
			Path:       sourcePath,
			Kind:       k,
			Violations: flatten(err),
		}
	}
	return nil
}

// decode memuat YAML atau JSON menjadi map dengan kunci string.
//
// yaml.v3 menghasilkan map[string]any untuk mapping, sehingga kompatibel
// dengan validator JSON Schema tanpa konversi tambahan. YAML juga superset
// JSON, sehingga satu jalur decode melayani keduanya.
func decode(raw []byte, path string) (map[string]any, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("mem-parse %s: %w", path, err)
	}
	if doc == nil {
		return nil, fmt.Errorf("%s kosong", path)
	}

	// Round-trip melalui JSON untuk memastikan seluruh nilai dapat
	// direpresentasikan JSON. Tipe khas YAML (timestamp, kunci non-string)
	// akan gagal di sini, bukan diam-diam lolos ke .task/contract.json.
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("%s memuat nilai yang tidak dapat dijadikan JSON: %w", path, err)
	}
	var normalized map[string]any
	if err := json.Unmarshal(b, &normalized); err != nil {
		return nil, fmt.Errorf("normalisasi %s: %w", path, err)
	}
	return normalized, nil
}

// flatten mengubah kesalahan validasi berjenjang menjadi daftar rata yang
// dapat dibaca operator.
//
// Memakai BasicOutput() milik library, bukan menelusuri Causes sendiri:
// ErrorKind.LocalizedString menuntut *message.Printer yang sah dan panic bila
// diberi nil, sedangkan BasicOutput menyediakan printer bawaannya.
func flatten(err error) []string {
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return []string{err.Error()}
	}

	basic := ve.BasicOutput()
	var out []string
	for _, u := range basic.Errors {
		if u.Error == nil {
			continue
		}
		msg := u.Error.String()
		// Buang unit pembungkus seperti "'allOf' failed" — ia tidak menunjuk
		// pelanggaran konkret, hanya kombinator yang menaunginya.
		if isWrapperMessage(msg) {
			continue
		}
		loc := u.InstanceLocation
		if loc == "" {
			loc = "/"
		}
		out = append(out, fmt.Sprintf("%s: %s", loc, msg))
	}

	// BasicOutput dapat mengembalikan unit tunggal tanpa daftar Errors bila
	// pelanggarannya berada di akar dokumen.
	if len(out) == 0 {
		if basic.Error != nil {
			out = append(out, basic.Error.String())
		} else {
			out = append(out, ve.Error())
		}
	}
	return out
}

// isWrapperMessage menandai pesan yang hanya menyebut kombinator schema tanpa
// menunjuk pelanggaran konkret. Menyimpannya membuat keluaran berderau tanpa
// menambah informasi.
func isWrapperMessage(msg string) bool {
	for _, kw := range []string{"'allOf' failed", "'anyOf' failed", "'oneOf' failed"} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// MaterializeJSON menulis dokumen sebagai JSON ter-indent.
//
// Inilah langkah materialisasi Q15: runner menulis snapshot ke
// <worktree>/.task/contract.json yang dibaca hook. YAML tidak pernah masuk
// worktree.
func MaterializeJSON(doc map[string]any, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("membuat direktori %s: %w", filepath.Dir(dest), err)
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("menulis JSON: %w", err)
	}
	b = append(b, '\n')

	// 0o444 read-only: agent tidak boleh mengubah contract-nya sendiri (Q15).
	// Tulis ke berkas sementara lalu rename agar tidak ada keadaan setengah.
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, b, 0o444); err != nil {
		return fmt.Errorf("menulis %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("memindahkan ke %s: %w", dest, err)
	}
	return nil
}
