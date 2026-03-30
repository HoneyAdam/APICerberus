export type PlaygroundKVRow = {
  id: string;
  key: string;
  value: string;
};

export type PlaygroundDraft = {
  method: string;
  path: string;
  apiKey: string;
  timeoutMs: number;
  body: string;
  queryRows: PlaygroundKVRow[];
  headerRows: PlaygroundKVRow[];
  selectedRouteID?: string;
};

export function newKVRow(key = "", value = ""): PlaygroundKVRow {
  return {
    id: crypto.randomUUID(),
    key,
    value,
  };
}

export function rowsToRecord(rows: PlaygroundKVRow[]) {
  const out: Record<string, string> = {};
  for (const row of rows) {
    const key = row.key.trim();
    if (!key) {
      continue;
    }
    out[key] = row.value;
  }
  return out;
}

export function recordToRows(record: Record<string, string> | undefined): PlaygroundKVRow[] {
  if (!record) {
    return [];
  }
  return Object.entries(record).map(([key, value]) => newKVRow(key, value));
}
