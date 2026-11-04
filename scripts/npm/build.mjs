#!/usr/bin/env node
// Builds the npm packages from GoReleaser output (esbuild-style layout):
//
//   <out>/<main>/                  launcher + optionalDependencies
//   <out>/<prefix>-<os>-<cpu>/     one per platform, containing only the binary
//
// Usage:
//   node scripts/npm/build.mjs [--dist dist] [--out dist/npm] [--version 1.2.3]
//                              [--name envguard-cli] [--prefix envguard-cli] [--smoke]
//
// The version defaults to dist/metadata.json. --smoke installs the host's
// packages into a temporary node_modules and runs `envguard version` through
// the launcher. Names can also be set with NPM_PACKAGE_NAME / NPM_PLATFORM_PREFIX.
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const REPOSITORY = "https://github.com/ardamoustafa1/envguard";

// GoReleaser target → npm os/cpu (process.platform / process.arch).
const TARGETS = [
  { goos: "darwin", goarch: "amd64", os: "darwin", cpu: "x64" },
  { goos: "darwin", goarch: "arm64", os: "darwin", cpu: "arm64" },
  { goos: "linux", goarch: "amd64", os: "linux", cpu: "x64" },
  { goos: "linux", goarch: "arm64", os: "linux", cpu: "arm64" },
  { goos: "windows", goarch: "amd64", os: "win32", cpu: "x64" },
  { goos: "windows", goarch: "arm64", os: "win32", cpu: "arm64" },
];

function parseArgs(argv) {
  const opts = {
    dist: "dist",
    out: "",
    version: "",
    name: process.env.NPM_PACKAGE_NAME || "envguard-cli",
    prefix: process.env.NPM_PLATFORM_PREFIX || "",
    smoke: false,
  };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    const next = () => {
      if (i + 1 >= argv.length) throw new Error(`${a} needs a value`);
      return argv[++i];
    };
    switch (a) {
      case "--dist": opts.dist = next(); break;
      case "--out": opts.out = next(); break;
      case "--version": opts.version = next(); break;
      case "--name": opts.name = next(); break;
      case "--prefix": opts.prefix = next(); break;
      case "--smoke": opts.smoke = true; break;
      default: throw new Error(`unknown argument ${a}`);
    }
  }
  opts.dist = path.resolve(repoRoot, opts.dist);
  opts.out = path.resolve(repoRoot, opts.out || path.join(opts.dist, "npm"));
  opts.prefix = opts.prefix || opts.name;
  return opts;
}

function readJSON(p) {
  return JSON.parse(fs.readFileSync(p, "utf8"));
}

function writeJSON(p, v) {
  fs.writeFileSync(p, JSON.stringify(v, null, 2) + "\n");
}

function common(version) {
  return {
    version,
    license: "MIT",
    homepage: REPOSITORY,
    repository: { type: "git", url: `git+${REPOSITORY}.git` },
    bugs: { url: `${REPOSITORY}/issues` },
  };
}

