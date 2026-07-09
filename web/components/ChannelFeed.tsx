"use client";

import { useEffect, useRef, useState } from "react";
import { HugeiconsIcon } from "@hugeicons/react";
import { SentIcon } from "@hugeicons/core-free-icons";
import { getChannel, postChannelMessage } from "@/lib/api";
import { usePoll } from "@/lib/usePoll";
import { timeAgo } from "@/lib/format";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export function ChannelFeed({ offline }: { offline: boolean }) {
  const { data: messages, refresh } = usePoll(getChannel, 3000);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // feed arrives newest-first; render oldest-first so the newest sits at the bottom
  const ordered = (messages ?? []).slice().reverse();

  // auto-scroll to the newest line
  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [ordered.length]);

  const send = async (e: React.FormEvent) => {
    e.preventDefault();
    const t = text.trim();
    if (!t || busy) return;
    setBusy(true);
    setNote(null);
    try {
      await postChannelMessage(t);
      setText("");
      await refresh();
    } catch {
      setNote("Send failed · daemon unreachable");
    } finally {
      setBusy(false);
      inputRef.current?.focus();
    }
  };

  return (
    <section className="flex flex-col">
      <h2 className="mb-4 flex items-center gap-2 font-heading text-xs font-medium tracking-widest text-muted-foreground uppercase">
        Geohash channel
        <Badge variant="secondary" className="font-mono tabular-nums">
          {ordered.length}
        </Badge>
      </h2>

      {messages === null ? (
        <p className="text-xs text-muted-foreground">Tuning in…</p>
      ) : ordered.length === 0 ? (
        <p className="rounded-lg border border-dashed border-white/15 p-4 text-xs leading-relaxed text-muted-foreground">
          No channel activity yet · messages from Bitchat users in this geohash
          appear here.
        </p>
      ) : (
        <div
          ref={scrollRef}
          className="max-h-96 overflow-y-auto rounded-lg"
        >
          <ul className="flex flex-col gap-0.5">
            {ordered.map((m) => (
              <li
                key={m.id}
                className={cn(
                  "rounded-md px-3 py-2",
                  m.mine ? "bg-primary/10" : "hover:bg-white/[0.04]"
                )}
              >
                <div className="flex items-baseline gap-2">
                  <span
                    className={cn(
                      "shrink-0 text-xs font-medium",
                      m.mine ? "text-primary" : "text-foreground"
                    )}
                  >
                    {m.mine ? "you" : m.nick || "anon"}
                  </span>
                  {m.listing && (
                    <Badge className="border border-primary/40 bg-primary/10 font-mono uppercase text-primary">
                      listing
                    </Badge>
                  )}
                  <span className="ml-auto shrink-0 text-[10px] text-muted-foreground">
                    {timeAgo(m.timestamp)}
                  </span>
                </div>
                <p className="mt-0.5 break-words whitespace-pre-wrap text-xs text-muted-foreground">
                  {m.text}
                </p>
              </li>
            ))}
          </ul>
        </div>
      )}

      <form onSubmit={send} className="mt-3">
        {note && <p className="mb-2 text-[11px] text-muted-foreground">{note}</p>}
        <div className="flex items-center gap-2">
          <Input
            ref={inputRef}
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder={offline ? "Daemon offline" : "Say something in the channel…"}
            disabled={offline}
            className="h-8 min-w-0 flex-1 rounded-full px-3"
          />
          <Button
            type="submit"
            size="icon-lg"
            className="rounded-full"
            disabled={busy || offline || !text.trim()}
            aria-label="post to channel"
          >
            <HugeiconsIcon icon={SentIcon} size={14} strokeWidth={1.8} />
          </Button>
        </div>
      </form>
    </section>
  );
}
