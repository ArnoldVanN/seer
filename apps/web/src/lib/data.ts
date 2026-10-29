import "server-only"
import { Code, ConnectError } from "@connectrpc/connect"
import { cacheLife } from "next/cache"
import { headers } from "next/headers"
import { notFound } from "next/navigation"
import { publicClient, privateClient } from "./api"
import { toDashboard, toEndpoint, toLinks, toRange, toResult, toSummary } from "./convert"
import type { Viewer } from "./session"
import type { Range, SiteView } from "./types"

// Public data is shared by every visitor, so it's cached briefly on the web
// server. During an outage a status page gets a traffic spike, and this keeps
// that load off the API and database.
const publicLife = { stale: 10, revalidate: 15, expire: 120 }

function orNotFound(err: unknown): never {
  if (err instanceof ConnectError && err.code === Code.NotFound) notFound()
  throw err
}

const DEFAULT_LOGO = { light: "/seer-logo.svg", dark: "/seer-logo-white.svg" }
// Recolors itself for dark browser themes.
const DEFAULT_FAVICON = "/seer-icon.svg"

// ui.logo unset uses Seer's logo pair; "none" hides it; a single custom logo
// is used in both modes unless ui.logoDark is also set.
function resolveLogo(light: string, dark: string): SiteView["logo"] {
  if (light === "none") return null
  if (!light) return { light: DEFAULT_LOGO.light, dark: dark || DEFAULT_LOGO.dark }
  return { light, dark: dark || light }
}

// The hostname the browser used, which picks the site.
export async function requestHost(): Promise<string> {
  const h = await headers()
  return (h.get("x-forwarded-host")?.split(",")[0] || h.get("host") || "").trim().toLowerCase()
}

// Cached per hostname; everything heavier is cached per site key instead, so
// odd Host headers can't multiply the dashboard caches.
export async function getSite(host: string): Promise<SiteView> {
  "use cache"
  cacheLife({ stale: 60, revalidate: 60, expire: 600 })
  const { site } = await publicClient({ host }).getSiteConfig({})
  return {
    key: site?.key ?? "",
    title: site?.title || "Status",
    description: site?.description ?? "",
    logo: resolveLogo(site?.logoUrl ?? "", site?.logoDarkUrl ?? ""),
    faviconUrl: site?.faviconUrl || DEFAULT_FAVICON,
    homeUrl: site?.homeUrl ?? "",
    footer: site?.footer ?? "",
    footerLinks: toLinks(site?.footerLinks ?? []),
    hidePoweredBy: site?.hidePoweredBy ?? false,
  }
}

export async function currentSite(): Promise<SiteView> {
  return getSite(await requestHost())
}

async function siteKey() {
  return (await currentSite()).key
}

export async function getCustomCss(host: string): Promise<string> {
  "use cache"
  cacheLife({ stale: 60, revalidate: 60, expire: 600 })
  const { site } = await publicClient({ host }).getSiteConfig({})
  return site?.customCss ?? ""
}

export async function listPublicDashboards() {
  return cachedPublicDashboards(await siteKey())
}

async function cachedPublicDashboards(key: string) {
  "use cache"
  cacheLife(publicLife)
  const { dashboards } = await publicClient({ key }).listDashboards({})
  return dashboards.map(toSummary)
}

export async function getPublicDashboard(key: string, range: Range | undefined) {
  return cachedPublicDashboard(await siteKey(), key, range)
}

async function cachedPublicDashboard(site: string, key: string, range: Range | undefined) {
  "use cache"
  cacheLife(publicLife)
  try {
    const { dashboard } = await publicClient({ key: site }).getDashboard({ key, historyRange: toRange(range) })
    return toDashboard(dashboard!)
  } catch (err) {
    orNotFound(err)
  }
}

export async function getPublicEndpoint(key: string, range: Range) {
  return cachedPublicEndpoint(await siteKey(), key, range)
}

async function cachedPublicEndpoint(site: string, key: string, range: Range) {
  "use cache"
  cacheLife(publicLife)
  try {
    const res = await publicClient({ key: site }).getEndpointHistory({ key, historyRange: toRange(range) })
    return { endpoint: toEndpoint(res.endpoint!), recent: res.recentResults.map(toResult) }
  } catch (err) {
    orNotFound(err)
  }
}

// Private data depends on the viewer's groups, so it isn't cached.

export async function listPrivateDashboards(viewer: Viewer) {
  const { dashboards } = await privateClient(viewer, { key: await siteKey() }).listDashboards({})
  return dashboards.map(toSummary)
}

export async function getPrivateDashboard(viewer: Viewer, key: string, range: Range | undefined) {
  try {
    const client = privateClient(viewer, { key: await siteKey() })
    const { dashboard } = await client.getDashboard({ key, historyRange: toRange(range) })
    return toDashboard(dashboard!)
  } catch (err) {
    orNotFound(err)
  }
}

export async function getPrivateEndpoint(viewer: Viewer, key: string, range: Range) {
  try {
    const client = privateClient(viewer, { key: await siteKey() })
    const res = await client.getEndpointHistory({ key, historyRange: toRange(range) })
    return { endpoint: toEndpoint(res.endpoint!), recent: res.recentResults.map(toResult) }
  } catch (err) {
    orNotFound(err)
  }
}
