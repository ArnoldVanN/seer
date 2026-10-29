"use client"

import { Area, CartesianGrid, ComposedChart, Line, ReferenceDot, XAxis, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, type ChartConfig } from "@/components/ui/chart"
import type { Bucket } from "@/lib/types"
import { formatBucket, formatUptime } from "./format"

const config = {
  avg: { label: "Average", color: "var(--chart-1)" },
  range: { label: "Min – max", color: "var(--chart-1)" },
} satisfies ChartConfig

type Point = {
  t: number
  avg: number | null
  range: [number, number] | null
  checks: number
  successes: number
  failed: boolean
}

// Average response time per bucket, with a band from the fastest to the
// slowest check and a red dot on buckets that had failures.
export function ResponseTimeChart({ history }: { history: Bucket[] }) {
  const daily = history.length > 1 && Date.parse(history[1].start) - Date.parse(history[0].start) >= 86_400_000
  const data: Point[] = history.map((b) => ({
    t: Date.parse(b.start),
    avg: b.checks ? Math.round(b.avgMs) : null,
    range: b.checks ? [b.minMs, b.maxMs] : null,
    checks: b.checks,
    successes: b.successes,
    failed: b.checks > 0 && b.successes < b.checks,
  }))
  // About six ticks, on bucket boundaries so they read as round times.
  const every = Math.max(1, Math.ceil(data.length / 6))
  const ticks = data.filter((_, i) => (data.length - 1 - i) % every === 0).map((p) => p.t)
  const tick = (t: number) =>
    new Date(t).toLocaleString("en-US", daily
      ? { month: "short", day: "numeric", timeZone: "UTC" }
      : { hour: "2-digit", minute: "2-digit", hour12: false, timeZone: "UTC" })

  return (
    <ChartContainer config={config} className="seer-rt-chart aspect-auto h-56 w-full">
      <ComposedChart data={data} margin={{ left: 4, right: 8, top: 8 }}>
        <defs>
          <linearGradient id="seer-rt-fill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--color-range)" stopOpacity={0.25} />
            <stop offset="100%" stopColor="var(--color-range)" stopOpacity={0.05} />
          </linearGradient>
        </defs>
        <CartesianGrid vertical={false} />
        <XAxis
          dataKey="t"
          type="number"
          domain={["dataMin", "dataMax"]}
          ticks={ticks}
          tickFormatter={tick}
          tickLine={false}
          axisLine={false}
        />
        <YAxis width={56} tickLine={false} axisLine={false} tickFormatter={(v) => `${v}ms`} />
        <ChartTooltip
          cursor={{ strokeDasharray: "3 3" }}
          content={({ active, payload }) => {
            const p = active ? (payload?.[0]?.payload as Point | undefined) : undefined
            if (!p) return null
            return (
              <div className="grid min-w-36 gap-1 rounded-lg border bg-background px-2.5 py-1.5 text-xs shadow-xl">
                <div className="font-medium">{formatBucket(new Date(p.t).toISOString(), daily)}</div>
                {p.checks === 0 ? (
                  <div className="text-muted-foreground">No data</div>
                ) : (
                  <>
                    <Row label="Average" value={`${p.avg} ms`} />
                    <Row label="Min – max" value={`${p.range?.[0]} – ${p.range?.[1]} ms`} />
                    <Row
                      label="Uptime"
                      value={`${formatUptime(p.successes / p.checks, p.checks)} (${p.successes}/${p.checks})`}
                      danger={p.failed}
                    />
                  </>
                )}
              </div>
            )
          }}
        />
        <Area dataKey="range" type="monotone" stroke="none" fill="url(#seer-rt-fill)" connectNulls={false} isAnimationActive={false} />
        <Line dataKey="avg" type="monotone" stroke="var(--color-avg)" strokeWidth={2} dot={false} connectNulls={false} isAnimationActive={false} />
        {data
          .filter((p) => p.failed && p.avg !== null)
          .map((p) => (
            <ReferenceDot key={p.t} x={p.t} y={p.avg!} r={3} fill="var(--seer-down)" stroke="none" />
          ))}
      </ComposedChart>
    </ChartContainer>
  )
}

function Row({ label, value, danger }: { label: string; value: string; danger?: boolean }) {
  return (
    <div className="flex justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className={danger ? "font-medium text-down" : "font-medium tabular-nums"}>{value}</span>
    </div>
  )
}
