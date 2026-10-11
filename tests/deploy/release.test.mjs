import { test } from "node:test";
import assert from "node:assert/strict";
import {
  mkdtempSync,
  mkdirSync,
  writeFileSync,
  readFileSync,
  copyFileSync,
  existsSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { spawnSync } from "node:child_process";

const root = resolve(".");
const bash =
  process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";
const posix = (path) =>
  path
    .replaceAll("\\", "/")
    .replace(/^([A-Za-z]):/, (_, drive) => `/${drive.toLowerCase()}`);
const backend = `ghcr.io/example/asset-hub/backend@sha256:${"1".repeat(64)}`;
const web = `ghcr.io/example/asset-hub/web@sha256:${"2".repeat(64)}`;
const release = `deploy/releases/${"a".repeat(40)}`;

function fixture(t) {
  const directory = mkdtempSync(join(tmpdir(), "assethub-release-test-"));
  t.after(() => {
    assert.equal(dirname(directory), resolve(tmpdir()));
    rmSync(directory, { recursive: true, force: true });
  });
  const bin = join(directory, "bin");
  mkdirSync(bin);
  const data = join(directory, "data");
  const backup = join(directory, "backup");
  for (const name of ["staging", "cache", "local"])
    mkdirSync(join(data, name), { recursive: true });
  mkdirSync(backup);
  writeFileSync(join(backup, ".assethub-backup"), "test-only");
  mkdirSync(join(directory, release), { recursive: true });
  writeFileSync(join(directory, ".env"), "test-only protected environment\n");
  writeFileSync(join(directory, "deploy/images.env"), "previous manifest\n");
  for (const name of [
    "compose.yaml",
    "compose.production.yaml",
    "compose.no-backup.yaml",
    "compose-release.sh",
  ]) {
    copyFileSync(join(root, "deploy", name), join(directory, release, name));
  }
  writeFileSync(
    join(directory, release, "images.env"),
    `BACKEND_IMAGE=${backend}\nWEB_IMAGE=${web}\n`,
  );
  const script = (name, content) =>
    writeFileSync(
      join(bin, name),
      `#!/usr/bin/env bash\nset -euo pipefail\n${content}\n`,
      { mode: 0o755 },
    );
  script("id", "printf '1001\\n'");
  script("mountpoint", '[[ "${FAKE_UNMOUNTED:-0}" != 1 ]]');
  script(
    "stat",
    'if [[ "${FAKE_SAME_FS:-0}" == 1 || "$*" == *"$FIXTURE_DATA"* ]]; then echo 1; else echo 2; fi',
  );
  script(
    "docker",
    `
printf '%s\\0' "$@" >> "$COMMAND_LOG"
printf '\\n' >> "$COMMAND_LOG"
if [[ "$*" == *'config --environment'* ]]; then
  printf 'APP_ENV=production\\nDATA_ROOT=%s\\nBACKUP_PATH=%s\\nBACKUP_ENABLED=%s\\n' "$FIXTURE_DATA" "$FIXTURE_BACKUP" "${"${BACKUP_ENABLED:-true}"}"
fi
if [[ "$1" == login ]]; then
  IFS= read -r credential
  [[ "$credential" == test-only-registry-token ]]
fi
if [[ "$1 $2" == 'image inspect' ]]; then
  if [[ "$*" == *'/backend:'* ]]; then task_id=1; else task_id=2; fi
  printf 'sha256:%s linux/%s\\n' "$(printf '%064d' 0 | tr 0 "$task_id")" "${"${FAKE_ARCH:-arm64}"}"
fi
if [[ "$1" == ps && "${"${FAKE_PORT_CONFLICT:-0}"}" == 1 ]]; then echo 'other-worker|other-project'; fi
if [[ "$*" == *'--exit-code-from migrate'* && "${"${FAKE_MIGRATION_FAILURE:-0}"}" == 1 ]]; then exit 7; fi
`,
  );
  const env = { ...process.env };
  for (const key of Object.keys(env))
    if (key.toUpperCase() === "PATH") delete env[key];
  Object.assign(env, {
    PATH: `${bin}${process.platform === "win32" ? ";" : ":"}${process.env.PATH ?? process.env.Path}`,
    MSYS_NO_PATHCONV: "1",
    COMMAND_LOG: posix(join(directory, "commands.log")),
    FIXTURE_DATA: posix(data),
    FIXTURE_BACKUP: posix(backup),
    FIXTURE_BIN: posix(bin),
    GHCR_USER: "test-user",
  });
  return {
    directory,
    bin,
    env,
    run(script, args = [], extra = {}) {
      return spawnSync(
        bash,
        [
          "-c",
          'export PATH="$FIXTURE_BIN:$PATH"; exec bash "$@"',
          "fixture",
          posix(join(root, script)),
          ...args,
        ],
        {
          cwd: directory,
          env: { ...env, ...extra },
          input: "test-only-registry-token\n",
          encoding: "utf8",
        },
      );
    },
    commands() {
      const file = join(directory, "commands.log");
      return existsSync(file)
        ? readFileSync(file, "utf8").replaceAll("\0", " ").trim()
        : "";
    },
  };
}

test("production Compose wrapper fixes the project and both environment files", (t) => {
  const f = fixture(t);
  const result = f.run("deploy/compose-release.sh", ["ps"]);
  assert.equal(result.status, 0, result.stderr);
  assert.match(
    f.commands(),
    /compose --project-name assethub --env-file \.env --env-file deploy\/images.env/,
  );
  assert.match(f.commands(), /-f deploy\/compose.production.yaml ps/);
});

test("mutable image tags are rejected before any Docker operation", (t) => {
  const f = fixture(t);
  writeFileSync(
    join(f.directory, release, "images.env"),
    `BACKEND_IMAGE=ghcr.io/example/backend:latest\nWEB_IMAGE=${web}\n`,
  );
  const result = f.run("scripts/ci/deploy-release.sh", [release]);
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /digests/);
  assert.equal(f.commands(), "");
});

