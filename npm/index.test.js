"use strict";

const { describe, it } = require("node:test");
const path = require("path");
const assert = require("assert");

const { getPlatformPackage, getBinaryPath } = require("./index");

describe("getPlatformPackage", () => {
  it("maps linux x64", () => {
    assert.strictEqual(getPlatformPackage("linux", "x64"), "juggernaut-bedrock-linux-x64");
  });
  it("maps linux arm64", () => {
    assert.strictEqual(getPlatformPackage("linux", "arm64"), "juggernaut-bedrock-linux-arm64");
  });
  it("maps darwin x64", () => {
    assert.strictEqual(getPlatformPackage("darwin", "x64"), "juggernaut-bedrock-darwin-x64");
  });
  it("maps darwin arm64", () => {
    assert.strictEqual(getPlatformPackage("darwin", "arm64"), "juggernaut-bedrock-darwin-arm64");
  });
  it("maps win32 x64", () => {
    assert.strictEqual(getPlatformPackage("win32", "x64"), "juggernaut-bedrock-win32-x64");
  });
  it("returns null for unsupported platform", () => {
    assert.strictEqual(getPlatformPackage("freebsd", "x64"), null);
  });
  it("returns null for unsupported arch", () => {
    assert.strictEqual(getPlatformPackage("linux", "ia32"), null);
  });
});

describe("getBinaryPath", () => {
  it("returns path under packages dir for linux", () => {
    const result = getBinaryPath("juggernaut-bedrock-linux-x64", "linux");
    assert.ok(result.endsWith(path.join("juggernaut-bedrock-linux-x64", "bin", "juggernaut")));
  });
  it("returns .exe path for win32", () => {
    const result = getBinaryPath("juggernaut-bedrock-win32-x64", "win32");
    assert.ok(result.endsWith(path.join("juggernaut-bedrock-win32-x64", "bin", "juggernaut.exe")));
  });
  it("path is under npm/packages/", () => {
    const result = getBinaryPath("juggernaut-bedrock-darwin-arm64", "darwin");
    assert.ok(result.includes(path.join("packages", "juggernaut-bedrock-darwin-arm64")));
  });
});
