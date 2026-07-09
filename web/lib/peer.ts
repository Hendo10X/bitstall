import { shortKey } from "@/lib/format";

/**
 * Bitchat DM peers are Nostr pubkeys prefixed with "bc:". The raw peer string
 * (including the prefix) must flow through to the API unchanged; this only
 * derives a friendly truncated form for display.
 */
export function peerDisplay(peer: string, head: number = 8): string {
  const raw = peer.startsWith("bc:") ? peer.slice(3) : peer;
  return shortKey(raw, head);
}