for (const [condition, variable] of [
  ["missing backup mount", "FAKE_UNMOUNTED"],
  ["shared backup filesystem", "FAKE_SAME_FS"],
]) {
  test(`${condition} prevents login, pulls and service stops`, (t) => {
    const f = fixture(t);
    const result = f.run("scripts/ci/deploy-release.sh", [release], {
      [variable]: "1",
    });
    assert.notEqual(result.status, 0);
    assert.doesNotMatch(f.commands(), /login|pull| stop /);
    assert.equal(
      readFileSync(join(f.directory, "deploy/images.env"), "utf8"),
      "previous manifest\n",
    );
  });
}

test("another application's published port prevents any rollout", (t) => {
  const f = fixture(t);
  const result = f.run("scripts/ci/deploy-release.sh", [release], {
    FAKE_PORT_CONFLICT: "1",
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /another application/);
  assert.doesNotMatch(f.commands(), /login|pull| stop /);
});

test("failed migration never starts new app images or promotes a manifest", (t) => {
  const f = fixture(t);
  const result = f.run("scripts/ci/deploy-release.sh", [release], {
    FAKE_MIGRATION_FAILURE: "1",
  });
  assert.equal(result.status, 7, result.stderr);
  assert.match(f.commands(), /stop --timeout 45 web api worker/);
  assert.doesNotMatch(f.commands(), /wait-timeout 180 api worker web/);
  assert.equal(
    readFileSync(join(f.directory, "deploy/images.env"), "utf8"),
    "previous manifest\n",
  );
  assert.equal(existsSync(join(f.directory, "deploy/current-release")), false);
});

test("healthy deployment promotes only after pull, quiesce, migration and health checks", (t) => {
  const f = fixture(t);
  const result = f.run("scripts/ci/deploy-release.sh", [release]);
  assert.equal(result.status, 0, result.stderr);
  const commands = f.commands();
  assert.ok(
    commands.indexOf("pull storage-init") < commands.indexOf("stop --timeout"),
  );
  assert.ok(
    commands.indexOf("stop --timeout") <
      commands.indexOf("--exit-code-from migrate"),
  );
  assert.ok(
    commands.indexOf("--exit-code-from migrate") <
      commands.indexOf("wait-timeout 180 api worker web"),
  );
  assert.doesNotMatch(
    commands,
    /test-only-registry-token|--remove-orphans|\bdown\b|\bbuild\b(?! --)/,
  );
  assert.equal(
    readFileSync(join(f.directory, "deploy/images.env"), "utf8"),
    `BACKEND_IMAGE=${backend}\nWEB_IMAGE=${web}\n`,
  );
  assert.equal(
    readFileSync(join(f.directory, "deploy/current-release"), "utf8").trim(),
    "a".repeat(40),
  );
  assert.equal(
    readFileSync(join(f.directory, ".env"), "utf8"),
    "test-only protected environment\n",
  );
});

test("SSH rejects shell injection before connecting", (t) => {
  const f = fixture(t);
  const result = f.run("scripts/ci/deploy-ssh.sh", [], {
    DEPLOY_HOST: "ssh.example.test",
    DEPLOY_USER: "panel",
    DEPLOY_SSH_KEY: "test-key",
    DEPLOY_DIR: "/srv/projects/asset-hub'; touch /tmp/unwanted",
    GITHUB_SHA: "a".repeat(40),
    GHCR_TOKEN: "test-token",
  });
  assert.notEqual(result.status, 0);
  assert.equal(f.commands(), "");
});

test("explicitly disabled backups allow a missing mount without modifying server secrets", (t) => {
  const f = fixture(t);
  rmSync(join(f.directory, "backup/.assethub-backup"));
  const result = f.run("scripts/ci/deploy-release.sh", [release], {
    DEPLOY_BACKUP_ENABLED: "false",
    FAKE_UNMOUNTED: "1",
    FAKE_SAME_FS: "1",
  });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /Backups explicitly disabled/);
  assert.match(
    f.commands(),
    /compose.no-backup.yaml.*--exit-code-from storage-init/,
  );
  assert.equal(
    readFileSync(join(f.directory, "deploy/backup.env"), "utf8"),
    "BACKUP_ENABLED=false\n",
  );
  assert.equal(
    readFileSync(join(f.directory, ".env"), "utf8"),
    "test-only protected environment\n",
  );
});

