# GitHub Actions deployment

AssetHub follows the Discord moderation bot's deployment pattern: verify the committed code, build native ARM64 images on GitHub, publish private GHCR packages, and connect to the Orange Pi using Cloudflare Access SSH. The server pulls images and never builds the application from source.

## Workflows

`.github/workflows/ci.yml` checks pull requests and is reused by Deploy. Frontend checks include Prettier, application/tooling type checks, ESLint, deployment-script tests, API generation drift, production build, four default PostgreSQL/Redis-backed browser workflows and a separate real upload/deletion workflow with backups disabled. Backend checks include gofmt, compilation, go vet, fresh unit/architecture/integration tests with the race detector, the actual snapshot/offline-key recovery drill, and ARM64 cross-compilation. Infrastructure checks validate workflows, shell formatting/lint and source, image-only and disabled-backup Compose models.

The production dependency audit blocks releases. The full development-tool audit is reported separately with its existing unpatched braces advisory; it is not represented as a passing full audit. The local combined verification script retains its full audit gate.

`.github/workflows/deploy.yml` runs on main pushes and manual dispatch on main. It waits for all CI jobs, then publishes backend and web images on `ubuntu-24.04-arm`. GitHub supports this standard runner in private repositories. The workflow does not use QEMU. Actions are pinned to verified commit SHAs; cloudflared is pinned to 2026.10.0 with its release checksum.

