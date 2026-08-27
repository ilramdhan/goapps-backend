# Panduan: Menyamakan Database Lokal dengan Production

> Panduan langkah demi langkah, dari nol sampai database lokal berisi data production.
> Untuk alasan teknis di balik setiap keputusan, lihat [`README.md`](./README.md).
>
> **Yang perlu Anda tahu di depan:** seluruh proses ini **tidak menulis apa pun ke
> production**. Ia hanya membaca. Tiga pengaman mencegah kesalahan arah, dan
> ketiganya sudah diuji dengan cara benar-benar dijalankan — bukan sekadar dibaca.
> Bagian §7 menjelaskan masing-masing.

---

## Yang akan terjadi

Sekarang lokal Anda punya 3 database terpisah, production punya 2 yang berbeda susunannya:

| Service | Lokal sekarang | Production |
|---|---|---|
| finance | `finance_db` @ `:5434` | DB **`goapps`** |
| iam | `iam_db` @ `:5435` | DB **`goapps`** — *sama dengan finance* |
| ppc | `ppc_db` @ `:5436` | DB **`ppc_db`** |

Setelah panduan ini, Anda punya **container keempat** di port `:5433` berisi 2 database
persis seperti production, terisi data production yang sudah discrub.

**Setup 3-database Anda yang lama tidak disentuh sama sekali.** Ia tetap jalan di
port 5434/5435/5436. Anda berpindah antara keduanya hanya dengan satu perintah
`source`, dan bisa kembali kapan saja.

Perkiraan waktu: 15–30 menit, tergantung ukuran dump dan kecepatan port-forward.

---

## §1. Prasyarat — pasang dulu, ini yang paling sering bikin gagal

### 1a. Klien PostgreSQL (WAJIB — saat ini belum ada di PATH Anda)

Dicek pada 2026-08-24: `pg_dump`, `pg_restore`, dan `psql` **tidak ada di PATH Anda**.
Skrip akan berhenti dengan `pg_dump not found` kalau ini dilewati.

Homebrew tidak menaut `libpq` ke PATH secara otomatis. Anda sudah punya `libpq@18`
terpasang, jadi cukup tambahkan ke PATH:

```bash
echo 'export PATH="/opt/homebrew/opt/libpq/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

Kalau belum terpasang: `brew install libpq`.

**Cek:**
```bash
pg_dump --version    # harus 18.x
psql --version
```

> **Versi klien harus ≥ versi server.** Production menjalankan PostgreSQL 18, jadi
> `pg_dump` 16 atau 17 akan menolak dengan *"server version mismatch"*. Anda punya
> `libpq` dan `libpq@18`; yang di `/opt/homebrew/opt/libpq/bin` sudah 18.6. Kalau
> `pg_dump --version` menunjukkan angka lebih rendah, ada libpq lama yang lebih dulu
> di PATH — jalankan `which -a pg_dump` untuk menemukannya.

### 1b. kubectl

Juga belum ada di PATH Anda. Diperlukan untuk port-forward ke production.

```bash
kubectl version --client    # harus jalan tanpa error
```

Kalau belum ada: `brew install kubectl`, lalu pastikan kubeconfig produksi Anda aktif
(`kubectl config current-context`).

### 1c. Docker

Sudah ada (`/opt/homebrew/bin/docker`). Pastikan Docker Desktop berjalan.

### 1d. Akses

- Bisa `kubectl port-forward` ke namespace `database` di cluster production.
- Password database production, dari secret:
  ```bash
  kubectl get secret postgres-secret -n database \
    -o jsonpath='{.data.POSTGRES_PASSWORD}' | base64 -d
  ```

---

## §2. Langkah 1 — nyalakan container mirror

Dari `goapps-backend/`:

```bash
docker compose -f docker-compose.mirror.yaml up -d
```

Ini membuat container `goapps-mirror-postgres` di port **5433**, terpisah penuh dari
compose 3-database Anda (project name-nya sendiri, volume-nya sendiri).

**Checkpoint — jangan lanjut sebelum ini benar:**

```bash
docker ps --format '{{.Names}}\t{{.Status}}' | grep mirror
# harapkan: goapps-mirror-postgres   Up ... (healthy)

psql -h localhost -p 5433 -U postgres -d postgres \
  -c "SELECT datname FROM pg_database WHERE datistemplate=false ORDER BY 1"
