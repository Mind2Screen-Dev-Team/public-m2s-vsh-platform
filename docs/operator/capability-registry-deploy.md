# Patch human-only — deploy capability registry

`governance/capability-registry.yaml` ditolak `deny` di `.claude/settings.json`
(component-inventory §7): Workflow Maintainer yang punya. Template kanonik ada
di `templates/governance/capability-registry.yaml`; penyalinan dilakukan
manusia.

## Deploy

```bash
mkdir -p governance
cp templates/governance/capability-registry.yaml governance/capability-registry.yaml
```

## Isi capability pertama (referensi)

`open-design` (nexu-io) tercatat sebagai capability opsional UI/UX (§8, §72),
sudah ada contoh di `schemas/examples/capability-open-design.valid.yaml`. Isi
bila benar-benar dipakai utk prototype — bukan lebih dulu.

## Alur penambahan capability (§9.4)

```
Agent/engineer mengajukan capability request
  → TL/SA menilai relevance dan overlap
  → Workflow Maintainer source review
  → license + security review
  → install pada sandbox project
  → evaluation
  → pin version + commit inventory (entry di registry ini)
```

## Verifikasi

`python3 -c "import yaml,sys; yaml.safe_load(open('governance/capability-registry.yaml'))"`
dan setiap entry memenuhi `schemas/capability.schema.json`. `make verify` hijau.
