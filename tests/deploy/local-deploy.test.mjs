import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import {
  deployImages,
  archiveParts,
  readServerRelease,
  sourceArchiveArguments,
  sshArguments,
  transferVerifiedBundle,
  validateConfig,
  verifySource,
} from "../../scripts/local-deploy.mjs";
import {
  checkGeneratedAPI,
  snapshotGeneratedAPI,
} from "../../scripts/ci/check-generated-api.mjs";

const root = resolve(".");
const sha = "a".repeat(40);
const config = {
  sshHost: "ssh.example.test",
  sshUser: "panel",
  sshPort: 22,
  sshIdentityFile: resolve("test-only-identity"),
  cloudflare: true,
  deployDirectory: "/srv/projects/asset-hub",
};

function fixture(t, overrides = {}) {
  const evidence = mkdtempSync(join(tmpdir(), "assethub-local-deploy-test-"));
  t.after(() => {
    assert.equal(dirname(evidence), resolve(tmpdir()));
    rmSync(evidence, { recursive: true, force: true });
  });
  const calls = [];
  const run = (command, args, options) => {
    calls.push({ command, args, options });
    if (command === "ssh" && args.at(-1).includes("uname -m"))
      return overrides.current ?? "b".repeat(40);
    if (command === "docker" && args[0] === "image" && args[1] === "inspect") {
      return `sha256:${(args.at(-1).includes("/backend:") ? "1" : "2").repeat(64)} ${overrides.platform ?? "linux/arm64"}`;
    }
    if (command === "docker" && args[0] === "image" && args[1] === "save")
      writeFileSync(args[3], "test-only image archive");
    if (
      overrides.failVerify &&
      command === "docker" &&
      args.includes("run") &&
      args.at(-1) === "verify"
    )
      throw new Error("Verification failed");
    if (overrides.failInfrastructure && args.includes("up"))
      throw new Error("Test services failed to become healthy");
    if (overrides.failCleanup && args.includes("down"))
      throw new Error("Verification cleanup failed");
    return "";
  };
  return { root, source: root, sha, config, evidence, run, calls };
}

test("unchanged server releases perform no build, verification or transfer", (t) => {
  const f = fixture(t, { current: sha });
  assert.equal(deployImages(f).status, "unchanged");
  assert.equal(f.calls.length, 1);
  assert.equal(f.calls[0].command, "ssh");
});

function sshFailure(stderr, status = 255) {
  return Object.assign(new Error("SSH command failed"), { status, stderr });
}

test("Cloudflare handshakes have a longer bounded timeout with strict authentication", () => {
  const args = sshArguments(config);
  assert.ok(args.includes("ConnectTimeout=60"));
  assert.ok(args.includes("StrictHostKeyChecking=yes"));
  assert.ok(args.includes("BatchMode=yes"));
  assert.ok(args.includes("IdentitiesOnly=yes"));
  assert.ok(
    sshArguments({ ...config, cloudflare: false }).includes(
      "ConnectTimeout=30",
    ),
  );
});

test("read-only preflight recovers from banner timeouts with bounded backoff", () => {
  const calls = [];
  const pauses = [];
  const notices = [];
  const result = readServerRelease({
    root,
    config,
    run(command, args, options) {
      calls.push({ command, args, options });
      if (calls.length < 3)
        throw sshFailure("Connection timed out during banner exchange");
      return sha;
    },
    pause: (milliseconds) => pauses.push(milliseconds),
    onRetry: (message) => notices.push(message),
  });
  assert.equal(result, sha);
  assert.equal(calls.length, 3);
  assert.deepEqual(pauses, [2000, 4000]);
  assert.equal(notices.length, 2);
  for (const call of calls) {
    assert.equal(call.command, "ssh");
    assert.equal(call.options.timeout, 75_000);
    assert.match(call.args.at(-1), /cat deploy\/current-release/);
    assert.doesNotMatch(
      call.args.at(-1),
      /mkdir|docker image load|deploy-release\.sh/,
    );
  }
});

test("three preflight timeouts stop before verification, builds or transfer", (t) => {
  const f = fixture(t);
  const calls = [];
  f.run = (command, args) => {
    calls.push({ command, args });
    throw sshFailure("Connection to UNKNOWN port 65535 timed out");
  };
  f.pause = () => {};
  f.onRetry = () => {};
  assert.throws(() => deployImages(f), /SSH command failed/);
  assert.equal(calls.length, 3);
  assert.ok(calls.every((call) => call.command === "ssh"));
});

