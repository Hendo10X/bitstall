"use client";

import { useState } from "react";
import { HugeiconsIcon } from "@hugeicons/react";
import { Cancel01Icon, Location01Icon } from "@hugeicons/core-free-icons";
import { setLocation } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function LocationPopover({
  current,
  onLocationChanged,
  onClose,
}: {
  current: { lat: number; lon: number } | null;
  onLocationChanged: () => void;
  onClose: () => void;
}) {
  const [lat, setLat] = useState(current ? String(current.lat) : "");
  const [lon, setLon] = useState(current ? String(current.lon) : "");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const latN = parseFloat(lat);
    const lonN = parseFloat(lon);
    if (
      Number.isNaN(latN) ||
      Number.isNaN(lonN) ||
      latN < -90 ||
      latN > 90 ||
      lonN < -180 ||
      lonN > 180
    ) {
      setError("Invalid coordinates");
      setResult(null);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const res = await setLocation(latN, lonN);
      setResult(res.geohash);
      onLocationChanged();
    } catch {
      setError("Daemon unreachable");
      setResult(null);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="absolute bottom-full left-0 z-40 mb-2 w-64 rounded-lg border border-white/10 bg-popover p-3">
      <div className="mb-3 flex items-center justify-between">
        <span className="font-heading text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
          Set location
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
          <Label htmlFor="loc-lat" className="w-8 text-[11px] text-muted-foreground">
            Lat
          </Label>
          <Input
            id="loc-lat"
            value={lat}
            onChange={(e) => setLat(e.target.value)}
            placeholder="-90 … 90"
            inputMode="decimal"
          />
        </div>
        <div className="flex items-center gap-2">
          <Label htmlFor="loc-lon" className="w-8 text-[11px] text-muted-foreground">
            Lon
          </Label>
          <Input
            id="loc-lon"
            value={lon}
            onChange={(e) => setLon(e.target.value)}
            placeholder="-180 … 180"
            inputMode="decimal"
          />
        </div>
        <Button
          type="submit"
          size="sm"
          className="mt-1 rounded-full"
          disabled={busy || !lat.trim() || !lon.trim()}
        >
          {busy ? "Updating…" : "Update"}
        </Button>
      </form>
      {result && (
        <p className="mt-2 flex items-center gap-1.5 text-[11px] text-primary">
          <HugeiconsIcon icon={Location01Icon} size={12} strokeWidth={1.8} />
          geohash → <span className="font-mono font-semibold">{result}</span>
        </p>
      )}
      {error && <p className="mt-2 text-[11px] text-muted-foreground">{error}</p>}
    </div>
  );
}
