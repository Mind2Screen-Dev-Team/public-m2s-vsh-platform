package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Mind2Screen-Dev-Team/public-m2s-vsh-platform/internal/contract"
	"github.com/Mind2Screen-Dev-Team/public-m2s-vsh-platform/internal/pathmatch"
	"github.com/Mind2Screen-Dev-Team/public-m2s-vsh-platform/internal/registry"
	"github.com/Mind2Screen-Dev-Team/public-m2s-vsh-platform/internal/status"
)

const lockTimeout = 30 * time.Second

// setup memuat validator, registry, dan store status dari akar control
// repository.
func setup(control string) (*contract.Validator, *registry.Registry, *status.Store, error) {
	v, err := contract.NewValidator(filepath.Join(control, "schemas"))
	if err != nil {
		return nil, nil, nil, err
	}
	reg, err := registry.Open(filepath.Join(control, "control", "reservations"), v)
	if err != nil {
		return nil, nil, nil, err
	}
	st, err := status.Open(filepath.Join(control, "control", "tasks", "status"), v)
	if err != nil {
		return nil, nil, nil, err
	}
	return v, reg, st, nil
}

// writeStatus menulis status task §33 dengan validasi transisi terhadap status
// yang ada (ADR-011). Runner menulis status deterministic di titik yang sudah
// ada; by selalu role pemilik task.
//
// Idempotent: transisi ke status yang sama tidak mengubah berkas (prinsip
// runner). Berkas yang belum ada diizinkan tanpa cek transisi dari.
func writeStatus(st *status.Store, taskID, to, by string, extra map[string]any) error {
	cur, err := st.Read(taskID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return st.Write(taskID, to, by, "", extra)
		}
		return err
	}
	from := cur.Status()
	if from == to {
		return nil // no-op
	}
	if !status.TransitionAllowed(from, to) {
		return fmt.Errorf("transisi %s → %s tidak diizinkan (state machine §33)", from, to)
	}
	return st.Write(taskID, to, by, "", extra)
}

// scaffoldFiles mendaftar berkas yang scaffolding sebuah stack PASTI hasilkan.
//
// H-03 (phase-8-hardening.md): Phase 7 menulis contract yang melarang go.mod
// dan src/app/layout.tsx, padahal `go mod init` dan `create-next-app` wajib
// membuatnya. Agent lalu berada di antara menaati contract atau menyelesaikan
// task, dan keduanya salah. Daftar ini membuat contract semacam itu ditolak
// sebelum agent mulai.
var scaffoldFiles = map[string][]string{
	"go":     {"go.mod"},
	"nextjs": {"src/app/layout.tsx", "src/app/globals.css"},
}

// checkScaffoldRealism memeriksa paths contract terhadap execution.scaffold.
//
// Opt-in: task tanpa execution.scaffold bukan task scaffolding dan tidak
// diperiksa, sehingga task pada repo yang sudah berdiri tetap boleh melarang
// go.mod — itu batas yang benar di sana.
//
// Mengembalikan daftar pelanggaran; kosong berarti lolos.
func checkScaffoldRealism(task map[string]any) []string {
	stack := str(task, "execution.scaffold")
	if stack == "" {
		return nil
	}
	want, ok := scaffoldFiles[stack]
	if !ok {
		return nil // enum schema sudah membatasi nilainya
	}

	allowed := strSlice(task, "paths.allowed")
	forbidden := strSlice(task, "paths.forbidden")

	var out []string
	for _, f := range want {
		// IsAllowed sekaligus menutup dua bentuk kegagalan: berkas tidak
		// tercakup allowed, dan berkas tercakup allowed tetapi ditutup
		// forbidden (forbidden mengalahkan allowed, matriks §4.8).
		if !pathmatch.IsAllowed(f, allowed, forbidden) {
			out = append(out, fmt.Sprintf(
				"scaffolding %s wajib menghasilkan %s, tetapi paths contract tidak mengizinkannya — agent akan terjepit antara menaati contract dan menyelesaikan task (H-03, §29.6)",
				stack, f))
		}
	}
	return out
}

// checkContractIDsExist memastikan tiap contract_ids yang dirujuk benar-benar
// ada sebagai spec di control repository.
//
// H-05/H-06: task yang menunjuk contract hilang lolos launch, lalu agent
// bekerja tanpa dokumen yang seharusnya mengikatnya.
func checkContractIDsExist(task map[string]any, control string) []string {
	var out []string
	for _, id := range strSlice(task, "task.contract_ids") {
		p := filepath.Join(control, "control", "tasks", "specifications", id+".yaml")
		if _, err := os.Stat(p); err != nil {
			out = append(out, fmt.Sprintf(
				"contract_ids memuat %s, tetapi %s tidak ada — task tidak boleh mulai tanpa contract yang dirujuknya (H-06)",
				id, p))
		}
	}
	return out
}

// --- validate-task ---

