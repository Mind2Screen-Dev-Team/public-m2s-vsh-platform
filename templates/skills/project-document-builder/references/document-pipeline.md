# Document Pipeline Reference

Tabel acuan utama untuk pipeline dokumen pre-development. Gunakan ini untuk menentukan fokus "5. Main Content" dan untuk memastikan konteks yang dibawa ke dokumen berikutnya sudah tepat.

| # | Dokumen | Brief Kegunaan | Purpose | Output | Owner / Persona |
|---|---------|------------------|---------|--------|------------------|
| 1 | Discovery Notes / Initial Requirement Notes | Catatan awal dari diskusi ide, kebutuhan, masalah, dan ekspektasi client. | Menangkap konteks awal client, menggali masalah utama, membedakan keinginan dan kebutuhan, menjadi bahan awal BRD/SOW. | Catatan meeting, problem statement awal, daftar kebutuhan kasar, daftar pertanyaan lanjutan. | Business Analyst / Business Consultant |
| 2 | BRD — Business Requirements Document | Dokumen yang menjelaskan kebutuhan bisnis client sebelum diterjemahkan menjadi fitur. | Menjelaskan masalah bisnis, tujuan proyek, stakeholder, scope bisnis, dan success criteria. | Business goals, pain points, business process overview, scope dan constraint, success metric. | Business Analyst / Business Consultant |
| 3 | SOW — Scope of Work / Proposal Scope | Dokumen batas pekerjaan yang disepakati dengan client. Penting untuk mencegah scope creep. | Mengunci scope pekerjaan, menjelaskan in-scope dan out-of-scope, menjadi dasar harga dan timeline, pegangan saat ada request tambahan. | Scope fitur, out of scope, timeline estimasi, deliverables, assumption & dependency. | Project Manager + Business Lead |
| 4 | PRD — Product Requirements Document | Dokumen yang menerjemahkan kebutuhan bisnis menjadi kebutuhan produk/fitur. | Menentukan fitur, user, use case, prioritas, acceptance criteria awal, dan menyelaraskan client, product, dan development team. | Feature list, user stories/use cases, MVP scope, prioritas fitur, acceptance criteria. | Product Manager / Product Owner |
| 5 | UI/UX Flow | Dokumen visual yang menjelaskan bagaimana user menggunakan sistem dari awal sampai akhir. | Memvisualisasikan alur penggunaan sistem, menguji flow, menemukan edge case lebih awal, menjadi dasar desain UI dan validasi client. | User flow, sitemap, wireframe, prototype, screen-by-screen flow. | UI/UX Designer / Product Designer |
| 6 | SRS — Software Requirements Specification | Dokumen spesifikasi software yang menjelaskan requirement secara detail dan teknis-fungsional. | Menjelaskan fungsi sistem secara detail, role dan permission, business rules, validasi data, serta menjadi dasar QA dan development. | Functional requirements, non-functional requirements, role access matrix, business rules, validation rules, error handling scenario. | System Analyst / Technical Business Analyst |
| 7 | TRD — Technical Requirements Document | Dokumen yang menjelaskan kebutuhan teknis, batasan teknis, dan standar teknis yang harus dipenuhi sistem. | Mengunci standar teknis proyek, menentukan kebutuhan performance, security, scalability, availability, constraint teknologi/infrastruktur client, dan integrasi teknis. | Performance requirement, security requirement, infrastructure constraint, integration protocol, browser/device compatibility, backup, logging, monitoring, audit requirement. | Tech Lead / Solution Architect / CTO |
| 8 | SDD / System Design Document | Dokumen rancangan teknis tentang bagaimana sistem akan dibangun. | Menentukan arsitektur sistem, desain database, desain API/service, integrasi eksternal, dan risiko teknis. | Architecture diagram, ERD/database schema, API contract, sequence diagram, infrastructure plan, security consideration. | Tech Lead / Solution Architect / CTO |

## Context Carry-Forward Rules

Setiap dokumen WAJIB menjadikan dokumen sebelumnya sebagai sumber konteks utama, sesuai pemetaan berikut:

- **BRD** → referensi: Discovery Notes
- **SOW** → referensi: Discovery Notes, BRD
- **PRD** → referensi: Discovery Notes, BRD, SOW
- **UI/UX Flow** → referensi: PRD, BRD, SOW
- **SRS** → referensi: PRD, UI/UX Flow
- **TRD** → referensi: SRS, PRD, UI/UX Flow, system constraints (dari BRD/SOW bila ada)
- **SDD** → referensi: TRD, SRS, PRD, UI/UX Flow

