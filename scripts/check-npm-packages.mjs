#!/usr/bin/env node
"use strict";

import fs from "node:fs";
import path from "node:path";

const root = readJSON("npm/root/package.json");
const platforms = [
  ["win32-x64", "@aibo666/ebo-win32-x64"],
  ["darwin-arm64", "@aibo666/ebo-darwin-arm64"]
];

assert(root.name === "@aibo666/ebo", "root package name mismatch");
assert(root.bin && root.bin.ebo === "bin/ebo.js", "root bin.ebo must be bin/ebo.js");
assert(root.publishConfig && root.publishConfig.access === "public", "root package must publish publicly");

for (const [dir, name] of platforms) {
  const pkg = readJSON(`npm/platforms/${dir}/package.json`);
  assert(pkg.name === name, `${dir} package name mismatch`);
  assert(pkg.version === root.version, `${name} version must match root version`);
  assert(root.optionalDependencies[name] === root.version, `${name} optional dependency version must match root version`);
  assert(pkg.publishConfig && pkg.publishConfig.access === "public", `${name} must publish publicly`);
}

console.log(`npm package metadata ok (${root.version})`);

function readJSON(file) {
  return JSON.parse(fs.readFileSync(path.join(process.cwd(), file), "utf8"));
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}
