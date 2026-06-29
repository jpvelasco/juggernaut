"use strict";

var nodeTest = require("node:test");
var describe = nodeTest.describe;
var it = nodeTest.it;
var assert = require("assert");

var preinstall = require("./preinstall");

describe("buildBlockMessage", function() {
  it("names the reinstall command", function() {
    assert.ok(preinstall.buildBlockMessage().indexOf("npm install -g juggernaut-bedrock") !== -1);
    return void 0;
  });
  it("tells the user to close sessions", function() {
    assert.ok(/close/i.test(preinstall.buildBlockMessage()));
    return void 0;
  });
  return void 0;
});

describe("isLockingProcessRunning", function() {
  it("returns a boolean and never throws", function() {
    assert.strictEqual(typeof preinstall.isLockingProcessRunning(), "boolean");
    return void 0;
  });
  it("fails open (false) on non-Windows platforms", function() {
    if (process.platform === "win32") {
      return void 0; // skip on Windows; non-Windows path is the fail-open contract
    }
    assert.strictEqual(preinstall.isLockingProcessRunning(), false);
    return void 0;
  });
  return void 0;
});
