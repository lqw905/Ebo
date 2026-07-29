#!/usr/bin/env node
"use strict";

import fs from "node:fs";
import path from "node:path";

const version = process.argv[2];
const distDir = process.argv[3] || "dist";

if (!version || !/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) {
  console.error("usage: node scripts/prepare-npm-packages.mjs <version> [dist-dir]");
  process.exit(1);
}

const targets = [
  {
    dir: "win32-x64",
    packageName: "@lqw905/ebo-win32-x64",
    artifact: "ebo-windows-amd64.exe",
    binary: "ebo.exe"
  },
  {
    dir: "linux-x64",
    packageName: "@lqw905/ebo-linux-x64",
    artifact: "ebo-linux-amd64",
    binary: "ebo"
  },
  {
    dir: "linux-arm64",
    packageName: "@lqw905/ebo-linux-arm64",
    artifact: "ebo-linux-arm64",
    binary: "ebo"
  },
  {
    dir: "darwin-x64",
    packageName: "@lqw905/ebo-darwin-x64",
    artifact: "ebo-darwin-amd64",
    binary: "ebo"
  },
  {
    dir: "darwin-arm64",
    packageName: "@lqw905/ebo-darwin-arm64",
    artifact: "ebo-darwin-arm64",
    binary: "ebo"
  }
];

const rootPackagePath = "npm/root/package.json";
const rootPackage = readJSON(rootPackagePath);
rootPackage.version = version;
for (const target of targets) {
  rootPackage.optionalDependencies[target.packageName] = version;
}
writeJSON(rootPackagePath, rootPackage);

for (const target of targets) {
  const packagePath = `npm/platforms/${target.dir}/package.json`;
  const pkg = readJSON(packagePath);
  pkg.version = version;
  writeJSON(packagePath, pkg);

  const source = path.join(distDir, target.artifact);
  const destinationDir = path.join("npm", "platforms", target.dir, "bin");
  const destination = path.join(destinationDir, target.binary);
  if (!fs.existsSync(source)) {
    throw new Error(`missing build artifact: ${source}`);
  }
  fs.mkdirSync(destinationDir, { recursive: true });
  fs.copyFileSync(source, destination);
  if (target.binary === "ebo") {
    fs.chmodSync(destination, 0o755);
  }
}

console.log(`prepared npm packages for ${version}`);

function readJSON(file) {
  return JSON.parse(fs.readFileSync(file, "utf8"));
}

function writeJSON(file, value) {
  fs.writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);
}