func cmdValidateTask(args []string) int {
	fs := newFlagSet("validate-task")
	taskPath := fs.String("task", "", "path task contract (.yaml)")
	controlFlag := fs.String("control", "", "akar control repository")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *taskPath == "" {
		return fail(exitError, "-task wajib diisi")
	}

	control, err := controlRoot(*controlFlag)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	v, _, _, err := setup(control)
	if err != nil {
		return fail(exitError, "%v", err)
	}

	doc, err := v.Load(*taskPath, contract.KindTask)
	if err != nil {
		if ve, ok := err.(*contract.ValidationError); ok {
			reportViolations(ve.Error(), ve.Violations)
			return exitViolation
		}
		return fail(exitError, "%v", err)
	}

	// Batas yang tidak dapat ditegakkan schema: merge ke main hanya manusia
	// (ADR-001 #2). Schema menerima base_branch main karena ia tidak tahu
	// siapa yang menjalankan merge; runner yang menolaknya.
	if base := str(doc, "ownership.base_branch"); base == "main" {
		reportViolations("task ditolak", []string{
			"ownership.base_branch = main — agent tidak boleh menargetkan main (ADR-001 #2)",
		})
		return exitViolation
	}

	// H-03 + H-05: pemeriksaan pra-launch. Contract yang sudah pasti membuat
	// task gagal ditolak SEBELUM agent mulai, bukan setelah kerja habis di CI.
	var violations []string
	violations = append(violations, checkScaffoldRealism(doc)...)
	violations = append(violations, checkContractIDsExist(doc, control)...)
	if len(violations) > 0 {
		reportViolations("task ditolak", violations)
		return exitViolation
	}

	fmt.Printf("ok  %s valid — %s pada %s\n",
		str(doc, "task.id"), str(doc, "ownership.role"), str(doc, "ownership.repository"))
	return exitOK
}

// --- reserve-paths ---

func cmdReservePaths(args []string) int {
	fs := newFlagSet("reserve-paths")
	taskPath := fs.String("task", "", "path task contract (.yaml)")
	controlFlag := fs.String("control", "", "akar control repository")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *taskPath == "" {
		return fail(exitError, "-task wajib diisi")
	}

	control, err := controlRoot(*controlFlag)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	v, reg, st, err := setup(control)
	if err != nil {
		return fail(exitError, "%v", err)
	}

	task, err := v.Load(*taskPath, contract.KindTask)
	if err != nil {
		if ve, ok := err.(*contract.ValidationError); ok {
			reportViolations(ve.Error(), ve.Violations)
			return exitViolation
		}
		return fail(exitError, "%v", err)
	}

	taskID := str(task, "task.id")
	repo := str(task, "ownership.repository")
	allowed := strSlice(task, "paths.allowed")

	wtRoot, err := worktreeRoot()
	if err != nil {
		return fail(exitError, "%v", err)
	}

	// Kunci menahan seluruh siklus periksa-lalu-tulis. Tanpa ini dua runner
	// dapat memeriksa konflik terhadap keadaan yang sama lalu sama-sama
	// menulis reservasi yang beririsan.
	lock, err := reg.Acquire("reserve-paths "+taskID, lockTimeout)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	defer lock.Release()

	sharedOwners := map[string]string{}
	if raw, ok := task["shared_file_ownership"].([]any); ok {
		for _, e := range raw {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			p, _ := m["path"].(string)
			owner, _ := m["owner_task_id"].(string)
			if p != "" && owner != "" {
				sharedOwners[p] = owner
			}
		}
	}

	conflicts, err := reg.CheckConflicts(taskID, repo, allowed, sharedOwners)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	if len(conflicts) > 0 {
		msgs := make([]string, len(conflicts))
		for i, c := range conflicts {
			msgs[i] = c.Error()
		}
		reportViolations(fmt.Sprintf("reservasi %s ditolak", taskID), msgs)
		return exitViolation
	}

	doc := map[string]any{
		"schema_version":  "1.0",
		"task_id":         taskID,
		"repository":      repo,
		"branch":          str(task, "ownership.branch"),
		"worktree":        filepath.Join(wtRoot, repo, taskID),
		"allowed_paths":   toAny(allowed),
		"reserved_paths":  toAny(allowed),
		"forbidden_paths": toAny(strSlice(task, "paths.forbidden")),
		"status":          registry.StatusActive,
		"owner_role":      str(task, "ownership.role"),
		"created_at":      time.Now().Format(time.RFC3339),
	}
	if raw, ok := task["shared_file_ownership"]; ok {
		doc["shared_file_ownership"] = raw
	}

	// Idempotent: reservasi aktif milik task yang sama dipertahankan apa
	// adanya agar created_at tidak bergeser saat perintah diulang.
	if existing, gerr := reg.Get(taskID); gerr == nil && existing.Status() == registry.StatusActive {
		fmt.Printf("ok  %s sudah direservasi (tidak ada perubahan)\n", taskID)
		return exitOK
	}

	if err := reg.Put(doc); err != nil {
		if ve, ok := err.(*contract.ValidationError); ok {
			reportViolations(ve.Error(), ve.Violations)
			return exitViolation
		}
		return fail(exitError, "%v", err)
	}

	// ADR-011: runner tulis status deterministic. active → reserved (by = role
	// pemilik task). Idempotent: transisi ke status yang sama tidak mengubah.
	if err := writeStatus(st, taskID, "reserved", str(task, "ownership.role"), nil); err != nil {
		return fail(exitError, "%v", err)
	}

	fmt.Printf("ok  %s direservasi — %d path pada %s\n", taskID, len(allowed), repo)
	return exitOK
}

