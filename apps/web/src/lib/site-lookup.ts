import { publicApiUrl } from "./api-url"

// What proxy.ts needs to know about a hostname's site. Pages get the full
// site config through lib/data instead.
export type SiteAccess = { private: boolean; hasPrivate: boolean }

const TTL_MS = 60_000
// Bounded so odd Host headers can't grow it without limit.
const MAX_ENTRIES = 200
const cache = new Map<string, { value: SiteAccess; expires: number }>()

// Looks up a host's site on the public API with a plain Connect JSON call.
// Returns null when the API can't be reached, so callers can fall back to
// letting the page handle it.
export async function siteAccess(host: string): Promise<SiteAccess | null> {
  const hit = cache.get(host)
  if (hit && hit.expires > Date.now()) return hit.value
  try {
    const res = await fetch(`${publicApiUrl()}/seer.public.v1.StatusService/GetSiteConfig`, {
      method: "POST",
      headers: { "content-type": "application/json", "x-forwarded-host": host },
      body: "{}",
      signal: AbortSignal.timeout(3000),
    })
    if (!res.ok) return null
    // Proto3 JSON leaves out false booleans.
    const { site } = (await res.json()) as { site?: { private?: boolean; hasPrivate?: boolean } }
    const value = { private: !!site?.private, hasPrivate: !!site?.hasPrivate }
    if (cache.size >= MAX_ENTRIES) cache.delete(cache.keys().next().value!)
    cache.set(host, { value, expires: Date.now() + TTL_MS })
    return value
  } catch {
    return null
  }
}
