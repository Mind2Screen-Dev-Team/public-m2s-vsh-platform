// Package registry mengelola registry reservasi path pada control repository.
//
// Registry adalah satu-satunya otoritas atas siapa yang boleh menulis path apa
// (§30). Ia dibaca dan ditulis HANYA oleh runner — worker tidak pernah
// menyentuhnya, dan `control/reservations/**` termasuk daftar human-only write.
//
// Siklus status mengikuti Q12, bukan teks §30:
//
//	active → reserved-pending-merge → released
//
// Reservasi ditahan sampai merge, bukan dilepas saat PR dibuat. Ini menutup
// A-05: celah antara PR dan merge tempat task lain dapat mereservasi path yang
// sama dan mengedit basis yang sudah berubah.
package registry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Mind2Screen-Dev-Team/public-m2s-vsh-platform/internal/contract"
	"github.com/Mind2Screen-Dev-Team/public-m2s-vsh-platform/internal/pathmatch"
)

// Status reservasi. Nilai harus sama dengan enum reservationStatus pada
// schemas/common.schema.json.
const (
	StatusActive               = "active"
	StatusReservedPendingMerge = "reserved-pending-merge"
	StatusReleased             = "released"
	StatusCancelled            = "cancelled"
)

// holdsPath melaporkan apakah status masih menahan path-nya. Hanya active dan
// reserved-pending-merge yang menghalangi task lain (Q12).
func holdsPath(status string) bool {
	return status == StatusActive || status == StatusReservedPendingMerge
}

// HoldsPath adalah bentuk exported dari holdsPath, dipakai runner review/QA
// untuk memastikan path implementer masih ditahan saat diff diambil.
func HoldsPath(status string) bool { return holdsPath(status) }

// Registry adalah direktori reservasi pada control repository.
type Registry struct {
	dir       string
	validator *contract.Validator
}

// Reservation adalah satu dokumen reservasi beserta lokasi berkasnya.
type Reservation struct {
	Doc  map[string]any
	Path string
}

func (r *Reservation) str(key string) string {
	if v, ok := r.Doc[key].(string); ok {
		return v
	}
	return ""
}

func (r *Reservation) TaskID() string     { return r.str("task_id") }
func (r *Reservation) Repository() string { return r.str("repository") }
func (r *Reservation) Status() string     { return r.str("status") }
func (r *Reservation) OwnerRole() string  { return r.str("owner_role") }
func (r *Reservation) Branch() string     { return r.str("branch") }
func (r *Reservation) PRURL() string      { return r.str("pr_url") }

// Worktree adalah path absolut worktree milik task ini. Selalu di luar
// repository, sesuai Q8.
func (r *Reservation) Worktree() string { return r.str("worktree") }

// ReservedPaths mengembalikan himpunan path yang dikunci reservasi ini.
func (r *Reservation) ReservedPaths() []string {
	raw, _ := r.Doc["reserved_paths"].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// SharedFileOwners memetakan path shared file ke task pemiliknya (R-04).
func (r *Reservation) SharedFileOwners() map[string]string {
	out := map[string]string{}
	raw, _ := r.Doc["shared_file_ownership"].([]any)
	for _, v := range raw {
		e, ok := v.(map[string]any)
		if !ok {
			continue
		}
		p, _ := e["path"].(string)
		owner, _ := e["owner_task_id"].(string)
		if p != "" && owner != "" {
			out[pathmatch.Normalize(p)] = owner
		}
	}
	return out
}

// Open membuka registry pada dir, membuat direktorinya bila belum ada.
func Open(dir string, v *contract.Validator) (*Registry, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("membuat direktori registry %s: %w", dir, err)
	}
	return &Registry{dir: dir, validator: v}, nil
}

// List memuat seluruh reservasi. Berkas yang tidak valid dilaporkan sebagai
// kesalahan, bukan dilewati diam-diam — registry rusak lebih berbahaya
// daripada registry yang menolak bekerja.
func (r *Registry) List() ([]*Reservation, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("membaca %s: %w", r.dir, err)
	}

	var out []*Reservation
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".yaml") || strings.HasPrefix(name, ".") {
			continue
		}
		p := filepath.Join(r.dir, name)
		doc, err := r.validator.Load(p, contract.KindReservation)
		if err != nil {
			return nil, fmt.Errorf("reservasi rusak %s: %w", name, err)
		}
		out = append(out, &Reservation{Doc: doc, Path: p})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TaskID() < out[j].TaskID() })
	return out, nil
}

// Active mengembalikan reservasi yang masih menahan path pada satu repository.
func (r *Registry) Active(repository string) ([]*Reservation, error) {
	all, err := r.List()
	if err != nil {
		return nil, err
	}
	var out []*Reservation
	for _, res := range all {
		if res.Repository() == repository && holdsPath(res.Status()) {
			out = append(out, res)
		}
	}
	return out, nil
}

