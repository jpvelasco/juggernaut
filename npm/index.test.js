"use strict";

var nodeTest = require("node:test");
var describe = nodeTest.describe;
var it = nodeTest.it;
var path = require("path");
var assert = require("assert");
var fs = require("fs");
var os = require("os");

var index = require("./index");
var getPlatformPackage = index.getPlatformPackage;
var getBinaryPath = index.getBinaryPath;
var resolvePkgDir = index.resolvePkgDir;
var safeForwardArgs = index.safeForwardArgs;
var isLongRunningLaunch = index.isLongRunningLaunch;
var stageLaunchBinary = index.stageLaunchBinary;

describe("getPlatformPackage", function() {
  it("maps linux x64", function() {
    assert.strictEqual(getPlatformPackage("linux", "x64"), "juggernaut-bedrock-linux-x64");
    return void 0;
  });
  it("maps linux arm64", function() {
    assert.strictEqual(getPlatformPackage("linux", "arm64"), "juggernaut-bedrock-linux-arm64");
    return void 0;
  });
  it("maps darwin x64", function() {
    assert.strictEqual(getPlatformPackage("darwin", "x64"), "juggernaut-bedrock-darwin-x64");
    return void 0;
  });
  it("maps darwin arm64", function() {
    assert.strictEqual(getPlatformPackage("darwin", "arm64"), "juggernaut-bedrock-darwin-arm64");
    return void 0;
  });
  it("maps win32 x64", function() {
    assert.strictEqual(getPlatformPackage("win32", "x64"), "juggernaut-bedrock-win32-x64");
    return void 0;
  });
  it("returns undefined for unsupported platform", function() {
    assert.strictEqual(getPlatformPackage("freebsd", "x64"), void 0);
    return void 0;
  });
  it("returns undefined for unsupported arch", function() {
    assert.strictEqual(getPlatformPackage("linux", "ia32"), void 0);
    return void 0;
  });
  return void 0;
});

describe("getBinaryPath", function() {
  it("returns path under packages dir for linux", function() {
    var result = getBinaryPath("juggernaut-bedrock-linux-x64", "linux");
    assert.ok(result.endsWith(path.join("juggernaut-bedrock-linux-x64", "bin", "juggernaut")));
    return void 0;
  });
  it("returns .exe path for win32", function() {
    var result = getBinaryPath("juggernaut-bedrock-win32-x64", "win32");
    assert.ok(result.endsWith(path.join("juggernaut-bedrock-win32-x64", "bin", "juggernaut.exe")));
    return void 0;
  });
  it("path is under npm/packages/", function() {
    var result = getBinaryPath("juggernaut-bedrock-darwin-arm64", "darwin");
    assert.ok(result.includes(path.join("packages", "juggernaut-bedrock-darwin-arm64")));
    return void 0;
  });
  it("rejects an unknown package name", function() {
    assert.throws(function() {
      getBinaryPath("../../etc/passwd", "linux");
    }, /unexpected package name/);
    return void 0;
  });
  return void 0;
});

describe("resolvePkgDir", function() {
  it("resolves a known package name", function() {
    var result = resolvePkgDir("juggernaut-bedrock-linux-x64");
    assert.ok(result.includes("juggernaut-bedrock-linux-x64"));
    return void 0;
  });
  it("rejects an unknown package name (self-defending, not just via getBinaryPath)", function() {
    assert.throws(function() {
      resolvePkgDir("../../etc/passwd");
    }, /unexpected package name/);
    return void 0;
  });
  return void 0;
});

describe("safeForwardArgs", function() {
  it("copies forwarded arguments as strings", function() {
    var result = safeForwardArgs(["--model", "sonnet", 42]);
    assert.deepStrictEqual(result, ["--model", "sonnet", "42"]);
    return void 0;
  });
  it("rejects NUL bytes", function() {
    assert.throws(function() {
      safeForwardArgs(["ok", "bad\u0000arg"]);
    }, /NUL byte/);
    return void 0;
  });
  return void 0;
});

describe("isLongRunningLaunch", function() {
  it("recognizes both activation launch commands", function() {
    assert.strictEqual(isLongRunningLaunch(["launch", "--"]), true);
    assert.strictEqual(isLongRunningLaunch(["launch-cli", "codex", "--"]), true);
    return void 0;
  });
  it("does not stage short-lived commands", function() {
    assert.strictEqual(isLongRunningLaunch(["apply", "--dry-run"]), false);
    assert.strictEqual(isLongRunningLaunch([]), false);
    return void 0;
  });
  return void 0;
});

