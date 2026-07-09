"use client";

import { HugeiconsIcon } from "@hugeicons/react";
import { Message01Icon } from "@hugeicons/core-free-icons";
import type { Listing } from "@/lib/api";
import { shortKey, timeAgo } from "@/lib/format";
import { cn } from "@/lib/utils";
import {
  Card,
  CardAction,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

export function ListingCard({
  listing,
  onOpenChat,
}: {
  listing: Listing;
  onOpenChat: (peer: string) => void;
}) {
  const clickable = !listing.mine;

  return (
    <Card
      size="sm"
      role={clickable ? "button" : undefined}
      tabIndex={clickable ? 0 : undefined}
      onClick={clickable ? () => onOpenChat(listing.sellerKey) : undefined}
      onKeyDown={
        clickable
          ? (e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onOpenChat(listing.sellerKey);
              }
            }
          : undefined
      }
      title={clickable ? "open chat with seller" : undefined}
      className={cn(
        "group border-transparent bg-white/[0.03] ring-0 transition-colors",
        clickable &&
          "cursor-pointer hover:bg-primary/10 focus-visible:bg-primary/10 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary/40"
      )}
    >
      <CardHeader>
        <CardTitle className="truncate text-sm font-medium">
          {listing.title}
        </CardTitle>
        {listing.mine && (
          <CardAction>
            <Badge className="border border-primary/40 bg-primary/10 font-mono uppercase text-primary">
              Yours
            </Badge>
          </CardAction>
        )}
      </CardHeader>

      <CardContent>
        <div className="text-[15px] font-semibold text-foreground">
          {listing.price}
        </div>
      </CardContent>

      <CardFooter className="items-center justify-between gap-2 text-[11px] text-muted-foreground">
        <span className="font-mono" title={listing.sellerKey}>
          {shortKey(listing.sellerKey, 6)}
        </span>
        <span className="font-mono text-primary">#{listing.geohash}</span>
        <span className="flex items-center gap-2">
          {timeAgo(listing.timestamp)}
          {clickable && (
            <span className="flex items-center gap-1 text-foreground opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">
              <HugeiconsIcon icon={Message01Icon} size={12} strokeWidth={1.8} />
              chat
            </span>
          )}
        </span>
      </CardFooter>
    </Card>
  );
}
