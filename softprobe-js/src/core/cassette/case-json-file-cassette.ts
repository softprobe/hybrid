import fs from 'fs/promises';
import path from 'path';

import type { SoftprobeCassetteRecord } from '../../types/schema';
import { buildCaseDocumentFromRecords, caseDocumentToCassetteRecords } from './case-bridge';

/**
 * Case JSON storage: one file per trace at `{cassetteDirectory}/{traceId}.case.json`.
 * Replaces the legacy NDJSON per-trace file for the default language-plane path.
 *
 * `saveRecord` calls are serialized so concurrent async captures cannot interleave
 * read/modify/write and produce invalid JSON on disk.
 */
export class CaseJsonFileCassette {
  private readonly filePath: string;
  private persistChain: Promise<void> = Promise.resolve();

  constructor(cassetteDirectory: string, traceId: string) {
    this.filePath = path.join(cassetteDirectory, `${traceId}.case.json`);
  }

  async loadTrace(): Promise<SoftprobeCassetteRecord[]> {
    try {
      const raw = await fs.readFile(this.filePath, 'utf8');
      if (!raw.trim()) {
        return [];
      }
      const doc = JSON.parse(raw) as unknown;
      // Return every v4.1 record in the file. NDJSON-era captures keyed the file by config
      // `traceId` while span records often carried the OTel hex trace id — filtering by basename
      // would drop all rows and break replay.
      return caseDocumentToCassetteRecords(doc);
    } catch (err: unknown) {
      if ((err as NodeJS.ErrnoException).code === 'ENOENT') return [];
      throw err;
    }
  }

  async saveRecord(record: SoftprobeCassetteRecord): Promise<void> {
    const traceId = path.basename(this.filePath, '.case.json');
    this.persistChain = this.persistChain.then(() => this.persistAppend(traceId, record));
    await this.persistChain;
  }

  async flush(): Promise<void> {
    await this.persistChain;
  }

  private async persistAppend(traceId: string, record: SoftprobeCassetteRecord): Promise<void> {
    const normalized = record.traceId === traceId ? record : { ...record, traceId };
    let existing: SoftprobeCassetteRecord[] = [];
    try {
      const raw = await fs.readFile(this.filePath, 'utf8');
      existing = caseDocumentToCassetteRecords(JSON.parse(raw) as unknown);
    } catch (err: unknown) {
      if ((err as NodeJS.ErrnoException).code !== 'ENOENT') throw err;
    }
    const merged = [...existing, normalized];
    const doc = buildCaseDocumentFromRecords(merged, { caseId: traceId, mode: 'capture' });
    await fs.mkdir(path.dirname(this.filePath), { recursive: true });
    await fs.writeFile(this.filePath, `${JSON.stringify(doc, null, 2)}\n`, 'utf8');
  }
}