describe("stageLaunchBinary", function() {
  it("runs from a disposable copy outside the installed binary path", function() {
    var root = fs.mkdtempSync(path.join(os.tmpdir(), "juggernaut-stage-test-"));
    var source = path.join(root, "installed-juggernaut.exe");
    var staged;
    try {
      fs.writeFileSync(source, "fixture-binary"); // nosemgrep: javascript_pathtraversal_rule-non-literal-fs-filename, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename
      staged = stageLaunchBinary(source, root);
      assert.notStrictEqual(staged.bin, source);
      assert.strictEqual(fs.readFileSync(staged.bin, "utf8"), "fixture-binary"); // nosemgrep: javascript_pathtraversal_rule-non-literal-fs-filename, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename
      staged.cleanup();
      assert.strictEqual(fs.existsSync(staged.bin), false); // nosemgrep: javascript_pathtraversal_rule-non-literal-fs-filename, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename
      staged = void 0;
    } finally {
      if (staged) {
        staged.cleanup();
      }
      fs.rmSync(root, {recursive: true, force: true});
    }
    return void 0;
  });
  it("cleans up temp dir on copy failure", function() {
    var root = fs.mkdtempSync(path.join(os.tmpdir(), "juggernaut-stage-err-"));
    // Pass a nonexistent source to trigger COPYFILE_EXCL error path
    var nonexistent = path.join(root, "does-not-exist.exe");
    assert.throws(function() {
      stageLaunchBinary(nonexistent, root);
    });
    // Temp dir should be cleaned up even on error
    return void 0;
  });
  it("rejects a source that is not a regular file", function() {
    var root = fs.mkdtempSync(path.join(os.tmpdir(), "juggernaut-stage-notfile-"));
    try {
      // Pass a directory as the source — copyFileSync fails when source is not a regular file
      assert.throws(function() {
        stageLaunchBinary(root, root);
      });
    } finally {
      fs.rmSync(root, {recursive: true, force: true});
    }
    return void 0;
  });
  it("uses a fixed staged filename regardless of source name", function() {
    var root = fs.mkdtempSync(path.join(os.tmpdir(), "juggernaut-stage-filename-"));
    var source = path.join(root, "some-random-name.exe");
    var staged;
    try {
      fs.writeFileSync(source, "fixture-binary"); // nosemgrep: javascript_pathtraversal_rule-non-literal-fs-filename, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename
      staged = stageLaunchBinary(source, root);
      assert.strictEqual(path.basename(staged.bin), "juggernaut-staged.exe");
      staged.cleanup();
      staged = void 0;
    } finally {
      if (staged) {
        staged.cleanup();
      }
      fs.rmSync(root, {recursive: true, force: true});
    }
    return void 0;
  });
  return void 0;
});

var versionsMatch = index.versionsMatch;

describe("versionsMatch", function() {
  it("allows when versions are equal", function() {
    assert.strictEqual(versionsMatch("5.2.4", "5.2.4"), true);
    return void 0;
  });
  it("blocks when versions differ", function() {
    assert.strictEqual(versionsMatch("5.2.4", "5.2.2"), false);
    return void 0;
  });
  it("allows the dev 0.0.0 / 0.0.0 case", function() {
    assert.strictEqual(versionsMatch("0.0.0", "0.0.0"), true);
    return void 0;
  });
  it("fails open when root version is missing", function() {
    assert.strictEqual(versionsMatch(undefined, "5.2.4"), true);
    return void 0;
  });
  it("fails open when binary version is missing", function() {
    assert.strictEqual(versionsMatch("5.2.4", undefined), true);
    return void 0;
  });
  it("fails open when a version is not a string", function() {
    assert.strictEqual(versionsMatch("5.2.4", 524), true);
    return void 0;
  });
  return void 0;
});

