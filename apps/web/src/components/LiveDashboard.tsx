"use client"

import { createClient } from "@connectrpc/connect"
import { createConnectTransport } from "@connectrpc/connect-web"
import { timestampDate } from "@bufbuild/protobuf/wkt"
import { StatusService } from "@seer/protos/public"
import { PrivateStatusService } from "@seer/protos/private"
import { ChevronRight, Search, X } from "lucide-react"
import Link from "next/link"
import { useRouter, useSearchParams } from "next/navigation"
import { useEffect, useMemo, useState, useSyncExternalStore } from "react"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { toStatus } from "@/lib/convert"
import { cn } from "@/lib/utils"
import type { DashboardView, EndpointView, Status } from "@/lib/types"
import { formatAgo, formatUptime } from "./format"
import { HistoryBars } from "./HistoryBars"

type Props = {
  initial: DashboardView
  scope: "public" | "private"
  linkBase: string
  range: string
}

// A clock for "12s ago" labels, ticking every 5s. The server snapshot is null
// so server HTML and hydration agree, and the times appear right after.
const CLOCK_MS = 5000
function subscribeClock(onTick: () => void) {
  const t = setInterval(onTick, CLOCK_MS)
  return () => clearInterval(t)
}
// Rounded so repeated reads within one tick return the same value.
const clockSnapshot = () => Math.floor(Date.now() / CLOCK_MS) * CLOCK_MS
const serverClockSnapshot = () => null

// Search and status filters only appear once a dashboard has this many
// endpoints; below that they're clutter.
const TOOLBAR_MIN_ENDPOINTS = 4

type Filter = "all" | "down" | "unstable"
const FILTERS: Filter[] = ["all", "down", "unstable"]

// The search and filter live in ?q= and ?status= so a filtered view can be
// shared. replaceState keeps typing out of the back-button history, and Next
// syncs it into useSearchParams.
function setUrlParam(name: string, value: string) {
  const params = new URLSearchParams(window.location.search)
  if (value) params.set(name, value)
  else params.delete(name)
  const qs = params.toString()
  window.history.replaceState(null, "", qs ? `?${qs}` : window.location.pathname)
}

// "unstable": currently up, but with failed checks in the shown range.
function matchesFilter(e: EndpointView, f: Filter): boolean {
  if (f === "down") return e.status === "down"
  if (f === "unstable") return e.status !== "down" && e.history.some((b) => b.successes < b.checks)
  return true
}

function matchesQuery(e: EndpointView, group: string, q: string): boolean {
  return !q || [e.name, e.key, group].some((s) => s.toLowerCase().includes(q))
}

// Collapsed groups are remembered per dashboard in localStorage. Read through
// useSyncExternalStore so the server render (everything expanded) hydrates
// cleanly; the raw string is the snapshot so it's stable between reads.
const COLLAPSE_EVENT = "seer:collapse"
function subscribeCollapsed(onChange: () => void) {
  window.addEventListener("storage", onChange)
  window.addEventListener(COLLAPSE_EVENT, onChange)
  return () => {
    window.removeEventListener("storage", onChange)
    window.removeEventListener(COLLAPSE_EVENT, onChange)
  }
}
function readCollapsed(storageKey: string): string {
  try {
    return localStorage.getItem(storageKey) ?? "[]"
  } catch {
    return "[]"
  }
}
function writeCollapsed(storageKey: string, groups: string[]) {
  try {
    localStorage.setItem(storageKey, JSON.stringify(groups))
  } catch {}
  window.dispatchEvent(new Event(COLLAPSE_EVENT))
}
function useCollapsedGroups(dashboardKey: string) {
  const storageKey = `seer:collapsed:${dashboardKey}`
  const raw = useSyncExternalStore(
    subscribeCollapsed,
    () => readCollapsed(storageKey),
    () => "[]",
  )
  const collapsed = useMemo(() => {
    try {
      const v: unknown = JSON.parse(raw)
      return new Set(Array.isArray(v) ? v.filter((g): g is string => typeof g === "string") : [])
    } catch {
      return new Set<string>()
    }
  }, [raw])
  const toggle = (group: string) => {
    const next = new Set(collapsed)
    if (!next.delete(group)) next.add(group)
    writeCollapsed(storageKey, [...next])
  }
  return [collapsed, toggle] as const
}

