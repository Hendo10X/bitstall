"use client";

import { useState } from "react";
import { HugeiconsIcon } from "@hugeicons/react";
import { PlusSignIcon } from "@hugeicons/core-free-icons";
import { getChats, getListings, getStatus } from "@/lib/api";
import { usePoll } from "@/lib/usePoll";
import { cn } from "@/lib/utils";
import { StatusBar } from "@/components/StatusBar";
import { ListingCard } from "@/components/ListingCard";
import { NewListingModal } from "@/components/NewListingModal";
import { ChatDrawer } from "@/components/ChatDrawer";
import { ChatsPanel } from "@/components/ChatsPanel";
import { ChannelFeed } from "@/components/ChannelFeed";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

export default function Dashboard() {
  const status = usePoll(getStatus, 2500);
  const listings = usePoll(getListings, 3000);
  const chats = usePoll(getChats, 3000);

  const [activePeer, setActivePeer] = useState<string | null>(null);
  const [showNewListing, setShowNewListing] = useState(false);

  // daemon considered offline when the status poll fails
  const offline = status.error;

  const sortedListings = (listings.data ?? [])
    .slice()
    .sort((a, b) => b.timestamp - a.timestamp);

  const activeChatSummary = (chats.data ?? []).find(
    (c) => c.peer === activePeer
  );

  return (
    <div
      className={cn(
        "flex min-h-dvh flex-col",
        offline ? "opacity-90" : ""
      )}
    >
      <main className="mx-auto w-full max-w-6xl flex-1 px-6 py-8">
        {/* brand + hero */}
        <section className="mb-12">
          <span className="inline-flex h-6 items-center rounded-full border border-primary px-3 font-mono text-[11px] font-bold uppercase tracking-widest text-primary">
            Bitstall
          </span>
          <h1 className="mt-6 max-w-3xl font-[family-name:var(--font-pixel)] leading-[1.1] tracking-tight text-foreground text-[clamp(1.75rem,7vw,3.75rem)]">
            The Bluetooth marketplace
          </h1>
          <p className="mt-4 max-w-xl font-sans text-sm leading-relaxed text-muted-foreground">
            Buy and sell with people nearby over Bitchat. No servers, no
            accounts, no signal required.
          </p>
        </section>

        <div className="flex flex-col gap-12 lg:flex-row lg:gap-10">
          {/* listings */}
          <section className="min-w-0 flex-1">
            <div className="mb-5 flex items-center justify-between gap-3">
              <h2 className="flex items-center gap-2 font-heading text-xs font-medium tracking-widest text-muted-foreground uppercase">
                Listings
                <Badge variant="secondary" className="font-mono tabular-nums">
                  {sortedListings.length}
                </Badge>
              </h2>
              <Button
                onClick={() => setShowNewListing(true)}
                variant="outline"
                className="rounded-full border-primary/50 font-mono uppercase tracking-wide text-primary hover:bg-primary/10 hover:text-primary"
              >
                <HugeiconsIcon icon={PlusSignIcon} strokeWidth={2} />
                Add listings
              </Button>
            </div>

            {listings.data === null && !listings.error ? (
              <p className="mt-12 text-center text-xs text-muted-foreground">
                Scanning mesh…
              </p>
            ) : sortedListings.length === 0 ? (
              <div className="mt-6 rounded-lg border border-dashed border-white/15 p-10 text-center">
                <p className="text-sm font-medium text-foreground">
                  {listings.error
                    ? "No data · daemon offline"
                    : "The mesh is quiet"}
                </p>
                {!listings.error && (
                  <p className="mt-1.5 text-xs text-muted-foreground">
                    No listings in range. Broadcast the first one.
                  </p>
                )}
              </div>
            ) : (
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
                {sortedListings.map((l) => (
                  <ListingCard
                    key={l.id}
                    listing={l}
                    onOpenChat={(peer) => setActivePeer(peer)}
                  />
                ))}
              </div>
            )}
          </section>

          {/* channel feed + chats list */}
          <aside className="flex w-full shrink-0 flex-col gap-10 lg:w-80">
            <ChannelFeed offline={offline} />
            <ChatsPanel
              chats={chats.data}
              activePeer={activePeer}
              onOpen={(peer) => setActivePeer(peer)}
            />
          </aside>
        </div>

        <footer className="mt-16 pt-4 font-mono text-[11px] text-muted-foreground">
          bitstall · local-first mesh market · daemon @ localhost:4617
        </footer>
      </main>

      <StatusBar
        status={status.data}
        offline={offline}
        onLocationChanged={() => void status.refresh()}
      />

      {showNewListing && (
        <NewListingModal
          onClose={() => setShowNewListing(false)}
          onCreated={() => void listings.refresh()}
        />
      )}

      {activePeer && (
        <ChatDrawer
          key={activePeer}
          peer={activePeer}
          transport={
            activeChatSummary?.transport ??
            (activePeer.startsWith("bc:") ? "bitchat" : "mesh")
          }
          established={activeChatSummary?.established ?? false}
          offline={offline}
          onClose={() => setActivePeer(null)}
        />
      )}
    </div>
  );
}
