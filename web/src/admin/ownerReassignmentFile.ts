import { unzipSync } from 'fflate';
import { readSheet } from 'read-excel-file/browser';

const MAX_FILE_SIZE = 1024 * 1024;
const COLUMNS = ['customer_id', 'expected_owner_staff_id', 'expected_updated_at', 'target_owner_staff_id'];
const ID_COLUMNS = [0, 1, 3];
const XML_DECODER = new TextDecoder();

const csvCell = (value: unknown): string => {
  const text = value === null ? '' : value instanceof Date ? value.toISOString() : String(value);
  return /[",\r\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
};

const nonEmpty = (value: unknown): boolean => value !== null && value !== undefined && String(value).trim() !== '';
const blankRow = (row: unknown[]): boolean => row.every((value) => !nonEmpty(value));

const xmlAttribute = (tag: string, name: string): string | undefined => {
  const escaped = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  return tag.match(new RegExp(`(?:^|\\s)${escaped}\\s*=\\s*["']([^"']*)["']`))?.[1];
};

const resolveZipPath = (target: string): string => {
  const parts: string[] = [];
  const path = target.startsWith('/') ? target.slice(1) : `xl/${target}`;
  for (const part of path.split('/')) {
    if (!part || part === '.') continue;
    if (part === '..') {
      if (!parts.length) throw new Error('invalid worksheet relationship');
      parts.pop();
    } else {
      parts.push(part);
    }
  }
  return parts.join('/');
};

const firstWorksheetPath = (archive: Record<string, Uint8Array>): string => {
  const workbook = XML_DECODER.decode(archive['xl/workbook.xml']);
  const firstSheet = workbook.match(/<sheet\b[^>]*\/?>/)?.[0];
  const relationshipId = firstSheet ? xmlAttribute(firstSheet, 'r:id') : undefined;
  if (!relationshipId) throw new Error('missing first worksheet relationship');

  const relationships = XML_DECODER.decode(archive['xl/_rels/workbook.xml.rels']);
  const relationship = [...relationships.matchAll(/<Relationship\b[^>]*\/?>/g)]
    .map((match) => match[0])
    .find((tag) => xmlAttribute(tag, 'Id') === relationshipId);
  const target = relationship ? xmlAttribute(relationship, 'Target') : undefined;
  if (!target) throw new Error('missing first worksheet');
  return resolveZipPath(target);
};

const rejectFormulaCells = async (file: File): Promise<void> => {
  const archive = unzipSync(new Uint8Array(await file.arrayBuffer()), {
    filter: ({ name }) => name.endsWith('.xml') || name.endsWith('.xml.rels'),
  });
  const worksheet = archive[firstWorksheetPath(archive)];
  if (!worksheet) throw new Error('missing first worksheet');
  if (/<(?:[A-Za-z_][\w.-]*:)?f(?:\s[^>]*)?\/?\s*>/.test(XML_DECODER.decode(worksheet))) {
    throw new Error('Excel 文件不能包含公式');
  }
};

export async function ownerReassignmentCsvFromFile(file: File): Promise<string> {
  const filename = file.name.toLowerCase();
  if (file.size > MAX_FILE_SIZE) throw new Error('上传文件不能超过 1 MiB');
  if (filename.endsWith('.csv')) return file.text();
  if (!filename.endsWith('.xlsx')) throw new Error('仅支持 CSV 或 XLSX 文件');

  let rows: unknown[][];
  try {
    await rejectFormulaCells(file);
    rows = await readSheet(file, { trim: false }) as unknown[][];
  } catch (error) {
    if (error instanceof Error && error.message === 'Excel 文件不能包含公式') throw error;
    throw new Error('Excel 文件无法解析');
  }
  if (!rows.length) throw new Error('Excel 第一张工作表不能为空');
  const header = rows[0];
  if (header.length !== COLUMNS.length || header.some((cell, index) => cell !== COLUMNS[index])) {
    throw new Error(`Excel 第一行必须且只能是：${COLUMNS.join(',')}`);
  }
  const csvRows = [COLUMNS.map((column) => csvCell(column)).join(',')];
  for (const [index, row] of rows.slice(1).entries()) {
    if (blankRow(row)) continue;
    if (row.slice(COLUMNS.length).some(nonEmpty)) throw new Error(`Excel 第 ${index + 2} 行存在额外列`);
    for (const column of ID_COLUMNS) {
      if (typeof row[column] === 'number' && !Number.isSafeInteger(row[column])) {
        throw new Error(`Excel 第 ${index + 2} 行的 ID 必须是安全整数`);
      }
    }
    csvRows.push(COLUMNS.map((_, column) => csvCell(row[column] ?? null)).join(','));
  }
  const csv = csvRows.join('\n') + '\n';
  if (new Blob([csv]).size > MAX_FILE_SIZE) throw new Error('Excel 转换后的 CSV 不能超过 1 MiB');
  return csv;
}