// Renders a dashboard and keeps it live over the WatchStatus stream. On
// reconnect it refreshes the server-rendered snapshot to catch anything missed.
export function LiveDashboard({ initial, scope, linkBase, range }: Props) {
  const [dash, setDash] = useState(initial)
  const [snapshot, setSnapshot] = useState(initial)
  const [live, setLive] = useState(false)
  const now = useSyncExternalStore(subscribeClock, clockSnapshot, serverClockSnapshot)
  const searchParams = useSearchParams()
  const [query, setQueryState] = useState(() => searchParams.get("q") ?? "")
  const [filter, setFilterState] = useState<Filter>(() => {
    const v = searchParams.get("status") as Filter
    return FILTERS.includes(v) ? v : "all"
  })
  const setQuery = (v: string) => {
    setQueryState(v)
    setUrlParam("q", v)
  }
  const setFilter = (v: Filter) => {
    setFilterState(v)
    setUrlParam("status", v === "all" ? "" : v)
  }
  const [collapsed, toggleGroup] = useCollapsedGroups(initial.key)
  const router = useRouter()

  // A new server snapshot (range change, router.refresh) replaces live state.
  if (snapshot !== initial) {
    setSnapshot(initial)
    setDash(initial)
  }

  useEffect(() => {
    const abort = new AbortController()
    const transport = createConnectTransport({
      baseUrl: new URL(scope === "public" ? "/rpc" : "/private-rpc", window.location.origin).toString(),
    })
    const client =
      scope === "public" ? createClient(StatusService, transport) : createClient(PrivateStatusService, transport)

    async function run() {
      let backoff = 1000
      let first = true
      while (!abort.signal.aborted) {
        try {
          const stream = client.watchStatus({ dashboardKey: initial.key }, { signal: abort.signal })
          for await (const ev of stream) {
            setLive(true)
            backoff = 1000
            if (!first) {
              first = true
              router.refresh()
            }
            if (ev.event.case !== "endpointUpdate") continue
            const u = ev.event.value
            setDash((d) =>
              applyUpdate(d, u.key, {
                status: toStatus(u.status),
                success: u.success,
                checkedAt: u.checkedAt ? timestampDate(u.checkedAt).toISOString() : null,
                responseTimeMs: Number(u.responseTimeMs),
              }),
            )
          }
        } catch {
          if (abort.signal.aborted) return
        }
        setLive(false)
        first = false
        await new Promise((r) => setTimeout(r, backoff))
        backoff = Math.min(backoff * 2, 30_000)
      }
    }
    run()
    return () => abort.abort()
  }, [initial.key, scope, router])

  const all = dash.groups.flatMap((g) => g.endpoints)
  // Without the toolbar there's no way to see or undo a filter from the URL,
  // so ignore it.
  const toolbar = all.length >= TOOLBAR_MIN_ENDPOINTS
  const q = toolbar ? query.trim().toLowerCase() : ""
  const f = toolbar ? filter : "all"
  const filtering = q !== "" || f !== "all"
  const groups = dash.groups
    .map((g) => ({ ...g, shown: g.endpoints.filter((e) => matchesFilter(e, f) && matchesQuery(e, g.name, q)) }))
    .filter((g) => g.shown.length > 0)
  const counts = {
    down: all.filter((e) => matchesFilter(e, "down")).length,
    unstable: all.filter((e) => matchesFilter(e, "unstable")).length,
  }

  return (
    <div className="seer-dashboard" data-dashboard={dash.key}>
      <OverallStatus endpoints={all} live={live} />
      {toolbar && (
        <div className="seer-filters mb-6 flex flex-col gap-2 sm:flex-row sm:items-center">
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="search"
              value={query}
              onChange={(ev) => setQuery(ev.target.value)}
              placeholder="Search endpoints"
              aria-label="Search endpoints"
              // Hides the browser's own clear button, which ignores the theme.
              className="seer-search pr-8 pl-8 [&::-webkit-search-cancel-button]:appearance-none"
            />
            {query && (
              <button
                type="button"
                onClick={() => setQuery("")}
                aria-label="Clear search"
                className="seer-search-clear absolute top-1/2 right-1.5 flex size-6 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
              >
                <X className="size-3.5" />
              </button>
            )}
          </div>
          <ToggleGroup
            type="single"
            size="sm"
            variant="outline"
            value={filter}
            onValueChange={(v) => v && setFilter(v as Filter)}
            className="seer-status-filter"
          >
            <ToggleGroupItem value="all">All</ToggleGroupItem>
            <ToggleGroupItem value="down">Down{counts.down > 0 && ` (${counts.down})`}</ToggleGroupItem>
            <ToggleGroupItem value="unstable" title="Up now, but with failed checks in this range">
              Unstable{counts.unstable > 0 && ` (${counts.unstable})`}
            </ToggleGroupItem>
          </ToggleGroup>
        </div>
      )}
      {groups.map((g) => {
        // Filtering shows every match, even inside collapsed groups.
        const open = !g.name || filtering || !collapsed.has(g.name)
        return (
          <section key={g.name} className="seer-group mb-6" data-group={g.name} data-collapsed={!open}>
            {g.name && (
              <GroupHeader
                name={g.name}
                endpoints={g.endpoints}
                open={open}
                disabled={filtering}
                onToggle={() => toggleGroup(g.name)}
              />
            )}
            {open && (
              <Card className="py-0">
                <ul className="seer-endpoints divide-y">
                  {g.shown.map((e) => (
                    <EndpointRow key={e.key} e={e} now={now} href={`${linkBase}/e/${e.key}?range=${range}`} />
                  ))}
                </ul>
              </Card>
            )}
          </section>
        )
      })}
      {all.length === 0 && <p className="seer-empty py-12 text-center text-muted-foreground">No endpoints on this dashboard yet.</p>}
      {all.length > 0 && groups.length === 0 && (
        <div className="seer-empty flex flex-col items-center gap-3 py-12 text-muted-foreground">
          No endpoints match.
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setQuery("")
              setFilter("all")
            }}
          >
            Clear filters
          </Button>
        </div>
      )}
    </div>
  )
}

