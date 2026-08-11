# M2S Virtual Software House Lite v0.1.0

Workflow pengembangan perangkat lunak berbasis **agen AI** yang terstruktur,
dapat diaudit, dan dikerjakan paralel.

## Apa itu M2S-VSH Lite?

M2S-VSH Lite adalah sebuah **cara kerja** (workflow) di mana beberapa agen AI
dengan peran berbeda membantu mengembangkan produk. Bukan satu bot yang
mengerjakan semuanya — melainkan **tim AI** dengan tugas dan batas yang jelas,
diatur lewat repositori pusat.

Tujuannya sederhana: membuktikan bahwa beberapa peran AI dapat bekerja bersama
secara terstruktur, paralel, dan tidak saling menimpa pekerjaan — sementara
keputusan penting tetap di tangan manusia.

## Mengapa ini dibuat?

Proyek ini lahir dari pertanyaan: *bisakah AI mengembangkan perangkat lunak
seperti tim sungguhan, bukan seperti "satu asisten yang mengerjakan semua"?*

Jawabannya diwujudkan sebagai workflow dengan tiga prinsip:

1. **Terstruktur** — setiap pekerjaan agen punya kontrak dan batas yang jelas.
2. **Paralel** — backend dan frontend dikerjakan bersamaan, tidak berurutan.
3. **Diaudit** — semua yang dikerjakan agen tercatat dan bisa diperiksa ulang.

## Tujuan

- Menjalankan pipeline pengembangan dari perencanaan sampai rilis dengan agen AI.
- Memisahkan peran dengan jelas: manajer proyek, analis teknis, desainer,
  engineer backend/frontend, penguji mutu (QA), penulis dokumentasi.
- Menguji mekanisme kerja paralel antar repo aplikasi.
- Menjaga **manusia sebagai penentu akhir** — agen tidak boleh mengambil
  keputusan yang tak bisa dibatalkan.

## Struktur project

Project ini terdiri dari satu repositori pengatur + satu atau lebih
repositori aplikasi.

| Repositori | Isi | Peran |
|---|---|---|
| `public-m2s-vsh-platform` | aturan, kontrak tugas, runner, template, dokumentasi | sumber pengatur |
| `m2s-vsh-project-backend` | server Go — endpoint status | repo aplikasi (backend) |
| `m2s-vsh-project-frontend` | aplikasi Next.js — dashboard status | repo aplikasi (frontend) |

> Repositori aplikasi tidak harus selalu backend+frontend. Arsitektur mendukung
> backend, frontend, mobile, fullstack, dan kombinasi lain.

## Bagaimana cara kerjanya?

**Alur singkat:**

1. **Persiapkan acuan.** Launcher `scripts/project-kickoff.sh` menyusun
   8 dokumen pre-development (Discovery Notes → SDD) sebagai acuan pengembangan.
2. **Kontrak dulu.** Skill `project-start` (atau analis teknis) menurunkan brief
   menjadi kontrak tugas — apa yang dikerjakan, di repo mana, file apa yang
   boleh diubah.
3. **Kerjakan paralel.** Engineer backend dan frontend bekerja bersamaan,
   masing-masing di area terpisah.
4. **Pipeline otomatis.** Setelah kontrak disetujui dan status task diubah ke
   `technical-ready`, jalankan `./scripts/pipeline.sh --task <ID>`. Pipeline
   mengurus implementasi, review, dan QA hingga status `merge-ready`. Task paralel
   (BE + FE) dapat dijalankan di background: `./scripts/pipeline.sh --task BE-XXX & ./scripts/pipeline.sh --task FE-XXX & wait`.
5. **Diperiksa.** Setiap pekerjaan lewat pemeriksaan otomatis (path, kontrak)
   dan ulasan manusia.
6. **Manusia yang menggabungkan.** Perubahan tidak langsung masuk ke cabang
   utama (`main`) — harus lewat alur `agent` → `develop` → `staging` → `main`,
   dan penggabungan akhir dilakukan manusia.

**Prinsip kunci:**

