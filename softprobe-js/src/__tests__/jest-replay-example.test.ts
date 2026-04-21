import fs from 'fs';
import path from 'path';

const EXAMPLE_DIR = path.resolve(__dirname, '..', '..', '..', 'e2e', 'jest-replay');
const README_PATH = path.join(EXAMPLE_DIR, 'README.md');
const EXAMPLE_TEST_PATH = path.join(EXAMPLE_DIR, 'fragment.replay.test.ts');
const EXAMPLE_PACKAGE_PATH = path.join(EXAMPLE_DIR, 'package.json');
const SDK_PACKAGE_PATH = path.resolve(__dirname, '..', '..', 'package.json');

describe('Task 4.0c Jest replay example', () => {
  it('uses the fragment upstream example consistently', () => {
    const readme = fs.readFileSync(README_PATH, 'utf8');
    const exampleTest = fs.readFileSync(EXAMPLE_TEST_PATH, 'utf8');
    const examplePackage = JSON.parse(fs.readFileSync(EXAMPLE_PACKAGE_PATH, 'utf8'));
    const sdkPackage = JSON.parse(fs.readFileSync(SDK_PACKAGE_PATH, 'utf8'));

    for (const content of [readme, exampleTest]) {
      expect(content).toContain('/fragment');
      expect(content).toContain('/hello');
      expect(content).toContain('fragment-happy-path.case.json');
      expect(content).toContain('await response.json()');
      expect(content).toContain("{ message: 'hello', dep: 'ok' }");
      expect(content).not.toMatch(/\bcheckout\b/);
      expect(content).not.toMatch(/\bstripe\b/);
      expect(content).not.toMatch(/\bpayment_intents\b/);
    }
    expect(readme).toContain('softprobe doctor');
    expect(readme).toContain('docs/design.md §5.3');
    expect(readme).toContain('x-softprobe-session-id');
    expect(readme).toContain('npm test');

    expect(examplePackage.private).toBe(true);
    expect(examplePackage.dependencies).toEqual({
      '@softprobe/softprobe-js': 'file:../../softprobe-js',
    });
    expect(examplePackage.scripts?.test).toContain('jest');
    expect(examplePackage.scripts?.test).toContain('fragment.replay.test.ts');

    expect(sdkPackage.scripts?.['example:jest-replay']).toBeUndefined();
  });
});
