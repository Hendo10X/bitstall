/**
 * Bitstall daemon API client.
 * The local Go daemon owns ALL networking (BLE mesh + Nostr relays).
 * This module is the frontend's single point of contact with it.
 */

export const API_BASE = "http://localhost:4617";

// ---------- Types (daemon API contract) ----------

export interface DaemonStatus {
  peers: number;
  geohash: string;
  location: { lat: number; lon: number };
  identity: { noisePub: string; nostrPub: string };
  // transport keys may be absent when disabled
  transports: { multicast?: boolean; ble?: boolean };
  bitchat: {
    geohash: string;
    selfPub: string;
    nickname: string;
    online: boolean;
    queued: number;
    participants: number;
    relays: string[];
  };
}

export interface Listing {
  id: string;
  title: string;
  price: string;
  sellerKey: string;
  geohash: string;
  timestamp: number; // unix seconds
  mine: boolean;
}

export interface ChatSummary {
  // Nostr/Noise pubkey. Bitchat DM peers are prefixed with "bc:".
  peer: string;
  transport: "mesh" | "bitchat";
  messages: number;
  lastTimestamp: number;
  established: boolean;
}

/** A line in the live Bitchat geohash channel feed. */
export interface ChannelMessage {
  id: string;
  pubkey: string;
  nick: string;
  text: string;
  timestamp: number; // unix seconds
  mine: boolean; // we posted it
  listing: boolean; // carried a structured Bitstall listing
}

export interface ChatMessage {
  from: string;
  text: string;
  timestamp: number;
  outgoing: boolean;
}

export interface SendResult {
  status: "sent" | "queued";
}

// ---------- Fetch helper ----------

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    cache: "no-store",
    ...init,
  });
  if (!res.ok) {
    throw new Error(`daemon responded ${res.status} for ${path}`);
  }
  return (await res.json()) as T;
}

function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

// ---------- Endpoints ----------

export const getStatus = (): Promise<DaemonStatus> =>
  request<DaemonStatus>("/api/status");

export const getListings = (): Promise<Listing[]> =>
  request<Listing[]>("/api/listings");

export const createListing = (title: string, price: string): Promise<Listing> =>
  post<Listing>("/api/listings", { title, price });

export const setLocation = (
  lat: number,
  lon: number
): Promise<{ geohash: string }> => post<{ geohash: string }>("/api/location", { lat, lon });

export const getChannel = (): Promise<ChannelMessage[]> =>
  request<ChannelMessage[]>("/api/channel");

export const postChannelMessage = (
  text: string
): Promise<{ status: "sent" }> => post<{ status: "sent" }>("/api/channel", { text });

export const setNickname = (
  nickname: string
): Promise<{ nickname: string }> =>
  post<{ nickname: string }>("/api/config", { nickname });

export const getChats = (): Promise<ChatSummary[]> =>
  request<ChatSummary[]>("/api/chats");

export const getChatMessages = (peer: string): Promise<ChatMessage[]> =>
  request<ChatMessage[]>(`/api/chats/${encodeURIComponent(peer)}`);

export const sendChatMessage = (peer: string, text: string): Promise<SendResult> =>
  post<SendResult>(`/api/chats/${encodeURIComponent(peer)}`, { text });