// --- launch-task ---

func cmdLaunchTask(args []string) int {
	fs := newFlagSet("launch-task")
	taskPath := fs.String("task", "", "path task contract (.yaml)")
	repoPath := fs.String("repo", "", "path repository target")
	controlFlag := fs.String("control", "", "akar control repository")
	dryRun := fs.Bool("dry-run", false, "siapkan worktree tanpa menjalankan agent")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *taskPath == "" || *repoPath == "" {
		return fail(exitError, "-task dan -repo wajib diisi")
	}

	control, err := controlRoot(*controlFlag)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	v, reg, st, err := setup(control)
	if err != nil {
		return fail(exitError, "%v", err)
	}

	task, err := v.Load(*taskPath, contract.KindTask)
	if err != nil {
		if ve, ok := err.(*contract.ValidationError); ok {
			reportViolations(ve.Error(), ve.Violations)
			return exitViolation
		}
		return fail(exitError, "%v", err)
	}
	taskID := str(task, "task.id")

	// Prasyarat platform diturunkan otomatis dari role sebelum diperiksa
	// (ADR-006 pertanyaan terbuka, ditutup Phase 3 §59). Task ios-developer
	// wajib platform darwin karena xcodebuild hanya tersedia di macOS; membuat
	// keharusan ini eksplisit di runner mencegah task ios lolos launch lalu
	// gagal jauh kemudian pada quality_gates di mesin yang salah.
	//
	// Ini penurunan, bukan penolakan: platform kosong/any pada role ios
	// diperlakukan sebagai darwin, sedangkan platform yang bertentangan
	// (linux) tetap ditolak sebagai kontrak yang salah.
	role := str(task, "ownership.role")
	wantPlatform := str(task, "execution.platform")
	if role == "ios-developer" {
		switch wantPlatform {
		case "", "any", "darwin":
			wantPlatform = "darwin"
		default:
			return fail(exitViolation,
				"%s berrole ios-developer menuntut platform darwin, tetapi contract menyatakan %s (ADR-006 §59)",
				taskID, wantPlatform)
		}
	}

	// Prasyarat platform diperiksa sebelum reservasi dan worktree disentuh
	// (ADR-006 #3). Menolak lebih awal berarti tidak ada worktree yatim yang
	// harus dibersihkan manual ketika runner-nya memang salah mesin.
	//
	// Ini kontrak yang ditolak, bukan runner yang rusak — karena itu
	// exitViolation, bukan exitError.
	if want := wantPlatform; want != "" && want != "any" {
		if want != runtime.GOOS {
			return fail(exitViolation,
				"%s menuntut platform %s, sedangkan runner berjalan pada %s",
				taskID, want, runtime.GOOS)
		}
	}

	// ── Preflight Phase 8 ────────────────────────────────────────────────
	//
	// Seluruhnya mendahului reservasi dan `git worktree add`, mengikuti pola
	// pemeriksaan platform di atas: penolakan tidak boleh meninggalkan worktree
	// yatim yang harus dibersihkan manual.

	// H-07: gate TL/SA. `technical-ready` adalah status §33 yang menandai
	// analisis teknis selesai dan disetujui — sign-off yang blueprint sebut
	// `approved`. Nilai itu tidak ada pada enum taskStatus, dan menambahkannya
	// akan meriakkan perubahan ke seluruh spec, test enum, dan dokumen state
	// machine tanpa menambah penegakan.
	if st := str(task, "task.status"); st != "technical-ready" {
		return fail(exitViolation,
			"%s berstatus %s — launch menuntut technical-ready sebagai sign-off TL/SA (H-07, §33)",
			taskID, st)
	}

	// H-06: contract yang dirujuk wajib ada. Lapis kedua — validate-task
	// memeriksa hal yang sama, tetapi launch tidak boleh bergantung pada
	// pemanggil yang menjalankannya lebih dulu.
	if v := checkContractIDsExist(task, control); len(v) > 0 {
		reportViolations(fmt.Sprintf("launch %s ditolak", taskID), v)
		return exitViolation
	}

	// H-03 lapis kedua, alasan yang sama.
	if v := checkScaffoldRealism(task); len(v) > 0 {
		reportViolations(fmt.Sprintf("launch %s ditolak", taskID), v)
		return exitViolation
	}

	// H-05: base branch wajib ada di repo target. Diperiksa di sini, bukan di
	// validate-task, karena hanya launch yang menerima -repo — validate-task
	// tidak tahu di mana repository berada.
	//
	// Dijalankan runner, bukan agent, sehingga blocklist Bash agent tidak
	// berlaku — pola yang sama dipakai `git worktree add` di bawah.
	if base := str(task, "ownership.base_branch"); base != "" {
		cmd := exec.Command("git", "-C", *repoPath, "show-ref", "--verify",
			"refs/heads/"+base)
		if err := cmd.Run(); err != nil {
			return fail(exitViolation,
				"base_branch %s tidak ada di %s — worktree tidak dapat dicabangkan dari branch yang belum ada (H-05)",
				base, *repoPath)
		}
	}

	// Worktree hanya dibuat bila reservasi sudah ada dan masih aktif.
	// Urutan ini berasal dari Q13: validasi contract dan reservasi mendahului
	// git worktree add.
	res, err := reg.Get(taskID)
	if err != nil {
		return fail(exitViolation, "%s belum memiliki reservasi — jalankan reserve-paths lebih dulu", taskID)
	}
	if res.Status() != registry.StatusActive {
		return fail(exitViolation, "reservasi %s berstatus %s, bukan active", taskID, res.Status())
	}

	worktree := res.Worktree()
	branch := str(task, "ownership.branch")
	base := str(task, "ownership.base_branch")

	// Jaminan Q8/A-01 yang tidak dapat diungkapkan schema: worktree harus
	// berada DI LUAR repository. Schema tidak mengetahui lokasi repository,
	// sehingga pemeriksaan ini hanya mungkin di sini.
	//
	// Worktree di dalam repo membuat akar kerja agent berada pada pohon yang
	// forbidden_paths-nya berlaku, dan berisiko ikut ter-commit.
	if inside, err := isInside(worktree, *repoPath); err != nil {
		return fail(exitError, "%v", err)
	} else if inside {
		return fail(exitViolation,
			"worktree %s berada di dalam repository %s — Q8 menuntut worktree di luar repo (A-01)",
			worktree, *repoPath)
	}

	// Runner yang menjalankan git worktree, bukan agent (Q13). Hook agent
	// tidak berlaku pada proses ini karena ia berada di luar sesi agent.
	if _, err := os.Stat(worktree); os.IsNotExist(err) {
		cmd := exec.Command("git", "-C", *repoPath, "worktree", "add", worktree, "-b", branch, base)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fail(exitError, "git worktree add gagal: %v\n%s", err, out)
		}
		fmt.Printf("ok  worktree dibuat: %s (branch %s dari %s)\n", worktree, branch, base)
	} else {
		fmt.Printf("ok  worktree sudah ada: %s\n", worktree)
	}

	// Materialisasi contract sebagai snapshot read-only (Q15). Hook membaca
	// berkas lokal ini, bukan registry lintas repository.
	dest := filepath.Join(worktree, ".task", "contract.json")
	if err := contract.MaterializeJSON(task, dest); err != nil {
		return fail(exitError, "%v", err)
	}
	fmt.Printf("ok  contract dimaterialisasi: %s\n", dest)

	// Snapshot berada di dalam worktree, sehingga `git add -A` milik agent
	// akan ikut men-stage-nya. Itu bertentangan dengan .task/** yang justru
	// ada pada forbidden_paths.
	//
	// Pengabaian ditulis ke .git/info/exclude, bukan .gitignore: berkas itu
	// milik worktree dan tidak pernah ter-commit, sehingga repo aplikasi tidak
	// perlu mengetahui detail runner.
	if err := excludeTaskDir(worktree); err != nil {
		return fail(exitError, "%v", err)
	}

	// ADR-011: reserved → running begitu worktree siap. Ditulis sebelum dry-run
	// agar dry-run pun (launch sukses) menandai task hidup — state machine
	// reserved → running adalah prasyarat collect-result berikutnya.
	if err := writeStatus(st, taskID, "running", str(task, "ownership.role"), nil); err != nil {
		return fail(exitError, "%v", err)
	}

	if *dryRun {
		fmt.Printf("ok  dry-run — sesi agent tidak dijalankan\n")
		return exitOK
	}

	fmt.Printf("siap  jalankan agent %s dengan cwd %s\n", str(task, "ownership.role"), worktree)
	return exitOK
}

