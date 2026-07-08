#!/usr/bin/env node
const { spawn, execSync } = require("child_process");
const fs = require("fs");
const path = require("path");
const https = require("https");
const crypto = require("crypto");
const os = require("os");

const pkg = require("./package.json");
const version = pkg.version;

const binName = process.platform === "win32" ? "gurtcli.exe" : "gurtcli";

const platformMap = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

const archMap = {
  x64: "amd64",
  arm64: "arm64",
};

const platformMagics = {
  linux: Buffer.from([0x7f, 0x45, 0x4c, 0x46]),
  darwin: null,
  win32: Buffer.from([0x4d, 0x5a]),
};

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    const req = https.get(url, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        file.close();
        if (fs.existsSync(dest)) fs.unlinkSync(dest);
        return download(res.headers.location, dest).then(resolve).catch(reject);
      }
      if (res.statusCode !== 200) {
        file.close();
        if (fs.existsSync(dest)) fs.unlinkSync(dest);
        reject(new Error(`HTTP ${res.statusCode}`));
        return;
      }
      res.pipe(file);
      file.on("finish", () => {
        file.close();
        resolve();
      });
    });
    req.on("error", (err) => {
      file.close();
      if (fs.existsSync(dest)) fs.unlinkSync(dest);
      reject(err);
    });
  });
}

async function downloadWithRetry(url, dest, attempts = 3) {
  for (let i = 0; i < attempts; i++) {
    try {
      await download(url, dest);
      return;
    } catch (err) {
      if (i === attempts - 1) throw err;
      await new Promise(r => setTimeout(r, Math.pow(2, i) * 1000));
    }
  }
}

async function verifyChecksum(filePath, expectedFilename) {
  const checksumsUrl = `https://github.com/sillygru/gurtcli/releases/download/v${version}/checksums.txt`;
  const checksumsPath = path.join(path.dirname(filePath), "checksums.txt");
  try {
    await downloadWithRetry(checksumsUrl, checksumsPath);
    const content = fs.readFileSync(checksumsPath, "utf-8");
    fs.unlinkSync(checksumsPath);
    const lines = content.split("\n").filter(l => l.includes(expectedFilename));
    if (lines.length === 0) return;
    const expectedHash = lines[0].split(/\s+/)[0];
    const fileBuffer = fs.readFileSync(filePath);
    const actualHash = crypto.createHash("sha256").update(fileBuffer).digest("hex");
    if (actualHash !== expectedHash) {
      throw new Error(
        `Checksum mismatch for ${expectedFilename}\n  expected: ${expectedHash}\n  actual:   ${actualHash}`
      );
    }
  } catch (err) {
    if (err.message.includes("Checksum mismatch")) throw err;
  }
}

function verifyPlatform(binaryPath) {
  const expected = platformMagics[process.platform];
  if (!expected) return;
  const fd = fs.openSync(binaryPath, "r");
  const buf = Buffer.alloc(4);
  fs.readSync(fd, buf, 0, 4, 0);
  fs.closeSync(fd);
  if (!buf.equals(expected)) {
    throw new Error(
      `Binary appears to be for the wrong platform (expected ${process.platform}, magic: ${buf.toString("hex")})`
    );
  }
}

async function ensureBinary() {
  const localBin = path.join(__dirname, "bin", binName);
  if (fs.existsSync(localBin)) {
    return localBin;
  }

  const cacheDir = path.join(os.homedir(), ".gurtcli", "bin");
  const cachedBin = path.join(cacheDir, binName);
  if (fs.existsSync(cachedBin)) {
    try { fs.chmodSync(cachedBin, 0o755); } catch {}
    return cachedBin;
  }

  const goos = platformMap[process.platform];
  const goarch = archMap[process.arch];
  if (!goos || !goarch) {
    console.error(`gurtcli does not support ${process.platform} ${process.arch}`);
    process.exit(1);
  }

  const archive = goos === "windows"
    ? `gurtcli_${version}_${goos}_${goarch}.zip`
    : `gurtcli_${version}_${goos}_${goarch}.tar.gz`;
  const baseUrl = `https://github.com/sillygru/gurtcli/releases/download/v${version}`;
  const archiveUrl = `${baseUrl}/${archive}`;

  fs.mkdirSync(cacheDir, { recursive: true });
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "gurtcli-"));

  try {
    const archivePath = path.join(tmpDir, archive);
    console.error(`Downloading gurtcli v${version} for ${goos}/${goarch}...`);
    await downloadWithRetry(archiveUrl, archivePath);
    await verifyChecksum(archivePath, archive);

    console.error("Extracting...");
    if (goos === "windows") {
      try {
        execSync(`powershell Expand-Archive -Path "${archivePath}" -DestinationPath "${tmpDir}" -Force`, { stdio: "pipe" });
      } catch {
        execSync(`tar -xf "${archivePath}" -C "${tmpDir}"`, { stdio: "pipe" });
      }
    } else {
      execSync(`tar -xzf "${archivePath}" -C "${tmpDir}"`, { stdio: "pipe" });
    }

    const entries = fs.readdirSync(tmpDir);
    for (const entry of entries) {
      const entryPath = path.join(tmpDir, entry);
      if (entry === archive) continue;
      if (fs.statSync(entryPath).isDirectory()) {
        const extractedBin = path.join(entryPath, binName);
        if (fs.existsSync(extractedBin)) {
          if (fs.existsSync(cachedBin)) fs.unlinkSync(cachedBin);
          fs.renameSync(extractedBin, cachedBin);
        }
      }
    }

    if (!fs.existsSync(cachedBin)) {
      throw new Error(`Binary ${binName} not found in extracted archive`);
    }

    fs.chmodSync(cachedBin, 0o755);
    verifyPlatform(cachedBin);
    return cachedBin;
  } finally {
    if (fs.existsSync(tmpDir)) fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

async function main() {
  let binPath = path.join(__dirname, "bin", binName);

  if (!fs.existsSync(binPath)) {
    try {
      binPath = await ensureBinary();
    } catch (err) {
      console.error("Failed to locate or download gurtcli binary:", err.message);
      process.exit(1);
    }
  }

  try {
    fs.chmodSync(binPath, 0o755);
  } catch {}

  const expected = platformMagics[process.platform];
  if (expected) {
    try {
      const fd = fs.openSync(binPath, "r");
      const buf = Buffer.alloc(4);
      fs.readSync(fd, buf, 0, 4, 0);
      fs.closeSync(fd);
      if (!buf.equals(expected)) {
        console.error(
          "gurtcli binary is corrupted or for a different platform. Reinstall."
        );
        process.exit(1);
      }
    } catch {}
  }

  const proc = spawn(binPath, process.argv.slice(2), { stdio: "inherit" });
  proc.on("exit", (code) => process.exit(code ?? 0));
}

main();