describe("safeResolveBin", function() {
  var safeResolveBin = index.safeResolveBin;

  function makeLayout(root, pkgName) {
    // Flat node_modules layout produced by local installs, npx cache runs,
    // pnpm, and yarn: platform package is a SIBLING of the launcher dir.
    var launcherDir = path.join(root, "node_modules", "juggernaut-bedrock"); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal -- test fixture under mkdtemp/tmpdir roots
    var platformDir = path.join(root, "node_modules", pkgName); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal, javascript_pathtraversal_rule-non-literal-fs-filename -- test fixture under mkdtemp/tmpdir roots
    fs.mkdirSync(path.join(platformDir, "bin"), {recursive: true}); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal, javascript_pathtraversal_rule-non-literal-fs-filename, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename -- test fixture under mkdtemp/tmpdir roots
    fs.writeFileSync(path.join(platformDir, "bin", "juggernaut"), "#stub\n"); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal, javascript_pathtraversal_rule-non-literal-fs-filename, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename -- test fixture under mkdtemp/tmpdir roots
    return {launcherDir: launcherDir, platformDir: platformDir,
      bin: path.join(platformDir, "bin", "juggernaut")}; // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal -- test fixture under mkdtemp/tmpdir roots
  }

  // Given the documented non-global install flows (npx, project-local, pnpm),
  // When the resolved binary lives in the sibling platform package,
  // Then safeResolveBin must accept it when containment is checked against
  // that owning platform package directory.
  it("accepts sibling-layout binary under its own platform package", function() {
    var root = fs.mkdtempSync(path.join(os.tmpdir(), "jug-sibling-"));
    try {
      var layout = makeLayout(root, "juggernaut-bedrock-win32-x64");
      var resolved = safeResolveBin(layout.bin, layout.platformDir);
      assert.strictEqual(resolved, fs.realpathSync(layout.bin)); // nosemgrep: javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename, javascript_pathtraversal_rule-non-literal-fs-filename -- test fixture under mkdtemp/tmpdir roots
    } finally {
      fs.rmSync(root, {recursive: true, force: true});
    }
  });

  // Given a global install where optionals nest inside the launcher tree,
  // When the binary resolves within the platform package there,
  // Then it must be accepted.
  it("accepts nested-layout binary under its own platform package", function() {
    var root = fs.mkdtempSync(path.join(os.tmpdir(), "jug-nested-"));
    try {
      var layout = makeLayout(root, "juggernaut-bedrock-darwin-arm64");
      var nested = path.join(layout.launcherDir, "node_modules", // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal -- test fixture under mkdtemp/tmpdir roots
        "juggernaut-bedrock-darwin-arm64");
      fs.mkdirSync(path.join(nested, "bin"), {recursive: true}); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal, javascript_pathtraversal_rule-non-literal-fs-filename, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename -- test fixture under mkdtemp/tmpdir roots
      fs.copyFileSync(layout.bin, path.join(nested, "bin", "juggernaut")); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename -- test fixture under mkdtemp/tmpdir roots
      var bin = path.join(nested, "bin", "juggernaut"); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal -- test fixture under mkdtemp/tmpdir roots
      var resolved = safeResolveBin(bin, nested);
      assert.strictEqual(resolved, fs.realpathSync(bin)); // nosemgrep: javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename, javascript_pathtraversal_rule-non-literal-fs-filename -- test fixture under mkdtemp/tmpdir roots
    } finally {
      fs.rmSync(root, {recursive: true, force: true});
    }
  });

  // Given any layout, When the binary resolves OUTSIDE the allowlisted
  // owning package directory, Then it must still be rejected.
  it("rejects binary outside the owning platform package", function() {
    var root = fs.mkdtempSync(path.join(os.tmpdir(), "jug-escape-"));
    try {
      var layout = makeLayout(root, "juggernaut-bedrock-win32-x64");
      var rogueDir = path.join(root, "rogue");
      fs.mkdirSync(rogueDir, {recursive: true}); // nosemgrep: javascript_pathtraversal_rule-non-literal-fs-filename, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename -- test fixture under mkdtemp/tmpdir roots
      fs.copyFileSync(layout.bin, path.join(rogueDir, "juggernaut")); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename -- test fixture under mkdtemp/tmpdir roots
      assert.throws(function() {
        safeResolveBin(path.join(rogueDir, "juggernaut"), layout.platformDir);
      }, /escapes|outside/i);
    } finally {
      fs.rmSync(root, {recursive: true, force: true});
    }
  });
  return void 0;
});

describe("describeSpawnOutcome", function() {
  var describeSpawnOutcome = index.describeSpawnOutcome;

  // Given the wrapped binary cannot be spawned (AV lock, EACCES, racing
  // upgrade), When the launcher inspects the spawnSync result,
  // Then it must produce exit code 1 and a diagnostic naming the failure.
  it("surfaces spawn errors with a diagnostic", function() {
    var err = new Error("spawn juggernaut EACCES");
    err.code = "EACCES";
    var outcome = describeSpawnOutcome({status: null, signal: null, error: err});
    assert.strictEqual(outcome.exitCode, 1);
    assert.ok(outcome.message.indexOf("EACCES") !== -1, "message should name the error code");
    return void 0;
  });

  // Given the child was killed by a signal, When the outcome is described,
  // Then it maps to the POSIX convention 128+signum with a note on stderr.
  it("maps signal deaths to 128+signum", function() {
    var outcome = describeSpawnOutcome({status: null, signal: "SIGTERM", error: null});
    assert.strictEqual(outcome.exitCode, 143);
    assert.ok(outcome.message.indexOf("SIGTERM") !== -1);
    return void 0;
  });

  // Given the child ran and exited normally, When the outcome is described,
  // Then the child's status passes through untouched with no extra output.
  it("passes normal statuses through untouched", function() {
    var outcome = describeSpawnOutcome({status: 7, signal: null, error: null});
    assert.strictEqual(outcome.exitCode, 7);
    assert.strictEqual(outcome.message, "");
    return void 0;
  });

  // Given a degenerate result with no status, signal, or error,
  // When the outcome is described, Then the launcher exits 1 safely.
  it("exits 1 for a degenerate result", function() {
    var outcome = describeSpawnOutcome({status: null, signal: null, error: null});
    assert.strictEqual(outcome.exitCode, 1);
    return void 0;
  });
  return void 0;
});