// --- collect-result ---

func cmdCollectResult(args []string) int {
	fs := newFlagSet("collect-result")
	handoffPath := fs.String("handoff", "", "path handoff (.yaml)")
	controlFlag := fs.String("control", "", "akar control repository")
	prURL := fs.String("pr", "", "URL pull request; memindahkan reservasi ke reserved-pending-merge")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *handoffPath == "" {
		return fail(exitError, "-handoff wajib diisi")
	}

	control, err := controlRoot(*controlFlag)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	v, reg, st, err := setup(control)
	if err != nil {
		return fail(exitError, "%v", err)
	}

	doc, err := v.Load(*handoffPath, contract.KindHandoff)
	if err != nil {
		if ve, ok := err.(*contract.ValidationError); ok {
			reportViolations(ve.Error(), ve.Violations)
			return exitViolation
		}
		return fail(exitError, "%v", err)
	}

	taskID, _ := doc["task_id"].(string)
	handoffStatus, _ := doc["status"].(string)
	role, _ := doc["role"].(string)
	fmt.Printf("ok  handoff %s valid — status %s\n", taskID, handoffStatus)

	// ADR-011: runner menulis status yang dilaporkan agent pada handoff
	// (implementation-complete, blocked, failed, dst). Status runner-owned
	// (reviewing) tidak ditulis dari sini — itu transisi setelah PR terbuka,
	// ditangani collect-review (ADR-012) agar gate implementation-complete
	// tetap berlaku.
	if handoffStatus != "" {
		if err := writeStatus(st, taskID, handoffStatus, role, nil); err != nil {
			return fail(exitError, "%v", err)
		}
	}

	if *prURL == "" {
		return exitOK
	}

	// Reservasi TIDAK dilepas di sini. Q12: ia ditahan sampai merge, hanya
	// berpindah ke reserved-pending-merge. Melepasnya sekarang membuka
	// kembali celah A-05.
	lock, err := reg.Acquire("collect-result "+taskID, lockTimeout)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	defer lock.Release()

	if err := reg.Transition(taskID, registry.StatusReservedPendingMerge, map[string]any{
		"pr_url": *prURL,
	}); err != nil {
		return fail(exitError, "%v", err)
	}
	fmt.Printf("ok  reservasi %s → reserved-pending-merge (masih menahan path sampai merge)\n", taskID)
	return exitOK
}

