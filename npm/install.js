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
const BIN_DIR = path.join(__dirname, "bin");
const ALLOWED_HOSTS = new Set([
  "github.com",
  "api.github.com",
  "release-assets.githubusercontent.com"
]);

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
  if (checksumsText.includes(tarArchive)) {
    return { name: tarArchive, kind: "tar.gz" };
  }
  const zipArchive = `juggernaut_${platform}.zip`;
  if (process.platform === "win32" && checksumsText.includes(zipArchive)) {
    return { name: zipArchive, kind: "zip" };
  }
  throw new Error(`No supported archive found in release checksums for ${platform}`);
}

function extractTarGz(archiveBuf) {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "juggernaut-install-"));
  const archivePath = path.join(tmpDir, "archive.tar.gz");
  try {
    fs.writeFileSync(archivePath, archiveBuf);
    fs.mkdirSync(BIN_DIR, { recursive: true });
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
      const binPath = path.join(BIN_DIR, "juggernaut");
      if (fs.existsSync(binPath)) {
        fs.chmodSync(binPath, 0o700);
      }
    }
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

async function extractZip(archiveBuf) {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "juggernaut-install-"));
  const archivePath = path.join(tmpDir, "archive.zip");
  try {
    fs.writeFileSync(archivePath, archiveBuf);
    fs.mkdirSync(BIN_DIR, { recursive: true });
    const script = [
      "$ErrorActionPreference = 'Stop'",
      `Expand-Archive -LiteralPath '${archivePath.replace(/'/g, "''")}' -DestinationPath '${BIN_DIR.replace(/'/g, "''")}' -Force`
    ].join("; ");
    await execFileAsync(
      "powershell",
      ["-NoProfile", "-NonInteractive", "-Command", script],
      { stdio: "pipe" }
    );
  } finally {
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

  if (kind === "zip") {
    await extractZip(archiveBuf);
  } else {
    extractTarGz(archiveBuf);
  }

  console.log(`Juggernaut v${version} installed successfully.`);
  console.log(`Run: juggernaut apply`);
}

main().catch((err) => {
  console.error("Installation failed:", err.message);
  process.exit(1);
});