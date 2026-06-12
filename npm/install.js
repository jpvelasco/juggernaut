"use strict";

const https = require("https");
const fs = require("fs");
const path = require("path");
const { Readable } = require("stream");
const crypto = require("crypto");

const REPO = "jpvelasco/juggernaut";
const BIN_DIR = path.join(__dirname, "bin");
const ALLOWED_HOSTS = new Set([
  "github.com",
  "api.github.com",
  "release-assets.githubusercontent.com",
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

async function main() {
  const { platform, os: osName } = getPlatform();
  const version = await getLatestVersion();
  const ext = osName === "windows" ? "zip" : "tar.gz";
  const archive = `juggernaut_${platform}.${ext}`;
  const baseUrl = `https://github.com/${REPO}/releases/download/v${version}`;

  console.log(`Downloading Juggernaut v${version} (${platform})...`);
  const archiveBuf = await httpsGetBuffer(`${baseUrl}/${archive}`);
  const checksumsBuf = await httpsGetBuffer(`${baseUrl}/checksums.txt`);

  const checksums = checksumsBuf.toString("utf8");
  const line = checksums.split("\n").find((l) => l.includes(archive));
  if (!line) throw new Error(`Checksum not found for ${archive}`);
  const expected = line.trim().split(/\s+/)[0].toLowerCase();
  const actual = crypto.createHash("sha256").update(archiveBuf).digest("hex").toLowerCase();
  if (actual !== expected) {
    throw new Error(`Checksum mismatch for ${archive}\n  expected: ${expected}\n  got:      ${actual}`);
  }

  if (ext === "zip") {
    const AdmZip = require("adm-zip");
    const zip = new AdmZip(archiveBuf);
    zip.extractEntryTo("juggernaut.exe", BIN_DIR, false, true);
  } else {
    const tar = require("tar");
    await tar.x({ cwd: BIN_DIR, mode: 0o700, strict: true }, Readable.from(archiveBuf));
  }

  console.log(`Juggernaut v${version} installed successfully.`);
  console.log(`Run: juggernaut apply`);
}

main().catch((err) => {
  console.error("Installation failed:", err.message);
  process.exit(1);
});