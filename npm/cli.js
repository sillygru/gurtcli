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

const proc = spawn(binPath, process.argv.slice(2), { stdio: "inherit" });
proc.on("exit", (code) => process.exit(code ?? 0));