// --- release-reservation ---

func cmdReleaseReservation(args []string) int {
	fs := newFlagSet("release-reservation")
	taskID := fs.String("task-id", "", "task ID pemilik reservasi")
	controlFlag := fs.String("control", "", "akar control repository")
	by := fs.String("by", "runner", "pelepas: runner atau human")
	cancel := fs.Bool("cancel", false, "batalkan alih-alih melepas setelah merge")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *taskID == "" {
		return fail(exitError, "-task-id wajib diisi")
	}
	if *by != "runner" && *by != "human" {
		return fail(exitViolation, "-by harus runner atau human — worker tidak boleh melepas reservasi (§30)")
	}

	control, err := controlRoot(*controlFlag)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	_, reg, st, err := setup(control)
	if err != nil {
		return fail(exitError, "%v", err)
	}

	lock, err := reg.Acquire("release-reservation "+*taskID, lockTimeout)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	defer lock.Release()

	res, err := reg.Get(*taskID)
	if err != nil {
		return fail(exitError, "%v", err)
	}

	// Idempotent: reservasi yang sudah terlepas tidak dianggap kesalahan.
	if s := res.Status(); s == registry.StatusReleased || s == registry.StatusCancelled {
		fmt.Printf("ok  reservasi %s sudah %s (tidak ada perubahan)\n", *taskID, s)
		return exitOK
	}

	target := registry.StatusReleased
	if *cancel {
		target = registry.StatusCancelled
	}

	if err := reg.Transition(*taskID, target, map[string]any{"released_by": *by}); err != nil {
		return fail(exitViolation, "%v", err)
	}

	// ADR-011 #4: sinkron reservasi → §33. Runner menulis status task mengikuti
	// status reservasi (active→reserved, reserved-pending-merge→merge-ready,
	// released→released, cancelled→cancelled). by = role pemilik task.
	//
	// Best-effort: bila transisi task ke status target tidak sah menurut state
	// machine (mis. task masih implementation-complete saat release), release
	// reservasi tetap jalan — release adalah operasi reservasi (Q12), bukan
	// transisi status task. Status task baru maju lewat jalurnya sendiri.
	if to := status.FromReservationStatus(target); to != "" {
		res, gerr := reg.Get(*taskID)
		if gerr == nil {
			if err := writeStatus(st, *taskID, to, res.OwnerRole(), nil); err != nil {
				fmt.Printf("m2s: status task %s tidak disinkronkan (%v) — reservasi tetap %s\n",
					*taskID, err, target)
			}
		}
	}
	fmt.Printf("ok  reservasi %s → %s oleh %s\n", *taskID, target, *by)
	return exitOK
}

// --- launch-review ---