# harapkan: goapps, postgres, ppc_db
```
Password: `postgres`.

Kalau `goapps` dan `ppc_db` belum ada, `init-mirror.sql` gagal jalan. Ulangi bersih:
```bash
docker compose -f docker-compose.mirror.yaml down -v
docker compose -f docker-compose.mirror.yaml up -d
```

> Container ini juga menanam tabel penanda `local_mirror_marker` di database `postgres`.
> Itulah yang membuat skrip sync mengenali "ini mirror yang boleh saya timpa". Lihat §7.

---

## §3. Langkah 2 — port-forward production

Di terminal **terpisah**, biarkan berjalan selama proses:

```bash
kubectl port-forward -n database svc/postgres 15432:5432
```

> **Arahkan ke `svc/postgres`, bukan PgBouncer.** PgBouncer berjalan dalam mode
> transaction pooling, yang tidak bisa mempertahankan satu snapshot konsisten yang
> dibutuhkan `pg_dump`. Hasilnya bisa gagal, atau lebih buruk: salinan yang robek,
> sambil memakan slot pool yang dibutuhkan calc worker. Skrip memperingatkan bila
> mencurigai ini.

**Checkpoint:**
```bash
PGPASSWORD='<password dari §1d>' psql -h localhost -p 15432 -U postgres \
  -d postgres -tAc "SELECT version()"
