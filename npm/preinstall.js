#!/usr/bin/env node
"use strict";

// Best-effort install-time guard (NOT a guarantee).
//
// npm runs this `preinstall` script only AFTER it reifies the dependency
// tree — including extracting the optional platform package that ships
// juggernaut.exe. So in the exact scenario this warns about (a running
// session holding a lock on juggernaut.exe under Windows), npm may already
// have hit EPERM overwriting that binary before this script ever runs. When
// that happens this gate cannot prevent the partial install.
//
// The reliable safety net is the runtime version-skew guard in index.js,
// which refuses to launch a partially-updated install. This script is an
// early, friendly heads-up for the cases where it does run first (e.g. a
// repeat install once npm has already aborted, or non-reifying flows); it is
// not the thing that makes a partial install safe.

var childProcess = require("node:child_process");

/**
 * @returns {string}
 */
function buildBlockMessage() {
  return (
    "juggernaut-bedrock: a Claude Code / Juggernaut session is currently " +
    "running and is holding a lock on the Juggernaut binary.\n" +
    "Installing now may leave the package in a partially-updated state.\n" +
    "Close all `claude` sessions and terminals, then re-run:\n" +
    "  npm install -g juggernaut-bedrock\n"
  );
}

/**
 * Detects a running juggernaut.exe on Windows. Fails open (returns false) on
 * non-Windows platforms and on any probe error, so a misfiring detection can
 * never wedge a legitimate repair install.
 * @returns {boolean}
 */
function isLockingProcessRunning() {
  if (process.platform !== "win32") {
    return false;
  }
  try {
    var out = childProcess.execFileSync(
      "tasklist",
      ["/FI", "IMAGENAME eq juggernaut.exe", "/NH"],
      { encoding: "utf8", windowsHide: true }
    );
    return out.toLowerCase().indexOf("juggernaut.exe") !== -1;
  } catch (_) {
    return false;
  }
}

function main() {
  if (isLockingProcessRunning()) {
    process.stderr.write(buildBlockMessage());
    process.exit(1);
  }
}

if (require.main === module) {
  main();
}

module.exports = {
  buildBlockMessage: buildBlockMessage,
  isLockingProcessRunning: isLockingProcessRunning,
  main: main
};
