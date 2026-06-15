"use strict";

const path = require("path");
const { spawnSync } = require("child_process");

const PACKAGES_DIR = path.join(__dirname, "packages");

const PLATFORM_MAP = {
  "linux-x64":    "juggernaut-bedrock-linux-x64",
  "linux-arm64":  "juggernaut-bedrock-linux-arm64",
  "darwin-x64":   "juggernaut-bedrock-darwin-x64",
  "darwin-arm64": "juggernaut-bedrock-darwin-arm64",
  "win32-x64":    "juggernaut-bedrock-win32-x64",
};

function getPlatformPackage(platform, arch) {
  return PLATFORM_MAP[`${platform}-${arch}`] || null;
}

function getBinaryPath(pkgName, platform) {
  const validPackages = new Set(Object.values(PLATFORM_MAP));
  if (!validPackages.has(pkgName)) {
    throw new Error(`unexpected package name: ${pkgName}`);
  }
  const binaryName = platform === "win32" ? "juggernaut.exe" : "juggernaut";
  return path.join(PACKAGES_DIR, pkgName, "bin", binaryName); // nosemgrep: javascript.lang.security.audit.path-traversal.path-join-resolve-traversal
}

if (require.main === module) {
  const pkg = getPlatformPackage(process.platform, process.arch);
  if (!pkg) {
    process.stderr.write(
      `juggernaut-bedrock: unsupported platform ${process.platform}/${process.arch}\n` +
      `Please file an issue: https://github.com/jpvelasco/juggernaut/issues\n`
    );
    process.exit(1);
  }

  const bin = getBinaryPath(pkg, process.platform);
  const fs = require("fs");
  if (!fs.existsSync(bin)) { // nosemgrep: javascript_pathtraversal_rule-non-literal-fs-filename
    process.stderr.write(
      `juggernaut-bedrock: binary not found at ${bin}\n` +
      `Try reinstalling: npm install -g juggernaut-bedrock\n` +
      `If the problem persists, file an issue: https://github.com/jpvelasco/juggernaut/issues\n`
    );
    process.exit(1);
  }

  // nosemgrep: javascript.lang.security.detect-child-process
  const result = spawnSync(bin, process.argv.slice(2), {
    stdio: "inherit",
    env: process.env,
  });
  process.exit(result.status ?? 1);
}

module.exports = { getPlatformPackage, getBinaryPath };
