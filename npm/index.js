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

function getPlatformPackage(platform, arch) {
  return PLATFORM_MAP[platform + "-" + arch] || null;
}

function getBinaryPath(pkgName, platform) {
  var validPackages = Object.keys(PLATFORM_MAP).map(function(k) { return PLATFORM_MAP[k]; });
  if (validPackages.indexOf(pkgName) === -1) {
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
    process.exit(1); // nosemgrep: eslint.n_no-process-exit
  }

  var bin = getBinaryPath(pkg, process.platform);
  if (!fs.existsSync(bin)) { // nosemgrep: javascript_pathtraversal_rule-non-literal-fs-filename
    process.stderr.write(
      "juggernaut-bedrock: binary not found at " + bin + "\n" +
      "Try reinstalling: npm install -g juggernaut-bedrock\n" +
      "If the problem persists, file an issue: https://github.com/jpvelasco/juggernaut/issues\n"
    );
    process.exit(1); // nosemgrep: eslint.n_no-process-exit
  }

  // nosemgrep: javascript.lang.security.detect-child-process
  var result = child_process.spawnSync(bin, process.argv.slice(2), {
    stdio: "inherit",
    env: process.env
  });
  process.exit(result.status !== null ? result.status : 1); // nosemgrep: eslint.n_no-process-exit
}

module.exports = { getPlatformPackage: getPlatformPackage, getBinaryPath: getBinaryPath };
