"use client"

import { Area, CartesianGrid, ComposedChart, Line, ReferenceArea, ReferenceDot, XAxis, YAxis } from "recharts"
import { ChartContainer, ChartTooltip, type ChartConfig } from "@/components/ui/chart"
import type { Bucket } from "@/lib/types"
import { bucketMs, formatBucket, formatUptime } from "./format"

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

// Runs of consecutive buckets where every check failed, as [from, to] times
// centered on the buckets like the line's points, clamped to the x domain.
function outages(data: Point[], stride: number): [number, number][] {
  const out: [number, number][] = []
  if (data.length === 0) return out
  const lo = data[0].t
  const hi = data[data.length - 1].t
  for (const p of data) {
    if (p.checks === 0 || p.successes > 0) continue
    const from = Math.max(lo, p.t - stride / 2)
    const to = Math.min(hi, p.t + stride / 2)
    const last = out[out.length - 1]
    if (last && last[1] === from) last[1] = to
    else out.push([from, to])
  }
  return out
}

// Average response time per bucket, with a band from the fastest to the
// slowest check and a red dot on buckets that had failures. Buckets where
// every check failed have no response time, so they get a red column instead.
export function ResponseTimeChart({ history }: { history: Bucket[] }) {
  const stride = bucketMs(history)
  // Ticks show dates once the chart spans more than a couple of days.
  const daily = stride * history.length > 2 * 86_400_000
  const data: Point[] = history.map((b) => ({
    t: Date.parse(b.start),
    avg: b.successes ? Math.round(b.avgMs) : null,
    range: b.successes ? [b.minMs, b.maxMs] : null,
    checks: b.checks,
    successes: b.successes,
    failed: b.checks > 0 && b.successes < b.checks,
  }))
  const hasTimes = data.some((p) => p.avg !== null)
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
        {/* With no successful checks there are no values to scale to, and
            recharts then drops the axis and everything drawn against it. A
            fixed domain (allowDataOverflow makes recharts keep it) gives the
            outage columns a scale; the axis itself is hidden. */}
        <YAxis
          width={56}
          domain={hasTimes ? [0, "auto"] : [0, 1]}
          hide={!hasTimes}
          allowDataOverflow={!hasTimes}
          tickLine={false}
          axisLine={false}
          tickFormatter={(v) => `${v}ms`}
        />
        <ChartTooltip
          cursor={{ strokeDasharray: "3 3" }}
          content={({ active, payload }) => {
            const p = active ? (payload?.[0]?.payload as Point | undefined) : undefined
            if (!p) return null
            return (
              <div className="grid min-w-36 gap-1 rounded-lg border bg-background px-2.5 py-1.5 text-xs shadow-xl">
                <div className="font-medium">{formatBucket(new Date(p.t).toISOString(), stride)}</div>
                {p.checks === 0 ? (
                  <div className="text-muted-foreground">No data</div>
                ) : (
                  <>
                    {p.avg !== null && (
                      <>
                        <Row label="Average" value={`${p.avg} ms`} />
                        <Row label="Min – max" value={`${p.range?.[0]} – ${p.range?.[1]} ms`} />
                      </>
                    )}
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
        {outages(data, stride).map(([x1, x2]) => (
          <ReferenceArea
            key={x1}
            x1={x1}
            x2={x2}
            fill="var(--seer-down)"
            fillOpacity={0.15}
            stroke="none"
            className="seer-rt-outage"
          />
        ))}
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
