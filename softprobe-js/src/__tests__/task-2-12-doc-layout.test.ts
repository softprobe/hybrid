import fs from 'node:fs';
import path from 'node:path';

describe('task 2.12 - docs layout sync', () => {
  it('documents the unified hybrid SDK flow', () => {
    const rootReadme = fs.readFileSync(path.resolve(__dirname, '..', '..', 'README.md'), 'utf8');

    expect(rootReadme).toContain('softprobe-runtime');
    expect(rootReadme).toContain('findInCase');
    expect(rootReadme).toContain('mockOutbound');
    expect(rootReadme).toContain('x-softprobe-session-id');
  });
});
