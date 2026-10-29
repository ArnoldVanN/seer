import { ExternalLink } from "lucide-react"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import type { EndpointView, Range, ResultView } from "@/lib/types"
import { formatUptime } from "./format"
import { HistoryBars } from "./HistoryBars"
import { RangePicker } from "./RangePicker"
import { RecentChecks } from "./RecentChecks"
import { ResponseTimeChart } from "./ResponseTimeChart"

export function EndpointDetail({
  endpoint: e,
  recent,
  range,
  backHref,
  pagePath,
}: {
  endpoint: EndpointView
  recent: ResultView[]
  range: Range
  backHref: string
  pagePath: string
}) {
  const checks = e.history.reduce((n, b) => n + b.checks, 0)
  // Response times cover successful checks only.
  const successes = e.history.reduce((n, b) => n + b.successes, 0)
  const avg = successes ? e.history.reduce((n, b) => n + b.avgMs * b.successes, 0) / successes : 0
  const max = Math.max(0, ...e.history.map((b) => b.maxMs))

  const stats: { label: string; value: string; status?: string }[] = [
    { label: "Status", value: e.status, status: e.status },
    { label: "Uptime", value: formatUptime(e.uptime, checks) },
    { label: "Avg response", value: successes ? `${Math.round(avg)} ms` : "—" },
    { label: "Slowest", value: successes ? `${max} ms` : "—" },
  ]

  return (
    <article className="seer-endpoint-detail space-y-6" data-status={e.status} data-endpoint={e.key}>
      <div>
        <Link href={backHref} className="seer-back text-sm text-muted-foreground hover:text-foreground">
          ← Back
        </Link>
        <div className="seer-toolbar mt-3 flex flex-wrap items-center justify-between gap-3">
          <h1 className="seer-dashboard-title flex items-center gap-2.5 text-2xl font-semibold">
            <span className="seer-status-dot" data-status={e.status} /> {e.name}
          </h1>
          <RangePicker current={range} base={pagePath} />
        </div>
        {e.description && <p className="seer-endpoint-description mt-2 text-sm text-muted-foreground">{e.description}</p>}
        {e.links.length > 0 && (
          <nav className="seer-endpoint-links mt-3 flex flex-wrap gap-2" aria-label="Links">
            {e.links.map((l) => (
              <Button key={l.url} variant="outline" size="sm" asChild>
                <a href={l.url}>
                  {l.name}
                  <ExternalLink />
                </a>
              </Button>
            ))}
          </nav>
        )}
      </div>

      <dl className="seer-stats grid grid-cols-2 gap-3 sm:grid-cols-4">
        {stats.map((s) => (
          <Card key={s.label} className="gap-1 px-4 py-3">
            <dt className="text-xs text-muted-foreground">{s.label}</dt>
            <dd
              data-status={s.status}
              className="text-xl font-semibold tabular-nums data-[status]:capitalize data-[status=down]:text-down data-[status=up]:text-up"
            >
              {s.value}
            </dd>
          </Card>
        ))}
      </dl>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Availability</CardTitle>
        </CardHeader>
        <CardContent>
          <HistoryBars history={e.history} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Response time</CardTitle>
        </CardHeader>
        <CardContent>
          <ResponseTimeChart history={e.history} />
        </CardContent>
      </Card>

      <RecentChecks results={recent} />
    </article>
  )
}
