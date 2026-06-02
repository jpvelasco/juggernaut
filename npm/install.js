"use strict";

const https = require("https");
const fs = require("fs");
const path = require("path");
const os = require("os");
const crypto = require("crypto");

const REPO = "jpvelasco/juggernaut";
const BIN_DIR = path.join(__dirname, "bin");

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

function httpsGet(url) {
  return new Promise((resolve, reject) => {
    https
      .get(url, { headers: { "User-Agent": "juggernaut-npm-installer/1.0" } }, (res) => {
        if (res.statusCode === 301 || res.statusCode === 302) {
          return httpsGet(res.headers.location).then(resolve).catch(reject);
        }
        const chunks = [];
        res.on("data", (chunk) => chunks.push(chunk));
        res.on("end", () => resolve(Buffer.concat(chunks)));
        res.on("error", reject);
      })
      .on("error", reject);
  });
}

function downloadFile(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    function fetch(fetchUrl) {
      https
        .get(fetchUrl, { headers: { "User-Agent": "juggernaut-npm-installer/1.0" } }, (res) => {
          if (res.statusCode === 301 || res.statusCode === 302) {
            return fetch(res.headers.location);
          }
          res.pipe(file);
          file.on("finish", () => file.close(resolve));
          file.on("error", reject);
        })
        .on("error", reject);
    }
    fetch(url);
  });
}

async function getLatestVersion() {
  const data = await httpsGet(`https://api.github.com/repos/${REPO}/releases/latest`);
  const release = JSON.parse(data.toString());
  return release.tag_name.replace(/^v/, "");
}

async function main() {
  const { platform, os: osName } = getPlatform();
  const version = await getLatestVersion();
  const ext = osName === "windows" ? "zip" : "tar.gz";
  const archive = `juggernaut_${platform}.${ext}`;
  const baseUrl = `https://github.com/${REPO}/releases/download/v${version}`;

  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "juggernaut-"));
  const archivePath = path.join(tmp, archive);
  const checksumPath = path.join(tmp, "checksums.txt");

  try {
    console.log(`Downloading Juggernaut v${version} (${platform})...`);
    await downloadFile(`${baseUrl}/${archive}`, archivePath);
    await downloadFile(`${baseUrl}/checksums.txt`, checksumPath);

    // Verify checksum.
    const checksums = fs.readFileSync(checksumPath, "utf8");
    const line = checksums.split("\n").find((l) => l.includes(archive));
    if (!line) throw new Error(`Checksum not found for ${archive}`);
    const expected = line.trim().split(/\s+/)[0].toLowerCase();
    const actual = crypto
      .createHash("sha256")
      .update(fs.readFileSync(archivePath))
      .digest("hex")
      .toLowerCase();
    if (actual !== expected) {
      throw new Error(`Checksum mismatch for ${archive}\n  expected: ${expected}\n  got:      ${actual}`);
    }

    fs.mkdirSync(BIN_DIR, { recursive: true });

    if (ext === "zip") {
      const AdmZip = require("adm-zip");
      const zip = new AdmZip(archivePath);
      zip.extractEntryTo("juggernaut.exe", BIN_DIR, false, true);
    } else {
      const tar = require("tar");
      await tar.x({ file: archivePath, cwd: BIN_DIR });
      fs.chmodSync(path.join(BIN_DIR, "juggernaut"), 0o755);
    }

    console.log(`Juggernaut v${version} installed successfully.`);
    console.log(`Run: juggernaut apply`);
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
}

main().catch((err) => {
  console.error("Installation failed:", err.message);
  process.exit(1);
});
