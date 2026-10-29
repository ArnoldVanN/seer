"use client"

import Link from "next/link"
import { useSearchParams } from "next/navigation"
import { cn } from "@/lib/utils"
import { RANGES, type Range } from "@/lib/types"

const LABELS: Record<Range, string> = { "24h": "24h", "7d": "7d", "30d": "30d", "90d": "90d", "1y": "1y" }

// Keeps the other query params (search, status filter) when switching range.
export function RangePicker({ current, base }: { current: Range; base: string }) {
  const searchParams = useSearchParams()
  const href = (r: Range) => {
    const params = new URLSearchParams(searchParams.toString())
    params.set("range", r)
    return `${base}?${params}`
  }
  return (
    <nav className="seer-ranges inline-flex rounded-lg border bg-card p-0.5" aria-label="History range">
      {RANGES.map((r) => (
        <Link
          key={r}
          href={href(r)}
          aria-current={r === current ? "true" : undefined}
          className={cn(
            "seer-range rounded-md px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground",
            r === current && "bg-muted text-foreground",
          )}
        >
          {LABELS[r]}
        </Link>
      ))}
    </nav>
  )
}