test("invalid backup policy fails before pulling or stopping services", (t) => {
  const f = fixture(t);
  const result = f.run("scripts/ci/deploy-release.sh", [release], {
    DEPLOY_BACKUP_ENABLED: "maybe",
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /true or false/);
  assert.equal(f.commands(), "");
});

test("manual wrapper selects the disabled-backup Compose model", (t) => {
  const f = fixture(t);
  const result = f.run("deploy/compose-release.sh", ["ps"], {
    BACKUP_ENABLED: "false",
  });
  assert.equal(result.status, 0, result.stderr);
  assert.match(f.commands(), /compose.no-backup.yaml ps/);
});

test("image tags follow committed service inputs and ignore unrelated docs", (t) => {
  const f = fixture(t);
  for (const dir of ["services/backend", "apps/web", "packages/api-client"])
    mkdirSync(join(f.directory, dir), { recursive: true });
  copyFileSync(
    join(root, "deploy/image-tags.sh"),
    join(f.directory, "deploy/image-tags.sh"),
  );
  writeFileSync(join(f.directory, ".dockerignore"), ".local\n");
  writeFileSync(join(f.directory, "services/backend/main.go"), "backend one\n");
  writeFileSync(join(f.directory, "apps/web/page.tsx"), "web one\n");
  const git = (...args) => {
    const result = spawnSync("git", args, {
      cwd: f.directory,
      encoding: "utf8",
    });
    assert.equal(result.status, 0, result.stderr);
  };
  git("init", "-q");
  git("config", "user.name", "AssetHub test");
  git("config", "user.email", "test@example.test");
  const commit = () => {
    git("add", ".");
    git("commit", "-qm", "test fixture");
  };
  const tag = (service) => {
    const result = f.run("deploy/image-tags.sh", [service]);
    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout.trim(), /^source-[a-f0-9]{64}$/);
    return result.stdout.trim();
  };
  commit();
  const firstBackend = tag("backend");
  const firstWeb = tag("web");
  writeFileSync(join(f.directory, "services/backend/main.go"), "backend two\n");
  assert.equal(tag("backend"), firstBackend);
  commit();
  const secondBackend = tag("backend");
  assert.notEqual(secondBackend, firstBackend);
  assert.equal(tag("web"), firstWeb);
  writeFileSync(join(f.directory, "README.md"), "unrelated documentation\n");
  commit();
  assert.equal(tag("backend"), secondBackend);
  assert.equal(tag("web"), firstWeb);
});

