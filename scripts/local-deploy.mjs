import { spawnSync } from "node:child_process";
import { createHash, randomBytes } from "node:crypto";
import {
  closeSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readFileSync,
  readSync,
  statSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { dirname, isAbsolute, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const shaPattern = /^[a-f0-9]{40}$/;
const idPattern = /^sha256:[a-f0-9]{64}$/;
const plumbing = [
  ".dockerignore",
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
  "scripts/ci/browser-test.sh",
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
    `ConnectTimeout=${config.cloudflare ? 60 : 30}`,
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
    timeout: options.timeout,
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    if (options.capture && result.stderr) process.stderr.write(result.stderr);
    throw Object.assign(
      new Error(
        `${command} failed with status ${result.status}. Deployment stopped.`,
      ),
      { status: result.status, stderr: result.stderr ?? "" },
    );
  }
  return result.stdout?.trim() ?? "";
}

function pauseBriefly(milliseconds) {
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, milliseconds);
}

function retryTransport(operation, { pause, onRetry, label }) {
  for (let attempt = 1; ; attempt++) {
    try {
      return operation();
    } catch (failure) {
      const stderr = failure.stderr ?? "";
      const transient =
        failure.status === 255 &&
        !/Permission denied|Host key verification failed|REMOTE HOST IDENTIFICATION HAS CHANGED|websocket: bad handshake|unauthorized|forbidden/i.test(
          stderr,
        ) &&
        /Connection timed out during banner exchange|Connection to [^\r\n]+ timed out|Timeout, server [^\r\n]+ not responding|Broken pipe|Connection (?:closed|reset) by (?:UNKNOWN|remote host|peer)|An existing connection was forcibly closed by the remote host/i.test(
          stderr,
        );
      if (!transient || attempt === 3) throw failure;
      const delay = attempt * 2_000;
      onRetry(
        `${label} connection failed (attempt ${attempt}/3). Retrying in ${delay / 1_000}s.`,
      );
      pause(delay);
    }
  }
}

export function readServerRelease({
  root,
  config,
  run = execute,
  pause = pauseBriefly,
  onRetry = (message) => console.error(message),
}) {
  validateConfig(config);
  const ssh = [
    ...sshArguments(config),
    "-p",
    String(config.sshPort ?? 22),
    `${config.sshUser}@${config.sshHost}`,
  ];
  const command = `set -eu; test "$(uname -m)" = aarch64; test "$(id -u)" != 0; cd '${config.deployDirectory}'; test -r .env; docker info > /dev/null; docker compose version > /dev/null; if test -f deploy/current-release; then cat deploy/current-release; fi`;
  return retryTransport(
    () =>
      run("ssh", [...ssh, command], {
        cwd: root,
        capture: true,
        timeout: 75_000,
      }),
    { pause, onRetry, label: "SSH preflight" },
  );
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
  // A fresh directory prevents a previous verification's export from being reused.
  const webExport = mkdtempSync(join(evidence, "web-export-"));
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
      LOCAL_WEB_EXPORT_DIRECTORY: webExport,
      LOCAL_TEST_POSTGRES_PASSWORD: randomBytes(32).toString("hex"),
      COMPOSE_ANSI: "never",
      COMPOSE_PROGRESS: "plain",
    },
  };
  try {
    const startupOptions = { ...options, timeout: 180_000 };
    run("docker", [...compose, "config", "--quiet"], startupOptions);
    run(
      "docker",
      [
        ...compose,
        "up",
        "--detach",
        "--wait",
        "--wait-timeout",
        "120",
        "postgres",
        "redis",
      ],
      startupOptions,
    );
    // A one-off verifier returns its exit code without an attached up shutdown loop.
    run(
      "docker",
      [
        ...compose,
        "run",
        "--rm",
        "--no-deps",
        "--interactive=false",
        "-T",
        "verify",
      ],
      options,
    );
  } finally {
    // Only this release's disposable project is removed; it publishes no host ports.
    run("docker", [...compose, "down", "--volumes", "--timeout", "10"], {
      ...options,
      timeout: 120_000,
    });
  }
  return webExport;
}

export function deployImages({
  root,
  source,
  sha,
  config,
  evidence,
  run = execute,
  verify = verifySource,
  pause = pauseBriefly,
  onRetry = (message) => console.error(message),
}) {
  validateConfig(config);
  if (!shaPattern.test(sha)) throw new Error("Invalid release commit.");
  const current = readServerRelease({ root, config, run, pause, onRetry });
  if (current === sha) return { status: "unchanged", sha };
  if (current && !shaPattern.test(current))
    throw new Error("The server returned an invalid active release.");
  if (current)
    run("git", ["merge-base", "--is-ancestor", current, sha], { cwd: root });
  const toolingHash = () =>
    plumbing.map((path) => fileHash(join(root, path))).join(":");
  const verifiedTooling = toolingHash();
  const webExport = verify({ root, source, sha, evidence, run });
  if (toolingHash() !== verifiedTooling)
    throw new Error(
      "Deployment helpers changed during verification. Run again.",
    );
  if (
    !webExport ||
    !existsSync(join(webExport, "index.html")) ||
    statSync(join(webExport, "index.html")).size === 0
  )
    throw new Error("Verification did not produce a tested static web export.");
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
        ...(service === "web"
          ? [
              "--target",
              "release",
              "--build-context",
              `web-export=${webExport}`,
            ]
          : []),
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
  return transferVerifiedBundle({
    root,
    config,
    sha,
    evidence,
    current,
    run,
    pause,
    onRetry,
  });
}

