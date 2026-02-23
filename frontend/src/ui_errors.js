export function formatErrorMessage(payload) {
  const data = payload || {};
  if (data.detail && data.message) {
    return `${data.message}: ${data.detail}`;
  }
  if (data.message) {
    return data.message;
  }
  return 'Unknown error';
}
