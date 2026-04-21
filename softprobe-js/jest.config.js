/** @type {import('jest').Config} */
module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  roots: ['<rootDir>/src'],
  testMatch: ['**/__tests__/**/*.test.ts'],
  collectCoverageFrom: ['src/**/*.ts', '!src/**/__tests__/**'],
  moduleFileExtensions: ['ts', 'js', 'json'],
  transform: {
    '^.+\\.tsx?$': ['ts-jest', { tsconfig: 'tsconfig.test.json' }],
  },
  // The generated fixture imports `@softprobe/softprobe-js` (the published
  // package name) so the test exercises what users see. In-tree we redirect
  // that to the local source.
  moduleNameMapper: {
    '^@softprobe/softprobe-js$': '<rootDir>/src/index.ts',
    '^@softprobe/softprobe-js/hooks$': '<rootDir>/src/hooks.ts',
    '^@softprobe/softprobe-js/suite$': '<rootDir>/src/suite.ts',
  },
};