export function transferVerifiedBundle({
  root,
  config,
  sha,
  evidence,
  current,
  run = execute,
  pause = pauseBriefly,
  onRetry = (message) => console.error(message),
}) {
  validateConfig(config);
  if (!shaPattern.test(sha) || (current && !shaPattern.test(current)))
    throw new Error("Invalid release commit.");
  const bundle = join(evidence, "bundle");
  const files = [
    "images.tar",
    "images.env",
    "image-ids.env",
    "compose.yaml",
    "compose.production.yaml",
    "compose.no-backup.yaml",
    "compose-release.sh",
    "deploy-release.sh",
  ];
  const entries = readFileSync(join(bundle, "SHA256SUMS"), "utf8")
    .trim()
    .split(/\r?\n/);
  const remaining = new Set(files);
  for (const entry of entries) {
    const match = /^([a-f0-9]{64})  (.+)$/.exec(entry);
    if (
      !match ||
      !remaining.delete(match[2]) ||
      fileHash(join(bundle, match[2])) !== match[1]
    )
      throw new Error("Release bundle checksum mismatch. Deployment stopped.");
  }
  if (remaining.size)
    throw new Error("Incomplete release bundle. Deployment stopped.");
  if (readServerRelease({ root, config, run, pause, onRetry }) !== current)
    throw new Error(
      "The active release changed before transfer. Deployment stopped.",
    );
  const connection = sshArguments(config);
  const target = `${config.sshUser}@${config.sshHost}`;
  const ssh = [...connection, "-p", String(config.sshPort ?? 22), target];
  const remote = config.deployDirectory;
  const release = `deploy/releases/${sha}`;
  const parts =
    statSync(join(bundle, "images.tar")).size > 4 * 1024 * 1024
      ? archiveParts(bundle)
      : [];
  retryTransport(
    () =>
      run(
        "ssh",
        [
          ...ssh,
          `set -eu; cd '${remote}'; mkdir -p '${release}/.image-parts'; chmod 700 '${release}' '${release}/.image-parts'`,
        ],
        { cwd: root, capture: true, timeout: 75_000 },
      ),
    { pause, onRetry, label: "Release staging" },
  );
  retryTransport(
    () =>
      run(
        "scp",
        [
          ...connection,
          "-P",
          String(config.sshPort ?? 22),
          ...[
            ...files.filter((file) => !parts.length || file !== "images.tar"),
            "SHA256SUMS",
          ].map((file) => join(bundle, file)),
          `${target}:${remote}/${release}/`,
        ],
        { cwd: root, capture: true },
      ),
    { pause, onRetry, label: "Release transfer" },
  );
  for (const [index, part] of parts.entries()) {
    const check = `set -eu; cd '${remote}/${release}'; if printf '%s  %s\n' '${part.hash}' '${part.name}' | sha256sum --check --status 2>/dev/null; then printf verified; fi`;
    const present = retryTransport(
      () =>
        run("ssh", [...ssh, check], {
          cwd: root,
          capture: true,
          timeout: 75_000,
        }),
      { pause, onRetry, label: "Image part check" },
    );
    if (present === "verified") continue;
    console.error(`Uploading image part ${index + 1}/${parts.length}.`);
    retryTransport(
      () =>
        run(
          "scp",
          [
            ...connection,
            "-P",
            String(config.sshPort ?? 22),
            join(bundle, part.name),
            `${target}:${remote}/${release}/${part.name}`,
          ],
          { cwd: root, capture: true },
        ),
      { pause, onRetry, label: `Image part ${index + 1}` },
    );
  }
  const assemble = parts.length
    ? `cat ${parts.map((part) => `'${part.name}'`).join(" ")} > images.tar; `
    : "";
  run(
    "ssh",
    [
      ...ssh,
      `set -eu; cd '${remote}/${release}'; ${assemble}sha256sum --check SHA256SUMS; docker image load --input images.tar; cd '${remote}'; DEPLOY_IMAGE_MODE=local DEPLOY_EXPECTED_RELEASE='${current}' bash '${release}/deploy-release.sh' '${release}'; test "$(cat deploy/current-release)" = '${sha}'; bash deploy/compose-release.sh ps; rm -- '${release}/images.tar'; ${parts.length ? `rm -- ${parts.map((part) => `'${remote}/${release}/${part.name}'`).join(" ")}; rmdir '${remote}/${release}/.image-parts'` : ":"}`,
    ],
    { cwd: root },
  );
  return { status: "deployed", sha };
}

export function archiveParts(bundle) {
  mkdirSync(join(bundle, ".image-parts"), { recursive: true });
  const descriptor = openSync(join(bundle, "images.tar"), "r");
  const buffer = Buffer.alloc(4 * 1024 * 1024);
  const parts = [];
  try {
    let count;
    while ((count = readSync(descriptor, buffer, 0, buffer.length, null)) > 0) {
      const name = `.image-parts/${String(parts.length).padStart(6, "0")}.part`;
      const bytes = buffer.subarray(0, count);
      writeFileSync(join(bundle, name), bytes);
      parts.push({
        name,
        hash: createHash("sha256").update(bytes).digest("hex"),
      });
    }
  } finally {
    closeSync(descriptor);
  }
  return parts;
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
