#!/usr/bin/env node
// EnvGuard npm launcher. The real program is a native binary shipped in a
// per-platform package (installed through optionalDependencies, so npm only
// downloads the one matching this machine). There is no postinstall script
// and no network access: this file only finds the binary and runs it.
"use strict";

const { spawnSync } = require("child_process");
const path = require("path");
const fs = require("fs");

// Filled in by scripts/npm/build.mjs.
const PLATFORM_PACKAGES = __PLATFORM_PACKAGES__;
const MAIN_PACKAGE = __MAIN_PACKAGE__;

function fail(message) {
  process.stderr.write(`envguard: ${message}\n`);
  process.exit(2);
}

function binaryPath() {
  if (process.env.ENVGUARD_BINARY) {
    return process.env.ENVGUARD_BINARY;
  }
  const key = `${process.platform}-${process.arch}`;
  const pkg = PLATFORM_PACKAGES[key];
  if (!pkg) {
    fail(
      `unsupported platform ${key}. Prebuilt binaries exist for: ${Object.keys(PLATFORM_PACKAGES).join(", ")}.\n` +
        "Install EnvGuard another way (Homebrew, go install, Docker, install.sh): " +
        "https://github.com/ardamoustafa1/envguard/blob/main/docs/installation.md"
    );
  }
  const exe = process.platform === "win32" ? "envguard.exe" : "envguard";
  try {
    return require.resolve(`${pkg}/bin/${exe}`);
  } catch {
    // Fall back to a sibling directory (unusual layouts, e.g. pnpm without hoisting).
    const sibling = path.join(__dirname, "..", "..", pkg, "bin", exe);
    if (fs.existsSync(sibling)) {
      return sibling;
    }
    fail(
      `the platform package ${pkg} is not installed.\n` +
        "It is an optional dependency of " + MAIN_PACKAGE + "; it is skipped when installing with\n" +
        "--no-optional / --omit=optional, or when a lockfile was created on another platform.\n" +
        `Reinstall without those options, or add it explicitly: npm install ${pkg}`
    );
  }
}

const result = spawnSync(binaryPath(), process.argv.slice(2), { stdio: "inherit", windowsHide: true });
if (result.error) {
  fail(`could not run the binary: ${result.error.message}`);
}
if (result.signal) {
  process.kill(process.pid, result.signal);
} else {
  process.exit(result.status === null ? 2 : result.status);
}
