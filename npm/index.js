#!/usr/bin/env node
"use strict";

var path = require("path");
var childProcess = require("node:child_process");
var fs = require("fs");
var os = require("os");

var PLATFORM_MAP = {
  "linux-x64": "juggernaut-bedrock-linux-x64",
  "linux-arm64": "juggernaut-bedrock-linux-arm64",
  "darwin-x64": "juggernaut-bedrock-darwin-x64",
  "darwin-arm64": "juggernaut-bedrock-darwin-arm64",
  "win32-x64": "juggernaut-bedrock-win32-x64"
};

var VALID_PACKAGES = [
  "juggernaut-bedrock-linux-x64",
  "juggernaut-bedrock-linux-arm64",
  "juggernaut-bedrock-darwin-x64",
  "juggernaut-bedrock-darwin-arm64",
  "juggernaut-bedrock-win32-x64"
];

/**
 * @param {string} platform
 * @param {string} arch
 * @returns {string|void}
 */
function getPlatformPackage(platform, arch) {
  return PLATFORM_MAP[platform + "-" + arch] || void 0;
}

function containsPackage(pkgName) {
  for (var i = 0; i < VALID_PACKAGES.length; i++) {
    if (VALID_PACKAGES[i] === pkgName) {
      return true;
    }
  }
  return false;
}

/**
 * Resolves the directory of a platform package. Self-defending: it rejects any
 * name not in the VALID_PACKAGES allowlist before building a path, so the
 * fallback path.join below can only ever join a known constant package name
 * (no separators, no "..") under __dirname and cannot be made to escape it.
 * @param {string} pkgName
 * @returns {string}
 */
function resolvePkgDir(pkgName) {
  if (!containsPackage(pkgName)) {
    throw new Error("unexpected package name: " + pkgName);
  }
  try {
    return path.dirname(require.resolve(pkgName + "/package.json"));
  } catch (_) {
    // pkgName is allowlist-validated above, so this join stays under __dirname.
    return path.join(__dirname, "packages", pkgName); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal
  }
}

function getBinaryPath(pkgName, platform) {
  if (!containsPackage(pkgName)) {
    throw new Error("unexpected package name: " + pkgName);
  }
  var binaryName = platform === "win32" ? "juggernaut.exe" : "juggernaut";
  // pkgName validated above and again inside resolvePkgDir; binaryName is a
  // local constant. The joined path therefore cannot escape the package dir.
  return path.join(resolvePkgDir(pkgName), "bin", binaryName); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal
}

/**
 * Resolves bin to a real absolute path and asserts it stays within
 * __dirname, preventing any tainted or traversed path from executing.
 * @param {string} binPath
 * @returns {string}
 */
function safeResolveBin(binPath) {
  var real = fs.realpathSync(binPath); // nosemgrep: javascript_pathtraversal_rule-non-literal-fs-filename, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename
  var base = fs.realpathSync(__dirname); // nosemgrep: javascript_pathtraversal_rule-non-literal-fs-filename
  if (!real.startsWith(base + path.sep) && real !== base) {
    throw new Error("binary path escapes package directory: " + real);
  }
  return real;
}

function safeForwardArgs(args) {
  var forwarded = [];
  for (var i = 0; i < args.length; i++) {
    var arg = String(args[i]);
    if (arg.indexOf("\u0000") !== -1) {
      throw new Error("invalid NUL byte in argument");
    }
    forwarded.push(arg);
  }
  return forwarded;
}

function isLongRunningLaunch(args) {
  return args.length > 0 && (args[0] === "launch" || args[0] === "launch-cli");
}

/**
 * Copies the standalone Go executable out of the npm package before a
 * long-running Windows launch. Windows locks a running .exe; running this copy
 * leaves npm free to replace or remove the installed package during a session.
 *
 * Safety: `bin` is validated by safeResolveBin (realpath'd + under __dirname).
 * The temp dir is OS-generated via mkdtempSync (unique, unpredictable suffix).
 * The staged filename is a fixed constant. No user input reaches any path.
 *
 * @param {string} bin  validated realpath (from safeResolveBin)
 * @param {string=} tempRoot  optional temp directory root for testing
 * @returns {{bin: string, cleanup: function(): void}}
 */
function stageLaunchBinary(bin, tempRoot) {
  var root = tempRoot || os.tmpdir();
  var tempDir = fs.mkdtempSync(path.join(root, "juggernaut-launch-")); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename — OS temp dir, constant prefix, unique suffix
  // Pin the staged filename to a known constant — no user input in the path.
  var stagedBin = path.join(tempDir, "juggernaut-staged.exe"); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename — tempDir is mkdtemp-generated; filename is a fixed constant
  try {
    // bin is validated by safeResolveBin (realpath'd + under __dirname).
    // COPYFILE_EXCL prevents overwriting an existing staged file.
    fs.copyFileSync(bin, stagedBin, fs.constants.COPYFILE_EXCL); // nosemgrep: javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename — COPYFILE_EXCL prevents overwrite; both paths validated above
  } catch (err) {
    fs.rmSync(tempDir, {recursive: true, force: true}); // nosemgrep: javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename — mkdtemp-generated temp dir
    throw err;
  }
  return {
    bin: stagedBin,
    cleanup: function() {
      fs.rmSync(tempDir, {recursive: true, force: true}); // nosemgrep: javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename — mkdtemp-generated temp dir
    }
  };
}