function localImages(f) {
  writeFileSync(
    join(f.directory, release, "images.env"),
    `BACKEND_IMAGE=assethub-local/backend:${"a".repeat(40)}\nWEB_IMAGE=assethub-local/web:${"a".repeat(40)}\n`,
  );
  writeFileSync(
    join(f.directory, release, "image-ids.env"),
    `BACKEND_IMAGE_ID=sha256:${"1".repeat(64)}\nWEB_IMAGE_ID=sha256:${"2".repeat(64)}\n`,
  );
}

test("local images require matching IDs and ARM64 architecture before any service stops", (t) => {
  const f = fixture(t);
  localImages(f);
  const result = f.run("scripts/ci/deploy-release.sh", [release], {
    DEPLOY_IMAGE_MODE: "local",
    FAKE_ARCH: "amd64",
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /verified ARM64/);
  assert.doesNotMatch(f.commands(), /stop --timeout|pull|login/);
});

test("local rollout pulls only infrastructure and preserves exact loaded application images", (t) => {
  const f = fixture(t);
  localImages(f);
  const result = f.run("scripts/ci/deploy-release.sh", [release], {
    DEPLOY_IMAGE_MODE: "local",
  });
  assert.equal(result.status, 0, result.stderr);
  assert.match(f.commands(), /pull storage-init postgres redis/);
  assert.doesNotMatch(
    f.commands(),
    /login|pull storage-init postgres redis migrate/,
  );
  assert.match(f.commands(), /up -d --pull never --no-build.*api worker web/);
  assert.equal(
    readFileSync(join(f.directory, "deploy/current-release"), "utf8").trim(),
    "a".repeat(40),
  );
});

test("rollout keeps the server's existing backup policy", (t) => {
  const f = fixture(t);
  writeFileSync(
    join(f.directory, "deploy/backup.env"),
    "BACKUP_ENABLED=false\n",
  );
  const result = f.run("scripts/ci/deploy-release.sh", [release]);
  assert.equal(result.status, 0, result.stderr);
  assert.match(
    f.commands(),
    /--env-file \.env --env-file deploy\/backup.env --env-file deploy\/releases\//,
  );
});

test("changed active releases and concurrent deployments prevent rollout", (t) => {
  const f = fixture(t);
  writeFileSync(join(f.directory, "deploy/current-release"), "c".repeat(40));
  const result = f.run("scripts/ci/deploy-release.sh", [release], {
    DEPLOY_EXPECTED_RELEASE: "b".repeat(40),
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /active release changed/);
  assert.equal(f.commands(), "");
  mkdirSync(join(f.directory, "deploy/.deployment-lock"));
  const locked = f.run("scripts/ci/deploy-release.sh", [release]);
  assert.notEqual(locked.status, 0);
  assert.match(locked.stderr, /Another deployment/);
  assert.equal(f.commands(), "");
});
