/** @type {import('jest').Config} */
module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  roots: ['<rootDir>'],
  testMatch: ['**/*.test.ts'],
  moduleFileExtensions: ['ts', 'js', 'json'],
  moduleNameMapper: {
    '^@softprobe/softprobe-js$': '<rootDir>/../../softprobe-js/src/index.ts',
    '^@softprobe/softprobe-js/init$': '<rootDir>/../../softprobe-js/src/init.ts',
  },
};
