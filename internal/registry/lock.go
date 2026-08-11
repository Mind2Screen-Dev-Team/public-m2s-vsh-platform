package registry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// LockName adalah nama berkas kunci di dalam direktori registry.
// Berakhiran .lock sehingga tercakup pola `control/reservations/*.lock`
// pada .gitignore — kunci tidak pernah ikut ter-commit.
const LockName = ".registry.lock"

// staleAfter menentukan kapan kunci dianggap yatim. Nilai ini harus lebih
// besar daripada durasi wajar satu operasi registry (baca, periksa konflik,
// tulis), tetapi cukup kecil agar runner yang mati tidak memblokir lama.
const staleAfter = 2 * time.Minute

// ErrLocked dikembalikan bila kunci dipegang proses lain yang masih hidup.
var ErrLocked = errors.New("registry sedang dikunci proses lain")

// Lock adalah kunci eksklusif atas registry.
//
// Implementasi memakai O_CREATE|O_EXCL yang bersifat atomik pada POSIX:
// tepat satu proses berhasil membuat berkas, sisanya gagal. Ini mencegah dua
// runner memeriksa konflik terhadap keadaan yang sama lalu sama-sama menulis
// reservasi yang beririsan — race yang akan meloloskan dua writer ke satu path.
type Lock struct {
	path string
	held bool
}

type lockInfo struct {
	PID        int    `yaml:"pid"`
	Host       string `yaml:"host"`
	AcquiredAt string `yaml:"acquired_at"`
	Operation  string `yaml:"operation"`
}

// Acquire mengambil kunci, menunggu sampai timeout bila sedang dipegang.
//
// Kunci yatim — dipegang proses yang sudah mati, atau lebih tua dari
// staleAfter — dibersihkan otomatis. Pembersihan hanya dilakukan runner,
// sesuai §30 yang melarang worker membersihkan reservasi basi.
func (r *Registry) Acquire(operation string, timeout time.Duration) (*Lock, error) {
	lockPath := filepath.Join(r.dir, LockName)
	l := &Lock{path: lockPath}

	deadline := time.Now().Add(timeout)
	for {
		err := l.tryCreate(operation)
		if err == nil {
			l.held = true
			return l, nil
		}
		if !errors.Is(err, ErrLocked) {
			return nil, err
		}

		if reclaimed, rerr := l.breakIfStale(); rerr != nil {
			return nil, rerr
		} else if reclaimed {
			continue // coba lagi segera setelah kunci yatim dibersihkan
		}

		if time.Now().After(deadline) {
			holder, _ := l.readInfo()
			if holder != nil {
				return nil, fmt.Errorf("%w: pid %d pada %s sejak %s (operasi %q)",
					ErrLocked, holder.PID, holder.Host, holder.AcquiredAt, holder.Operation)
			}
			return nil, ErrLocked
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (l *Lock) tryCreate(operation string) error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return ErrLocked
		}
		return fmt.Errorf("membuat kunci %s: %w", l.path, err)
	}
	defer f.Close()

	host, _ := os.Hostname()
	info := lockInfo{
		PID:        os.Getpid(),
		Host:       host,
		AcquiredAt: time.Now().Format(time.RFC3339),
		Operation:  operation,
	}
	b, err := yaml.Marshal(info)
	if err != nil {
		return fmt.Errorf("menulis isi kunci: %w", err)
	}
	if _, err := f.Write(b); err != nil {
		return fmt.Errorf("menulis kunci %s: %w", l.path, err)
	}
	return nil
}

func (l *Lock) readInfo() (*lockInfo, error) {
	b, err := os.ReadFile(l.path)
	if err != nil {
		return nil, err
	}
	var info lockInfo
	if err := yaml.Unmarshal(b, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// breakIfStale menghapus kunci yang pemegangnya sudah mati atau terlalu tua.
// Mengembalikan true bila kunci berhasil dibersihkan.
func (l *Lock) breakIfStale() (bool, error) {
	info, err := l.readInfo()
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil // pemegang melepasnya sendiri
		}
		// Kunci rusak dan tidak dapat dibaca: perlakukan sebagai yatim,
		// karena membiarkannya memblokir registry selamanya.
		return l.remove()
	}

	if info.PID > 0 && processAlive(info.PID) {
		if t, perr := time.Parse(time.RFC3339, info.AcquiredAt); perr == nil {
			if time.Since(t) < staleAfter {
				return false, nil // pemegang hidup dan masih dalam batas waktu
			}
		} else {
			return false, nil // stempel waktu tidak terbaca, jangan rebut
		}
	}

	// PID kosong berarti kunci terbaca antara pembuatan berkas dan penulisan
	// isinya — pemegangnya baru saja mengambil kunci, bukan yatim. Merebutnya
	// di sini akan menghasilkan dua pemegang sekaligus.
	if info.PID == 0 {
		return false, nil
	}
	return l.remove()
}

func (l *Lock) remove() (bool, error) {
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("membersihkan kunci yatim %s: %w", l.path, err)
	}
	return true, nil
}

// Release melepas kunci. Aman dipanggil lebih dari sekali.
func (l *Lock) Release() error {
	if l == nil || !l.held {
		return nil
	}
	l.held = false
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("melepas kunci %s: %w", l.path, err)
	}
	return nil
}

// writeYAMLAtomic menulis dokumen sebagai YAML lewat berkas sementara dan
// rename, sehingga pembaca tidak pernah melihat berkas setengah tertulis.
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

// processAlive memeriksa apakah PID masih berjalan.
//
// Memakai syscall.Signal(0): tidak mengirim sinyal apa pun, tetapi tetap
// melakukan pemeriksaan keberadaan proses dan izin.
//
// JANGAN memakai Signal(nil) — nil bukan tipe sinyal yang sah dan selalu
// mengembalikan "os: unsupported signal type", sehingga setiap kunci akan
// dianggap yatim dan direbut. Itu membuat penguncian tidak berfungsi sama
// sekali. Ditangkap TestLockExclusive.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
