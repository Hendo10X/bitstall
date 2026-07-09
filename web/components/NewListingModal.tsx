"use client";

import { useRef, useState } from "react";
import { createListing } from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const TITLE_MAX = 40;

export function NewListingModal({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: () => void;
}) {
  const [title, setTitle] = useState("");
  const [price, setPrice] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const titleRef = useRef<HTMLInputElement>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const t = title.trim();
    const p = price.trim();
    if (!t || !p || busy) return;
    setBusy(true);
    setError(null);
    try {
      await createListing(t, p);
      onCreated();
      onClose();
    } catch {
      setError("Broadcast failed. Is the daemon running?");
      setBusy(false);
    }
  };

  const remaining = TITLE_MAX - title.length;

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="sm:max-w-md" initialFocus={titleRef}>
        <DialogHeader>
          <DialogTitle>New listing</DialogTitle>
          <DialogDescription>
            Broadcast an item to peers in range.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={submit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <div className="flex items-baseline justify-between">
              <Label htmlFor="listing-title">Title</Label>
              <span
                className={cn(
                  "text-[11px] tabular-nums",
                  remaining <= 5 ? "text-foreground" : "text-muted-foreground"
                )}
              >
                {title.length}/{TITLE_MAX}
              </span>
            </div>
            <Input
              id="listing-title"
              ref={titleRef}
              value={title}
              maxLength={TITLE_MAX}
              onChange={(e) => setTitle(e.target.value.slice(0, TITLE_MAX))}
              placeholder="What are you selling?"
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="listing-price">Price</Label>
            <Input
              id="listing-price"
              value={price}
              onChange={(e) => setPrice(e.target.value)}
              placeholder="e.g. 20 USD / 50k sats"
            />
          </div>

          {error && <p className="text-xs text-muted-foreground">{error}</p>}

          <Button
            type="submit"
            className="rounded-full"
            disabled={busy || !title.trim() || !price.trim()}
          >
            {busy ? "Broadcasting…" : "Broadcast to mesh"}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}
