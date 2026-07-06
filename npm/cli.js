#!/usr/bin/env node
const { spawn } = require("child_process");
const fs = require("fs");
const path = require("path");

const binName = process.platform === "win32" ? "gurtcli.exe" : "gurtcli";
const binPath = path.join(__dirname, "bin", binName);

if (!fs.existsSync(binPath)) {
  console.error(
    "gurtcli binary not found. Try reinstalling: npm install -g gurtcli"
  );
  process.exit(1);
}

try {
  fs.chmodSync(binPath, 0o755);
} catch {
  // best-effort; pnpm store may strip executable bit
}

const platformMagics = {
  linux: Buffer.from([0x7f, 0x45, 0x4c, 0x46]),
  darwin: null,
  win32: Buffer.from([0x4d, 0x5a]),
};

const expected = platformMagics[process.platform];
if (expected) {
  try {
    const fd = fs.openSync(binPath, "r");
    const buf = Buffer.alloc(4);
    fs.readSync(fd, buf, 0, 4, 0);
    fs.closeSync(fd);
    if (!buf.equals(expected)) {
      console.error(
        "gurtcli binary is corrupted or for a different platform. Reinstall: npm install -g gurtcli"
      );
      process.exit(1);
    }
  } catch {
    // best-effort; skip validation if we cannot read
  }
}

const proc = spawn(binPath, process.argv.slice(2), { stdio: "inherit" });
proc.on("exit", (code) => process.exit(code ?? 0));