/**
 * @param {*} rootVersion
 * @param {*} binVersion
 * @returns {boolean} true to allow exec, false to block on confirmed skew
 */
function versionsMatch(rootVersion, binVersion) {
  // Fail open: if either version is unreadable/non-string, do not block.
  if (typeof rootVersion !== "string" || typeof binVersion !== "string") {
    return true;
  }
  return rootVersion === binVersion;
}

/**
 * @param {string} pkgJsonPath
 * @returns {string|void} the version field, or undefined on any failure
 */
function readPkgVersion(pkgJsonPath) {
  try {
    var raw = fs.readFileSync(pkgJsonPath, "utf8"); // nosemgrep: javascript_pathtraversal_rule-non-literal-fs-filename, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename
    return JSON.parse(raw).version;
  } catch (_) {
    return void 0;
  }
}
if (require.main === module) {
  var pkg = getPlatformPackage(process.platform, process.arch);
  if (!pkg) {
    process.stderr.write(
      "juggernaut-bedrock: unsupported platform " + process.platform + "/" + process.arch + "\n" +
      "Please file an issue: https://github.com/jpvelasco/juggernaut/issues\n"
    );
    process.exit(1);
  }

  var binRaw = getBinaryPath(pkg, process.platform);
  if (!fs.existsSync(binRaw)) { // nosemgrep: javascript_pathtraversal_rule-non-literal-fs-filename, javascript.lang.security.audit.detect-non-literal-fs-filename.detect-non-literal-fs-filename
    process.stderr.write(
      "juggernaut-bedrock: binary not found at " + binRaw + "\n" +
      "Try reinstalling: npm install -g juggernaut-bedrock\n" +
      "If the problem persists, file an issue: https://github.com/jpvelasco/juggernaut/issues\n"
    );
    process.exit(1);
  }

  var bin = safeResolveBin(binRaw);
  var rootVersion = readPkgVersion(path.join(__dirname, "package.json"));
  // Read the version from the package that owns the binary we just validated,
  // not by re-resolving `pkg` independently — so the skew check compares the
  // version of the exact binary we are about to execute. `bin` was realpath'd
  // and asserted to be contained under __dirname by safeResolveBin above, so
  // binPkgDir is derived from an already-validated path (the path-join finding
  // below is a false positive on that basis).
  var binPkgDir = path.dirname(path.dirname(bin));
  var binVersion = readPkgVersion(path.join(binPkgDir, "package.json")); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal
  if (!versionsMatch(rootVersion, binVersion)) {
    process.stderr.write(
      "juggernaut-bedrock is in a broken or partially-updated state " +
      "(launcher v" + rootVersion + ", binary v" + binVersion + ").\n" +
      "This usually happens when the package was updated while a Claude Code " +
      "session was running.\n" +
      "Close all `claude` sessions and terminals, then re-run:\n" +
      "  npm install -g juggernaut-bedrock\n"
    );
    process.exit(1);
  }
  var args = safeForwardArgs(process.argv.slice(2));
  var staged = process.platform === "win32" && isLongRunningLaunch(args)
    ? stageLaunchBinary(bin)
    : void 0;
  var runBin = staged ? staged.bin : bin;
  // When staging, pass the original installed binary path so resolveBinary
  // can also skip PATH candidates hardlinked to it (os.Executable() returns
  // the temp copy, not the installed binary).
  var launchEnv = Object.assign({}, process.env);
  if (staged) {
    launchEnv.JUGGERNAUT_ORIGINAL_BIN = bin;
  }
  var result;
  try {
    result = childProcess.spawnSync(runBin, args, {
      stdio: "inherit",
      env: launchEnv,
      shell: false,
      windowsHide: true
    });
  } finally {
    if (staged) {
      // Cleanup must not replace the launched CLI's exit status with a
      // transient Windows file-removal error (for example, from antivirus).
      try {
        staged.cleanup();
      } catch (_) {
        // The uniquely named temporary directory is safe to leave behind.
      }
    }
  }
  process.exit(result.status !== null ? result.status : 1);
}

module.exports = {
  getPlatformPackage: getPlatformPackage,
  getBinaryPath: getBinaryPath,
  resolvePkgDir: resolvePkgDir,
  safeForwardArgs: safeForwardArgs,
  isLongRunningLaunch: isLongRunningLaunch,
  stageLaunchBinary: stageLaunchBinary,
  versionsMatch: versionsMatch
};