The frontend builds once in CI. After browser tests and the production dependency audit pass, CI uploads its static export as a commit-specific tar artifact. The web publish job downloads that artifact from the same workflow run and selects the Dockerfile's `release` target, which packages the tested files into ARM64 Nginx without installing pnpm dependencies or running another Next.js build. An existing source-hashed web image skips both the artifact download and image build. Ordinary source Docker/Compose builds keep the default `source` target. Docker's [named build contexts](https://docs.docker.com/build/concepts/context/#named-contexts) carry the export separately from the source checkout.

Backend/api/worker/migrator share `ghcr.io/sempatpanick/asset-hub/backend`; the frontend uses `ghcr.io/sempatpanick/asset-hub/web`. Source hashes cover committed Docker build inputs, so unchanged images are reused. Deployment uses the registry's immutable image digests, recorded in release artifacts. No production secrets enter image builds or release artifacts.

## GitHub configuration

Add secrets in [AssetHub Actions secrets](https://github.com/sempatpanick/asset-hub/settings/secrets/actions):

| Secret                                        | Value                                                                                                |
| --------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| DEPLOY_HOST                                   | The same Orange Pi Cloudflare SSH hostname used by the Discord bot                                   |
| DEPLOY_USER                                   | Dedicated non-root SSH user with Docker access and permission to the installation directory          |
| DEPLOY_SSH_KEY                                | Its unencrypted private SSH key, including the BEGIN/END lines; not the public key                   |
| CF_ACCESS_CLIENT_ID / CF_ACCESS_CLIENT_SECRET | Optional pair if the existing Access SSH policy requires a service token                             |
| DEPLOY_KNOWN_HOSTS                            | Optional verified SSH host-key entry for DEPLOY_HOST; when set, strict host-key matching is required |

Without DEPLOY_KNOWN_HOSTS, the workflow uses the Discord bot's `accept-new` policy in a temporary known-hosts file. Do not run ssh-keyscan against a Cloudflare SSH hostname: it is not a raw SSH listener.

The repository variable DEPLOY_PATH is `/srv/projects/asset-hub`. GitHub automatically supplies GITHUB_TOKEN for package publication and private image pulls; no persistent GHCR password is required. Registry and SSH credentials are kept in temporary restricted directories and removed after the job. The registry token is passed to the server on SSH stdin rather than command arguments.

TUNNEL_TOKEN remains optional and blank when the existing Orange Pi connector handles the public dashboard. Access SSH credentials are separate from the dashboard's tunnel connector token. The Actions workflow does not change the connector or its hostname routes.

## Prepare the server once

Use the same non-root account as DEPLOY_USER, a 64-bit Linux OS, Docker and Docker Compose 2.24.4 or newer (`!reset` removes source-build definitions from production). From an AssetHub checkout on the server:

```bash
sudo install -d -o "$(id -u)" -g "$(id -g)" -m 0750 /srv/projects/asset-hub
# Mount the independent backup filesystem first; verify the physical disks.
sudo bash scripts/prepare-host.sh /srv/assethub /mnt/assethub-backup
cp .env.example /srv/projects/asset-hub/.env
chmod 600 /srv/projects/asset-hub/.env
```

Fill the protected server environment with the actual public HTTPS origin, PostgreSQL/runtime/Redis passwords, Telegram bot/channel, master and signing keys. Keep APP_ENV=production and DATA_ROOT=/srv/assethub. Backups default to enabled with BACKUP_PATH=/mnt/assethub-backup. To defer backup preparation, set the repository Actions variable BACKUP_ENABLED=false; skip prepare-host.sh for now. Deployment securely prepares staging/cache/local directories itself. Preserve existing keys and database passwords when upgrading; deployment never creates or replaces them. Follow docs/OPERATIONS.md for independent mounts and offline recovery copies. The workflow also stages the environment template and prepare-host script with each release, but never copies a real `.env`.

The deployment user must be able to write `/srv/projects/asset-hub` and run Docker. Keep the existing tunnel route pointed at http://127.0.0.1:3005. AssetHub's API/PostgreSQL/Redis bindings are 8085/5435/6385 on loopback. Ensure the Discord worker on the server uses 8095; the earlier local Windows port change does not prove the Orange Pi mapping changed. Deployment rejects Docker port conflicts before rollout and does not modify other applications. Bootstrap the first administrator interactively after the first healthy deployment:

```bash
cd /srv/projects/asset-hub
bash deploy/compose-release.sh run --rm api /app/admin bootstrap
```

## Release and recovery behavior

Push to main, or select **Actions → Deploy → Run workflow** on main. Concurrent releases queue instead of interrupting migrations. Pull-request jobs cannot publish images or connect to production.

Each release is staged under deploy/releases/COMMIT_SHA. Deployment validates both GHCR digests, the existing `.env`, production mode and port availability before any service is stopped. When BACKUP_ENABLED=true (the default), it also requires an independent backup mount and marker. When false, it removes backup mounts explicitly. A one-shot initializer prepares the SSD application directories before rollout. It downloads images, starts/checks AssetHub's databases, stops web/API/worker for a brief maintenance window, runs the migrator explicitly, then waits for API/worker/web health. It uses the fixed Compose project `assethub`, never `down`, volume deletion, or `--remove-orphans`. The optional tunnel container and unrelated applications are not restarted.

Only after successful health checks are the active Compose files, deploy/images.env, deploy/backup.env and deploy/current-release promoted. Old release directories retain their manifests and configuration. A failed migration leaves mutation services stopped and does not promote the release; a failed app-health check can leave candidate containers running. Review the failed stage and preserve the database before recovery. Deployment does not automatically reverse migrations, restore snapshots, rotate secrets or delete retained releases.

Maintain the existing nightly independent database snapshots and take a verified snapshot before a schema-changing release. An old image is not a safe database rollback. After checking schema compatibility, an operator can use an earlier retained release's Compose files and images.env against the existing protected `.env`; restore databases only through the supervised procedure in docs/OPERATIONS.md.

Successful CI or a healthy container does not replace live Telegram, physical backup, native Orange Pi endurance, HTTPS/cache and operator recovery release gates. Workflow failures show the exact missing server prerequisite. Reports and test data are disposable and contain no production configuration.

Sources: [GitHub native ARM64 runners in private repositories](https://github.blog/changelog/2026-01-29-arm64-standard-runners-are-now-available-in-private-repositories/), [publishing images to GHCR](https://docs.github.com/en/actions/tutorials/publish-packages/publish-docker-images), [Docker image manifest inspection](https://docs.docker.com/reference/cli/docker/buildx/imagetools/inspect/).

The BACKUP_ENABLED repository variable overrides only the backup policy, preserving server credentials. Use false temporarily; set true after provisioning an independent mount. Backup-off assets have no guaranteed recovery or retained deletion copy. Enabling backups later does not backfill them. See docs/OPERATIONS.md for the full mode transition procedure.

The BACKUP_ENABLED repository variable overrides only the backup policy, preserving server credentials. Use false temporarily; set true after provisioning an independent mount. Backup-off assets have no guaranteed recovery or retained deletion copy. Enabling backups later does not backfill them. See docs/OPERATIONS.md for the full mode transition procedure.
