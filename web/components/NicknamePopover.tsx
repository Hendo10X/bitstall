"use client";

import { useRef, useState } from "react";
import { HugeiconsIcon } from "@hugeicons/react";
import { Cancel01Icon, UserCircleIcon } from "@hugeicons/core-free-icons";
import { setNickname } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const NICK_MAX = 24;

export function NicknamePopover({
  current,
  onSaved,
  onClose,
}: {
  current: string;
  onSaved: () => void;
  onClose: () => void;
}) {
  const [nick, setNick] = useState(current);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const n = nick.trim();
    if (!n) {
      setError("Enter a nickname");
      setResult(null);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const res = await setNickname(n);
      setResult(res.nickname);
      onSaved();
    } catch {
      setError("Daemon unreachable");
      setResult(null);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="absolute right-0 bottom-full z-40 mb-2 w-64 rounded-lg border border-white/10 bg-popover p-3">
      <div className="mb-3 flex items-center justify-between">
        <span className="font-heading text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
          Bitchat nickname
        </span>
        <Button
          variant="ghost"
          size="icon-xs"
          className="rounded-full"
          onClick={onClose}
          aria-label="close"
        >
          <HugeiconsIcon icon={Cancel01Icon} size={12} strokeWidth={1.8} />
        </Button>
      </div>
      <form onSubmit={submit} className="flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <Label
            htmlFor="nick-input"
            className="w-8 text-[11px] text-muted-foreground"
          >
            Nick
          </Label>
          <Input
            id="nick-input"
            ref={inputRef}
            value={nick}
            maxLength={NICK_MAX}
            onChange={(e) => setNick(e.target.value.slice(0, NICK_MAX))}
            placeholder="shown next to your posts"
          />
        </div>
        <Button
          type="submit"
          size="sm"
          className="mt-1 rounded-full"
          disabled={busy || !nick.trim()}
        >
          {busy ? "Saving…" : "Save"}
        </Button>
      </form>
      {result && (
        <p className="mt-2 flex items-center gap-1.5 text-[11px] text-primary">
          <HugeiconsIcon icon={UserCircleIcon} size={12} strokeWidth={1.8} />
          posting as <span className="font-semibold">{result}</span>
        </p>
      )}
      {error && <p className="mt-2 text-[11px] text-muted-foreground">{error}</p>}
    </div>
  );
}
