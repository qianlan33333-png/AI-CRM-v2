import { build } from 'esbuild';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const outdir = fs.mkdtempSync(path.join(os.tmpdir(), 'aicrm-admin-adapter-'));
await build({ entryPoints: { 'admin.test': path.join(root, 'src/api/admin.test.ts'), 'external_effects.test': path.join(root, 'src/api/external_effects.test.ts'), 'push_observability.test': path.join(root, 'src/api/push_observability.test.ts'), ownerReassignmentFile: path.join(root, 'src/admin/ownerReassignmentFile.ts') }, bundle: true, platform: 'node', format: 'esm', outdir, logLevel: 'warning' });
try {
  await (await import(pathToFileURL(path.join(outdir, 'admin.test.js')).href)).runAdminAdapterTests();
  await (await import(pathToFileURL(path.join(outdir, 'external_effects.test.js')).href)).runExternalEffectsAdapterTests();
  await (await import(pathToFileURL(path.join(outdir, 'push_observability.test.js')).href)).runPushObservabilityAdapterTests();
  const xlsx = new Blob([fs.readFileSync(path.join(root, 'src/admin/fixtures/owner-reassignment-valid.xlsx'))], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' });
  Object.defineProperty(xlsx, 'name', { value: 'owners.xlsx' });
  const csv = await (await import(pathToFileURL(path.join(outdir, 'ownerReassignmentFile.js')).href)).ownerReassignmentCsvFromFile(xlsx);
  if (csv !== 'customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id\n7,3,2026-08-25T00:00:00Z,9\n') throw new Error('owner reassignment XLSX fixture did not normalize to the preview CSV contract');
  console.log('admin-adapter-contract: PASS');
} finally { fs.rmSync(outdir, { recursive: true, force: true }); }
