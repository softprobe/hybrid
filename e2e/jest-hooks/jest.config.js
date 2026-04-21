/** @type {import('jest').Config} */
module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  roots: ['<rootDir>'],
  testMatch: ['**/*.test.ts'],
  moduleFileExtensions: ['ts', 'js', 'json'],
  // In-repo test uses local source directly so iterating on the SDK does not
  // require `npm run build`. A downstream consumer would instead depend on
  // the published `@softprobe/softprobe-js` package.
  moduleNameMapper: {
    '^@softprobe/softprobe-js$': '<rootDir>/../../softprobe-js/src/index.ts',
    '^@softprobe/softprobe-js/hooks$': '<rootDir>/../../softprobe-js/src/hooks.ts',
    '^@softprobe/softprobe-js/suite$': '<rootDir>/../../softprobe-js/src/suite.ts',
  },
};
