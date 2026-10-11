import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
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
  sourceArchiveArguments,
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
      args.includes("--exit-code-from")
    )
      throw new Error("Verification failed");
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
  const verification = f.calls.findIndex((call) =>
    call.args.includes("--exit-code-from"),
  );
  const build = f.calls.findIndex((call) => call.args.includes("buildx"));
  const transfer = f.calls.findIndex((call) => call.command === "scp");
  assert.ok(verification < build && build < transfer);
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
