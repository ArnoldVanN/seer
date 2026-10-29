// Plain view models handed to client components. Protobuf messages carry
// bigints and class-like metadata, so pages convert them first.

export type Status = "up" | "down" | "unknown"

export type Bucket = {
  start: string
  checks: number
  successes: number
  avgMs: number
  minMs: number
  maxMs: number
}

export type EndpointView = {
  key: string
  name: string
  group: string
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
}

export const RANGES = ["24h", "7d", "30d", "90d", "1y"] as const
export type Range = (typeof RANGES)[number]

export function parseRange(v: string | string[] | undefined): Range {
  return RANGES.includes(v as Range) ? (v as Range) : "90d"
}
