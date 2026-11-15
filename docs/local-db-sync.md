# Local DB sync (`make v2-sync`)

Refresh your local v2 Postgres (docker container `goapps-v2-postgres`, `localhost:5440`) with realistic data, without ever touching raw production data.

## What it is

```
prod backup -> staging .60 (every Sunday ~06:30 WIB: restore + SANITIZE)
            -> p9-devsnap (Sunday ~09:30 WIB: pg_dump of the sanitized DBs, profiles full/slim/remote)
            -> your laptop (make v2-sync: rsync over ssh, resumable, sha256-verified, restored in your container)
```

Raw/unsanitized production data never reaches a laptop. The script refuses any snapshot whose database comments do not start with `sanitized p9`.

Profiles:
- `slim` (default): iam_db, ppc_db complete; finance_db without the data of the big history/log tables (schema kept). Much faster.
- `remote`: like `slim` but also without the data of `cst_product_cost` and `cst_mb_cost` (the largest finance tables); much smaller download for slow VPN links. Cost screens stay empty until you calculate data locally.
- `full`: everything (finance_db is ~30 GB restored, large download).

## One-time setup

Required tools: `bash`, `docker`, `ssh`, `rsync`, `sha256sum` or `shasum`. No local Postgres client needed. You also need `migrate` (golang-migrate) for `make v2-migrate`, see CLAUDE.md "Tool Installation".

| OS | Docker |
|----|--------|
| macOS | `brew install colima docker docker-compose rsync` (GNU rsync from brew is needed for resumable downloads; macOS built-in openrsync cannot resume) then `colima start`. The script auto-detects `~/.colima/default/docker.sock`. |
| Ubuntu | Docker Engine (`apt install docker.io docker-compose-v2 rsync`), add yourself to the `docker` group. |
| Windows | WSL2 Ubuntu + Docker Desktop (enable WSL integration for your distro). Run everything inside the WSL shell, and keep the repo and cache inside the WSL filesystem (not `/mnt/c`). |

1. Generate a key (skip if you already have one): `ssh-keygen -t ed25519 -C "dev-<yourname>"`
2. Send the PUBLIC key (`~/.ssh/id_ed25519.pub`) to the admin. Never send the private key. Wait for confirmation.
3. Outside the office, connect the VPN first (the server is `192.168.0.60`).
4. Start the local stack: `make v2-up`

Optional settings (env or a gitignored `.env.devsnap` in the repo root):
`DEVSNAP_HOST` (default 192.168.0.60), `DEVSNAP_USER` (devsync), `DEVSNAP_KEY` (path to a specific private key), `DEVSNAP_PROFILE` (slim|full|remote), `DEVSNAP_CACHE` (default `~/.cache/goapps/devsnap`).

## Daily use

```bash
make v2-sync                              # slim profile, asks for confirmation
make v2-sync ARGS="--profile full"        # everything
make v2-sync ARGS="--profile remote"      # slow VPN: smallest download, no cost results
make v2-sync ARGS="--dry-run"             # download + verify only
make v2-sync ARGS="--yes --keep-cache"    # no prompt, keep older cached snapshots
```

Steps: preflight, fetch MANIFEST, verify "sanitized p9", download (resumable, shows speed), sha256 verify, confirm, restore into `<db>_refresh` and swap, `make v2-migrate`, print versions. If a download is interrupted, re-run: it resumes, and files already cached and valid are skipped.

Timing: the download is bound by your link/VPN speed. finance_db dump sizes (2026-11): full ~2.7 GB, slim ~1.5 GB (~3 h at 135 KB/s VPN), remote ~60 MB (~8 min); restore of slim takes minutes, full much longer. After the first sync, a re-run with the same snapshot skips the download.

## After sync

- Application logins in the synced data now use the shared staging password: password manager entry `goapps/staging/app-default-password`.
- DataGrip/other connections (`ilham` / `dev_ilham`, etc.) are unchanged; `make v2-migrate` runs automatically at the end of the sync and re-applies grants.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `container goapps-v2-postgres is not running` / cannot connect to docker | `make v2-up`; on Mac `colima start`; check `DOCKER_HOST` |
| ssh timeout / cannot fetch snapshot | Connect VPN / office network; check `ssh -i <key> devsync@192.168.0.60` reaches the host |
| `Permission denied (publickey)` | Your public key is not registered, or the wrong key is used: set `DEVSNAP_KEY`; ask the admin |
| `checksum mismatch` | The bad file was removed from the cache; re-run `make v2-sync` |
| `REFUSING ... not 'sanitized p9'` | Do NOT work around it. Tell the admin; the snapshot is unsafe |
| not enough disk | Need ~3x the snapshot size free; use `slim` or `remote`, free space, or set `DEVSNAP_CACHE` |
| `pg_restore ... errors` | All-or-nothing: all three databases are restored as `*_refresh` first and swapped in one transaction, so on any error ALL local databases stay untouched. Check the message (version mismatch, disk) and re-run |
| `not enough disk inside container` | A restore needs ~12x the compressed snapshot (slim 1.5 GB → ~17 GB) free inside Docker, on top of the current databases. Colima: `colima stop && colima start --disk 100` (can only grow); Docker Desktop: Settings > Resources > Disk image size; `docker system prune` frees unused images |
| `profile 'remote' is not in this snapshot` | The snapshot was built before the remote profile existed; wait for the next Sunday build or use `slim` |
| snapshot missing / rsync error | Snapshot is built Sunday ~09:30 WIB; during build or refresh it may be briefly unavailable, retry |
| `v2-migrate` fails | Install `migrate` CLI; then run `make v2-migrate` manually |

## FAQ

- Why not direct from prod? Prod data is raw (personal/financial); only sanitized staging data may leave the server.
- How often does data change? Weekly: staging is refreshed Sunday ~06:30 WIB, the developer snapshot is built ~09:30 WIB.
- Why not `make v2-refresh`? It streams `pg_dump` over ssh and is unusable over VPN; it remains for the LAN.
