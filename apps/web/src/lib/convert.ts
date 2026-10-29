import { timestampDate, type Timestamp } from "@bufbuild/protobuf/wkt"
import {
  HistoryRange,
  Status,
  Visibility,
  type Dashboard,
  type DashboardSummary as PbSummary,
  type Endpoint,
  type Result,
} from "@seer/protos/common"
import type {
  DashboardSummary,
  DashboardView,
  EndpointView,
  Range,
  ResultView,
  Status as StatusView,
} from "./types"

export function toRange(r: Range): HistoryRange {
  return {
    "24h": HistoryRange.HISTORY_RANGE_24H,
    "7d": HistoryRange.HISTORY_RANGE_7D,
    "30d": HistoryRange.HISTORY_RANGE_30D,
    "90d": HistoryRange.HISTORY_RANGE_90D,
    "1y": HistoryRange.HISTORY_RANGE_1Y,
  }[r]
}

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
