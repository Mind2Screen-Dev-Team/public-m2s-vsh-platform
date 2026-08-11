package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func loadSchemaJSON(t *testing.T, name string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(schemaDir(t), name))
	if err != nil {
		t.Fatalf("membaca %s: %v", name, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("mem-parse %s: %v", name, err)
	}
	return doc
}

func enumAt(t *testing.T, doc map[string]any, path string) []string {
	t.Helper()
	var cur any = doc
	for _, p := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("path %q terputus pada %q", path, p)
		}
		cur = m[p]
	}
	m, ok := cur.(map[string]any)
	if !ok {
		t.Fatalf("path %q bukan objek", path)
	}
	raw, ok := m["enum"].([]any)
	if !ok {
		t.Fatalf("path %q tidak memiliki enum", path)
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, _ := v.(string)
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// TestWriterRoleIsSubsetOfRole memastikan setiap writerRole juga merupakan role
// yang sah. writerRole yang tidak ada pada role akan menghasilkan reservasi
// yang valid tetapi tidak dapat menghasilkan handoff.
func TestWriterRoleIsSubsetOfRole(t *testing.T) {
	common := loadSchemaJSON(t, "common.schema.json")
	roles := enumAt(t, common, "$defs.role")
	writers := enumAt(t, common, "$defs.writerRole")

	set := map[string]bool{}
	for _, r := range roles {
		set[r] = true
	}
	for _, w := range writers {
		if !set[w] {
			t.Errorf("writerRole %q tidak terdaftar pada role", w)
		}
	}

	// code-reviewer wajib TIDAK menjadi writerRole: plan mode tanpa
	// Write/Edit, sehingga ia tidak dapat memegang reservasi (A-03, Q9).
	for _, w := range writers {
		if w == "code-reviewer" {
			t.Error("code-reviewer tidak boleh menjadi writerRole (A-03)")
		}
		if w == "project-manager" {
			t.Error("project-manager tidak boleh menjadi writerRole — write-nya terbatas control/** (Q11)")
		}
	}
}

// TestWriterRolesMustReportChanges menjaga duplikasi daftar role pada
// handoff.schema.json agar tidak menyimpang dari writerRole.
//
// Blok if/then di sana mewajibkan changed_files tidak kosong saat status
// implementation-complete. Role penulis yang terlewat dari daftar itu dapat
// melaporkan "selesai" tanpa satu pun berkas berubah — persis yang §35
// larang.
func TestWriterRolesMustReportChanges(t *testing.T) {
	common := loadSchemaJSON(t, "common.schema.json")
	writers := enumAt(t, common, "$defs.writerRole")

	handoff := loadSchemaJSON(t, "handoff.schema.json")
	allOf, ok := handoff["allOf"].([]any)
	if !ok {
		t.Fatal("handoff.schema.json tidak memiliki allOf")
	}

	var listed []string
	for _, entry := range allOf {
		m, _ := entry.(map[string]any)
		ifBlock, _ := m["if"].(map[string]any)
		props, _ := ifBlock["properties"].(map[string]any)
		statusBlock, _ := props["status"].(map[string]any)
		if statusBlock == nil || statusBlock["const"] != "implementation-complete" {
			continue
		}
		roleBlock, _ := props["role"].(map[string]any)
		raw, _ := roleBlock["enum"].([]any)
		for _, v := range raw {
			s, _ := v.(string)
			listed = append(listed, s)
		}
	}
	if len(listed) == 0 {
		t.Fatal("tidak menemukan blok implementation-complete pada handoff.schema.json")
	}

	set := map[string]bool{}
	for _, r := range listed {
		set[r] = true
	}
	for _, w := range writers {
		if !set[w] {
			t.Errorf("writerRole %q tidak wajib melaporkan changed_files pada handoff.schema.json —\n"+
				"    ia dapat melaporkan implementation-complete tanpa perubahan berkas (§35)", w)
		}
	}
	if set["code-reviewer"] {
		t.Error("code-reviewer tidak boleh masuk daftar ini — changed_files-nya wajib kosong")
	}
}

// TestRoleNamesAreFileSafe menegakkan bentuk nama role.
//
// Nama role menjadi nama berkas .claude/agents/<role>.md (§37). Huruf besar
// berisiko berperilaku berbeda antara macOS/Windows (case-insensitive) dan
// Linux (case-sensitive) — alasan yang sama dengan path-overlap-matrix.md §3.
func TestRoleNamesAreFileSafe(t *testing.T) {
	common := loadSchemaJSON(t, "common.schema.json")
	for _, r := range enumAt(t, common, "$defs.role") {
		if r != strings.ToLower(r) {
			t.Errorf("role %q memuat huruf besar — nama berkas agent harus kebab-case huruf kecil (§37)", r)
		}
		if strings.ContainsAny(r, " _./\\") {
			t.Errorf("role %q memuat karakter yang tidak aman untuk nama berkas", r)
		}
	}
}
