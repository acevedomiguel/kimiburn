#!/usr/bin/env node
import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

const semanticVersion = /^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

export function validateModuleVersion(modulePath, version) {
  if (typeof modulePath !== "string" || modulePath.trim() === "") {
    throw new Error("Go module path is required");
  }
  const match = semanticVersion.exec(version);
  if (match === null) {
    throw new Error(`invalid semantic version: ${version}`);
  }
  const suffix = /\/v([1-9]\d*)$/.exec(modulePath);
  const expectedMajor = suffix === null ? 1 : Number(suffix[1]);
  const actualMajor = Number(match[1]);
  if (actualMajor !== expectedMajor) {
    throw new Error(`Go module ${modulePath} expects v${expectedMajor} releases, received ${version}`);
  }
}

function readModulePath() {
  const goMod = readFileSync(new URL("../go.mod", import.meta.url), "utf8");
  const match = /^module\s+(\S+)\s*$/m.exec(goMod);
  if (match === null) {
    throw new Error("go.mod does not declare a module path");
  }
  return match[1];
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const version = process.argv[2];
    if (!version) {
      throw new Error("usage: node scripts/check-module-version.mjs <version>");
    }
    const modulePath = readModulePath();
    validateModuleVersion(modulePath, version);
    console.log(`${modulePath} accepts ${version}.`);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
