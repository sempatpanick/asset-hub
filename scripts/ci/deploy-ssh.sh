#!/usr/bin/env bash
set -euo pipefail
: "${DEPLOY_HOST:?Set the DEPLOY_HOST secret}"
: "${DEPLOY_USER:?Set the DEPLOY_USER secret}"
: "${DEPLOY_SSH_KEY:?Set the DEPLOY_SSH_KEY secret}"
: "${DEPLOY_DIR:?Set the deployment directory}"
: "${GITHUB_SHA:?A release commit is required}"
: "${GHCR_USER:?A registry username is required}"
: "${GHCR_TOKEN:?A registry token is required}"
case "${DEPLOY_BACKUP_ENABLED:-}" in '' | true | false) ;; *)
  echo 'BACKUP_ENABLED must be true or false.' >&2
  exit 1
  ;;
esac
[[ "$DEPLOY_HOST" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]+$ && "$DEPLOY_HOST" == *.* ]] || {
  echo 'Use the Cloudflare SSH hostname.' >&2
  exit 1
}
[[ "$DEPLOY_USER" =~ ^[a-z_][a-z0-9_-]*$ && "$DEPLOY_USER" != root ]] || exit 1
[[ "$DEPLOY_DIR" =~ ^/srv/[a-zA-Z0-9_./-]+$ && "$DEPLOY_DIR" != *..* && "$DEPLOY_DIR" != /srv/ ]] || exit 1
[[ "$GITHUB_SHA" =~ ^[a-f0-9]{40}$ && "$GHCR_USER" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 1
task_tmp_root=$(realpath -- "${TMPDIR:-/tmp}")
task_ssh=$(mktemp -d "$task_tmp_root/assethub-ssh.XXXXXXXX")
[[ $(realpath -- "$task_ssh") == "$task_tmp_root"/assethub-ssh.* ]] || exit 1
cleanup() { rm -rf -- "$task_ssh"; }
trap cleanup EXIT
chmod 700 "$task_ssh"
printf '%s\n' "$DEPLOY_SSH_KEY" | sed 's/\r$//' > "$task_ssh/key"
chmod 600 "$task_ssh/key"
ssh-keygen -y -P '' -f "$task_ssh/key" > /dev/null
task_host_policy=accept-new
if [[ -n "${DEPLOY_KNOWN_HOSTS:-}" ]]; then
  printf '%s\n' "$DEPLOY_KNOWN_HOSTS" > "$task_ssh/known_hosts"
  task_host_policy=yes
fi
cat > "$task_ssh/config" << EOF
Host orangepi
  HostName ${DEPLOY_HOST}
  User ${DEPLOY_USER}
  IdentityFile ${task_ssh}/key
  IdentitiesOnly yes
  BatchMode yes
  ConnectTimeout 30
  ServerAliveInterval 15
  ServerAliveCountMax 4
  StrictHostKeyChecking ${task_host_policy}
  UserKnownHostsFile ${task_ssh}/known_hosts
  ProxyCommand cloudflared access ssh --hostname %h
EOF
task_remote_release="$DEPLOY_DIR/deploy/releases/$GITHUB_SHA"
ssh -F "$task_ssh/config" orangepi "mkdir -p '$task_remote_release'"
scp -F "$task_ssh/config" deploy/compose.yaml deploy/compose.production.yaml deploy/compose.no-backup.yaml deploy/compose-release.sh \
  deploy/images.env scripts/ci/deploy-release.sh .env.example scripts/prepare-host.sh "orangepi:$task_remote_release/"
printf '%s\n' "$GHCR_TOKEN" | ssh -F "$task_ssh/config" orangepi \
  "cd '$DEPLOY_DIR' && GHCR_USER='$GHCR_USER' DEPLOY_BACKUP_ENABLED='${DEPLOY_BACKUP_ENABLED:-}' bash 'deploy/releases/$GITHUB_SHA/deploy-release.sh' 'deploy/releases/$GITHUB_SHA'"
