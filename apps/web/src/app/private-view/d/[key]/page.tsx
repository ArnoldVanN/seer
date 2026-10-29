import { Suspense } from "react"
import { DashboardPage, DashboardSkeleton } from "@/components/DashboardPage"
import { getPrivateDashboard, listPrivateDashboards } from "@/lib/data"
import { privateBase } from "@/lib/routing"
import { requireViewer } from "@/lib/session"
import { parseRange } from "@/lib/types"

export default function Page(props: PageProps<"/private-view/d/[key]">) {
  return (
    <Suspense fallback={<DashboardSkeleton />}>
      <PrivateDashboard {...props} />
    </Suspense>
  )
}

async function PrivateDashboard({ params, searchParams }: PageProps<"/private-view/d/[key]">) {
  const { key } = await params
  const base = privateBase()
  const viewer = await requireViewer(`${base}/d/${key}`)
  const range = parseRange((await searchParams).range)
  const [dashboards, dashboard] = await Promise.all([
    listPrivateDashboards(viewer),
    getPrivateDashboard(viewer, key, range),
  ])
  return (
    <>
      <DashboardPage
        dashboard={dashboard}
        dashboards={dashboards}
        pickedRange={range}
        scope="private"
        linkBase={base}
        pagePath={`${base}/d/${key}`}
      />
    </>
  )
}
