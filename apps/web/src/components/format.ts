export function formatUptime(u: number, checks: number): string {
  if (checks === 0) return "—"
  const pct = u * 100
  return pct === 100 ? "100%" : `${pct.toFixed(pct >= 99 ? 3 : 2)}%`
}

export function formatAgo(iso: string | null, now: number): string {
  if (!iso) return "never"
  const s = Math.max(0, Math.round((now - Date.parse(iso)) / 1000))
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}

const DAY_MS = 86_400_000

// Bucket length of a history series, from its first two buckets.
export function bucketMs(history: { start: string }[]): number {
  return history.length > 1 ? Date.parse(history[1].start) - Date.parse(history[0].start) : DAY_MS
}

// The span a bucket covers, e.g. "Oct 29, 08:00 – 12:00 UTC" or
// "Oct 28 – Oct 29, 2026". A fixed locale, not the viewer's: these render on
// the server and hydrate in the browser, and a locale difference would make
// the markup mismatch.
export function formatBucket(iso: string, stride: number): string {
  const start = new Date(iso)
  const date = (d: Date, year?: boolean) =>
    d.toLocaleDateString("en-US", { month: "short", day: "numeric", year: year ? "numeric" : undefined, timeZone: "UTC" })
  if (stride >= DAY_MS) {
    if (stride === DAY_MS) return date(start, true)
    return `${date(start)} – ${date(new Date(start.getTime() + stride - DAY_MS), true)}`
  }
  const time = (d: Date) => d.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit", hourCycle: "h23", timeZone: "UTC" })
  return `${date(start)}, ${time(start)} – ${time(new Date(start.getTime() + stride))} UTC`
}
