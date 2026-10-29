import Link from "next/link"
import { Button } from "@/components/ui/button"
import type { DashboardSummary, Range } from "@/lib/types"

export function DashboardTabs({
  dashboards,
  current,
  linkBase,
  pickedRange,
}: {
  dashboards: DashboardSummary[]
  current: string
  linkBase: string
  // A range picked in the URL carries over to the other tabs; without one,
  // each dashboard opens on its own default range.
  pickedRange?: Range
}) {
  const query = pickedRange ? `?range=${pickedRange}` : ""
  if (dashboards.length < 2) return null
  return (
    // overflow-x-auto alone also makes the nav scroll vertically, so a
    // sub-pixel overflow shows a scrollbar. The padding (undone by the negative
    // margin) keeps focus rings from being clipped.
    <nav
      className="seer-tabs -m-1 mb-3 flex gap-1 overflow-x-auto overflow-y-hidden p-1"
      aria-label="Dashboards"
    >
      {dashboards.map((d) => {
        const active = d.key === current
        return (
          <Button key={d.key} asChild variant={active ? "outline" : "ghost"} size="sm" className="rounded-full">
            <Link
              href={`${linkBase}/d/${d.key}${query}`}
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
