import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom',
    coverage: {
      provider: 'v8',
      all: true,
      reporter: ['text', 'lcov', 'html', 'json-summary'],
      include: ['src/**/*.js'],
      exclude: ['src/**/*.test.js', 'src/main.js'],
      thresholds: {
        lines: 80,
        statements: 80,
        functions: 80,
        branches: 70,
      },
    },
  },
});