function build(opts) {
  const meta = fs.existsSync(path.join(opts.dist, "metadata.json")) ? readJSON(path.join(opts.dist, "metadata.json")) : {};
  const version = (opts.version || meta.version || "").replace(/^v/, "");
  if (!/^\d+\.\d+\.\d+([-+].+)?$/.test(version)) {
    throw new Error(`no valid version (got "${version}"); pass --version or run GoReleaser first`);
  }
  const artifacts = readJSON(path.join(opts.dist, "artifacts.json"));
  const binaries = artifacts.filter((a) => a.type === "Binary" && a.extra?.ID === "envguard");

  fs.rmSync(opts.out, { recursive: true, force: true });
  fs.mkdirSync(opts.out, { recursive: true });
  const license = fs.readFileSync(path.join(repoRoot, "LICENSE"));

  const platformPackages = {};
  const optionalDependencies = {};
  for (const t of TARGETS) {
    const bin = binaries.find((b) => b.goos === t.goos && b.goarch === t.goarch);
    if (!bin) throw new Error(`GoReleaser produced no binary for ${t.goos}/${t.goarch}`);
    const name = `${opts.prefix}-${t.os}-${t.cpu}`;
    const dir = path.join(opts.out, name);
    const exe = t.os === "win32" ? "envguard.exe" : "envguard";
    fs.mkdirSync(path.join(dir, "bin"), { recursive: true });
    fs.copyFileSync(path.resolve(repoRoot, bin.path), path.join(dir, "bin", exe));
    fs.chmodSync(path.join(dir, "bin", exe), 0o755);
    fs.writeFileSync(path.join(dir, "LICENSE"), license);
    fs.writeFileSync(
      path.join(dir, "README.md"),
      `# ${name}\n\nThe ${t.os}-${t.cpu} binary of [EnvGuard](${REPOSITORY}). ` +
        `Install \`${opts.name}\` instead; it selects this package automatically.\n`
    );
    writeJSON(path.join(dir, "package.json"), {
      name,
      ...common(version),
      description: `The ${t.os}-${t.cpu} binary for ${opts.name}`,
      os: [t.os],
      cpu: [t.cpu],
      files: ["bin", "LICENSE", "README.md"],
      preferUnplugged: true,
    });
    platformPackages[`${t.os}-${t.cpu}`] = name;
    optionalDependencies[name] = version;
  }

  const mainDir = path.join(opts.out, opts.name);
  fs.mkdirSync(path.join(mainDir, "bin"), { recursive: true });
  const launcher = fs
    .readFileSync(path.join(repoRoot, "npm", "envguard", "bin", "envguard.js"), "utf8")
    .replace("__PLATFORM_PACKAGES__", JSON.stringify(platformPackages, null, 2))
    .replace("__MAIN_PACKAGE__", JSON.stringify(opts.name));
  fs.writeFileSync(path.join(mainDir, "bin", "envguard.js"), launcher, { mode: 0o755 });
  fs.writeFileSync(
    path.join(mainDir, "README.md"),
    fs
      .readFileSync(path.join(repoRoot, "npm", "envguard", "README.md"), "utf8")
      .replaceAll("__MAIN_PACKAGE__", opts.name)
      .replaceAll("__PLATFORM_PREFIX__", opts.prefix)
  );
  fs.writeFileSync(path.join(mainDir, "LICENSE"), license);
  writeJSON(path.join(mainDir, "package.json"), {
    name: opts.name,
    ...common(version),
    description: "Catch environment-variable and configuration mistakes before you deploy",
    keywords: ["env", "dotenv", "environment-variables", "config", "lint", "ci", "secrets"],
    bin: { envguard: "bin/envguard.js" },
    files: ["bin", "LICENSE", "README.md"],
    engines: { node: ">=16" },
    optionalDependencies,
  });

  // Publish order for CI: platform packages first, then the main package.
  const order = [...Object.keys(optionalDependencies), opts.name];
  fs.writeFileSync(path.join(opts.out, "publish-order.txt"), order.join("\n") + "\n");
  return { version, order, platformPackages, mainDir };
}

// smoke installs the main package and the host platform package into a
// temporary node_modules (no registry) and runs the launcher.
function smoke(opts, res) {
  const key = `${process.platform}-${process.arch}`;
  const pkg = res.platformPackages[key];
  if (!pkg) {
    console.log(`smoke test skipped: no package for ${key}`);
    return;
  }
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "envguard-npm-"));
  try {
    for (const name of [opts.name, pkg]) {
      fs.cpSync(path.join(opts.out, name), path.join(tmp, "node_modules", name), { recursive: true });
    }
    const launcher = path.join(tmp, "node_modules", opts.name, "bin", "envguard.js");
    const out = execFileSync(process.execPath, [launcher, "version"], { encoding: "utf8" });
    if (!out.includes(`envguard ${res.version}`)) {
      throw new Error(`smoke test: unexpected output:\n${out}`);
    }
    console.log(`smoke test ok (${key}): ${out.split("\n")[0]}`);
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
}

try {
  const opts = parseArgs(process.argv.slice(2));
  const res = build(opts);
  console.log(`built ${res.order.length} npm packages (version ${res.version}) in ${path.relative(repoRoot, opts.out)}:`);
  for (const name of res.order) console.log(`  ${name}`);
  if (opts.smoke) smoke(opts, res);
} catch (err) {
  console.error(`build.mjs: ${err.message}`);
  process.exit(1);
}
