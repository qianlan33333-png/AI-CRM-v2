import { readSheet } from 'read-excel-file/browser';

const MAX_FILE_SIZE = 1024 * 1024;
const COLUMNS = ['customer_id', 'expected_owner_staff_id', 'expected_updated_at', 'target_owner_staff_id'];

const csvCell = (value: unknown): string => {
  const text = value === null ? '' : value instanceof Date ? value.toISOString() : String(value);
  return /[",\r\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
};

const nonEmpty = (value: unknown): boolean => value !== null && String(value).trim() !== '';

export async function ownerReassignmentCsvFromFile(file: File): Promise<string> {
  const filename = file.name.toLowerCase();
  if (file.size > MAX_FILE_SIZE) throw new Error('上传文件不能超过 1 MiB');
  if (filename.endsWith('.csv')) return file.text();
  if (!filename.endsWith('.xlsx')) throw new Error('仅支持 CSV 或 XLSX 文件');

  let rows: unknown[][];
  try {
    rows = await readSheet(file) as unknown[][];
  } catch {
    throw new Error('Excel 文件无法解析');
  }
  if (!rows.length) throw new Error('Excel 第一张工作表不能为空');
  const header = rows[0];
  if (header.length !== COLUMNS.length || header.some((cell, index) => cell !== COLUMNS[index])) {
    throw new Error(`Excel 第一行必须且只能是：${COLUMNS.join(',')}`);
  }
  const csvRows = rows.map((row, index) => {
    if (row.slice(COLUMNS.length).some(nonEmpty)) throw new Error(`Excel 第 ${index + 1} 行存在额外列`);
    return COLUMNS.map((_, column) => csvCell(row[column] ?? null)).join(',');
  });
  const csv = csvRows.join('\n') + '\n';
  if (new Blob([csv]).size > MAX_FILE_SIZE) throw new Error('Excel 转换后的 CSV 不能超过 1 MiB');
  return csv;
}
