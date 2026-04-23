#!/usr/bin/env node
'use strict';

const { spawnSync } = require('child_process');
const path = require('path');
const fs = require('fs');

const bin = path.join(__dirname, process.platform === 'win32' ? 'softprobe.exe' : 'softprobe');

if (!fs.existsSync(bin)) {
  console.error('softprobe binary not found. Re-run: npm install @softprobe/cli');
  process.exit(1);
}

const result = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit' });
process.exit(result.status ?? 1);
