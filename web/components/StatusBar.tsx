"use client";

import { useEffect, useRef, useState } from "react";
import { HugeiconsIcon } from "@hugeicons/react";
import {
  BluetoothIcon,
  Copy01Icon,
  Globe02Icon,
  Location01Icon,
  MoreHorizontalIcon,
  Satellite01Icon,
  Tick02Icon,
  UserCircleIcon,
  UserGroupIcon,
  Wifi01Icon,
} from "@hugeicons/core-free-icons";
import type { DaemonStatus } from "@/lib/api";
import { shortKey } from "@/lib/format";
import { cn } from "@/lib/utils";
import { LocationPopover } from "@/components/LocationPopover";
import { NicknamePopover } from "@/components/NicknamePopover";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

// Green-outlined pill (active) vs dim-white pill (neutral/off).
const chip =
  "inline-flex h-6 shrink-0 items-center gap-1.5 rounded-full border px-2.5 text-[11px] font-mono uppercase tracking-wide whitespace-nowrap";
const chipOn = "border-primary/40 bg-primary/10 text-primary";
const chipDim = "border-white/15 text-muted-foreground";

function TransportChip({
  on,
  label,
  icon,
}: {
  on: boolean;
  label: string;
  icon: typeof Wifi01Icon;
}) {
  return (
    <span
      className={cn(chip, on ? chipOn : chipDim)}
      title={`${label}: ${on ? "up" : "down"}`}
    >
      <HugeiconsIcon icon={icon} size={12} strokeWidth={1.8} />
      {label}
    </span>
  );
}

function CopyKey({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    if (!value) return;
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // clipboard unavailable, ignore
    }
  };

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger
          onClick={copy}
          className={cn(
            chip,
            chipDim,
            "transition-colors hover:border-white/30 hover:text-foreground"
          )}
          aria-label="copy identity key"
        >
          <span>{value ? shortKey(value) : "…"}</span>
          <HugeiconsIcon
            icon={copied ? Tick02Icon : Copy01Icon}
            size={12}
            strokeWidth={1.8}
            className={copied ? "text-primary" : undefined}
          />
        </TooltipTrigger>
        <TooltipContent side="top">
          {copied ? "Copied" : "Copy identity key"}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

export function StatusBar({
  status,
  offline,
  onLocationChanged,
}: {
  status: DaemonStatus | null;
  offline: boolean;
  onLocationChanged: () => void;
}) {
  const [locOpen, setLocOpen] = useState(false);
  const locRef = useRef<HTMLDivElement>(null);
  const [nickOpen, setNickOpen] = useState(false);
  const nickRef = useRef<HTMLDivElement>(null);
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    if (!locOpen) return;
    const onDown = (e: MouseEvent) => {
      if (locRef.current && !locRef.current.contains(e.target as Node)) {
        setLocOpen(false);
      }
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [locOpen]);

  useEffect(() => {
    if (!nickOpen) return;
    const onDown = (e: MouseEvent) => {
      if (nickRef.current && !nickRef.current.contains(e.target as Node)) {
        setNickOpen(false);
      }
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [nickOpen]);

  const bitchat = status?.bitchat;

  return (
    <div className="sticky bottom-0 z-30 border-t border-white/10 bg-background/90 backdrop-blur-sm">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-2 gap-y-2 px-6 py-3">
        {/* geohash pill → location popover */}
        <div className="relative" ref={locRef}>
          <button
            onClick={() => setLocOpen((v) => !v)}
            title="set location"
            className={cn(
              chip,
              "transition-colors",
              status?.geohash ? chipOn : chipDim,
              locOpen && "border-primary"
            )}
          >
            <HugeiconsIcon icon={Location01Icon} size={12} strokeWidth={1.8} />
            <span>{status?.geohash || "no-fix"}</span>
          </button>
          {locOpen && (
            <LocationPopover
              current={status?.location ?? null}
              onLocationChanged={onLocationChanged}
              onClose={() => setLocOpen(false)}
            />
          )}
        </div>

        {/* peers */}
        <span
          className={cn(chip, status?.peers ? chipOn : chipDim)}
          title="mesh peers in range"
        >
          <HugeiconsIcon icon={UserGroupIcon} size={12} strokeWidth={1.8} />
          peers {status ? status.peers : "0"}
        </span>

        {/* transports: only shown when active */}
        {status?.transports.multicast && (
          <TransportChip on label="mcast" icon={Wifi01Icon} />
        )}
        {status?.transports.ble && (
          <TransportChip on label="ble" icon={BluetoothIcon} />
        )}

        {/* … expander for the rest */}
        <button
          onClick={() => setExpanded((v) => !v)}
          aria-label={expanded ? "hide details" : "show more status"}
          className={cn(
            chip,
            "transition-colors",
            expanded
              ? "border-primary/40 bg-primary/10 text-primary"
              : "border-white/15 text-muted-foreground hover:border-white/30 hover:text-foreground"
          )}
        >
          <HugeiconsIcon icon={MoreHorizontalIcon} size={14} strokeWidth={1.8} />
        </button>

        {/* expanded detail chips */}
        {expanded && (
          <>
            {/* bitchat online/offline (green vs dim) */}
            <span
              className={cn(chip, bitchat?.online ? chipOn : chipDim)}
              title={
                bitchat?.relays?.length
                  ? `relays:\n${bitchat.relays.join("\n")}`
                  : "no relays"
              }
            >
              <HugeiconsIcon
                icon={Satellite01Icon}
                size={12}
                strokeWidth={1.8}
              />
              {bitchat
                ? bitchat.online
                  ? "bitchat online"
                  : "bitchat offline"
                : "bitchat n/a"}
            </span>
            {bitchat && bitchat.queued > 0 && (
              <span
                className={cn(chip, chipDim)}
                title="messages queued for relay delivery"
              >
                {bitchat.queued} queued
              </span>
            )}

            {/* participants */}
            <span
              className={cn(chip, bitchat?.participants ? chipOn : chipDim)}
              title="people currently in this geohash channel"
            >
              <HugeiconsIcon icon={Globe02Icon} size={12} strokeWidth={1.8} />
              {bitchat ? bitchat.participants : "0"} in channel
            </span>

            {/* nickname → editor popover */}
            <div className="relative" ref={nickRef}>
              <button
                onClick={() => setNickOpen((v) => !v)}
                title="set the nickname shown next to your Bitchat posts"
                className={cn(
                  chip,
                  "transition-colors",
                  bitchat?.nickname ? chipOn : chipDim,
                  nickOpen && "border-primary"
                )}
              >
                <HugeiconsIcon
                  icon={UserCircleIcon}
                  size={12}
                  strokeWidth={1.8}
                />
                {bitchat?.nickname || "set nick"}
              </button>
              {nickOpen && (
                <NicknamePopover
                  current={bitchat?.nickname ?? ""}
                  onSaved={onLocationChanged}
                  onClose={() => setNickOpen(false)}
                />
              )}
            </div>

            {/* own pubkey copy */}
            <CopyKey value={status?.identity.noisePub ?? ""} />
          </>
        )}

        {/* daemon offline: dim, not red */}
        {offline && (
          <span
            className={cn(chip, chipDim, "ml-auto")}
            title="retrying localhost:4617"
          >
            <span className="inline-block size-1.5 rounded-full border border-white/40" />
            daemon offline
          </span>
        )}
      </div>
    </div>
  );
}
