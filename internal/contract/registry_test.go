package contract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// TestRegistrySchemasCompile memastikan empat schema dokumen (failure,
// review-report, capability, task-state) dapat dikompilasi — bukan hanya
// terdaftar. Mereka bukan Kind validator runner, sehingga NewValidator tidak
// mengompilasikannya; test ini menutup celah itu agar schema yang rusak
// terdeteksi saat CI, bukan saat pemakaian pertama.
func TestRegistrySchemasCompile(t *testing.T) {
	dir := schemaDir(t)

	for _, name := range []string{
		"failure.schema.json",
		"review-report.schema.json",
		"capability.schema.json",
		"task-state.schema.json",
	} {
		path := filepath.Join(dir, name)
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("membuka %s: %v", name, err)
		}
		doc, err := jsonschema.UnmarshalJSON(f)
		f.Close()
		if err != nil {
			t.Fatalf("membaca %s: %v", name, err)
		}

		// Register pada compiler sendiri, bukan NewValidator, karena NewValidator
		// hanya mengompilasi Kind. Resource $ref (common, handoff) didaftarkan
		// dari direktori yang sama.
		c := jsonschema.NewCompiler()
		for _, dep := range []string{"common.schema.json", "handoff.schema.json"} {
			df, err := os.Open(filepath.Join(dir, dep))
			if err != nil {
				t.Fatalf("membuka dependensi %s: %v", dep, err)
			}
			ddoc, err := jsonschema.UnmarshalJSON(df)
			df.Close()
			if err != nil {
				t.Fatalf("membaca dependensi %s: %v", dep, err)
			}
			if err := c.AddResource(schemaBaseURI+dep, ddoc); err != nil {
				t.Fatalf("mendaftarkan dependensi %s: %v", dep, err)
			}
		}
		if err := c.AddResource(schemaBaseURI+name, doc); err != nil {
			t.Fatalf("mendaftarkan %s: %v", name, err)
		}
		if _, err := c.Compile(schemaBaseURI + name); err != nil {
			t.Fatalf("mengompilasi %s: %v", name, err)
		}
	}
}

// TestRegistryExamplesValidate memvalidasi contoh valid di schemas/examples/
// terhadap schema dokumen non-Kind. Contoh yang gagal berarti schema dan contoh
// sudah menyimpang — sama dengan TestLoadExamples untuk schema Kind.
func TestRegistryExamplesValidate(t *testing.T) {
	dir := schemaDir(t)

	cases := map[string]string{
		"failure-BE-101.valid.yaml":         "failure.schema.json",
		"review-report-BE-101.valid.yaml":   "review-report.schema.json",
		"capability-open-design.valid.yaml": "capability.schema.json",
		"task-state-BE-101.valid.yaml":      "task-state.schema.json",
	}

	v := newV(t)
	_ = v // memastikan seluruh schema Kind tetap dapat dikompilasi lebih dulu

	for example, schemaName := range cases {
		doc, err := decodeExample(filepath.Join(dir, "examples", example))
		if err != nil {
			t.Errorf("%s: tidak dapat memuat: %v", example, err)
			continue
		}

		path := filepath.Join(dir, schemaName)
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("membuka %s: %v", schemaName, err)
		}
		sdoc, err := jsonschema.UnmarshalJSON(f)
		f.Close()
		if err != nil {
			t.Fatalf("membaca %s: %v", schemaName, err)
		}

		c := jsonschema.NewCompiler()
		for _, dep := range []string{"common.schema.json", "handoff.schema.json"} {
			df, err := os.Open(filepath.Join(dir, dep))
			if err != nil {
				t.Fatalf("membuka dependensi %s: %v", dep, err)
			}
			ddoc, err := jsonschema.UnmarshalJSON(df)
			df.Close()
			if err != nil {
				t.Fatalf("membaca dependensi %s: %v", dep, err)
			}
			if err := c.AddResource(schemaBaseURI+dep, ddoc); err != nil {
				t.Fatalf("mendaftarkan dependensi %s: %v", dep, err)
			}
		}
		if err := c.AddResource(schemaBaseURI+schemaName, sdoc); err != nil {
			t.Fatalf("mendaftarkan %s: %v", schemaName, err)
		}
		compiled, err := c.Compile(schemaBaseURI + schemaName)
		if err != nil {
			t.Fatalf("mengompilasi %s: %v", schemaName, err)
		}

		if err := compiled.Validate(doc); err != nil {
			t.Errorf("%s tidak valid terhadap %s: %v", example, schemaName, err)
		}
	}
}

// decodeExample memuat contoh YAML/JSON dan mengembalikan bentuk ter-normalisasi.
// Tidak memakai v.Load karena schema-nya bukan Kind validator.
func decodeExample(path string) (any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decode(raw, path)
}
