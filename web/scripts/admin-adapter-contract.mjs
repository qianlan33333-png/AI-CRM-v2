import { build } from 'esbuild';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const outdir = fs.mkdtempSync(path.join(os.tmpdir(), 'aicrm-admin-adapter-'));
await build({ entryPoints: [path.join(root, 'src/api/admin.test.ts'), path.join(root, 'src/api/external_effects.test.ts')], bundle: true, platform: 'node', format: 'esm', outdir, logLevel: 'warning' });
try {
  await (await import(pathToFileURL(path.join(outdir, 'admin.test.js')).href)).runAdminAdapterTests();
  await (await import(pathToFileURL(path.join(outdir, 'external_effects.test.js')).href)).runExternalEffectsAdapterTests();
  console.log('admin-adapter-contract: PASS');
} finally { fs.rmSync(outdir, { recursive: true, force: true }); }