// cmdLaunchReview menyiapkan sesi Code Reviewer (ADR-012 #1).
//
// Gate: status task = implementation-complete. Spawn agent dilakukan
// orchestrator luar (scripts/review.sh) — runner hanya menyiapkan: memeriksa
// gate, membaca reservasi implementer untuk branch/repo, dan mencetak
// instruksi. Code Reviewer read-only (A-03) tidak memegang reservasi baru.
func cmdLaunchReview(args []string) int {
	fs := newFlagSet("launch-review")
	taskID := fs.String("task", "", "task ID")
	controlFlag := fs.String("control", "", "akar control repository")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *taskID == "" {
		return fail(exitError, "-task wajib diisi")
	}

	control, err := controlRoot(*controlFlag)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	_, reg, st, err := setup(control)
	if err != nil {
		return fail(exitError, "%v", err)
	}

	// Gate ADR-012: hanya spawn review bila implementer menyatakan selesai.
	cur, err := st.Read(*taskID)
	if err != nil {
		return fail(exitViolation, "%s — launch-review menuntut implementation-complete", *taskID)
	}
	if cur.Status() != "implementation-complete" {
		return fail(exitViolation,
			"%s berstatus %s — launch-review menuntut implementation-complete (ADR-012)",
			*taskID, cur.Status())
	}

	// Ambil reservasi implementer untuk branch + repo. Reservasi harus masih
	// menahan path (active/pending-merge) agar diff review dapat diambil.
	res, err := reg.Get(*taskID)
	if err != nil {
		return fail(exitViolation, "%s belum memiliki reservasi", *taskID)
	}
	if !registry.HoldsPath(res.Status()) {
		return fail(exitViolation, "reservasi %s berstatus %s — review menuntut path masih ditahan", *taskID, res.Status())
	}

	fmt.Printf("siap  jalankan code-reviewer (read-only) dengan repo %s branch %s worktree %s\n",
		res.Repository(), res.Branch(), res.Worktree())
	return exitOK
}

// --- collect-review ---

// cmdCollectReview menulis hasil review dari handoff Code Reviewer (ADR-012 #1).
//
// Handoff memakai schema handoff.schema.json dengan decision (review-report
// adalah superstructure). Runner menulis status atas nama code-reviewer —
// reviewer read-only tidak memegang file status (Q9, A-03).
func cmdCollectReview(args []string) int {
	fs := newFlagSet("collect-review")
	handoffPath := fs.String("handoff", "", "path handoff review (.yaml)")
	controlFlag := fs.String("control", "", "akar control repository")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *handoffPath == "" {
		return fail(exitError, "-handoff wajib diisi")
	}

	control, err := controlRoot(*controlFlag)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	v, _, st, err := setup(control)
	if err != nil {
		return fail(exitError, "%v", err)
	}

	doc, err := v.Load(*handoffPath, contract.KindHandoff)
	if err != nil {
		if ve, ok := err.(*contract.ValidationError); ok {
			reportViolations(ve.Error(), ve.Violations)
			return exitViolation
		}
		return fail(exitError, "%v", err)
	}

	taskID, _ := doc["task_id"].(string)
	role, _ := doc["role"].(string)
	if role != "code-reviewer" {
		return fail(exitViolation, "collect-review menuntut role code-reviewer, dapat %s", role)
	}
	decision, _ := doc["decision"].(string)

	// Approve / approve-with-nonblocking-notes → reviewing (menunggu QA).
	// request-changes → changes-requested (kembali implementer). by = code-reviewer
	// sesuai tabel owner ADR-011; runner hanya menyalin keputusan reviewer.
	switch decision {
	case "approve", "approve-with-nonblocking-notes":
		if err := writeStatus(st, taskID, "reviewing", role, nil); err != nil {
			return fail(exitViolation, "%v", err)
		}
		fmt.Printf("ok  review %s approve — status reviewing\n", taskID)
	case "request-changes":
		if err := writeStatus(st, taskID, "changes-requested", role, nil); err != nil {
			return fail(exitViolation, "%v", err)
		}
		fmt.Printf("ok  review %s request-changes — status changes-requested\n", taskID)
	default:
		return fail(exitViolation, "decision tak dikenal: %s", decision)
	}
	return exitOK
}

// --- launch-qa ---

