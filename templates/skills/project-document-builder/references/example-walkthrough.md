# Contoh Penggunaan & Trigger Prompt

## Contoh Trigger Prompt

Skill ini sebaiknya aktif ketika user memberikan permintaan seperti:

- "Tolong buatkan dokumen pre-development untuk project [nama project] ini, mulai dari discovery."
- "Aku punya ide produk: [deskripsi ide]. Bisa bantu susun BRD dan PRD-nya?"
- "Berikut hasil diskusi dengan client kami: [notes/transcript]. Bisa diubah jadi Discovery Notes dan lanjut ke dokumen berikutnya?"
- "Buatkan SRS untuk fitur [X], based on PRD yang sudah kita bahas sebelumnya."
- "Kami butuh SOW untuk membatasi scope project [nama project] sebelum kirim proposal ke client."
- "Tolong lanjutkan ke dokumen berikutnya, Discovery Notes sudah saya approve."
- "Revisi BRD bagian success metric, tambahkan target retention 30%."

## Contoh Alur Percakapan (Ringkas)

**User:**
> Tolong bantu siapkan dokumen pre-development untuk aplikasi booking lapangan futsal. Targetnya pemilik lapangan futsal kecil-menengah yang masih pakai buku catatan manual untuk booking. Mereka sering double booking dan kesulitan tracking pembayaran. Budget terbatas, timeline 2 bulan untuk MVP.

**Respons Claude (ringkas):**
1. Menjelaskan akan memulai dari Discovery Notes sebagai BA/Business Consultant.
2. Menyusun Discovery Notes dengan:
   - Catatan: masalah double booking, tracking pembayaran manual, target user = pemilik lapangan kecil-menengah.
   - Wants vs Needs: client minta "aplikasi booking", tapi need sebenarnya adalah "sistem yang mencegah double booking dan mempermudah rekonsiliasi pembayaran".
   - Daftar kebutuhan kasar: kalender booking, manajemen lapangan, pencatatan pembayaran, notifikasi.
   - Pertanyaan lanjutan: berapa jumlah lapangan tipikal per client, apakah perlu pembayaran online atau cukup pencatatan manual, apakah multi-cabang.
3. Mengisi Assumptions, Constraints (budget terbatas, timeline 2 bulan), Risks (misal: risiko scope MVP membengkak jika fitur pembayaran online dianggap wajib).
4. Menutup dengan Quality Gate Review, status misalnya `NEEDS_USER_REVIEW` karena beberapa asumsi penting (jumlah lapangan, model pembayaran) perlu dikonfirmasi.
5. STOP — menunggu user menjawab pertanyaan atau memberi approval.

**User:**
> Oke, jumlah lapangan biasanya 2-4 per venue, pembayaran online belum perlu di MVP (cukup pencatatan manual + status lunas/belum). Lanjut ke BRD.

**Respons Claude (ringkas):**
1. Beralih ke persona BA/Business Consultant untuk BRD, menggunakan Discovery Notes sebagai referensi utama.
2. Memasukkan info baru (2-4 lapangan per venue, pembayaran manual di MVP) ke Background/Context BRD, dan mengupdate asumsi terkait.
3. Menyusun BRD: business goals (mengurangi double booking, mempermudah rekonsiliasi), pain points, business process overview (proses booking manual saat ini vs target), scope bisnis, success metrics (misal: 0% double booking dalam X bulan, waktu rekonsiliasi pembayaran berkurang Y%).
4. Quality Gate Review, status `APPROVED_TO_CONTINUE` atau `NEEDS_USER_REVIEW` sesuai kondisi.
5. STOP — menunggu approval/instruksi lanjut ke SOW.

...dan seterusnya hingga SDD, masing-masing dimulai dari mengambil konteks dokumen sebelumnya, menulis dengan persona yang sesuai, lalu Quality Gate Review dan STOP.

## Catatan untuk Kasus "Lompat Dokumen"

Jika user langsung minta dokumen di tengah pipeline (misal langsung minta SRS tanpa BRD/SOW/PRD sebelumnya):

- Boleh dikerjakan, tapi sebutkan di Background/Context bahwa dokumen-dokumen sebelumnya dalam pipeline belum tersedia, sehingga sebagian konteks (fitur, prioritas, user roles) diasumsikan langsung dari brief yang diberikan user.
- Tandai asumsi-asumsi tersebut secara eksplisit di section Assumptions, dan pertimbangkan status Quality Gate `NEEDS_USER_REVIEW` atau `NEEDS_REVISION` tergantung seberapa besar gap-nya.
- Tawarkan opsi ke user (di luar struktur dokumen, dalam 1-2 kalimat penutup): apakah ingin tetap lanjut dengan asumsi ini, atau mundur dulu untuk menyusun dokumen-dokumen sebelumnya agar referensinya lebih solid.
