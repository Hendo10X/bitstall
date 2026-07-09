import type { ChatSummary } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";

/** Flat badge distinguishing the two chat transports. */
export function TransportBadge({
  transport,
  className,
}: {
  transport: ChatSummary["transport"];
  className?: string;
}) {
  return (
    <Badge
      className={cn(
        "border font-mono uppercase",
        transport === "bitchat"
          ? "border-primary/40 bg-primary/10 text-primary"
          : "border-white/20 bg-transparent text-muted-foreground",
        className
      )}
    >
      {transport}
    </Badge>
  );
}
