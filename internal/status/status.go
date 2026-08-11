// Package status mengelola status task §33 pada control repository.
//
// Satu berkas per task: control/tasks/status/<task-id>.yaml (ADR-011). Berkas
// adalah snapshot status terakhir, bukan riwayat — riwayat transisi dicatat
// schema terpisah task-state.schema.json yang merupakan dokumen referensi.
//
// Status ditulis oleh mekanisme yang menyelesaikan tahap (ADR-011, hybrid):
//   - runner menulis status deterministic (reserved, running, reviewing,
//     merged, released) pada titik m2s yang sudah ada, memakai by = role
//     pemilik task; dan
//   - agent menulis status judgement (implementation-complete, defect-found,
//     dst) lewat subcommand m2s update-status, memakai by = role agent.
//
// Satu penulis per status (prinsip #4): tabel owner ADR-011 ditegakkan di
// sini via CanWrite, bukan di prompt. Runner tidak lewat CanWrite — runner
// adalah satu-satunya penulis yang dipercaya menulis status runner-owned.
package status

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Mind2Screen-Dev-Team/public-m2s-vsh-platform/internal/contract"
	"gopkg.in/yaml.v3"
)

// Nilai taskStatus yang menjadi terminal. Task pada status ini tidak berlanjut
// tanpa task baru (§33). released terminal sukses; sisanya status berhenti.
func IsTerminal(s string) bool {
	switch s {
	case "released", "cancelled", "failed", "superseded", "blocked":
		return true
	}
	return false
}

// forward adalah rantai maju state machine §33. Hanya transisi satu langkah ke
// depan yang diizinkan; lompatan (mis. running → merge-ready) ditolak.
var forward = map[string][]string{
	"draft":                         {"needs-business-clarification"},
	"needs-business-clarification":  {"analysis-ready"},
	"analysis-ready":                {"technical-analysis"},
	"technical-analysis":            {"needs-technical-clarification", "technical-ready"},
	"needs-technical-clarification": {"technical-ready"},
	"technical-ready":               {"reserved"},
	"reserved":                      {"running"},
	"running":                       {"implementation-complete"},
	"implementation-complete":       {"reviewing"},
	"reviewing":                     {"changes-requested", "qa-testing"},
	"changes-requested":             {"running"},
	"qa-testing":                    {"defect-found", "ci-passed"},
	"defect-found":                  {"running"},
	"ci-passed":                     {"merge-ready"},
	"merge-ready":                   {"merged"},
	"merged":                        {"documented"},
	"documented":                    {"staging-verified"},
	"staging-verified":              {"released"},
}

// TransitionAllowed menegakkan state machine §33.
//
// Aturan:
//   - transisi ke status yang sama diizinkan (no-op idempotent);
//   - dari status terminal tidak ada transisi keluar;
//   - ke released hanya dari fase pasca-merge (merge-ready, merged, documented,
//     staging-verified) — release adalah operasi reservasi yang terjadi setelah
//     merge (Q12), bukan jalan pintas dari tengah pengerjaan;
//   - ke terminal berhenti lain (cancelled, failed, superseded, blocked)
//     diizinkan dari status hidup mana pun — task dapat berhenti di tahap mana;
//   - selain itu hanya satu langkah maju pada rantai §33.
func TransitionAllowed(from, to string) bool {
	if from == to {
		return true
	}
	if IsTerminal(from) {
		return false
	}
	switch to {
	case "released":
		switch from {
		case "merge-ready", "merged", "documented", "staging-verified":
			return true
		}
		return false
	case "cancelled", "failed", "superseded", "blocked":
		return true
	}
	for _, n := range forward[from] {
		if n == to {
			return true
		}
	}
	return false
}

// CanWrite menegakkan tabel owner ADR-011 (prinsip #4): role mana yang boleh
// menulis status mana lewat m2s update-status. Status runner-owned (reserved,
// running, reviewing, ci-passed, merged, released) tidak dapat ditulis agen
// mana pun — satu-satunya penulisnya adalah runner.
func CanWrite(by, to string) bool {
	switch to {
	case "draft", "technical-ready":
		return by == "technical-lead-system-analyst"
	case "needs-business-clarification", "analysis-ready",
		"technical-analysis", "needs-technical-clarification":
		return by == "project-manager"
	case "implementation-complete":
		return isWriterRole(by)
	case "changes-requested":
		return by == "code-reviewer"
	case "qa-testing":
		return by == "qa-engineer"
	case "defect-found":
		return by == "qa-engineer"
	case "merge-ready":
		return by == "qa-engineer" || by == "project-manager"
	case "documented":
		return by == "technical-writer"
	case "staging-verified":
		return by == "devops-release"
	case "cancelled", "failed", "superseded", "blocked":
		return by == "project-manager"
	default:
		// reserved, running, reviewing, ci-passed, merged, released:
		// runner-only — tidak ada agen yang berhak menulisnya (prinsip #4).
		return false
	}
}

