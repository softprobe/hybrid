/** @type {import('jest').Config} */
module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  roots: ['<rootDir>/src'],
  testMatch: ['**/__tests__/**/*.test.ts'],
  collectCoverageFrom: ['src/**/*.ts', '!src/**/__tests__/**'],
  moduleFileExtensions: ['ts', 'js', 'json'],
  // The generated fixture imports `@softprobe/softprobe-js` (the published
  // package name) so the test exercises what users see. In-tree we redirect
  // that to the local source.
  moduleNameMapper: {
    '^@softprobe/softprobe-js$': '<rootDir>/src/index.ts',
  },
};
