import { timestampDate, type Timestamp } from "@bufbuild/protobuf/wkt"
import {
  HistoryRange,
  Status,
  Visibility,
  type Dashboard,
  type DashboardSummary as PbSummary,
  type Endpoint,
  type Link,
  type Result,
} from "@seer/protos/common"
import {
  DEFAULT_RANGE,
  type DashboardSummary,
  type DashboardView,
  type EndpointView,
  type LinkView,
  type Range,
  type ResultView,
  type Status as StatusView,
} from "./types"

const HISTORY_RANGES: Record<Range, HistoryRange> = {
  "24h": HistoryRange.HISTORY_RANGE_24H,
  "7d": HistoryRange.HISTORY_RANGE_7D,
  "30d": HistoryRange.HISTORY_RANGE_30D,
  "90d": HistoryRange.HISTORY_RANGE_90D,
  "1y": HistoryRange.HISTORY_RANGE_1Y,
}

// Undefined leaves it to the server: the dashboard's default range.
export function toRange(r: Range | undefined): HistoryRange {
  return r ? HISTORY_RANGES[r] : HistoryRange.HISTORY_RANGE_UNSPECIFIED
}

function fromRange(r: HistoryRange): Range {
  return (Object.keys(HISTORY_RANGES) as Range[]).find((k) => HISTORY_RANGES[k] === r) ?? DEFAULT_RANGE
}

export const toLinks = (ls: Link[]): LinkView[] => ls.map((l) => ({ name: l.name, url: l.url }))

export function toStatus(s: Status): StatusView {
  if (s === Status.UP) return "up"
  if (s === Status.DOWN) return "down"
  return "unknown"
}

const iso = (t?: Timestamp) => (t ? timestampDate(t).toISOString() : null)

export function toEndpoint(e: Endpoint): EndpointView {
  return {
    key: e.key,
    name: e.name,
    group: e.group,
    description: e.description,
    links: toLinks(e.links),
    status: toStatus(e.status),
    lastCheckedAt: iso(e.lastCheckedAt),
    lastResponseTimeMs: Number(e.lastResponseTimeMs),
    uptime: e.uptime,
    history: e.history.map((b) => ({
      start: iso(b.start) ?? "",
      checks: Number(b.checks),
      successes: Number(b.successes),
      avgMs: b.avgResponseTimeMs,
      minMs: Number(b.minResponseTimeMs),
      maxMs: Number(b.maxResponseTimeMs),
    })),
  }
}

const vis = (v: Visibility) => (v === Visibility.PRIVATE ? "private" : "public")

export function toDashboard(d: Dashboard): DashboardView {
  return {
    key: d.key,
    title: d.title,
    visibility: vis(d.visibility),
    description: d.description,
    range: fromRange(d.historyRange),
    groups: d.groups.map((g) => ({ name: g.name, endpoints: g.endpoints.map(toEndpoint) })),
    customCss: d.customCss,
  }
}

export function toSummary(d: PbSummary): DashboardSummary {
  return { key: d.key, title: d.title, visibility: vis(d.visibility) }
}

export function toResult(r: Result): ResultView {
  return {
    checkedAt: iso(r.checkedAt) ?? "",
    success: r.success,
    responseTimeMs: Number(r.responseTimeMs),
    error: r.error,
  }
}