test("authentication, host-key and remote preflight failures are never retried", () => {
  for (const failure of [
    sshFailure("Permission denied (publickey,password)."),
    sshFailure("Host key verification failed."),
    sshFailure(
      "Connection to UNKNOWN port 65535 timed out\nPermission denied (publickey).",
    ),
    sshFailure(
      "Connection timed out during banner exchange\nREMOTE HOST IDENTIFICATION HAS CHANGED!",
    ),
    sshFailure("Remote Docker preflight failed", 1),
  ]) {
    let calls = 0;
    assert.throws(
      () =>
        readServerRelease({
          root,
          config,
          run() {
            calls++;
            throw failure;
          },
          pause: () => assert.fail("Permanent failures must not wait"),
        }),
      (error) => error === failure,
    );
    assert.equal(calls, 1);
  }
});

test("a timed-out rollout command is never replayed", (t) => {
  const f = fixture(t);
  const run = f.run;
  let rollouts = 0;
  f.run = (command, args, options) => {
    if (command === "ssh" && args.at(-1).includes("docker image load")) {
      rollouts++;
      throw sshFailure("Connection to UNKNOWN port 65535 timed out");
    }
    return run(command, args, options);
  };
  assert.throws(() => deployImages(f), /SSH command failed/);
  assert.equal(rollouts, 1);
  assert.equal(f.calls.filter((call) => call.command === "scp").length, 1);
});

test("connection resets in the read-only preflight recover without remote writes", () => {
  let calls = 0;
  const pauses = [];
  assert.equal(
    readServerRelease({
      root,
      config,
      run() {
        if (++calls === 1)
          throw sshFailure(
            "An existing connection was forcibly closed by the remote host.\nConnection closed by UNKNOWN port 65535",
          );
        return sha;
      },
      pause: (delay) => pauses.push(delay),
      onRetry: () => {},
    }),
    sha,
  );
  assert.equal(calls, 2);
  assert.deepEqual(pauses, [2000]);
});

test("image parts preserve exact bytes and have independent verified hashes", (t) => {
  const f = fixture(t);
  const bundle = join(f.evidence, "parts");
  mkdirSync(bundle);
  const bytes = Buffer.alloc(4 * 1024 * 1024 + 73, 29);
  writeFileSync(join(bundle, "images.tar"), bytes);
  const parts = archiveParts(bundle);
  assert.equal(parts.length, 2);
  const rebuilt = parts.map((part) => {
    const content = readFileSync(join(bundle, part.name));
    assert.equal(createHash("sha256").update(content).digest("hex"), part.hash);
    assert.ok(content.length <= 4 * 1024 * 1024);
    return content;
  });
  assert.deepEqual(Buffer.concat(rebuilt), bytes);
});

test("verified remote image parts are reused and assembly precedes complete checksum validation", (t) => {
  const f = fixture(t);
  const run = f.run;
  f.run = (command, args, options) => {
    const result = run(command, args, options);
    if (command === "docker" && args[0] === "image" && args[1] === "save")
      writeFileSync(args[3], Buffer.alloc(4 * 1024 * 1024 + 73, 29));
    if (command === "ssh" && args.at(-1).includes("sha256sum --check --status"))
      return args.at(-1).includes("000000.part") ? "verified" : "";
    return result;
  };
  assert.equal(deployImages(f).status, "deployed");
  const copies = f.calls.filter((call) => call.command === "scp");
  assert.equal(copies.length, 2);
  assert.equal(
    copies[0].args.some((arg) => arg.endsWith("/images.tar")),
    false,
  );
  assert.ok(copies[1].args.some((arg) => arg.endsWith("000001.part")));
  const command = f.calls.at(-1).args.at(-1);
  assert.ok(
    command.indexOf("cat '.image-parts/000000.part'") <
      command.indexOf("sha256sum --check SHA256SUMS"),
  );
  assert.ok(
    command.indexOf("sha256sum --check SHA256SUMS") <
      command.indexOf("docker image load"),
  );
});

test("interrupted transfers overwrite only the checked candidate and precede one rollout", (t) => {
  const f = fixture(t);
  const run = f.run;
  const pauses = [];
  let transfers = 0;
  f.run = (command, args, options) => {
    const result = run(command, args, options);
    if (command === "scp" && ++transfers < 3)
      throw sshFailure(
        "Connection to UNKNOWN port 65535 timed out\nscp: Connection closed",
      );
    return result;
  };
  f.pause = (delay) => pauses.push(delay);
  f.onRetry = () => {};
  assert.equal(deployImages(f).status, "deployed");
  assert.equal(transfers, 3);
  assert.deepEqual(pauses, [2000, 4000]);
  const copies = f.calls.filter((call) => call.command === "scp");
  assert.deepEqual(copies[0].args, copies[2].args);
  assert.ok(copies.every((call) => call.options.capture));
  assert.equal(
    f.calls.filter(
      (call) =>
        call.command === "ssh" &&
        call.args.at(-1).includes("docker image load"),
    ).length,
    1,
  );
});