### Yang harus dijaga konsisten lintas dokumen

1. **Istilah** — nama fitur, nama modul, nama role/user type harus sama persis (atau jika berubah, sebutkan eksplisit alasan perubahannya).
2. **Fitur** — fitur yang muncul di PRD harus tercermin di UI/UX Flow, SRS, dan akhirnya di SDD. Jangan ada fitur "baru" yang muncul tiba-tiba di SRS/TRD/SDD tanpa jejak di PRD — jika memang perlu ditambahkan, tandai sebagai temuan baru dan beri catatan di Background/Context.
3. **Scope** — in-scope/out-of-scope dari SOW menjadi batas untuk PRD dan seterusnya. Jika PRD/SRS menyinggung sesuatu yang out-of-scope di SOW, tandai sebagai potensi scope creep di Quality Gate.
4. **Business rules** — aturan bisnis yang muncul di BRD/PRD harus diformalkan secara lebih teknis di SRS, lalu diimplementasikan strukturnya di SDD (misal lewat validasi DB/API).
5. **Constraints & asumsi** — constraint teknologi/budget/timeline dari SOW/BRD harus terbawa ke TRD sebagai batasan teknis nyata, bukan diabaikan.

## Catatan Tambahan per Dokumen

### Discovery Notes
Ini adalah titik awal, jadi tidak punya dokumen referensi sebelumnya. Fokus pada menangkap apa yang user/client katakan secara mentah, lalu mulai memisahkan:
- **Wants** (yang diminta/disebutkan client) vs **Needs** (masalah inti yang sebenarnya perlu diselesaikan) — keduanya bisa berbeda.
- Problem statement awal yang ringkas (1-3 kalimat per masalah utama).
- Daftar kebutuhan kasar, belum perlu prioritas detail.
- Pertanyaan lanjutan untuk menggali lebih dalam di sesi berikutnya atau yang perlu dijawab client sebelum BRD disusun.

### BRD
Jembatan antara "apa yang client mau" dan "apa yang akan dibangun". Hindari menyebut nama fitur teknis secara spesifik di sini — fokus ke level bisnis (proses, tujuan, KPI). Detail fitur baru muncul di PRD.

### SOW
Dokumen yang paling "mengikat" secara komersial. Harus eksplisit dan defensif: apa yang TIDAK termasuk sama pentingnya dengan apa yang termasuk. Timeline boleh berupa estimasi per fase (bukan tanggal pasti) kecuali user memberi tanggal nyata.

### PRD
Di sinilah fitur mulai konkret. Gunakan format user story (`Sebagai [role], saya ingin [aksi], agar [tujuan]`) untuk use case utama. MVP scope harus jelas dipisahkan dari fitur fase berikutnya (nice-to-have/phase 2).

### UI/UX Flow
Karena ini text-based (Markdown), representasikan flow dengan kombinasi:
- Narasi alur per skenario utama (misal: "Flow Registrasi User Baru").
- Diagram Mermaid (`flowchart` atau `graph`) untuk flow yang punya banyak percabangan/decision point.
- Daftar screen dengan deskripsi elemen utama (bukan wireframe visual, tapi deskripsi tekstual yang cukup untuk dijadikan dasar wireframe oleh designer).

### SRS
Gunakan ID untuk requirement (misal `FR-01`, `FR-02` untuk functional; `NFR-01` untuk non-functional) agar mudah dirujuk di TRD/SDD dan dokumen QA di masa depan. Role access matrix bisa berbentuk tabel: baris = role, kolom = modul/fitur, isi = level akses (Create/Read/Update/Delete/None).

### TRD
Fokus pada "batasan dan standar", bukan "cara implementasi" (itu di SDD). Misal: TRD menyebutkan "harus mendukung 500 concurrent users dengan response time <2s", SDD menjelaskan arsitektur yang mencapai itu.

### SDD
Dokumen paling teknis. ERD bisa direpresentasikan sebagai tabel Markdown (nama tabel, kolom, tipe data, relasi) atau diagram Mermaid `erDiagram`. API contract cukup level ringkas (method, endpoint, deskripsi singkat request/response) — bukan spesifikasi OpenAPI lengkap kecuali diminta. Sequence diagram pakai Mermaid `sequenceDiagram` untuk flow kritis (misal: proses checkout, autentikasi, atau flow approval).
