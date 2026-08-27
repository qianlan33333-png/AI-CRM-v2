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
  const { ownerReassignmentCsvFromFile } = await import(pathToFileURL(path.join(outdir, 'ownerReassignmentFile.js')).href);
  const fixture = (name) => {
    const xlsx = new Blob([fs.readFileSync(path.join(root, 'src/admin/fixtures', name))], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' });
    Object.defineProperty(xlsx, 'name', { value: name });
    return xlsx;
  };
  const csv = await ownerReassignmentCsvFromFile(fixture('owner-reassignment-valid.xlsx'));
  if (csv !== 'customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id\n7,3,2026-08-25T00:00:00Z,9\n') throw new Error('owner reassignment XLSX fixture did not normalize to the preview CSV contract');
  const blankRowCsv = await ownerReassignmentCsvFromFile(fixture('owner-reassignment-blank-row.xlsx'));
  if (blankRowCsv !== csv) throw new Error('owner reassignment XLSX blank row was not ignored');
  const expectRejected = async (name, message) => {
    try {
      await ownerReassignmentCsvFromFile(fixture(name));
    } catch (error) {
      if (error instanceof Error && error.message === message) return;
      throw new Error(`${name} rejected with an unexpected error`);
    }
    throw new Error(`${name} was unexpectedly accepted`);
  };
  await expectRejected('owner-reassignment-formula.xlsx', 'Excel 文件不能包含公式');
  await expectRejected('owner-reassignment-unsafe-id.xlsx', 'Excel 第 2 行的 ID 必须是安全整数');
  await expectRejected('owner-reassignment-strict-header.xlsx', 'Excel 第一行必须且只能是：customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id');
  console.log('admin-adapter-contract: PASS');
} finally { fs.rmSync(outdir, { recursive: true, force: true }); }