// cmdLaunchQA menyiapkan sesi QA Engineer (ADR-012 #2).
//
// Gate: status task = reviewing (review approve, belum QA). Spawn agent oleh
// orchestrator luar. QA read-write, tetapi tanpa reservasi terpisah: worktree
// QA dibuat dari branch PR yang sama, path QA (tests/** dst) tidak beririsan
// dengan reserved_paths implementer (batas dibahas ADR-012 konsekuensi).
func cmdLaunchQA(args []string) int {
	fs := newFlagSet("launch-qa")
	taskID := fs.String("task", "", "task ID")
	controlFlag := fs.String("control", "", "akar control repository")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *taskID == "" {
		return fail(exitError, "-task wajib diisi")
	}

	control, err := controlRoot(*controlFlag)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	_, reg, st, err := setup(control)
	if err != nil {
		return fail(exitError, "%v", err)
	}

	cur, err := st.Read(*taskID)
	if err != nil {
		return fail(exitViolation, "%s — launch-qa menuntut reviewing", *taskID)
	}
	if cur.Status() != "reviewing" {
		return fail(exitViolation,
			"%s berstatus %s — launch-qa menuntut reviewing (ADR-012)", *taskID, cur.Status())
	}

	res, err := reg.Get(*taskID)
	if err != nil {
		return fail(exitViolation, "%s belum memiliki reservasi", *taskID)
	}
	if !registry.HoldsPath(res.Status()) {
		return fail(exitViolation, "reservasi %s berstatus %s — QA menuntut path masih ditahan", *taskID, res.Status())
	}

	// ADR-011: qa-testing ditulis saat QA mulai (reviewing → qa-testing).
	// dari sini QA pass → ci-passed → merge-ready atau defect-found → running.
	if err := writeStatus(st, *taskID, "qa-testing", "qa-engineer", nil); err != nil {
		return fail(exitViolation, "%v", err)
	}

	fmt.Printf("siap  jalankan qa-engineer dengan repo %s branch %s worktree %s\n",
		res.Repository(), res.Branch(), res.Worktree())
	return exitOK
}

// --- collect-qa ---

// cmdCollectQA menulis hasil QA dari handoff (ADR-012 #2).
//
// Handoff QA: status implementation-complete / defect-found + findings. Runner
// menulis status atas nama qa-engineer. Fix loop ADR-012 #5: defect-found →
// running agar implementer lanjut di worktree sama.
func cmdCollectQA(args []string) int {
	fs := newFlagSet("collect-qa")
	handoffPath := fs.String("handoff", "", "path handoff QA (.yaml)")
	controlFlag := fs.String("control", "", "akar control repository")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *handoffPath == "" {
		return fail(exitError, "-handoff wajib diisi")
	}

	control, err := controlRoot(*controlFlag)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	v, _, st, err := setup(control)
	if err != nil {
		return fail(exitError, "%v", err)
	}

	doc, err := v.Load(*handoffPath, contract.KindHandoff)
	if err != nil {
		if ve, ok := err.(*contract.ValidationError); ok {
			reportViolations(ve.Error(), ve.Violations)
			return exitViolation
		}
		return fail(exitError, "%v", err)
	}

	taskID, _ := doc["task_id"].(string)
	role, _ := doc["role"].(string)
	if role != "qa-engineer" {
		return fail(exitViolation, "collect-qa menuntut role qa-engineer, dapat %s", role)
	}
	statusVal, _ := doc["status"].(string)

	switch statusVal {
	case "defect-found":
		// ADR-012 #5: defect → running, implementer perbaiki di worktree sama.
		// Transisi dari qa-testing (ditulis launch-qa) — §33: qa-testing →
		// defect-found → running.
		if err := writeStatus(st, taskID, "defect-found", role, nil); err != nil {
			return fail(exitViolation, "%v", err)
		}
		if err := writeStatus(st, taskID, "running", role, nil); err != nil {
			return fail(exitViolation, "%v", err)
		}
		fmt.Printf("ok  QA %s defect — status running (fix loop ADR-012)\n", taskID)
	case "implementation-complete", "ci-passed":
		// QA pass: qa-testing → ci-passed → merge-ready (qa-testing sudah
		// ditulis launch-qa).
		for _, to := range []string{"ci-passed", "merge-ready"} {
			if err := writeStatus(st, taskID, to, role, nil); err != nil {
				return fail(exitViolation, "%v", err)
			}
		}
		fmt.Printf("ok  QA %s pass — status merge-ready\n", taskID)
	default:
		return fail(exitViolation, "status QA tak dikenal: %s", statusVal)
	}
	return exitOK
}

// --- update-status ---

