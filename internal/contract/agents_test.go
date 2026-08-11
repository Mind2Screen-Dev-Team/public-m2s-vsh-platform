package contract

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Test pada berkas ini menegakkan ADR-006 dan membuktikan kriteria Done §57:
// "setiap agent menunjukkan tool dan path boundary yang berbeda."
//
// Ia ditempatkan pada paket ini karena enum role adalah sumber kebenaran jumlah
// dan nama agent, dan roles_test.go sudah menyediakan loadSchemaJSON serta
// enumAt untuk membacanya.

// agentFrontmatter memuat field yang diperiksa test. Field lain yang sah tetapi
// tidak diperiksa ditangkap `rest` agar TestAgentFrontmatterFieldsAreSupported
// dapat menolak field yang tidak dikenal.
type agentFrontmatter struct {
	Name           string   `yaml:"name"`
	Description    string   `yaml:"description"`
	Model          string   `yaml:"model"`
	Effort         string   `yaml:"effort"`
	PermissionMode string   `yaml:"permissionMode"`
	Background     *bool    `yaml:"background"`
	Isolation      string   `yaml:"isolation"`
	MaxTurns       int      `yaml:"maxTurns"`
	Tools          []string `yaml:"tools"`
	Skills         []string `yaml:"skills"`
}

type agentDoc struct {
	file        string
	frontmatter agentFrontmatter
	fields      map[string]any
	body        string
}

func templatesAgentDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.Abs(filepath.Join("..", "..", "templates", "agents"))
	if err != nil {
		t.Fatalf("resolve templates/agents/: %v", err)
	}
	return d
}

func deployedAgentDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.Abs(filepath.Join("..", "..", ".claude", "agents"))
	if err != nil {
		t.Fatalf("resolve .claude/agents/: %v", err)
	}
	return d
}

// splitFrontmatter memisahkan blok YAML di antara dua penanda "---" dari body
// Markdown. Bentuk yang tidak sesuai adalah kegagalan, bukan berkas tanpa
// frontmatter — definisi agent tanpa frontmatter tidak dapat dimuat.
func splitFrontmatter(t *testing.T, path string) (string, string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("membaca %s: %v", path, err)
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		t.Fatalf("%s tidak diawali frontmatter '---'", filepath.Base(path))
	}
	rest := s[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		t.Fatalf("%s tidak memiliki penutup frontmatter '---'", filepath.Base(path))
	}
	return rest[:end], rest[end+len("\n---\n"):]
}

func loadAgent(t *testing.T, dir, name string) agentDoc {
	t.Helper()
	path := filepath.Join(dir, name+".md")
	raw, body := splitFrontmatter(t, path)

	var fm agentFrontmatter
	if err := yaml.Unmarshal([]byte(raw), &fm); err != nil {
		t.Fatalf("mem-parse frontmatter %s: %v", name, err)
	}
	fields := map[string]any{}
	if err := yaml.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatalf("mem-parse frontmatter %s sebagai map: %v", name, err)
	}
	return agentDoc{file: path, frontmatter: fm, fields: fields, body: body}
}

func allRoles(t *testing.T) []string {
	t.Helper()
	return enumAt(t, loadSchemaJSON(t, "common.schema.json"), "$defs.role")
}

func allAgents(t *testing.T) map[string]agentDoc {
	t.Helper()
	dir := templatesAgentDir(t)
	out := map[string]agentDoc{}
	for _, r := range allRoles(t) {
		out[r] = loadAgent(t, dir, r)
	}
	return out
}

func hasTool(tools []string, want string) bool {
	for _, tool := range tools {
		if tool == want {
			return true
		}
	}
	return false
}

