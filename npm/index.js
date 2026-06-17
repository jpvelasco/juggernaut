#!/usr/bin/env node
"use strict";

var path = require("path");
var childProcess = require("node:child_process");
var fs = require("fs");

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
 * @param {string} pkgName
 * @returns {string}
 */
function resolvePkgDir(pkgName) {
  try {
    return path.dirname(require.resolve(pkgName + "/package.json"));
  } catch (_) {
    return path.join(__dirname, "packages", pkgName); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal
  }
}

function getBinaryPath(pkgName, platform) {
  if (!containsPackage(pkgName)) {
    throw new Error("unexpected package name: " + pkgName);
  }
  var binaryName = platform === "win32" ? "juggernaut.exe" : "juggernaut";
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
  var args = safeForwardArgs(process.argv.slice(2));
  var result = childProcess.spawnSync(bin, args, {
    stdio: "inherit",
    env: Object.assign({}, process.env),
    shell: false,
    windowsHide: true
  });
  process.exit(result.status !== null ? result.status : 1);
}

module.exports = {
  getPlatformPackage: getPlatformPackage,
  getBinaryPath: getBinaryPath,
  safeForwardArgs: safeForwardArgs
};