test("tampered checked bundles stop before any server connection", (t) => {
  const f = fixture(t);
  deployImages(f);
  f.calls.length = 0;
  writeFileSync(join(f.evidence, "bundle/images.tar"), "changed archive");
  assert.throws(
    () => transferVerifiedBundle({ ...f, current: "b".repeat(40) }),
    /checksum mismatch/,
  );
  assert.equal(f.calls.length, 0);
});

test("an active release changed during builds prevents candidate staging and transfer", (t) => {
  const f = fixture(t);
  const run = f.run;
  let probes = 0;
  f.run = (command, args, options) => {
    const result = run(command, args, options);
    if (command === "ssh" && args.at(-1).includes("uname -m") && ++probes === 2)
      return "c".repeat(40);
    return result;
  };
  assert.throws(() => deployImages(f), /active release changed/);
  assert.equal(
    f.calls.some(
      (call) =>
        call.command === "scp" ||
        (call.command === "ssh" && call.args.at(-1).includes("mkdir")),
    ),
    false,
  );
});

test("failed real-service verification cleans only the disposable project and prevents rollout", (t) => {
  const f = fixture(t, { failVerify: true });
  assert.throws(() => deployImages(f), /Verification failed/);
  assert.equal(f.calls.at(-1).command, "docker");
  assert.ok(f.calls.at(-1).args.includes("down"));
  assert.ok(
    f.calls.at(-1).args.includes(`assethub-verify-${sha.slice(0, 12)}`),
  );
  assert.equal(
    f.calls.some((call) => call.command === "scp"),
    false,
  );
  assert.equal(
    f.calls.some((call) => call.args.includes("buildx")),
    false,
  );
});

test("successful checks precede ARM64 builds and a checksummed SSH rollout", (t) => {
  const f = fixture(t);
  assert.equal(deployImages(f).status, "deployed");
  const verification = f.calls.findIndex((call) => call.args.includes("run"));
  const startup = f.calls.findIndex((call) => call.args.includes("up"));
  const cleanup = f.calls.findIndex((call) => call.args.includes("down"));
  const build = f.calls.findIndex((call) => call.args.includes("buildx"));
  const transfer = f.calls.findIndex((call) => call.command === "scp");
  assert.ok(
    startup < verification && verification < cleanup && cleanup < build,
  );
  assert.ok(build < transfer);
  assert.ok(f.calls[startup].args.includes("--wait"));
  assert.ok(f.calls[verification].args.includes("--rm"));
  assert.ok(f.calls[verification].args.includes("--no-deps"));
  assert.ok(f.calls[verification].args.includes("--interactive=false"));
  assert.equal(
    f.calls.some((call) => call.args.includes("--abort-on-container-exit")),
    false,
  );
  assert.equal(f.calls[cleanup].options.timeout, 120_000);
  assert.ok(f.calls[build].args.includes("linux/arm64"));
  const command = f.calls.at(-1).args.at(-1);
  assert.ok(
    command.indexOf("sha256sum --check") < command.indexOf("docker image load"),
  );
  assert.match(
    command,
    /DEPLOY_IMAGE_MODE=local DEPLOY_EXPECTED_RELEASE='b{40}'/,
  );
  assert.match(command, /compose-release.sh ps/);
  assert.equal(
    f.calls.some(
      (call) => call.args.includes("login") || call.args.includes("push"),
    ),
    false,
  );
  const bundle = join(f.evidence, "bundle");
  assert.match(
    readFileSync(join(bundle, "images.env"), "utf8"),
    /BACKEND_IMAGE=assethub-local\/backend:a{40}/,
  );
  assert.match(
    readFileSync(join(bundle, "image-ids.env"), "utf8"),
    /WEB_IMAGE_ID=sha256:2{64}/,
  );
  assert.match(
    readFileSync(join(bundle, "SHA256SUMS"), "utf8"),
    /[a-f0-9]{64}  images.tar/,
  );
  assert.equal(
    f.calls[transfer].args.some((value) => value.endsWith(".env")),
    true,
  );
  assert.equal(
    f.calls[transfer].args.some((value) => value === join(root, ".env")),
    false,
  );
});

