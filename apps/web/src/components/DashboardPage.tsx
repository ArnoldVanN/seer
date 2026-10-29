import { Skeleton } from "@/components/ui/skeleton"
import type { DashboardSummary, DashboardView, Range } from "@/lib/types"
import { DashboardTabs } from "./DashboardTabs"
import { LiveDashboard } from "./LiveDashboard"
import { RangePicker } from "./RangePicker"

export function DashboardPage({
  dashboard,
  dashboards,
  range,
  scope,
  linkBase,
  pagePath,
}: {
  dashboard: DashboardView
  dashboards: DashboardSummary[]
  range: Range
  scope: "public" | "private"
  linkBase: string
  pagePath: string
}) {
  return (
    <>
      {/* React hoists this into <head>; the precedence puts it after the
          site's /custom.css. */}
      {dashboard.customCss && (
        <style href={`seer-dashboard-${dashboard.key}`} precedence="dashboard">
          {dashboard.customCss}
        </style>
      )}
      <DashboardTabs dashboards={dashboards} current={dashboard.key} linkBase={linkBase} />
      <div className="seer-toolbar mb-4 flex flex-wrap items-center justify-between gap-3">
        <h1 className="seer-dashboard-title text-2xl font-semibold">{dashboard.title}</h1>
        <RangePicker current={range} base={pagePath} />
      </div>
      {/* Keyed so switching dashboards resets search, filters and live state. */}
      <LiveDashboard key={dashboard.key} initial={dashboard} scope={scope} linkBase={linkBase} range={range} />
    </>
  )
}

export function DashboardSkeleton() {
  return (
    <div className="seer-dashboard seer-skeleton space-y-6" aria-busy="true">
      <Skeleton className="h-8 w-48" />
      <Skeleton className="h-14 w-full rounded-xl" />
      <Skeleton className="h-40 w-full rounded-xl" />
    </div>
  )
}

export function EmptyState({ title, children }: { title?: string; children: React.ReactNode }) {
  return (
    <div className="seer-empty py-12 text-center text-muted-foreground">
      {title && <h1 className="mb-2 text-2xl font-semibold text-foreground">{title}</h1>}
      <p>{children}</p>
    </div>
  )
}
