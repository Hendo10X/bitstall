"use client";

import { useEffect, useRef, useState } from "react";
import { HugeiconsIcon } from "@hugeicons/react";
import { MessageLock01Icon, SentIcon } from "@hugeicons/core-free-icons";
import { getChatMessages, sendChatMessage } from "@/lib/api";
import type { ChatSummary } from "@/lib/api";
import { usePoll } from "@/lib/usePoll";
import { shortKey, timeAgo } from "@/lib/format";
import { peerDisplay } from "@/lib/peer";
import { cn } from "@/lib/utils";
import { TransportBadge } from "@/components/TransportBadge";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export function ChatDrawer({
  peer,
  transport,
  established,
  offline,
  onClose,
}: {
  peer: string;
  transport: ChatSummary["transport"];
  established: boolean;
  offline: boolean;
  onClose: () => void;
}) {
  const { data: messages, refresh } = usePoll(
    () => getChatMessages(peer),
    2000
  );
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // auto-scroll on new messages
  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages?.length]);

  // queued messages flush once the secure channel comes up
  useEffect(() => {
    if (established) {
      setNote((n) => (n === "Queued for relay delivery" ? null : n));
    }
  }, [established]);

  const send = async (e: React.FormEvent) => {
    e.preventDefault();
    const t = text.trim();
    if (!t || busy) return;
    setBusy(true);
    setNote(null);
    try {
      const res = await sendChatMessage(peer, t);
      setText("");
      setNote(res.status === "queued" ? "Queued for relay delivery" : null);
      await refresh();
    } catch {
      setNote("Send failed · daemon unreachable");
    } finally {
      setBusy(false);
      inputRef.current?.focus();
    }
  };

  return (
    <Sheet
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <SheetContent
        side="right"
        className="w-full gap-0 bg-background p-0 sm:max-w-md"
      >
        {/* header */}
        <SheetHeader className="gap-2 border-b border-border p-4 pr-12">
          <div className="flex items-center gap-2">
            <HugeiconsIcon
              icon={MessageLock01Icon}
              size={16}
              strokeWidth={1.8}
              className="text-muted-foreground"
            />
            <SheetTitle>Secure chat</SheetTitle>
            <TransportBadge transport={transport} />
          </div>
          <SheetDescription
            className="truncate font-mono text-[11px]"
            title={peer}
          >
            {peerDisplay(peer, 12)}
          </SheetDescription>
          <div>
            {established ? (
              <Badge className="border border-primary/40 bg-primary/10 font-mono uppercase text-primary">
                <span className="size-1.5 rounded-full bg-primary" />
                Channel established
              </Badge>
            ) : (
              <Badge className="border border-white/20 bg-transparent font-mono uppercase text-muted-foreground">
                <span className="size-1.5 rounded-full border border-white/40 bg-transparent" />
                Establishing secure channel…
              </Badge>
            )}
          </div>
        </SheetHeader>

        {/* messages */}
        <div ref={scrollRef} className="flex-1 overflow-y-auto px-4 py-4">
          {messages === null ? (
            <p className="text-center text-xs text-muted-foreground">
              Decrypting…
            </p>
          ) : messages.length === 0 ? (
            <p className="mt-8 text-center text-xs text-muted-foreground">
              No messages yet. Say hello.
            </p>
          ) : (
            <ul className="flex flex-col gap-2">
              {messages.map((m, i) => (
                <li
                  key={`${m.timestamp}-${i}`}
                  className={cn(
                    "flex",
                    m.outgoing ? "justify-end" : "justify-start"
                  )}
                >
                  <div
                    className={cn(
                      "max-w-[80%] rounded-2xl px-3.5 py-2 text-sm",
                      m.outgoing
                        ? "rounded-br-md bg-primary text-primary-foreground"
                        : "rounded-bl-md bg-muted text-foreground"
                    )}
                  >
                    <p className="break-words whitespace-pre-wrap">{m.text}</p>
                    <p
                      className={cn(
                        "mt-1 text-[10px]",
                        m.outgoing
                          ? "text-primary-foreground/60"
                          : "text-muted-foreground"
                      )}
                    >
                      {m.outgoing ? "you" : shortKey(m.from, 4)} ·{" "}
                      {timeAgo(m.timestamp)}
                    </p>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* composer */}
        <form onSubmit={send} className="border-t border-border p-3">
          {note && <p className="mb-2 text-[11px] text-muted-foreground">{note}</p>}
          <div className="flex items-center gap-2">
            <Input
              ref={inputRef}
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder={offline ? "Daemon offline" : "Type a message…"}
              disabled={offline}
              className="h-8 min-w-0 flex-1 rounded-full px-3"
            />
            <Button
              type="submit"
              size="icon-lg"
              className="rounded-full"
              disabled={busy || offline || !text.trim()}
              aria-label="send message"
            >
              <HugeiconsIcon icon={SentIcon} size={14} strokeWidth={1.8} />
            </Button>
          </div>
        </form>
      </SheetContent>
    </Sheet>
  );
}