// TestEveryRoleHasAgentTemplate memeriksa dua arah: setiap role memiliki
// template, dan setiap template merupakan role yang sah.
//
// Arah kedua penting — template yatim yang tersisa setelah role dihapus akan
// terus terbaca sebagai definisi yang berlaku.
func TestEveryRoleHasAgentTemplate(t *testing.T) {
	dir := templatesAgentDir(t)
	roles := allRoles(t)

	for _, r := range roles {
		if _, err := os.Stat(filepath.Join(dir, r+".md")); err != nil {
			t.Errorf("role %q tidak memiliki templates/agents/%s.md (ADR-006 #1)", r, r)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("membaca templates/agents/: %v", err)
	}
	known := map[string]bool{}
	for _, r := range roles {
		known[r] = true
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		count++
		if base := strings.TrimSuffix(e.Name(), ".md"); !known[base] {
			t.Errorf("templates/agents/%s tidak sesuai role mana pun pada common.schema.json", e.Name())
		}
	}
	if count != len(roles) {
		t.Errorf("jumlah template %d, sedangkan role %d (ADR-005 #5)", count, len(roles))
	}
}

// TestAgentFrontmatterFieldsAreSupported menolak field di luar daftar
// terverifikasi component-inventory.md §6. Field yang tidak didukung diabaikan
// diam-diam saat dimuat, sehingga batas yang ditulis di sana tidak pernah
// berlaku.
func TestAgentFrontmatterFieldsAreSupported(t *testing.T) {
	supported := map[string]bool{
		"name": true, "description": true, "tools": true, "disallowedTools": true,
		"model": true, "effort": true, "permissionMode": true, "background": true,
		"isolation": true, "maxTurns": true, "skills": true, "hooks": true,
		"mcpServers": true, "color": true,
	}
	for role, doc := range allAgents(t) {
		for k := range doc.fields {
			if !supported[k] {
				t.Errorf("%s: field frontmatter %q tidak ada pada daftar terverifikasi (component-inventory.md §6)", role, k)
			}
		}
	}
}

// TestAgentNameMatchesFileName menjaga name, nama berkas, dan nilai enum tetap
// satu. Ketiganya dirujuk di tempat berbeda; penyimpangan salah satunya membuat
// agent tidak dapat ditemukan runner.
func TestAgentNameMatchesFileName(t *testing.T) {
	for role, doc := range allAgents(t) {
		if doc.frontmatter.Name != role {
			t.Errorf("%s: name %q tidak sama dengan nama berkas dan nilai enum role (§37, ADR-005 #4)",
				role, doc.frontmatter.Name)
		}
		if strings.TrimSpace(doc.frontmatter.Description) == "" {
			t.Errorf("%s: description kosong", role)
		}
	}
}

// TestNoAgentHasAgentTool menegakkan pencabutan tool Agent sebagai aturan
// tunggal bagi seluruh role.
//
// Q11 mencabutnya dari Project Manager; ADR-006 #1 memperluasnya karena alasan
// yang dipakai tidak khusus PM — role dijalankan runner sebagai sesi top-level
// terpisah, bukan nested subagent.
func TestNoAgentHasAgentTool(t *testing.T) {
	for role, doc := range allAgents(t) {
		if hasTool(doc.frontmatter.Tools, "Agent") {
			t.Errorf("%s: memegang tool Agent — dicabut bagi seluruh role (Q11, ADR-006 #1)", role)
		}
	}
}

// TestReadOnlyRolesHaveNoWriteTools menjaga sifat read-only code-reviewer.
//
// Ia bukan writerRole (A-03, Q9): runner yang menuliskan review report ke
// reviews/code/**. Tool write pada definisinya akan membuat reviewer dapat
// menyunting kode yang sedang direviewnya sendiri.
func TestReadOnlyRolesHaveNoWriteTools(t *testing.T) {
	doc := allAgents(t)["code-reviewer"]

	if doc.frontmatter.PermissionMode != "plan" {
		t.Errorf("code-reviewer: permissionMode %q, seharusnya plan (A-03, Q9)", doc.frontmatter.PermissionMode)
	}
	for _, tool := range []string{"Edit", "Write", "NotebookEdit"} {
		if hasTool(doc.frontmatter.Tools, tool) {
			t.Errorf("code-reviewer: memegang tool %s — review wajib read-only (§23.5)", tool)
		}
	}
	if doc.frontmatter.Isolation != "" {
		t.Errorf("code-reviewer: isolation %q — ia tidak menulis sehingga tidak memerlukan worktree",
			doc.frontmatter.Isolation)
	}
}

// TestWriterRolesDeclareWorktreeIsolation memastikan setiap role yang boleh
// memegang reservasi juga berjalan pada worktree terpisah.
//
// writerRole tanpa isolation akan menulis pada checkout utama — melanggar §16.2
// dan menghapus jaminan single-writer yang justru menjadi alasan reservasi ada.
func TestWriterRolesDeclareWorktreeIsolation(t *testing.T) {
	writers := enumAt(t, loadSchemaJSON(t, "common.schema.json"), "$defs.writerRole")
	agents := allAgents(t)

	for _, w := range writers {
		doc, ok := agents[w]
		if !ok {
			t.Errorf("writerRole %q tidak memiliki template", w)
			continue
		}
		if doc.frontmatter.Isolation != "worktree" {
			t.Errorf("%s: isolation %q, seharusnya worktree (§16.2, §58, Q13)", w, doc.frontmatter.Isolation)
		}
		for _, tool := range []string{"Edit", "Write"} {
			if !hasTool(doc.frontmatter.Tools, tool) {
				t.Errorf("%s: writerRole tanpa tool %s tidak dapat menjalankan task-nya", w, tool)
			}
		}
	}

	// project-manager menulis control/** dan bukan writerRole, tetapi tetap
	// tidak boleh menyatakan isolation worktree: ia bekerja pada control repo,
	// bukan pada worktree repo aplikasi.
	if pm := agents["project-manager"]; pm.frontmatter.Isolation == "worktree" {
		t.Error("project-manager: isolation worktree — write-nya terbatas control/**, bukan worktree repo aplikasi (Q11)")
	}
}

// TestForbiddenPathBaselinePresent memeriksa ketiga path yang wajib forbidden
// bagi seluruh role (roles-extension-v0.1.0.md § Ringkasan forbidden_paths).
//
// .task/** paling mengikat: tanpanya agent dapat menulis ulang contract-nya
// sendiri dan melonggarkan batas yang mengikatnya (Q15).
func TestForbiddenPathBaselinePresent(t *testing.T) {
	required := []string{".claude/**", ".task/**", ".mneme/**"}
	for role, doc := range allAgents(t) {
		for _, p := range required {
			if !strings.Contains(doc.body, p) {
				t.Errorf("%s: tidak menyebut %s pada forbidden paths baseline", role, p)
			}
		}
	}
}

// TestEveryRoleHasEffort menutup kesenjangan yang dicatat component-inventory.md
// §6. Nilai ini menjadi baseline pengukuran token Phase 8 (§64); role tanpa
// effort membuat pembandingnya hilang.
func TestEveryRoleHasEffort(t *testing.T) {
	valid := map[string]bool{"low": true, "medium": true, "high": true}
	for role, doc := range allAgents(t) {
		if !valid[doc.frontmatter.Effort] {
			t.Errorf("%s: effort %q tidak sah — harus low, medium, atau high (ADR-006 #2)",
				role, doc.frontmatter.Effort)
		}
	}
}

// TestDeployedAgentsMatchTemplates menjaga salinan aktif tetap identik dengan
// template kanoniknya.
//
// Salinan dipilih daripada symlink karena Git tidak menjamin perilaku symlink
// lintas platform. Konsekuensinya konsistensi harus dijaga test — tanpa ini
// keduanya menyimpang diam-diam.
func TestDeployedAgentsMatchTemplates(t *testing.T) {
	// Q10: control repository menjalankan PM + TL/SA (Phase 1). Phase 5
	// (§61) menambah frontend-engineer + technical-writer untuk tool pilot.
	// Semua 13 role kini di-deploy (2026-08-07) — lihat templates/agents/.
	want := []string{
		"android-developer", "backend-engineer", "code-reviewer", "devops-release",
		"frontend-engineer", "fullstack-engineer", "ios-developer", "mobile-engineer",
		"project-manager", "qa-engineer", "technical-lead-system-analyst",
		"technical-writer", "ui-ux-designer",
	}

	deployed := deployedAgentDir(t)
	entries, err := os.ReadDir(deployed)
	if err != nil {
		t.Fatalf("membaca .claude/agents/: %v", err)
	}
	var found []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			found = append(found, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	sort.Strings(found)

	if strings.Join(found, ",") != strings.Join(want, ",") {
		t.Errorf(".claude/agents/ memuat %v, seharusnya %v (Q10, ADR-006 #5)", found, want)
	}

	for _, role := range want {
		a, err := os.ReadFile(filepath.Join(templatesAgentDir(t), role+".md"))
		if err != nil {
			t.Errorf("membaca template %s: %v", role, err)
			continue
		}
		b, err := os.ReadFile(filepath.Join(deployed, role+".md"))
		if err != nil {
			t.Errorf("membaca deployment %s: %v", role, err)
			continue
		}
		if string(a) != string(b) {
			t.Errorf("%s: .claude/agents/ menyimpang dari templates/agents/ — salin ulang templatnya", role)
		}
	}
}

// TestArchitectureConstraintsPresent — H-04 (phase-8-hardening.md).
//
// Kesalahan Phase 7 (PR agent → main, branch planning agent/*) semuanya karena
// section arsitektur tidak dimuat ke konteks agent sebelum bekerja. Setiap role
// wajib memuat blok Architecture Constraints yang mendaftar section yang HARUS
// dibaca sebelum mulai — termasuk code-reviewer dan project-manager yang bukan
// writerRole tetapi tetap bekerja di dalam batas arsitektur yang sama.
func TestArchitectureConstraintsPresent(t *testing.T) {
	const marker = "## Architecture Constraints (wajib baca sebelum kerja)"
	for role, doc := range allAgents(t) {
		if !strings.Contains(doc.body, marker) {
			t.Errorf("%s: tidak memuat blok %q (H-04)", role, marker)
		}
	}
}

// TestAgentBoundariesAreDistinct adalah pembuktian kriteria Done §57:
// "setiap agent menunjukkan tool dan path boundary yang berbeda."
//
// Menulis tiga belas berkas serupa secara berurutan membuat penyalinan tanpa
// penyesuaian menjadi kegagalan yang paling mungkin terjadi — dan kegagalan itu
// tidak terlihat saat dibaca, karena setiap berkas tampak masuk akal
// sendiri-sendiri.
//
// Setiap pasangan role wajib berbeda pada sekurangnya satu dari: tool set,
// permissionMode, atau blok writable paths.
func TestAgentBoundariesAreDistinct(t *testing.T) {
	agents := allAgents(t)
	roles := allRoles(t)

	signature := func(doc agentDoc) (string, string, string) {
		tools := append([]string(nil), doc.frontmatter.Tools...)
		sort.Strings(tools)
		return strings.Join(tools, ","), doc.frontmatter.PermissionMode, writablePathBlock(t, doc)
	}

	for i := 0; i < len(roles); i++ {
		for j := i + 1; j < len(roles); j++ {
			a, b := agents[roles[i]], agents[roles[j]]
			toolsA, modeA, pathsA := signature(a)
			toolsB, modeB, pathsB := signature(b)

			if toolsA == toolsB && modeA == modeB && pathsA == pathsB {
				t.Errorf("%s dan %s memiliki boundary identik — tool set, permissionMode, dan writable paths sama.\n"+
					"    Done §57 menuntut setiap agent menunjukkan tool dan path boundary yang berbeda.",
					roles[i], roles[j])
			}
		}
	}
}

// writablePathBlock mengambil isi blok kode pertama pada bagian
// "Typical Writable Paths". Role tanpa writable path mengembalikan penanda
// khusus, bukan string kosong, agar dua role tanpa path tidak dianggap identik
// hanya karena keduanya kosong.
func writablePathBlock(t *testing.T, doc agentDoc) string {
	t.Helper()
	idx := strings.Index(doc.body, "## Typical Writable Paths")
	if idx < 0 {
		if strings.Contains(doc.body, "## Writable Paths — tidak ada") {
			return "<tanpa writable path: " + doc.frontmatter.Name + ">"
		}
		t.Errorf("%s: tidak memiliki bagian 'Typical Writable Paths'", doc.frontmatter.Name)
		return "<hilang: " + doc.frontmatter.Name + ">"
	}
	rest := doc.body[idx:]
	start := strings.Index(rest, "```")
	if start < 0 {
		t.Errorf("%s: bagian writable paths tanpa blok kode", doc.frontmatter.Name)
		return "<hilang: " + doc.frontmatter.Name + ">"
	}
	rest = rest[start+3:]
	if nl := strings.Index(rest, "\n"); nl >= 0 {
		rest = rest[nl+1:]
	}
	end := strings.Index(rest, "```")
	if end < 0 {
		t.Errorf("%s: blok kode writable paths tidak ditutup", doc.frontmatter.Name)
		return "<hilang: " + doc.frontmatter.Name + ">"
	}
	return strings.TrimSpace(rest[:end])
}
