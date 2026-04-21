import fs from 'fs';
import path from 'path';

const README_PATH = path.resolve(__dirname, '..', '..', 'README.md');

describe('Task 10.1 README API snippets', () => {
  it('contains the session-based replay flow and no legacy cassettePath API examples', () => {
    const readme = fs.readFileSync(README_PATH, 'utf8');

    expect(readme).not.toMatch(/runWithContext\s*\(/);
    expect(readme).not.toMatch(/softprobe\.run\(\s*\{/);
    expect(readme).not.toMatch(/\bcassettePath\s*:/);

    expect(readme).toContain('startSession');
    expect(readme).toContain('loadCaseFromFile');
    expect(readme).toContain('findInCase');
    expect(readme).toContain('mockOutbound');
    expect(readme).toContain('x-softprobe-session-id');
  });
});
