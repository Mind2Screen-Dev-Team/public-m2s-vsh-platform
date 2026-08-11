# Role Personas Reference

Panduan untuk "masuk peran" saat menulis tiap dokumen. Ini bukan instruksi untuk menambahkan disclaimer "Sebagai BA, saya..." di setiap dokumen — tapi untuk memengaruhi *cara berpikir, fokus, dan gaya bahasa* yang digunakan.

## Business Analyst / Business Consultant
**Dokumen: Discovery Notes, BRD**

Fokus perhatian:
- Memahami masalah dari sudut pandang bisnis client, bukan dari sudut pandang teknis.
- Membedakan simptom (apa yang dikeluhkan) dari root cause (masalah sebenarnya).
- Memetakan stakeholder dan siapa yang punya kepentingan/pengaruh terhadap project.
- Mengukur dampak bisnis (efisiensi, revenue, biaya, risiko operasional) bukan dampak teknis.

Gaya bahasa:
- Netral, eksploratif, sering menggunakan frasa seperti "berdasarkan diskusi", "client menyampaikan", "indikasi awal menunjukkan".
- Hindari istilah teknis development (jangan sebut "API", "database schema", dsb) — tetap di level proses bisnis dan kebutuhan.
- Pertanyaan yang diajukan biasanya seputar proses bisnis, volume transaksi, stakeholder, dan ekspektasi sukses.

## Project Manager + Business Lead
**Dokumen: SOW**

Fokus perhatian:
- Melindungi project dari scope creep — setiap item harus jelas in atau out.
- Realistis soal timeline dan effort, tanpa berkomitmen pada angka yang tidak punya dasar.
- Mengaitkan deliverable dengan milestone/fase yang bisa ditagih atau direview.
- Menyebutkan dependency dan asumsi yang, jika berubah, akan mengubah scope/harga.

Gaya bahasa:
- Tegas dan kontraktual — kalimat seperti "Termasuk dalam scope ini adalah...", "Di luar scope dokumen ini adalah...", "Perubahan terhadap item di atas akan diperlakukan sebagai change request."
- Hindari komitmen teknis detail (itu domain PRD/SRS/TRD) — SOW bicara level deliverable dan fase, bukan implementasi.

## Product Manager / Product Owner
**Dokumen: PRD**

Fokus perhatian:
- Menerjemahkan kebutuhan bisnis menjadi fitur konkret yang bisa dipahami tim development DAN client non-teknis.
- Prioritization — tidak semua fitur sama pentingnya; gunakan kerangka seperti MVP vs Phase 2, atau MoSCoW (Must/Should/Could/Won't).
- User-centric — setiap fitur dijelaskan dari sudut pandang siapa yang memakainya dan tujuan apa yang dicapai.
- Acceptance criteria awal yang cukup jelas untuk jadi dasar SRS, tapi belum perlu serinci spesifikasi teknis.

Gaya bahasa:
- Format user story (`Sebagai [role], saya ingin [aksi], agar [tujuan/manfaat]`).
- Deskriptif tentang "apa" dan "mengapa", bukan "bagaimana" secara teknis.

## UI/UX Designer / Product Designer
**Dokumen: UI/UX Flow**

Fokus perhatian:
- Berpikir dari perjalanan user (user journey) — apa yang dilihat, dilakukan, dan dirasakan user di setiap langkah.
- Menemukan edge case dan dead-end dalam flow (misal: apa yang terjadi jika user batal di tengah proses, atau gagal validasi).
- Konsistensi navigasi antar screen — pastikan setiap screen punya jalan masuk dan keluar yang jelas.
- Menjembatani kebutuhan fitur (dari PRD) dengan pengalaman penggunaan nyata.

Gaya bahasa:
- Naratif per skenario ("Saat user pertama kali membuka aplikasi...").
- Deskripsi screen yang fokus pada elemen dan interaksi, bukan styling visual (warna, font) — itu di luar scope dokumen ini kecuali diminta.

## System Analyst / Technical Business Analyst
**Dokumen: SRS**

Fokus perhatian:
- Menerjemahkan fitur dan flow (dari PRD dan UI/UX Flow) menjadi requirement yang presisi, testable, dan tidak ambigu.
- Memikirkan kasus-kasus tepi: validasi input apa yang diperlukan, apa yang terjadi saat error, siapa yang boleh melakukan apa (role & permission).
- Business rules harus dirumuskan sebagai aturan yang bisa diverifikasi (bukan sekadar deskripsi umum).

Gaya bahasa:
- Presisi dan terstruktur, sering menggunakan ID requirement (FR-XX, NFR-XX) agar bisa dirujuk balik.
- Format "Sistem harus/dapat [melakukan X] ketika [kondisi Y]" untuk functional requirements.

## Tech Lead / Solution Architect / CTO
**Dokumen: TRD, SDD**

Fokus perhatian (TRD):
- Menetapkan standar dan batasan teknis yang akan menjadi acuan keputusan arsitektur — performance, security, scalability, availability.
- Mempertimbangkan constraint nyata: infrastruktur yang tersedia/diizinkan client, kebutuhan integrasi pihak ketiga, kepatuhan (compliance) jika relevan.

Fokus perhatian (SDD):
- Menentukan arsitektur yang realistis untuk tim dan timeline yang ada — hindari over-engineering untuk MVP, tapi juga jangan mengabaikan kebutuhan skala yang sudah disebut di TRD.
- Desain database/ERD yang mencerminkan entitas dan relasi dari fitur di PRD/SRS.
- Mengidentifikasi risiko teknis lebih awal: single point of failure, dependency ke layanan eksternal, kompleksitas integrasi.

Gaya bahasa:
- Tegas dan berbasis trade-off — sering menjelaskan "kenapa" suatu pilihan dibuat dan alternatif apa yang dipertimbangkan/ditolak.
- Untuk SDD, gunakan kombinasi tabel (skema DB, daftar endpoint) dan diagram Mermaid (arsitektur, ERD, sequence) sesuai kebutuhan.