# harapkan: PostgreSQL 18.x ...
```

---

## §4. Langkah 3 — jalankan sync

```bash
export PGPASSWORD='<password production dari §1d>'
./scripts/local-mirror/sync-from-prod.sh
```

Skrip berjalan dalam empat fase, dan mencetak progres tiap fase:

| Fase | Yang terjadi |
|---|---|
| **Preflight** | Cek tool ada; cek mirror hidup dan bertanda; cek sumber ≠ tujuan |
| **Dump** | `pg_dump` `goapps` dan `ppc_db` dari prod — **read-only**, dibatasi timeout |
| **Restore** | DROP + CREATE database di **mirror**, lalu `pg_restore` |
| **Anonymize** | Scrub data pribadi dan kredensial di `goapps` |
| **Verify** | Bandingkan versi migrasi mirror dengan file migrasi di disk |

Fase dump adalah satu-satunya yang menyentuh production, dan hanya membaca.

**Berapa lama?** Dump bergantung ukuran database dan bandwidth port-forward. File
dump muncul di `scripts/local-mirror/dumps/<timestamp>/` sambil jalan — pantau
ukurannya kalau ingin tahu progres.

### Kalau gagal di tengah

Log per-database ada di `dumps/<timestamp>/*.dump.log` dan `*.restore.log`.

| Pesan | Artinya | Tindakan |
|---|---|---|
| `canceling statement due to lock timeout` | Ada migrasi/DDL berjalan di prod. Dump mundur **dengan sengaja** — tidak ada yang rusak di prod. | Tunggu selesai, ulangi. |
| `server version mismatch` | Klien lebih tua dari server | §1a |
| `REFUSING TO WRITE` | Target bukan mirror | §7, guard 1 |
| `REFUSING TO RUN: same postgres server` | Sumber dan tujuan server yang sama | §7, guard 2 |

Skrip aman diulang. Ia selalu DROP + CREATE database mirror lebih dulu, jadi tidak ada
sisa dari percobaan yang gagal.

---

## §5. Langkah 4 — arahkan service ke mirror

```bash
source scripts/local-mirror/env.mirror.sh
cd services/finance && make run
```

Ini mengekspor `DATABASE_*` (finance + iam → `goapps` @ 5433) dan `PPC_DATABASE_*`
(ppc → `ppc_db` @ 5433).

**Tidak perlu mengubah `config.yaml` sama sekali.** Ketiga service sudah memakai
viper `AutomaticEnv`, jadi variabel lingkungan menimpa file config
(`v.AutomaticEnv()` — finance `config.go:208`, iam `config.go:213`, ppc `config.go:165`).

Kembali ke setup 3-database lama:
```bash
source scripts/local-mirror/env.mirror.sh --unset
```

> Variabel ini hanya hidup di shell tempat Anda `source`. Terminal lain masih memakai
> setup lama — hati-hati, ini sumber kebingungan yang umum. Cek dengan `echo $DATABASE_PORT`
> (5433 = mirror, kosong = setup lama).

---

## §6. Langkah 5 — verifikasi hasilnya

```bash
psql -h localhost -p 5433 -U postgres -d goapps <<'SQL'
\echo '=== jumlah tabel (harus jauh lebih banyak dari finance_db lama) ==='
SELECT count(*) FROM information_schema.tables
WHERE table_schema='public' AND table_type='BASE TABLE';

\echo '=== versi migrasi ==='
SELECT 'finance' AS svc, version, dirty FROM schema_migrations_finance
UNION ALL SELECT 'iam', version, dirty FROM schema_migrations_iam;

\echo '=== anonimisasi benar-benar jalan? ==='
SELECT count(*) AS email_belum_discrub FROM mst_user
WHERE email NOT LIKE '%@example.local';          -- harus 0
SELECT count(*) AS sesi_tersisa FROM user_sessions;  -- harus 0

\echo '=== data bisnis harus TETAP ADA ==='
SELECT count(*) AS jumlah_user FROM mst_user;    -- harus > 0
SQL
```

Login ke aplikasi memakai **username production Anda** dengan password **`Dev12345!`**.
Username sengaja dipertahankan (itu identitas login Anda); yang diganti adalah password,
email, dan data pribadi.

---

## §7. Kenapa ini tidak akan merusak production

Tiga pengaman, semuanya dibuktikan dengan dijalankan, bukan hanya ditulis:

**Guard 1 — penanda mirror.** Sebelum menulis apa pun, skrip menuntut tabel
`local_mirror_marker` ada di target. Hanya `init-mirror.sql` yang membuatnya. Kalau
`LOCAL_PORT` salah ketik, atau port-forward tanpa sengaja mendarat di 5433, skrip
berhenti sebelum satu pun `DROP DATABASE`.

Diuji: mengarahkan skrip ke database finance asli di `:5434` menghasilkan penolakan:
> *"NOT the disposable container -- it may be a real database. This script drops
> databases, so it stops here."*

**Guard 2 — deteksi server yang sama.** Membandingkan
`pg_control_system().system_identifier` sumber dan tujuan. Kalau identik, sumber dan
tujuan adalah server yang sama — skrip berhenti daripada mendump prod ke atas dirinya
sendiri.

**Guard 3 — batas beban di production.** Semua perintah ke prod berjalan dengan:

| Setting | Alasan |
|---|---|
| `default_transaction_read_only=on` | Sesi ke prod tidak bisa menulis, bahkan bila ada bug di skrip |
| `lock_timeout=10s` | Kalau ada DDL berjalan, dump **mundur** alih-alih mengantre di belakang lock dan memblokir query lain — ini yang mencegah "database down" |
| `statement_timeout=30min` | Batas atas satu perintah |
| `idle_in_transaction_session_timeout=5min` | Dump terlantar tidak menahan snapshot dan memblokir vacuum |

Pengaturan ini **tidak diekspor** ke lingkungan global; ia diterapkan per-perintah
prod saja. (Sebelumnya sempat diekspor dan itu justru merusak fase restore lokal
dengan `cannot execute DROP DATABASE in a read-only transaction` — ketahuan karena
skripnya benar-benar dijalankan, lalu diperbaiki.)

**Yang skrip lakukan ke production:** `SELECT version()`, `SELECT system_identifier`,
dan dua `pg_dump` read-only. Tidak ada yang lain. Tidak ada DDL, tidak ada tulis,
tidak ada perubahan setting server.

---

## §8. Alternatif tanpa menyentuh production sama sekali

Kalau masih ragu menyentuh prod — dan ini pilihan yang wajar — production sudah
mendump kedua database **tiga kali sehari** ke MinIO
(`goapps-infra/base/backup/cronjobs/postgres-backup.yaml`). Menariknya dari sana
memberi **nol beban** ke database production:

```bash
mc alias set prod https://<minio-endpoint> <access> <secret>
mc ls prod/postgres-backups/ | tail -5
mc cp prod/postgres-backups/production_goapps_<ts>.sql.gz /tmp/
mc cp prod/postgres-backups/production_ppc_db_<ts>.sql.gz /tmp/
```

File-file itu **plain gzip**, bukan custom format, jadi restore dengan `psql`
(bukan `pg_restore`):

```bash
gunzip -c /tmp/production_goapps_<ts>.sql.gz | psql -h localhost -p 5433 -U postgres -d goapps
psql -h localhost -p 5433 -U postgres -d goapps -f scripts/local-mirror/anonymize.sql
```

Konsekuensinya hanya data berumur sampai 8 jam. Untuk kerja pengembangan, biasanya
tidak masalah. **Kalau Anda ragu, mulailah dari sini.**

---

## §9. Kebersihan

**Dump berisi data production sungguhan.** `scripts/local-mirror/dumps/` sudah masuk
`.gitignore` — pastikan tetap begitu sebelum commit. Hapus dump lama setelah tidak
dipakai.

`--no-anonymize` menaruh data pribadi asli di laptop Anda. Jangan pakai kecuali ada
alasan spesifik dan Anda paham konsekuensinya.

**Bongkar:**
```bash
docker compose -f docker-compose.mirror.yaml down -v   # -v ikut menghapus datanya
source scripts/local-mirror/env.mirror.sh --unset
```

---

## §10. Ringkasan perintah

```bash
# sekali saja
echo 'export PATH="/opt/homebrew/opt/libpq/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc

# tiap kali ingin data segar
docker compose -f docker-compose.mirror.yaml up -d       # terminal 1
kubectl port-forward -n database svc/postgres 15432:5432 # terminal 2, biarkan jalan
export PGPASSWORD='<password prod>'                      # terminal 1
./scripts/local-mirror/sync-from-prod.sh                 # terminal 1
source scripts/local-mirror/env.mirror.sh                # terminal 1
cd services/finance && make run
```

Flag lain: `--dump-only` (hanya bikin file dump), `--from-dir DIR` (restore dump yang
sudah ada, tanpa akses prod), `--no-anonymize` (data mentah — hati-hati).
