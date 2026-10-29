import Link from "next/link"
import { Button } from "@/components/ui/button"
import type { DashboardSummary } from "@/lib/types"

export function DashboardTabs({
  dashboards,
  current,
  linkBase,
}: {
  dashboards: DashboardSummary[]
  current: string
  linkBase: string
}) {
  if (dashboards.length < 2) return null
  return (
    <nav className="seer-tabs mb-4 flex gap-1 overflow-x-auto" aria-label="Dashboards">
      {dashboards.map((d) => {
        const active = d.key === current
        return (
          <Button key={d.key} asChild variant={active ? "outline" : "ghost"} size="sm" className="rounded-full">
            <Link
              href={`${linkBase}/d/${d.key}`}
              className="seer-tab"
              data-visibility={d.visibility}
              aria-current={active ? "page" : undefined}
            >
              {d.title}
              {d.visibility === "private" && <span className="text-xs text-muted-foreground">internal</span>}
            </Link>
          </Button>
        )
      })}
    </nav>
  )
}