// Conflict menjelaskan satu tabrakan reservasi.
type Conflict struct {
	TaskID        string // pemegang reservasi yang sudah ada
	Status        string
	ExistingPath  string
	RequestedPath string
	Reason        string
}

func (c Conflict) Error() string {
	return fmt.Sprintf("%s (status %s) sudah menahan %q yang beririsan dengan %q — %s",
		c.TaskID, c.Status, c.ExistingPath, c.RequestedPath, c.Reason)
}

// CheckConflicts memeriksa permintaan reservasi terhadap seluruh reservasi
// aktif pada repository yang sama.
//
// Aturan yang ditegakkan (path-overlap-matrix.md):
//   - overlap glob dianggap konflik, termasuk parent/child (§4.2)
//   - shared file hanya boleh satu owner aktif (§4.6, R-04)
//
// Reservasi milik taskID yang sama dilewati, sehingga operasi bersifat idempotent.
func (r *Registry) CheckConflicts(taskID, repository string, requested []string, sharedOwners map[string]string) ([]Conflict, error) {
	active, err := r.Active(repository)
	if err != nil {
		return nil, err
	}

	var conflicts []Conflict
	for _, ex := range active {
		if ex.TaskID() == taskID {
			continue // reservasi milik sendiri
		}

		if a, b, found := pathmatch.AnyOverlap(ex.ReservedPaths(), requested); found {
			conflicts = append(conflicts, Conflict{
				TaskID:        ex.TaskID(),
				Status:        ex.Status(),
				ExistingPath:  a,
				RequestedPath: b,
				Reason:        "overlap path",
			})
			continue
		}

		// Shared file: dua task tidak boleh mengklaim owner berbeda atas path
		// yang sama, meski reserved_paths keduanya tidak beririsan.
		exOwners := ex.SharedFileOwners()
		for p, owner := range sharedOwners {
			if exOwner, ok := exOwners[p]; ok && exOwner != owner {
				conflicts = append(conflicts, Conflict{
					TaskID:        ex.TaskID(),
					Status:        ex.Status(),
					ExistingPath:  p,
					RequestedPath: p,
					Reason: fmt.Sprintf("shared file diklaim owner berbeda (%s vs %s) — §29.6 single owner",
						exOwner, owner),
				})
			}
		}
	}
	return conflicts, nil
}

// Put menulis reservasi ke registry setelah memvalidasinya terhadap schema.
//
// Penulisan bersifat atomik lewat rename agar tidak pernah ada berkas
// reservasi yang setengah tertulis.
func (r *Registry) Put(doc map[string]any) error {
	taskID, _ := doc["task_id"].(string)
	if taskID == "" {
		return errors.New("reservasi tanpa task_id")
	}
	dest := filepath.Join(r.dir, taskID+".yaml")

	if err := r.validator.Validate(doc, contract.KindReservation, dest); err != nil {
		return err
	}
	return writeYAMLAtomic(doc, dest)
}

// Get memuat satu reservasi berdasarkan task ID.
func (r *Registry) Get(taskID string) (*Reservation, error) {
	p := filepath.Join(r.dir, taskID+".yaml")
	doc, err := r.validator.Load(p, contract.KindReservation)
	if err != nil {
		return nil, err
	}
	return &Reservation{Doc: doc, Path: p}, nil
}

// Transition memindahkan status reservasi, menegakkan urutan yang sah.
//
// Transisi yang diizinkan (Q12):
//
//	active → reserved-pending-merge → released
//	active → cancelled
//	reserved-pending-merge → cancelled
//
// Melepas reservasi menuntut releasedBy bernilai "runner" atau "human";
// worker tidak boleh membersihkan reservasi (§30).
func (r *Registry) Transition(taskID, to string, fields map[string]any) error {
	res, err := r.Get(taskID)
	if err != nil {
		return err
	}
	from := res.Status()

	allowed := map[string][]string{
		StatusActive:               {StatusReservedPendingMerge, StatusCancelled},
		StatusReservedPendingMerge: {StatusReleased, StatusCancelled},
	}
	ok := false
	for _, t := range allowed[from] {
		if t == to {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("transisi %s → %s tidak diizinkan untuk %s", from, to, taskID)
	}

	doc := res.Doc
	doc["status"] = to
	for k, v := range fields {
		doc[k] = v
	}

	if to == StatusReleased || to == StatusCancelled {
		if _, has := doc["released_at"]; !has {
			doc["released_at"] = time.Now().Format(time.RFC3339)
		}
		by, _ := doc["released_by"].(string)
		if by != "runner" && by != "human" {
			return fmt.Errorf("released_by harus runner atau human, dapat %q — worker tidak boleh melepas reservasi (§30)", by)
		}
	}

	// Schema menolak active yang sudah memuat jejak pelepasan; Put memvalidasi
	// ulang sehingga transisi tidak dapat menghasilkan dokumen yang tidak sah.
	return r.Put(doc)
}
