#!/usr/bin/env node
"use strict";

var childProcess = require("node:child_process");

/**
 * @returns {string}
 */
function buildBlockMessage() {
  return (
    "juggernaut-bedrock: a Claude Code / Juggernaut session is currently " +
    "running and is locking the Juggernaut binary.\n" +
    "Installing now would leave the package in a broken state.\n" +
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
