"use strict";

var nodeTest = require("node:test"); /* eslint-disable-line n/no-nodejs-modules, n/no-unsupported-features/node-builtins */
var describe = nodeTest.describe;
var it = nodeTest.it;
var path = require("path"); /* eslint-disable-line n/no-nodejs-modules */
var assert = require("assert"); /* eslint-disable-line n/no-nodejs-modules */

var index = require("./index");
var getPlatformPackage = index.getPlatformPackage;
var getBinaryPath = index.getBinaryPath;

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
  return void 0;
});
