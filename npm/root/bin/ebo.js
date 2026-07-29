#!/usr/bin/env node
"use strict";

const { spawn } = require("node:child_process");
const path = require("node:path");

const targets = {
  "win32-x64": { packageName: "@aibo666/ebo-win32-x64", binary: "bin/ebo.exe" },
  "darwin-arm64": { packageName: "@aibo666/ebo-darwin-arm64", binary: "bin/ebo" }
};

const overrideBinary = process.env.EBO_BIN;
const target = targets[`${process.platform}-${process.arch}`];

let binary;
if (overrideBinary) {
  binary = overrideBinary;
} else if (target) {
  try {
    const packageJSON = require.resolve(`${target.packageName}/package.json`);
    binary = path.join(path.dirname(packageJSON), target.binary);
  } catch (error) {
    fail(`Could not find ${target.packageName}. Reinstall @aibo666/ebo for ${process.platform}-${process.arch}.`);
  }
} else {
  fail(`Ebo does not ship an npm binary for ${process.platform}-${process.arch}.`);
}

const child = spawn(binary, process.argv.slice(2), { stdio: "inherit" });

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    if (!child.killed) {
      child.kill(signal);
    }
  });
}

child.on("error", (error) => {
  fail(`Failed to run Ebo binary: ${error.message}`);
});

child.on("exit", (code, signal) => {
  if (signal) {
    process.exit(1);
  }
  process.exit(code === null ? 1 : code);
});

function fail(message) {
  console.error(`ebo: ${message}`);
  process.exit(1);
}
