#!/usr/bin/env node
// Downloads the platform-specific softprobe binary from GitHub Releases.
'use strict';

const https = require('https');
const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

const REPO = 'softprobe/softprobe-runtime';
const BIN_DIR = path.join(__dirname, '..', 'bin');
const BIN_PATH = path.join(BIN_DIR, process.platform === 'win32' ? 'softprobe.exe' : 'softprobe');

function platformAsset() {
  const { platform, arch } = process;
  const os = platform === 'darwin' ? 'darwin' : platform === 'linux' ? 'linux' : null;
  const cpu = arch === 'x64' ? 'amd64' : arch === 'arm64' ? 'arm64' : null;
  if (!os || !cpu) {
    throw new Error(`Unsupported platform: ${platform}/${arch}`);
  }
  return `softprobe-${os}-${cpu}`;
}

function fetch(url) {
  return new Promise((resolve, reject) => {
    https.get(url, { headers: { 'User-Agent': 'softprobe-cli-install' } }, res => {
      if (res.statusCode === 301 || res.statusCode === 302) {
        return fetch(res.headers.location).then(resolve, reject);
      }
      if (res.statusCode !== 200) {
        return reject(new Error(`HTTP ${res.statusCode} fetching ${url}`));
      }
      const chunks = [];
      res.on('data', c => chunks.push(c));
      res.on('end', () => resolve(Buffer.concat(chunks)));
      res.on('error', reject);
    }).on('error', reject);
  });
}

async function latestVersion() {
  const buf = await fetch(`https://api.github.com/repos/${REPO}/releases/latest`);
  const { tag_name } = JSON.parse(buf.toString());
  if (!tag_name) throw new Error('Could not resolve latest release tag');
  return tag_name;
}

async function main() {
  // Skip download in CI environments that already have the binary on PATH
  if (process.env.SOFTPROBE_SKIP_INSTALL) return;

  const pkg = require('../package.json');
  const version = `v${pkg.version}`;
  const asset = platformAsset();
  const url = `https://github.com/${REPO}/releases/download/${version}/${asset}`;

  console.log(`softprobe: downloading ${asset} @ ${version}`);
  const buf = await fetch(url);
  fs.mkdirSync(BIN_DIR, { recursive: true });
  fs.writeFileSync(BIN_PATH, buf, { mode: 0o755 });
  console.log(`softprobe: installed to ${BIN_PATH}`);
}

main().catch(err => {
  console.error(`softprobe install failed: ${err.message}`);
  console.error('You can still install manually: https://softprobe.dev/install/cli.sh');
  // Non-fatal — don't block npm install
  process.exit(0);
});