test("wrong-platform images prevent transfer even after checks pass", (t) => {
  const f = fixture(t, { platform: "linux/amd64" });
  assert.throws(() => deployImages(f), /ARM64/);
  assert.equal(
    f.calls.some((call) => call.command === "scp"),
    false,
  );
});

test("invalid server state and connection injection are rejected", (t) => {
  const f = fixture(t, { current: "a'; unexpected" });
  assert.throws(() => deployImages(f), /invalid active release/);
  for (const change of [
    { sshHost: "host; unwanted" },
    { sshUser: "root" },
    { sshPort: 0 },
    { deployDirectory: "/srv/project'; unwanted" },
    { deployDirectory: "/srv/../etc" },
    { sshIdentityFile: "relative-key" },
  ])
    assert.throws(() => validateConfig({ ...config, ...change }), /Invalid/);
});

test("a failing verifier always removes its isolated services", (t) => {
  const f = fixture(t, { failVerify: true });
  assert.throws(() => verifySource(f), /Verification failed/);
  assert.ok(f.calls.at(-1).args.includes("--volumes"));
});

test("unhealthy verification services prevent the verifier and rollout but still clean up", (t) => {
  const f = fixture(t, { failInfrastructure: true });
  assert.throws(() => deployImages(f), /failed to become healthy/);
  assert.ok(f.calls.at(-1).args.includes("down"));
  assert.equal(
    f.calls.some((call) => call.args.includes("run") || call.command === "scp"),
    false,
  );
});

test("failed verification cleanup prevents application builds and transfer", (t) => {
  const f = fixture(t, { failCleanup: true });
  assert.throws(() => deployImages(f), /Verification cleanup failed/);
  assert.equal(
    f.calls.some(
      (call) => call.args.includes("buildx") || call.command === "scp",
    ),
    false,
  );
});

test("generated API freshness accepts CRLF/LF differences but rejects changed content", (t) => {
  const f = fixture(t);
  const paths = [
    "packages/api-client/src/schema.d.ts",
    "apps/web/public/api-docs/openapi.json",
    "apps/web/public/api-docs/openapi.yaml",
  ];
  const original = "first line\r\nsecond line\r\n";
  for (const path of paths) {
    mkdirSync(dirname(join(f.evidence, path)), { recursive: true });
    writeFileSync(join(f.evidence, path), original);
  }
  const snapshot = snapshotGeneratedAPI(f.evidence);
  for (const path of paths)
    writeFileSync(join(f.evidence, path), original.replaceAll("\r\n", "\n"));
  assert.doesNotThrow(() => checkGeneratedAPI(f.evidence, snapshot));

  for (const path of paths) {
    for (const content of [
      "first line\nchanged contract\n",
      "first line \nsecond line\n",
      "first line\nsecond line",
    ]) {
      writeFileSync(join(f.evidence, path), content);
      assert.throws(
        () => checkGeneratedAPI(f.evidence, snapshot),
        (error) => {
          assert.ok(error.message.includes(path));
          return /outdated/.test(error.message);
        },
      );
    }
    writeFileSync(join(f.evidence, path), original);
  }
});

test("release archives retain LF content despite Windows Git line-ending settings", (t) => {
  const f = fixture(t);
  const run = (command, args) => {
    const result = spawnSync(command, args, {
      cwd: f.evidence,
      encoding: "utf8",
    });
    assert.equal(result.status, 0, result.stderr || result.error?.message);
    return result.stdout;
  };
  run("git", ["init", "--quiet"]);
  run("git", ["config", "core.autocrlf", "true"]);
  run("git", ["config", "core.eol", "crlf"]);
  writeFileSync(join(f.evidence, "contract.txt"), "contract\r\nunchanged\r\n");
  run("git", ["add", "contract.txt"]);
  run("git", [
    "-c",
    "user.name=Deployment Test",
    "-c",
    "user.email=deployment@example.test",
    "commit",
    "--quiet",
    "--no-gpg-sign",
    "-m",
    "Test line-ending conversion",
  ]);
  const commit = run("git", ["rev-parse", "HEAD"]).trim();
  const archive = join(f.evidence, "source.tar");
  run("git", sourceArchiveArguments(commit, archive));
  assert.equal(
    run("tar", ["-xOf", archive, "contract.txt"]),
    "contract\nunchanged\n",
  );
  assert.throws(() => sourceArchiveArguments("invalid", archive), /Invalid/);
});