// cmdUpdateStatus menulis status task §33 lewat agent (ADR-011 opsi B).
//
// Validasi tiga lapis: status anggota enum taskStatus (schema), transisi sah
// dari status saat ini (state machine §33), dan role penulis berhak atas status
// itu (tabel owner ADR-011, prinsip #4). Agent memanggil lewat runner dengan
// identitas role-nya — by tidak pernah "runner".
func cmdUpdateStatus(args []string) int {
	fs := newFlagSet("update-status")
	taskID := fs.String("task", "", "task ID")
	statusVal := fs.String("status", "", "taskStatus target (state machine §33)")
	by := fs.String("by", "", "role yang menulis (tabel owner ADR-011)")
	reason := fs.String("reason", "", "alasan transisi (opsional)")
	controlFlag := fs.String("control", "", "akar control repository")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *taskID == "" || *statusVal == "" || *by == "" {
		return fail(exitError, "-task, -status, dan -by wajib diisi")
	}

	control, err := controlRoot(*controlFlag)
	if err != nil {
		return fail(exitError, "%v", err)
	}
	_, _, st, err := setup(control)
	if err != nil {
		return fail(exitError, "%v", err)
	}

	// Owner: role penulis harus berhak atas status target. Berlaku untuk status
	// apa pun — runner-owned (reserved, running, dst) menolak semua agent.
	if !status.CanWrite(*by, *statusVal) {
		reportViolations("update-status ditolak", []string{
			fmt.Sprintf("%s tidak boleh menulis status %s (tabel owner ADR-011)", *by, *statusVal),
		})
		return exitViolation
	}

	// Transisi: dari status yang ada. Berkas belum ada → tulis langsung (status
	// awal task, mis. technical-ready oleh TL/SA).
	cur, err := st.Read(*taskID)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fail(exitError, "%v", err)
		}
	} else if !status.TransitionAllowed(cur.Status(), *statusVal) {
		reportViolations("update-status ditolak", []string{
			fmt.Sprintf("transisi %s → %s tidak diizinkan (state machine §33)", cur.Status(), *statusVal),
		})
		return exitViolation
	}

	if err := st.Write(*taskID, *statusVal, *by, *reason, nil); err != nil {
		if ve, ok := err.(*contract.ValidationError); ok {
			reportViolations(ve.Error(), ve.Violations)
			return exitViolation
		}
		return fail(exitError, "%v", err)
	}
	fmt.Printf("ok  %s → %s oleh %s\n", *taskID, *statusVal, *by)
	return exitOK
}

// excludeTaskDir memastikan .task/ tidak pernah masuk staging area agent.
//
// Ditulis ke .git/info/exclude milik worktree — bukan .gitignore — karena
// berkas itu tidak ter-commit, sehingga repository aplikasi tidak perlu memuat
// detail runner. Idempotent: pemanggilan berulang tidak menduplikasi entri.
//
// Pada worktree, .git adalah BERKAS berisi "gitdir: <path>", bukan direktori;
// lokasi info/exclude karena itu ditanyakan kepada git.
func excludeTaskDir(worktree string) error {
	out, err := exec.Command("git", "-C", worktree, "rev-parse", "--git-path", "info/exclude").Output()
	if err != nil {
		return fmt.Errorf("menentukan lokasi info/exclude: %w", err)
	}
	excludePath := strings.TrimSpace(string(out))
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktree, excludePath)
	}

	const entry = ".task/"
	if b, err := os.ReadFile(excludePath); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.TrimSpace(line) == entry {
				return nil // sudah ada
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("membaca %s: %w", excludePath, err)
	}

	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return fmt.Errorf("membuat direktori %s: %w", filepath.Dir(excludePath), err)
	}
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("membuka %s: %w", excludePath, err)
	}
	defer f.Close()
	if _, err := f.WriteString("\n# snapshot task contract milik runner (Q15)\n" + entry + "\n"); err != nil {
		return fmt.Errorf("menulis %s: %w", excludePath, err)
	}
	return nil
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// isInside melaporkan apakah target berada di dalam pohon direktori parent.
//
// Kedua path di-resolve lewat EvalSymlinks sehingga symlink tidak dapat dipakai
// menyelundupkan worktree ke dalam repository.
//
// Target biasanya BELUM ADA saat pemeriksaan — worktree dibuat setelahnya.
// EvalSymlinks gagal pada path yang belum ada, karena itu resolusi dilakukan
// pada leluhur terdekat yang sudah ada lalu sisanya disambung kembali.
//
// Tanpa penyambungan itu, penjagaan ini gagal-BUKA pada macOS: `/var` adalah
// symlink ke `/private/var`, sehingga parent yang ada teresolusi menjadi
// `/private/var/...` sementara target yang belum ada tetap `/var/...`. Keduanya
// lalu dianggap berada pada pohon berbeda dan worktree di dalam repo lolos.
// Ditangkap TestIsInside.
func isInside(target, parent string) (bool, error) {
	t, err := resolveEventual(target)
	if err != nil {
		return false, err
	}
	p, err := resolveEventual(parent)
	if err != nil {
		return false, err
	}

	rel, err := filepath.Rel(p, t)
	if err != nil {
		return false, nil // pada volume berbeda: pasti di luar
	}
	if rel == "." || rel == ".." {
		return false, nil
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

// resolveEventual mengembalikan bentuk path yang seluruh symlink-nya sudah
// di-resolve, termasuk untuk path yang belum ada.
func resolveEventual(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", p, err)
	}

	// Cari leluhur terdekat yang sudah ada, kumpulkan sisa segmennya.
	remaining := ""
	cur := abs
	for {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			if remaining == "" {
				return real, nil
			}
			return filepath.Join(real, remaining), nil
		}
		next := filepath.Dir(cur)
		if next == cur {
			// Mencapai akar tanpa menemukan yang ada; pakai bentuk absolut.
			return abs, nil
		}
		remaining = filepath.Join(filepath.Base(cur), remaining)
		cur = next
	}
}