// Group title that collapses the group. While collapsed it still shows how
// many endpoints are up, so a hidden outage isn't missed.
function GroupHeader({
  name,
  endpoints,
  open,
  disabled,
  onToggle,
}: {
  name: string
  endpoints: EndpointView[]
  open: boolean
  disabled: boolean
  onToggle: () => void
}) {
  const down = endpoints.filter((e) => e.status === "down").length
  const up = endpoints.filter((e) => e.status === "up").length
  const status: Status = down > 0 ? "down" : up === endpoints.length ? "up" : "unknown"
  return (
    <button
      type="button"
      onClick={onToggle}
      disabled={disabled}
      aria-expanded={open}
      className="seer-group-title mb-2 flex w-full items-center gap-1.5 text-left text-xs font-medium tracking-wider text-muted-foreground uppercase hover:text-foreground disabled:hover:text-muted-foreground"
    >
      <ChevronRight className={cn("size-3.5 transition-transform", open && "rotate-90")} />
      <span>{name}</span>
      {!open && (
        <span className="ml-auto flex items-center gap-2 tracking-normal normal-case">
          <span className="seer-status-dot" data-status={status} />
          {up}/{endpoints.length} up
        </span>
      )}
    </button>
  )
}

function applyUpdate(
  d: DashboardView,
  key: string,
  u: { status: Status; success: boolean; checkedAt: string | null; responseTimeMs: number },
): DashboardView {
  return {
    ...d,
    groups: d.groups.map((g) => ({
      ...g,
      endpoints: g.endpoints.map((e) => {
        if (e.key !== key) return e
        // Count the check into the newest bar so it moves without a reload.
        const history = e.history.slice()
        const last = history[history.length - 1]
        if (last) {
          history[history.length - 1] = {
            ...last,
            checks: last.checks + 1,
            successes: last.successes + (u.success ? 1 : 0),
            avgMs: (last.avgMs * last.checks + u.responseTimeMs) / (last.checks + 1),
            minMs: last.checks ? Math.min(last.minMs, u.responseTimeMs) : u.responseTimeMs,
            maxMs: Math.max(last.maxMs, u.responseTimeMs),
          }
        }
        return { ...e, status: u.status, lastCheckedAt: u.checkedAt, lastResponseTimeMs: u.responseTimeMs, history }
      }),
    })),
  }
}

function OverallStatus({ endpoints, live }: { endpoints: EndpointView[]; live: boolean }) {
  const down = endpoints.filter((e) => e.status === "down").length
  let state: "operational" | "degraded" | "outage" = "operational"
  let text = "All systems operational"
  if (down > 0 && (down === endpoints.length || down > endpoints.length / 2)) {
    state = "outage"
    text = "Major outage"
  } else if (down > 0) {
    state = "degraded"
    text = `${down} ${down === 1 ? "service" : "services"} down`
  }
  return (
    <div className="seer-overall mb-6 flex items-center justify-between rounded-xl px-5 py-4 font-semibold" data-state={state}>
      <span className="seer-overall-text">{text}</span>
      <span
        className="seer-live flex items-center gap-1.5 text-xs font-medium opacity-90"
        data-live={live}
        title={live ? "Live updates connected" : "Reconnecting…"}
      >
        <span className={cn("size-2 rounded-full bg-current", live ? "animate-pulse" : "opacity-50")} />
        {live ? "Live" : "Offline"}
      </span>
    </div>
  )
}

function EndpointRow({ e, now, href }: { e: EndpointView; now: number | null; href: string }) {
  const checks = e.history.reduce((n, b) => n + b.checks, 0)
  return (
    <li className="seer-endpoint px-4 py-3.5" data-status={e.status} data-endpoint={e.key}>
      <div className="seer-endpoint-head mb-2 flex items-center gap-2.5">
        <span className="seer-status-dot" data-status={e.status} aria-label={e.status} />
        <Link href={href} className="seer-endpoint-name font-medium hover:underline">
          {e.name}
        </Link>
        <span className="seer-endpoint-meta ml-auto hidden text-xs text-muted-foreground sm:inline">
          {e.status !== "unknown" && <span>{e.lastResponseTimeMs} ms · </span>}
          {now !== null && <span>{formatAgo(e.lastCheckedAt, now)}</span>}
        </span>
        <span className="seer-uptime min-w-16 text-right text-sm tabular-nums max-sm:ml-auto">
          {formatUptime(e.uptime, checks)}
        </span>
      </div>
      <HistoryBars history={e.history} />
    </li>
  )
}
