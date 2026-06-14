"use strict";

const https = require("https");
const fs = require("fs");
const path = require("path");
const os = require("os");
const crypto = require("crypto");
const { execFile, execFileSync } = require("child_process");
const { promisify } = require("util");

const execFileAsync = promisify(execFile);

const REPO = "jpvelasco/juggernaut";
const PACKAGE_DIR = __dirname;
const BIN_DIR = path.join(PACKAGE_DIR, "bin");
const TMP_PREFIX = path.join(os.tmpdir(), "juggernaut-install-");
const ARCHIVE_TAR = "archive.tar.gz";
const ARCHIVE_ZIP = "archive.zip";
const BIN_NAME = "juggernaut";
const ALLOWED_HOSTS = new Set([
  "github.com",
  "api.github.com",
  "release-assets.githubusercontent.com"
]);

function joinUnder(base, ...segments) {
  if (base.indexOf("\0") !== -1) {
    throw new Error("Invalid base path");
  }
  for (const seg of segments) {
    if (seg.indexOf("..") !== -1 || seg.indexOf("/") !== -1 || seg.indexOf("\\") !== -1) {
      throw new Error(`Invalid path segment: ${seg}`);
    }
  }
  const joined = path.join(base, ...segments);
  const normalBase = path.normalize(base) + path.sep;
  const normalJoined = path.normalize(joined);
  const compare = process.platform === "win32"
    ? (a, b) => a.toLowerCase().startsWith(b.toLowerCase())
    : (a, b) => a.startsWith(b);
  if (!compare(normalJoined, normalBase)) {
    const exactBase = process.platform === "win32"
      ? path.normalize(base).toLowerCase()
      : path.normalize(base);
    const exactJoined = process.platform === "win32"
      ? normalJoined.toLowerCase()
      : normalJoined;
    if (exactJoined !== exactBase) {
      throw new Error(`Path traversal detected: ${joined} is outside ${base}`);
    }
  }
  return normalJoined;
}

function getPlatform() {
  const osMap = { darwin: "darwin", linux: "linux", win32: "windows" };
  const archMap = { x64: "amd64", arm64: "arm64" };
  const p = osMap[process.platform];
  const a = archMap[process.arch];
  if (!p || !a) {
    throw new Error(`Unsupported platform: ${process.platform}/${process.arch}`);
  }
  return { os: p, arch: a, platform: `${p}_${a}` };
}

function httpsGetBuffer(url) {
  const parsed = new URL(url);
  if (!ALLOWED_HOSTS.has(parsed.hostname)) {
    throw new Error(`URL host not allowed: ${parsed.hostname}`);
  }
  return new Promise((resolve, reject) => {
    https
      .get(url, { headers: { "User-Agent": "juggernaut-npm-installer/1.0" } }, (res) => {
        if (res.statusCode === 301 || res.statusCode === 302) {
          const location = res.headers.location;
          const redirectParsed = new URL(location);
          if (!ALLOWED_HOSTS.has(redirectParsed.hostname)) {
            reject(new Error(`Redirect host not allowed: ${redirectParsed.hostname}`));
            return;
          }
          httpsGetBuffer(location).then(resolve).catch(reject);
          return;
        }
        const chunks = [];
        res.on("data", (chunk) => chunks.push(chunk));
        res.on("end", () => resolve(Buffer.concat(chunks)));
        res.on("error", reject);
      })
      .on("error", reject);
  });
}

async function getLatestVersion() {
  const data = await httpsGetBuffer(`https://api.github.com/repos/${REPO}/releases/latest`);
  const release = JSON.parse(data.toString());
  return release.tag_name.replace(/^v/, "");
}

function pickArchive(platform, checksumsText) {
  const tarArchive = `juggernaut_${platform}.tar.gz`;
  if (checksumsText.includes(tarArchive)) return { name: tarArchive, kind: "tar.gz" };
  const zipArchive = `juggernaut_${platform}.zip`;
  if (process.platform === "win32" && checksumsText.includes(zipArchive)) {
    return { name: zipArchive, kind: "zip" };
  }
  throw new Error(`No supported archive found in release checksums for ${platform}`);
}