- **Kontrak tugas** — setiap pekerjaan agen punya spesifikasi tertulis:
  repo, cabang, file yang boleh disentuh, kriteria selesai.
- **Eksekusi terisolasi** — tiap agen bekerja di ruang kerja terpisah;
  tidak ada dua agen menulis file yang sama.
- **Gate manusia** — persetujuan kontrak, persetujuan desain, dan penggabungan
  akhir selalu di tangan manusia.
- **Repositori aplikasi independen** — backend dan frontend terhubung lewat
  kontrak di repositori pengatur, bukan lewat kode yang saling merujuk.

## Supported Tech Stack:
![Claude Code](https://img.shields.io/badge/Claude_Code-%23D97757.svg?style=plastic&logo=anthropic&logoColor=white) ![9Router](https://img.shields.io/badge/9router-%236366F1.svg?style=plastic&logo=router&logoColor=white) ![Frontend](https://img.shields.io/badge/Frontend-All_Stack-%2361DAFB.svg?style=plastic&logo=html5&logoColor=white) ![Backend](https://img.shields.io/badge/Backend-All_Stack-%23339933.svg?style=plastic&logo=node.js&logoColor=white) ![Fullstack](https://img.shields.io/badge/Fullstack-Software_Engineering-%23000000.svg?style=plastic&logo=visualstudiocode&logoColor=white) ![Mobile Apps](https://img.shields.io/badge/Mobile-iOS_%26_Android-%23A4C639.svg?style=plastic&logo=android&logoColor=white) ![Database](https://img.shields.io/badge/Database-SQL_%26_NoSQL-%234479A1.svg?style=plastic&logo=databricks&logoColor=white) ![DevOps](https://img.shields.io/badge/DevOps-Cloud_%26_Containerization-%232471A3.svg?style=plastic&logo=docker&logoColor=white) ![Testing](https://img.shields.io/badge/Testing-QA_%26_Automation-%23C21325.svg?style=plastic&logo=testinglibrary&logoColor=white)

## Project serupa (komparasi)

Ruang "pengembangan perangkat lunak berbasis agen AI" sudah ramai. Berikut
proyek-proyek yang berada di ruang yang sama dengan M2S-VSH Lite:

| Project | Jenis | URL/Link Repository | Deskripsi | Bedanya dengan M2S-VSH |
|---|---|---|---|---|
| MetaGPT | Multi-agent framework | https://github.com/FoundationAgents/MetaGPT | "Perusahaan AI pertama" — tim agen berperan (PM/arsitek/engineer) berbasis SOP | Simulasi SOP internal, bukan repo GitHub nyata + penegakan aturan |
| deer-flow (ByteDance) | Agent harness | https://github.com/bytedance/deer-flow | Agen long-horizon yang meneliti, menulis kode, dan membuat | Satu harness, bukan tim multi-peran + gerbang manusia |
| ruflo (ruvnet) | Agent meta-harness | https://github.com/ruvnet/ruflo | Harness agen untuk kawanan (swarm) multi-agent | Orkestrasi fleksibel, tanpa kontrak tugas sebagai pengendali |
| langchain | Agent platform | https://github.com/langchain-ai/langchain | Platform rekayasa agen: orkestrasi + peralatan | Pustaka umum, bukan alur kerja GitHub |
| agent-orchestrator (Untrivial-ai) | Agent IDE | https://github.com/Untrivial-ai/agent-orchestrator | Kelola kawanan agen koding, rencanakan tugas, munculkan agen | IDE, bukan repo + CI + penegakan aturan |
| agent-framework (Microsoft) | Agent framework | https://github.com/microsoft/agent-framework | Kerangka untuk membangun, mengorkestrasi, dan menerapkan agen multi-agent | SDK, bukan pipeline repo |
| adk-python (Google) | Agent toolkit | https://github.com/google/adk-python | Kit Python untuk membangun, mengevaluasi, dan menerapkan agen AI | SDK, bukan alur kerja GitHub |
| nanobot (HKUDS) | Agent framework | https://github.com/HKUDS/nanobot | Kerangka agen ringan dengan WebUI, peralatan, memori, MCP, alur multi-agent | Tidak berbasis kontrak |
| haystack (deepset) | Orkestrasi | https://github.com/deepset-ai/haystack | Kerangka orkestrasi AI dengan pipeline modular dan alur agen | Berorientasi pipeline, bukan software house |
| CowAgent | Agent harness | https://github.com/zhayujie/CowAgent | Asisten agen: merencanakan tugas, menjalankan peralatan dan skill | Satu asisten, bukan tim |
| agents (wshobson) | Pasar plugin | https://github.com/wshobson/agents | Pasar plugin agen untuk Claude Code, Codex, Cursor | Plugin, bukan alur kerja |
| Orca (stablyai) | Orkestrasi | https://github.com/stablyai/orca | Lingkungan pengembangan untuk kawanan agen paralel | IDE, bukan workflow repo + enforcement |
| CrewAI | Multi-agent framework | https://github.com/crewAIInc/crewAI | Kru agen dengan peran berbeda dan penugasan tugas | Pustaka, tanpa penegakan GitHub |
| AutoGen (Microsoft) | Multi-agent framework | https://github.com/microsoft/autogen | Agen berperan berbasis percakapan | Berorientasi dialog, bukan kontrak |
| OpenHands (ex OpenDevin) | Autonomous agent | https://github.com/All-Hands-AI/OpenHands | Agen dev dengan sandbox terisolasi | Satu agen, bukan tim multi-peran terstruktur |
| Cognition Devin | Autonomous SWE | https://github.com/cognition-ai/devin | Agen perangkat lunak otonom | Satu agen, proprietary, gerbang manusia lemah |
| ChatDev | Multi-agent | https://github.com/OpenBMB/ChatDev | Simulasi "perusahaan software" dengan agen berperan (CEO/CTO/developer) | Simulasi percakapan, bukan repo + CI nyata |
| Aider | Pair coding agent | https://github.com/Aider-AI/aider | Agen AI yang membantu mengedit kode, sadar-git | Pair, bukan tim multi-peran |
| Cline / Roo Code | Coding agent | https://github.com/cline/cline · https://github.com/RooCodeInc/Roo-Code | Agen eksekusi tugas dev dengan tool use | Agen tunggal + plugin, bukan orkestrasi peran |
| GitHub Copilot Workspace / Agentic | GitHub-native | https://github.com/features/ai/github-app | Agen dari issue ke merge di dalam GitHub | Tanpa ruleset, required check, dan branch protection seketat M2S-VSH |

### Nilai jual M2S-VSH Lite

Di tengah ramainya proyek sejenis, M2S-VSH Lite berbeda di **empat hal**:

1. **Kontrak tugas sebagai kontrak sungguhan** — setiap pekerjaan agen punya
   spesifikasi tertulis (repo, cabang, file yang boleh disentuh, kriteria
   selesai) yang dijadikan **gerbang**, bukan sekadar instruksi longgar.
2. **Penegakan nyata di GitHub** — ruleset, required check
   `validate-changed-paths`, branch protection, dan isolasi worktree. Sebagian
   besar proyek sejenis (MetaGPT, CrewAI, AutoGen) tidak punya lapisan
   penegakan ini.
3. **Manusia sebagai penentu akhir** — penggabungan ke `main` hanya oleh
   manusia, area tertentu bersifat human-only, dan agen tidak bisa mengambil
   keputusan yang tak dapat dibatalkan.
4. **Claude Code native sebagai runtime** — bukan kerangka sendiri, melainkan
   memanfaatkan kemampuan Claude Code + GitHub sebagai lapisan pengatur.

**Singkatnya:** proyek lain menjual *"AI yang bisa menulis kode"*. M2S-VSH
menjual *"AI yang bekerja seperti tim — dengan kontrak, pengawasan, dan
pertanggungjawaban"*.

## Status

Project ini masih dalam pengembangan. Dokumentasi dan panduan setup lengkap
ada di repositori pengatur (`docs/`).
