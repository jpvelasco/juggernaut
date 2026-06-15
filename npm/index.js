#!/usr/bin/env node
"use strict";

var path = require("path");
var child_process = require("child_process");
var fs = require("fs");

var PACKAGES_DIR = path.join(__dirname, "packages");

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
 * @param {string} platform
 * @returns {string}
 */
function getBinaryPath(pkgName, platform) {
  if (!containsPackage(pkgName)) {
    throw new Error("unexpected package name: " + pkgName);
  }
  var binaryName = platform === "win32" ? "juggernaut.exe" : "juggernaut";
  return path.join(PACKAGES_DIR, pkgName, "bin", binaryName); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal.path-join-resolve-traversal
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

  var bin = getBinaryPath(pkg, process.platform);
  // nosemgrep: javascript_pathtraversal_rule-non-literal-fs-filename, javascript.lang.security.audit.detect-non-literal-fs-filename
  if (!fs.existsSync(bin)) {
    process.stderr.write(
      "juggernaut-bedrock: binary not found at " + bin + "\n" +
      "Try reinstalling: npm install -g juggernaut-bedrock\n" +
      "If the problem persists, file an issue: https://github.com/jpvelasco/juggernaut/issues\n"
    );
    process.exit(1);
  }

  // nosemgrep: javascript.lang.security.detect-child-process, javascript_exec_rule-child-process
  var result = child_process.spawnSync(bin, process.argv.slice(2), {
    stdio: "inherit",
    env: process.env
  });
  process.exit(result.status !== null ? result.status : 1);
}

module.exports = { getPlatformPackage: getPlatformPackage, getBinaryPath: getBinaryPath };