function extractTarGz(archiveBuf) {
  const tmpDir = fs.mkdtempSync(TMP_PREFIX);
  const archivePath = joinUnder(tmpDir, ARCHIVE_TAR);
  const prevCwd = process.cwd();
  try {
    process.chdir(tmpDir);
    fs.writeFileSync(ARCHIVE_TAR, archiveBuf);
    process.chdir(PACKAGE_DIR);
    fs.mkdirSync("bin", { recursive: true });
    try {
      execFileSync("tar", ["-xzf", archivePath, "-C", BIN_DIR], { stdio: "pipe" });
    } catch (err) {
      if (err.code === "ENOENT") {
        throw new Error(
          "tar not found on PATH. On Windows, use Windows 10 version 1803 or later (includes tar.exe)."
        );
      }
      const detail = err.stderr ? err.stderr.toString().trim() : err.message;
      throw new Error(`tar extraction failed: ${detail}`);
    }
    if (process.platform !== "win32") {
      process.chdir(BIN_DIR);
      if (fs.existsSync(BIN_NAME)) {
        fs.chmodSync(BIN_NAME, 0o700);
      }
    }
  } finally {
    process.chdir(prevCwd);
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

async function extractZip(archiveBuf) {
  const tmpDir = fs.mkdtempSync(TMP_PREFIX);
  const archivePath = joinUnder(tmpDir, ARCHIVE_ZIP);
  const prevCwd = process.cwd();
  try {
    process.chdir(tmpDir);
    fs.writeFileSync(ARCHIVE_ZIP, archiveBuf);
    process.chdir(PACKAGE_DIR);
    fs.mkdirSync("bin", { recursive: true });
    const script = [
      "$ErrorActionPreference = 'Stop'",
      `Expand-Archive -LiteralPath '${archivePath.replace(/'/g, "''")}' -DestinationPath '${BIN_DIR.replace(/'/g, "''")}' -Force`
    ].join("; ");
    try {
      await execFileAsync(
        "powershell",
        ["-NoProfile", "-NonInteractive", "-Command", script],
        { stdio: "pipe" }
      );
    } catch (err) {
      if (err.code === "ENOENT") {
        throw new Error(
          "PowerShell not found. PowerShell 5.0+ is required to extract archives on Windows."
        );
      }
      const detail = err.stderr ? err.stderr.toString().trim() : err.message;
      throw new Error(`ZIP extraction failed: ${detail}`);
    }
  } finally {
    process.chdir(prevCwd);
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

async function main() {
  const { platform } = getPlatform();
  const version = await getLatestVersion();
  const baseUrl = `https://github.com/${REPO}/releases/download/v${version}`;
  const checksumsBuf = await httpsGetBuffer(`${baseUrl}/checksums.txt`);
  const checksums = checksumsBuf.toString("utf8");
  const { name: archive, kind } = pickArchive(platform, checksums);

  console.log(`Downloading Juggernaut v${version} (${platform})...`);
  const archiveBuf = await httpsGetBuffer(`${baseUrl}/${archive}`);

  const line = checksums.split("\n").find((l) => l.includes(archive));
  if (!line) throw new Error(`Checksum not found for ${archive}`);
  const expected = line.trim().split(/\s+/)[0].toLowerCase();
  const actual = crypto.createHash("sha256").update(archiveBuf).digest("hex").toLowerCase();
  if (actual !== expected) {
    throw new Error(`Checksum mismatch for ${archive}\n  expected: ${expected}\n  got:      ${actual}`);
  }

  if (kind === "zip") await extractZip(archiveBuf);
  else extractTarGz(archiveBuf);

  console.log(`Juggernaut v${version} installed successfully.`);
  console.log(`Run: juggernaut apply`);
}

main().catch((err) => {
  console.error("Installation failed:", err.message);
  process.exit(1);
});