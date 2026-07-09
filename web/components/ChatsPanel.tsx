"use client";

import type { ChatSummary } from "@/lib/api";
import { timeAgo } from "@/lib/format";
import { cn } from "@/lib/utils";
import { peerDisplay } from "@/lib/peer";
import { Badge } from "@/components/ui/badge";
import { TransportBadge } from "@/components/TransportBadge";

export function ChatsPanel({
  chats,
  activePeer,
  onOpen,
}: {
  chats: ChatSummary[] | null;
  activePeer: string | null;
  onOpen: (peer: string) => void;
}) {
  const sorted = (chats ?? [])
    .slice()
    .sort((a, b) => b.lastTimestamp - a.lastTimestamp);

  return (
    <section>
      <h2 className="mb-4 flex items-center gap-2 font-heading text-xs font-medium tracking-widest text-muted-foreground uppercase">
        Chats
        <Badge variant="secondary" className="font-mono tabular-nums">
          {sorted.length}
        </Badge>
      </h2>

      {chats === null ? (
        <p className="text-xs text-muted-foreground">Scanning…</p>
      ) : sorted.length === 0 ? (
        <p className="rounded-lg border border-dashed border-white/15 p-4 text-xs leading-relaxed text-muted-foreground">
          No conversations yet.
          <br />
          Tap a listing to message its seller.
        </p>
      ) : (
        <ul className="flex flex-col gap-0.5">
          {sorted.map((c) => (
            <li key={c.peer}>
              <button
                onClick={() => onOpen(c.peer)}
                className={cn(
                  "flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary/40",
                  activePeer === c.peer
                    ? "bg-primary/10"
                    : "bg-transparent hover:bg-white/[0.04]"
                )}
                title={c.peer}
              >
                <span
                  className={cn(
                    "size-1.5 shrink-0 rounded-full",
                    c.established
                      ? "bg-primary"
                      : "border border-white/40 bg-transparent"
                  )}
                  title={
                    c.established ? "channel established" : "establishing…"
                  }
                />
                <span className="min-w-0 flex-1 truncate font-mono text-xs text-foreground">
                  {peerDisplay(c.peer, 10)}
                </span>
                <TransportBadge transport={c.transport} />
                <span className="shrink-0 text-[11px] text-muted-foreground tabular-nums">
                  {c.messages} msg{c.messages === 1 ? "" : "s"}
                </span>
                <span className="shrink-0 text-[11px] text-muted-foreground">
                  {c.lastTimestamp ? timeAgo(c.lastTimestamp) : ""}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
