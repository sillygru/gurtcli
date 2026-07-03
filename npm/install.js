#!/usr/bin/env node
const https = require("https");
const fs = require("fs");
const path = require("path");
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
const archive = `gurtcli_${version}_${goos}_${goarch}.tar.gz`;
const url = `https://github.com/sillygru/gurtcli/releases/download/v${version}/${archive}`;
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
        fs.unlinkSync(dest);
        return download(res.headers.location, dest).then(resolve).catch(reject);
      }
      if (res.statusCode !== 200) {
        file.close();
        fs.unlinkSync(dest);
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

download(url, archivePath)
  .then(() => {
    console.log("Extracting...");
    fs.mkdirSync(binDir, { recursive: true });
    execSync(`tar -xzf "${archivePath}" -C "${binDir}"`, {
      stdio: "pipe",
    });
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

    console.log("gurtcli v" + version + " installed");
  })
  .catch((err) => {
    console.error("Failed to install gurtcli:", err.message);
    if (fs.existsSync(tmpDir))
      fs.rmSync(tmpDir, { recursive: true, force: true });
    process.exit(1);
  });
