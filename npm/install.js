#!/usr/bin/env node
const https = require("https");
const fs = require("fs");
const path = require("path");
const crypto = require("crypto");
const { execSync } = require("child_process");
const os = require("os");

const pkg = require("./package.json");
const version = pkg.version;

const platformMap = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

const archMap = {
  x64: "amd64",
  arm64: "arm64",
};

const goos = platformMap[process.platform];
const goarch = archMap[process.arch];

if (!goos || !goarch) {
  console.error(
    `gurtcli does not support ${process.platform} ${process.arch}`
  );
  process.exit(1);
}

const binaryName = goos === "windows" ? "gurtcli.exe" : "gurtcli";
const archive = goos === "windows" 
  ? `gurtcli_${version}_${goos}_${goarch}.zip`
  : `gurtcli_${version}_${goos}_${goarch}.tar.gz`;
const baseUrl = `https://github.com/sillygru/gurtcli/releases/download/v${version}`;
const archiveUrl = `${baseUrl}/${archive}`;
const checksumsUrl = `${baseUrl}/checksums.txt`;
const binDir = path.join(__dirname, "bin");
const binPath = path.join(binDir, binaryName);
const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "gurtcli-"));

console.log(`Downloading gurtcli v${version} for ${goos}/${goarch}...`);

const archivePath = path.join(tmpDir, archive);

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
      console.log(`  Download failed (attempt ${i + 1}/${attempts}), retrying in ${Math.pow(2, i)}s...`);
      await new Promise(r => setTimeout(r, Math.pow(2, i) * 1000));
    }
  }
}

async function verifyChecksum(filePath, expectedFilename) {
  const checksumsPath = path.join(path.dirname(filePath), "checksums.txt");
  try {
    await downloadWithRetry(checksumsUrl, checksumsPath);
    const content = fs.readFileSync(checksumsPath, "utf-8");
    fs.unlinkSync(checksumsPath);
    const lines = content.split("\n").filter(l => l.includes(expectedFilename));
    if (lines.length === 0) {
      console.warn("  Warning: no checksum found in release for", expectedFilename);
      return;
    }
    const expectedHash = lines[0].split(/\s+/)[0];
    const fileBuffer = fs.readFileSync(filePath);
    const actualHash = crypto.createHash("sha256").update(fileBuffer).digest("hex");
    if (actualHash !== expectedHash) {
      throw new Error(
        `Checksum mismatch for ${expectedFilename}\n  expected: ${expectedHash}\n  actual:   ${actualHash}`
      );
    }
    console.log("  Checksum verified");
  } catch (err) {
    if (err.message.includes("Checksum mismatch")) throw err;
    console.warn(`  Warning: could not verify checksum (${err.message})`);
  }
}

const platformMagics = {
  linux: Buffer.from([0x7f, 0x45, 0x4c, 0x46]),
  darwin: null,
  win32: Buffer.from([0x4d, 0x5a]),
};

function verifyPlatform(binaryPath) {
  const expected = platformMagics[process.platform];
  if (!expected) return;

  const fd = fs.openSync(binaryPath, "r");
  const buf = Buffer.alloc(4);
  fs.readSync(fd, buf, 0, 4, 0);
  fs.closeSync(fd);

  if (process.platform === "darwin") {
    const macho32 = Buffer.from([0xfe, 0xed, 0xfa, 0xce]);
    const macho64 = Buffer.from([0xfe, 0xed, 0xfa, 0xcf]);
    const universal = Buffer.from([0xca, 0xfe, 0xba, 0xbe]);
    if (buf.equals(macho32) || buf.equals(macho64) || buf.equals(universal)) {
      console.log("  Platform verified");
      return;
    }
    throw new Error(
      `Binary is not a Mach-O executable (magic: ${buf.toString("hex")})`
    );
  }

  if (!buf.equals(expected)) {
    throw new Error(
      `Binary appears to be for the wrong platform (expected ${process.platform}, magic: ${buf.toString("hex")})`
    );
  }
  console.log("  Platform verified");
}

async function main() {
  try {
    await downloadWithRetry(archiveUrl, archivePath);

    await verifyChecksum(archivePath, archive);

    console.log("Extracting...");
    fs.mkdirSync(binDir, { recursive: true });
    if (goos === "windows") {
      execSync(`powershell Expand-Archive -Path "${archivePath}" -DestinationPath "${binDir}" -Force`, { stdio: "pipe" });
    } else {
      execSync(`tar -xzf "${archivePath}" -C "${binDir}"`, { stdio: "pipe" });
    }
    fs.unlinkSync(archivePath);
    fs.rmdirSync(tmpDir);

    const entries = fs.readdirSync(binDir);
    for (const entry of entries) {
      const entryPath = path.join(binDir, entry);
      if (fs.statSync(entryPath).isDirectory()) {
        const extractedBin = path.join(entryPath, binaryName);
        if (fs.existsSync(extractedBin)) {
          if (fs.existsSync(binPath)) fs.unlinkSync(binPath);
          fs.renameSync(extractedBin, binPath);
        }
        fs.rmSync(entryPath, { recursive: true, force: true });
      }
    }

    if (process.platform !== "win32") {
      fs.chmodSync(binPath, 0o755);
    }

    verifyPlatform(binPath);

    console.log("gurtcli v" + version + " installed");
  } catch (err) {
    console.error("Failed to install gurtcli:", err.message);
    if (fs.existsSync(tmpDir))
      fs.rmSync(tmpDir, { recursive: true, force: true });
    process.exit(1);
  }
}

main();
