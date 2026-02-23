import { describe, expect, it } from 'vitest';

import { formatErrorMessage } from './ui_errors';

describe('formatErrorMessage', () => {
  it('formats message with detail', () => {
    expect(formatErrorMessage({ message: 'Audio streaming issue', detail: 'broken pipe' })).toBe('Audio streaming issue: broken pipe');
  });

  it('falls back to message only', () => {
    expect(formatErrorMessage({ message: 'Startup failed' })).toBe('Startup failed');
  });

  it('returns unknown for empty payload', () => {
    expect(formatErrorMessage({})).toBe('Unknown error');
    expect(formatErrorMessage(null)).toBe('Unknown error');
  });
});
