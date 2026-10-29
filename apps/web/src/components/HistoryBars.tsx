"use client"

import { useEffect, useState } from "react"
import { createPortal } from "react-dom"
import { cn } from "@/lib/utils"
import type { Bucket } from "@/lib/types"
import { bucketMs, formatBucket, formatUptime } from "./format"

// One bar per bucket. data-level lets custom CSS recolor partial outages.
function level(b: Bucket): "none" | "ok" | "partial" | "down" {
  if (b.checks === 0) return "none"
  const r = b.successes / b.checks
  if (r === 1) return "ok"
  if (r >= 0.5) return "partial"
  return "down"
}

// Half the hover card's max width (max-w-56) plus a small gutter, so it never
// runs off the side of the viewport.
const HALF_CARD_PX = 120

type Hover = { i: number; x: number; top: number; bottom: number }

// One shared hover card per row rather than a tooltip per bar, since a
// dashboard can have thousands of bars. It is portaled to <body> with fixed
// positioning so containers with overflow-hidden (cards) don't clip it.
export function HistoryBars({ history, className }: { history: Bucket[]; className?: string }) {
  const [hover, setHover] = useState<Hover | null>(null)
  const stride = bucketMs(history)
  const b = hover === null ? null : history[hover.i]

  // Fixed coordinates go stale on scroll, so hide the card instead.
  useEffect(() => {
    if (!hover) return
    const hide = () => setHover(null)
    window.addEventListener("scroll", hide, { passive: true, capture: true })
    return () => window.removeEventListener("scroll", hide, { capture: true })
  }, [hover])

  return (
    <div className={cn("relative", className)} onMouseLeave={() => setHover(null)}>
      <div className="seer-history flex h-7 gap-0.5" role="img" aria-label="Availability history">
        {history.map((bucket, i) => (
          <span
            key={bucket.start}
            className="seer-history-bar min-w-px flex-1 rounded-[2px] transition-opacity hover:opacity-70"
            data-level={level(bucket)}
            onMouseEnter={(ev) => {
              const r = ev.currentTarget.getBoundingClientRect()
              setHover({ i, x: r.left + r.width / 2, top: r.top, bottom: r.bottom })
            }}
          />
        ))}
      </div>
      {b &&
        hover &&
        createPortal(
          <HoverCard bucket={b} stride={stride} hover={hover} />,
          document.body,
        )}
    </div>
  )
}

function HoverCard({ bucket: b, stride, hover }: { bucket: Bucket; stride: number; hover: Hover }) {
  const left = Math.min(Math.max(hover.x, HALF_CARD_PX), window.innerWidth - HALF_CARD_PX)
  // Above the bar unless that would leave the viewport, then below.
  const above = hover.top > 72
  return (
    <div
      className="seer-history-tooltip pointer-events-none fixed z-50 w-max max-w-56 -translate-x-1/2 rounded-md border bg-popover px-2.5 py-1.5 text-xs text-popover-foreground shadow-md"
      style={
        above
          ? { left, bottom: window.innerHeight - hover.top + 8 }
          : { left, top: hover.bottom + 8 }
      }
    >
      <div className="font-medium">{formatBucket(b.start, stride)}</div>
      {b.checks === 0 ? (
        <div className="text-muted-foreground">No data</div>
      ) : (
        <div className="text-muted-foreground">
          {formatUptime(b.successes / b.checks, b.checks)} up · {b.successes}/{b.checks} checks
          {b.successes > 0 && ` · avg ${Math.round(b.avgMs)} ms`}
        </div>
      )}
    </div>
  )
}
