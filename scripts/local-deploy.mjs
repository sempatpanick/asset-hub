import { spawnSync } from "node:child_process";
import { createHash, randomBytes } from "node:crypto";
import {
  closeSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  openSync,
  readFileSync,
  readSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { dirname, isAbsolute, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const shaPattern = /^[a-f0-9]{40}$/;
const idPattern = /^sha256:[a-f0-9]{64}$/;
const plumbing = [
  "deploy/backend.Dockerfile",
  "deploy/web.Dockerfile",
  "deploy/local-verify.Dockerfile",
  "deploy/compose.local-verify.yaml",
  "deploy/compose.yaml",
  "deploy/compose.production.yaml",
  "deploy/compose.no-backup.yaml",
  "deploy/compose-release.sh",
  "scripts/local-deploy.mjs",
  "scripts/ci/local-verify.sh",
  "scripts/ci/check-generated-api.mjs",
  "scripts/ci/deploy-release.sh",
  "tests/deploy/local-deploy.test.mjs",
  "tests/deploy/release.test.mjs",
];

export function validateConfig(config) {
  if (
    !/^[a-zA-Z0-9][a-zA-Z0-9.-]+$/.test(config.sshHost ?? "") ||
    !/^[a-z_][a-z0-9_-]*$/.test(config.sshUser ?? "") ||
    config.sshUser === "root" ||
    !Number.isInteger(config.sshPort ?? 22) ||
    (config.sshPort ?? 22) < 1 ||
    (config.sshPort ?? 22) > 65535 ||
    !isAbsolute(config.sshIdentityFile ?? "") ||
    typeof config.cloudflare !== "boolean" ||
    !/^\/srv\/[a-zA-Z0-9_./-]+$/.test(config.deployDirectory ?? "") ||
    config.deployDirectory.includes("..") ||
    config.deployDirectory === "/srv/"
  ) {
    throw new Error(
      "Invalid local deployment configuration. See scripts/local-deploy.example.json.",
    );
  }
  return config;
}

export function sshArguments(config) {
  return [
    "-o",
    "BatchMode=yes",
    "-o",
    "StrictHostKeyChecking=yes",
    "-o",
    "ConnectTimeout=30",
    "-o",
    "ServerAliveInterval=15",
    "-o",
    "ServerAliveCountMax=4",
    "-o",
    "IdentitiesOnly=yes",
    "-i",
    config.sshIdentityFile,
    ...(config.cloudflare
      ? ["-o", "ProxyCommand=cloudflared access ssh --hostname %h"]
      : []),
  ];
}

export function sourceArchiveArguments(sha, archive) {
  if (!shaPattern.test(sha)) throw new Error("Invalid release commit.");
  return [
    "-c",
    "core.autocrlf=false",
    "-c",
    "core.eol=lf",
    "archive",
    "--format=tar",
    `--output=${archive}`,
    sha,
  ];
}

function execute(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd,
    encoding: "utf8",
    stdio: options.capture ? ["ignore", "pipe", "pipe"] : "inherit",
    env: { ...process.env, ...options.env },
    maxBuffer: 16 * 1024 * 1024,
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    if (options.capture && result.stderr) process.stderr.write(result.stderr);
    throw new Error(
      `${command} failed with status ${result.status}. Deployment stopped.`,
    );
  }
  return result.stdout?.trim() ?? "";
}

function fileHash(path) {
  const descriptor = openSync(path, "r");
  const hash = createHash("sha256");
  const buffer = Buffer.alloc(1024 * 1024);
  try {
    let count;
    while ((count = readSync(descriptor, buffer, 0, buffer.length, null)) > 0)
      hash.update(buffer.subarray(0, count));
    return hash.digest("hex");
  } finally {
    closeSync(descriptor);
  }
}

export function verifySource({ root, source, sha, evidence, run = execute }) {
  if (!shaPattern.test(sha)) throw new Error("Invalid release commit.");
  mkdirSync(evidence, { recursive: true });
  const image = `assethub-local/verify:${sha}`;
  run(
    "docker",
    [
      "build",
      "--build-context",
      `automation=${root}`,
      "--tag",
      image,
      "--file",
      join(root, "deploy/local-verify.Dockerfile"),
      source,
    ],
    { cwd: root },
  );
  const compose = [
    "compose",
    "--project-name",
    `assethub-verify-${sha.slice(0, 12)}`,
    "--file",
    join(root, "deploy/compose.local-verify.yaml"),
  ];
  const options = {
    cwd: root,
    env: {
      LOCAL_VERIFY_IMAGE: image,
      LOCAL_EVIDENCE_DIRECTORY: evidence,
      LOCAL_TEST_POSTGRES_PASSWORD: randomBytes(32).toString("hex"),
    },
  };
  try {
    run("docker", [...compose, "config", "--quiet"], options);
    run(
      "docker",
      [
        ...compose,
        "up",
        "--abort-on-container-exit",
        "--exit-code-from",
        "verify",
        "verify",
      ],
      options,
    );
  } finally {
    // Only this release's disposable project is removed; it publishes no host ports.
    run("docker", [...compose, "down", "--volumes"], options);
  }
}

export function deployImages({
  root,
  source,
  sha,
  config,
  evidence,
  run = execute,
  verify = verifySource,
}) {
  validateConfig(config);
  if (!shaPattern.test(sha)) throw new Error("Invalid release commit.");
  const connection = sshArguments(config);
  const target = `${config.sshUser}@${config.sshHost}`;
  const ssh = [...connection, "-p", String(config.sshPort ?? 22), target];
  const remote = config.deployDirectory;
  const current = run(
    "ssh",
    [
      ...ssh,
      `set -eu; test "$(uname -m)" = aarch64; test "$(id -u)" != 0; cd '${remote}'; test -r .env; docker info > /dev/null; docker compose version > /dev/null; if test -f deploy/current-release; then cat deploy/current-release; fi`,
    ],
    { cwd: root, capture: true },
  );
  if (current === sha) return { status: "unchanged", sha };
  if (current && !shaPattern.test(current))
    throw new Error("The server returned an invalid active release.");
  if (current)
    run("git", ["merge-base", "--is-ancestor", current, sha], { cwd: root });
  const toolingHash = () =>
    plumbing.map((path) => fileHash(join(root, path))).join(":");
  const verifiedTooling = toolingHash();
  verify({ root, source, sha, evidence, run });
  if (toolingHash() !== verifiedTooling)
    throw new Error(
      "Deployment helpers changed during verification. Run again.",
    );
  const bundle = join(evidence, "bundle");
  mkdirSync(bundle, { recursive: true });
  const images = {};
  const ids = {};
  for (const service of ["backend", "web"]) {
    const image = `assethub-local/${service}:${sha}`;
    run(
      "docker",
      [
        "buildx",
        "build",
        "--platform",
        "linux/arm64",
        "--load",
        "--tag",
        image,
        "--file",
        join(root, `deploy/${service}.Dockerfile`),
        source,
      ],
      { cwd: root },
    );
    const inspection = run(
      "docker",
      [
        "image",
        "inspect",
        "--format",
        "{{.Id}} {{.Os}}/{{.Architecture}}",
        image,
      ],
      { cwd: root, capture: true },
    );
    const [id, platform] = inspection.split(" ");
    if (!idPattern.test(id) || platform !== "linux/arm64")
      throw new Error("Built image is not a verified ARM64 image.");
    images[service] = image;
    ids[service] = id;
  }
  run(
    "docker",
    [
      "image",
      "save",
      "--output",
      join(bundle, "images.tar"),
      images.backend,
      images.web,
    ],
    { cwd: root },
  );
  writeFileSync(
    join(bundle, "images.env"),
    `BACKEND_IMAGE=${images.backend}\nWEB_IMAGE=${images.web}\n`,
  );
  writeFileSync(
    join(bundle, "image-ids.env"),
    `BACKEND_IMAGE_ID=${ids.backend}\nWEB_IMAGE_ID=${ids.web}\n`,
  );
  const files = ["images.tar", "images.env", "image-ids.env"];
  for (const path of [
    "deploy/compose.yaml",
    "deploy/compose.production.yaml",
    "deploy/compose.no-backup.yaml",
    "deploy/compose-release.sh",
    "scripts/ci/deploy-release.sh",
  ]) {
    const name = path.split("/").at(-1);
    copyFileSync(join(root, path), join(bundle, name));
    files.push(name);
  }
  writeFileSync(
    join(bundle, "SHA256SUMS"),
    files.map((file) => `${fileHash(join(bundle, file))}  ${file}\n`).join(""),
  );
  const release = `deploy/releases/${sha}`;
  run(
    "ssh",
    [
      ...ssh,
      `set -eu; cd '${remote}'; mkdir -p '${release}'; chmod 700 '${release}'`,
    ],
    { cwd: root },
  );
  run(
    "scp",
    [
      ...connection,
      "-P",
      String(config.sshPort ?? 22),
      ...[...files, "SHA256SUMS"].map((file) => join(bundle, file)),
      `${target}:${remote}/${release}/`,
    ],
    { cwd: root },
  );
  run(
    "ssh",
    [
      ...ssh,
      `set -eu; cd '${remote}/${release}'; sha256sum --check SHA256SUMS; docker image load --input images.tar; cd '${remote}'; DEPLOY_IMAGE_MODE=local DEPLOY_EXPECTED_RELEASE='${current}' bash '${release}/deploy-release.sh' '${release}'; test "$(cat deploy/current-release)" = '${sha}'; bash deploy/compose-release.sh ps; rm -- '${release}/images.tar'`,
    ],
    { cwd: root },
  );
  return { status: "deployed", sha };
}

function main() {
  const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
  const state = join(root, ".local/local-deploy");
  mkdirSync(state, { recursive: true });
  const lock = join(state, "run.lock");
  const descriptor = openSync(lock, "wx", 0o600);
  writeFileSync(descriptor, `${process.pid}\n`);
  try {
    const verifyOnly = process.argv.includes("--verify-current");
    if (!verifyOnly)
      execute("git", ["fetch", "--no-tags", "origin", "main"], { cwd: root });
    const sha = execute(
      "git",
      ["rev-parse", verifyOnly ? "HEAD" : "refs/remotes/origin/main"],
      { cwd: root, capture: true },
    );
    if (!shaPattern.test(sha)) throw new Error("Invalid main commit.");
    const evidence = join(state, "releases", sha);
    mkdirSync(evidence, { recursive: true });
    if (verifyOnly) {
      verifySource({ root, source: root, sha, evidence });
      return;
    }
    const config = validateConfig(
      JSON.parse(readFileSync(join(state, "config.json"), "utf8")),
    );
    if (!existsSync(config.sshIdentityFile))
      throw new Error("Configured SSH identity file is missing.");
    const source = join(evidence, "source");
    mkdirSync(source, { recursive: true });
    const archive = join(evidence, "source.tar");
    execute("git", sourceArchiveArguments(sha, archive), {
      cwd: root,
    });
    execute("tar", ["-xf", archive, "-C", source], { cwd: root });
    const result = deployImages({ root, source, sha, config, evidence });
    writeFileSync(
      join(state, "last-result.json"),
      JSON.stringify(
        { ...result, finishedAt: new Date().toISOString() },
        null,
        2,
      ) + "\n",
    );
    console.log(`${result.status}: ${sha}`);
  } finally {
    closeSync(descriptor);
    unlinkSync(lock);
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  try {
    main();
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
