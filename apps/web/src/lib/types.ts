// Plain view models handed to client components. Protobuf messages carry
// bigints and class-like metadata, so pages convert them first.

export type Status = "up" | "down" | "unknown"

// Response times (avgMs, minMs, maxMs) cover successful checks only, and are
// 0 when there were none.
export type Bucket = {
  start: string
  checks: number
  successes: number
  avgMs: number
  minMs: number
  maxMs: number
}

export type LinkView = { name: string; url: string }

export type EndpointView = {
  key: string
  name: string
  group: string
  description: string
  links: LinkView[]
  status: Status
  lastCheckedAt: string | null
  lastResponseTimeMs: number
  uptime: number
  history: Bucket[]
}

export type DashboardView = {
  key: string
  title: string
  visibility: "public" | "private"
  description: string
  // The range the history covers: the requested one or the dashboard's default.
  range: Range
  groups: { name: string; endpoints: EndpointView[] }[]
  // The dashboard's own CSS from config, if any.
  customCss: string
}

export type DashboardSummary = {
  key: string
  title: string
  visibility: "public" | "private"
}

export type ResultView = {
  checkedAt: string
  success: boolean
  responseTimeMs: number
  error: string
}

export type SiteView = {
  // Which site of the deployment this is (Config.sites).
  key: string
  title: string
  description: string
  // Resolved logos: null when hidden with logo: none.
  logo: { light: string; dark: string } | null
  faviconUrl: string
  // Logo link target; empty means the status page itself.
  homeUrl: string
  footer: string
  footerLinks: LinkView[]
  hidePoweredBy: boolean
}

export const RANGES = ["24h", "7d", "30d", "90d", "1y"] as const
export type Range = (typeof RANGES)[number]

// Where nothing else picks one, e.g. an endpoint's own page.
export const DEFAULT_RANGE: Range = "90d"

// Undefined when the URL doesn't pick a range, so a dashboard's own default applies.
export function parseRange(v: string | string[] | undefined): Range | undefined {
  return RANGES.includes(v as Range) ? (v as Range) : undefined
}
