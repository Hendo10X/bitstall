/** "3m ago" style relative time from unix seconds. */
export function timeAgo(unixSeconds: number): string {
  const diff = Math.floor(Date.now() / 1000) - unixSeconds;
  if (diff < 10) return "just now";
  if (diff < 60) return `${diff}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

/** Truncate a hex pubkey: "a1b2c3d4…e5f6" */
export function shortKey(key: string, head: number = 8): string {
  if (!key) return "n/a";
  if (key.length <= head + 5) return key;
  return `${key.slice(0, head)}…${key.slice(-4)}`;
}