// isWriterRole melaporkan apakah role adalah implementer (writerRole pada
// schemas/common.schema.json). Implementer adalah satu-satunya yang dapat
// menyatakan implementation-complete (ADR-011, tabel owner).
func isWriterRole(by string) bool {
	switch by {
	case "technical-lead-system-analyst", "ui-ux-designer", "backend-engineer",
		"frontend-engineer", "qa-engineer", "devops-release", "technical-writer",
		"fullstack-engineer", "mobile-engineer", "android-developer", "ios-developer":
		return true
	}
	return false
}

// FromReservationStatus memetakan status reservasi (internal/registry) ke
// taskStatus §33, menutup dua bahasa state yang paralel (ADR-011 #4):
//
//	active              → reserved
//	reserved-pending-merge → merge-ready
//	released            → released
//	cancelled           → cancelled
//
// Nilai tak dikenal mengembalikan "".
func FromReservationStatus(reservationStatus string) string {
	switch reservationStatus {
	case "active":
		return "reserved"
	case "reserved-pending-merge":
		return "merge-ready"
	case "released":
		return "released"
	case "cancelled":
		return "cancelled"
	}
	return ""
}

// Status adalah satu snapshot status task beserta lokasi berkasnya.
type Status struct {
	Doc  map[string]any
	Path string
}

func (s *Status) str(key string) string {
	if v, ok := s.Doc[key].(string); ok {
		return v
	}
	return ""
}

func (s *Status) TaskID() string { return s.str("task_id") }
func (s *Status) Status() string { return s.str("status") }
func (s *Status) By() string     { return s.str("by") }

// Store adalah direktori status pada control repository.
type Store struct {
	dir       string
	validator *contract.Validator
}

// Open membuka store pada dir, membuat direktori bila belum ada.
func Open(dir string, v *contract.Validator) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("membuat direktori status %s: %w", dir, err)
	}
	return &Store{dir: dir, validator: v}, nil
}

// Read memuat status terakhir satu task. Berkas yang belum ada mengembalikan
// error yang membungkus os.ErrNotExist — pemanggil dapat memakai errors.Is.
func (s *Store) Read(taskID string) (*Status, error) {
	p := filepath.Join(s.dir, taskID+".yaml")
	doc, err := s.validator.Load(p, contract.KindTaskStatus)
	if err != nil {
		return nil, err
	}
	return &Status{Doc: doc, Path: p}, nil
}

// Write menimpa snapshot status task. Memvalidasi terhadap
// task-status.schema.json lalu menulis secara atomik (temp + rename) agar
// pembaca tidak pernah melihat berkas setengah tertulis.
//
// extra memuat properti tambahan (mis. pr_url) yang diizinkan schema.
func (s *Store) Write(taskID, statusVal, by, reason string, extra map[string]any) error {
	doc := map[string]any{
		"schema_version": "1.0",
		"task_id":        taskID,
		"status":         statusVal,
		"updated_at":     time.Now().Format(time.RFC3339),
		"by":             by,
	}
	if reason != "" {
		doc["reason"] = reason
	}
	for k, v := range extra {
		doc[k] = v
	}

	if err := s.validator.Validate(doc, contract.KindTaskStatus, filepath.Join(s.dir, taskID+".yaml")); err != nil {
		return err
	}
	return writeYAMLAtomic(doc, filepath.Join(s.dir, taskID+".yaml"))
}

// writeYAMLAtomic menulis dokumen sebagai YAML lewat berkas sementara dan
// rename, sehingga pembaca tidak pernah melihat berkas setengah tertulis.
// Salinan lokal dari internal/registry — paket tidak boleh bergantung lintas
// internal yang memperluas muka publik registry.
func writeYAMLAtomic(doc map[string]any, dest string) error {
	b, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("menulis YAML: %w", err)
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("menulis %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("memindahkan ke %s: %w", dest, err)
	}
	return nil
}
