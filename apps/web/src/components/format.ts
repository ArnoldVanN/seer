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

// A fixed locale, not the viewer's: these render on the server and hydrate in
// the browser, and a locale difference would make the markup mismatch.
export function formatBucket(iso: string, daily: boolean): string {
  const d = new Date(iso)
  return daily
    ? d.toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric", timeZone: "UTC" })
    : d.toLocaleString("en-US", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", timeZone: "UTC" }) + " UTC"
}
