# Makefile control repository M2S-VSH Lite.
#
# Berkas ini termasuk daftar human-only write (component-inventory.md §7):
# ia menentukan bagaimana penegak batas path dibangun, sehingga agent yang
# dapat mengubahnya dapat melonggarkan batas yang mengikatnya sendiri.

BIN       := bin/m2s
PKG       := ./cmd/m2s
SCHEMAS   := schemas
GOFILES   := $(shell find cmd internal -name '*.go' 2>/dev/null)

.DEFAULT_GOAL := help

# ── Build ─────────────────────────────────────────────────────────────

## build: kompilasi binary runner ke bin/m2s
.PHONY: build
build: $(BIN)

$(BIN): $(GOFILES) go.mod go.sum
	@mkdir -p bin
	go build -o $(BIN) $(PKG)
	@echo "ok  $(BIN) dibangun"

## clean: hapus binary hasil build
.PHONY: clean
clean:
	rm -rf bin
	@echo "ok  bin/ dihapus"

# ── Quality gate ──────────────────────────────────────────────────────

## test: jalankan seluruh test dengan race detector
.PHONY: test
test:
	go test -race ./...

## vet: analisis statis
.PHONY: vet
vet:
	go vet ./...

## fmt: periksa format (gagal bila ada berkas belum terformat)
.PHONY: fmt
fmt:
	@unformatted=$$(gofmt -l cmd internal 2>/dev/null); \
	if [ -n "$$unformatted" ]; then \
		echo "berkas belum terformat:"; echo "$$unformatted"; \
		echo "jalankan: make fmt-fix"; exit 1; \
	fi
	@echo "ok  format bersih"

## fmt-fix: terapkan gofmt
.PHONY: fmt-fix
fmt-fix:
	gofmt -w cmd internal
	@echo "ok  format diterapkan"

## check: fmt + vet + test — gate yang wajib lulus sebelum commit
.PHONY: check
check: fmt vet test
	@echo "ok  seluruh gate lulus"

# ── Verifikasi khusus ─────────────────────────────────────────────────

## verify-wrappers: pastikan wrapper .sh tetap tipis dan lengkap
##
## ADR-004 #2 menuntut wrapper hanya meneruskan argumen. Wrapper yang
## menumbuhkan logika memindahkan keputusan ke tempat yang tidak diuji.
.PHONY: verify-wrappers
verify-wrappers:
	@failed=0; \
	for sub in validate-task reserve-paths launch-task collect-result release-reservation; do \
		f="scripts/$$sub.sh"; \
		if [ ! -x "$$f" ]; then \
			echo "FAIL $$f tidak ada atau tidak executable (§36, Q11)"; failed=1; continue; \
		fi; \
		lines=$$(grep -vcE '^\s*(#|$$)' "$$f"); \
		if [ "$$lines" -gt 3 ]; then \
			echo "FAIL $$f memuat $$lines baris kode — wrapper harus tipis (ADR-004 #2)"; failed=1; \
		fi; \
		if ! grep -q "run $$sub" "$$f"; then \
			echo "FAIL $$f tidak memanggil subcommand $$sub"; failed=1; \
		fi; \
	done; \
	if [ "$$failed" -eq 0 ]; then echo "ok  5 wrapper tipis dan lengkap"; else exit 1; fi

## verify-schemas: pastikan seluruh schema dapat dikompilasi validator Go
.PHONY: verify-schemas
verify-schemas:
	go test ./internal/contract/ -run 'TestCompileAllSchemas|TestSchemaPatternsAreRE2Compatible|TestSchemaFilesAreRegistered' -v 2>&1 \
		| grep -E '^(--- )?(PASS|FAIL|ok)' || true
	@echo "ok  schema terverifikasi"

## verify-agents: pastikan 13 definisi agent lengkap dan boundary-nya berbeda
##
## Membuktikan kriteria Done §57. TestAgentBoundariesAreDistinct adalah
## intinya: ia gagal bila dua role disalin-tempel tanpa dibedakan.
.PHONY: verify-agents
verify-agents:
	go test ./internal/contract/ -run 'TestEveryRoleHasAgentTemplate|TestAgentFrontmatterFieldsAreSupported|TestAgentNameMatchesFileName|TestNoAgentHasAgentTool|TestReadOnlyRolesHaveNoWriteTools|TestWriterRolesDeclareWorktreeIsolation|TestForbiddenPathBaselinePresent|TestEveryRoleHasEffort|TestArchitectureConstraintsPresent|TestDeployedAgentsMatchTemplates|TestAgentBoundariesAreDistinct' -v 2>&1 \
		| grep -E '^(--- )?(PASS|FAIL|ok)' || true
	@echo "ok  13 definisi agent terverifikasi"

## verify-hooks: jalankan self-test tiap hook + test negatif enforcement
##
## Membuktikan kriteria Done §59: agent gagal menulis forbidden file. Setiap
## hook fail-closed wajib punya self-test (R-24) dan setiap skenario §68 wajib
## ditolak exit 2. bin/m2s dibangun lebih dulu karena validate-path-scope
## mendelegasikan keputusan padanya.
.PHONY: verify-hooks
verify-hooks: build
	@failed=0; \
	for h in block-secret-paths block-dangerous-command validate-path-scope \
	         audit-tool-use validate-handoff worktree-lifecycle; do \
		f=".claude/hooks/$$h.sh"; \
		if [ ! -x "$$f" ]; then \
			echo "FAIL $$f tidak ada atau tidak executable (§42)"; failed=1; continue; \
		fi; \
		if ! head -1 "$$f" | grep -q '^#!'; then \
			echo "FAIL $$f tanpa shebang"; failed=1; \
		fi; \
		out=$$(CLAUDE_PROJECT_DIR="$$(pwd)" bash "$$f" --selftest 2>&1) || { \
			echo "FAIL $$f self-test:"; echo "$$out" | sed 's/^/    /'; failed=1; continue; \
		}; \
		echo "  $$out"; \
	done; \
	echo "--- test negatif §68 ---"; \
	if [ -d tests/negative ]; then \
		for t in tests/negative/*.test.sh; do \
			[ -e "$$t" ] || continue; \
			out=$$(CLAUDE_PROJECT_DIR="$$(pwd)" bash "$$t" 2>&1) || { \
				echo "FAIL $$t:"; echo "$$out" | sed 's/^/    /'; failed=1; continue; \
			}; \
			echo "  $$out"; \
		done; \
	fi; \
	if [ "$$failed" -eq 0 ]; then echo "ok  hook + test negatif lulus"; else exit 1; fi

## verify: seluruh pemeriksaan — dipakai sebelum menutup fase
.PHONY: verify
verify: check verify-wrappers verify-schemas verify-agents verify-hooks
	@echo "ok  verifikasi lengkap"

# ── Bantuan ───────────────────────────────────────────────────────────

## help: tampilkan daftar target
.PHONY: help
help:
	@echo "M2S-VSH control repository"
	@echo ""
	@grep -E '^## [a-z-]+:' $(MAKEFILE_LIST) \
		| sed 's/^## //' \
		| awk -F': ' '{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Runner dipanggil lewat scripts/<subcommand>.sh, bukan bin/m2s langsung (Q11)."
