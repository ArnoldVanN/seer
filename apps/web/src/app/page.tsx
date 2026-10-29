import { connection } from "next/server"
import { Suspense } from "react"
import { DashboardPage, DashboardSkeleton, EmptyState } from "@/components/DashboardPage"
import { getPublicDashboard, listPublicDashboards } from "@/lib/data"
import { parseRange } from "@/lib/types"

export default function Home(props: PageProps<"/">) {
  return (
    <Suspense fallback={<DashboardSkeleton />}>
      <PublicHome {...props} />
    </Suspense>
  )
}

async function PublicHome({ searchParams }: PageProps<"/">) {
  await connection()
  const range = parseRange((await searchParams).range)
  const dashboards = await listPublicDashboards()
  if (dashboards.length === 0) {
    return <EmptyState>No public dashboards are configured.</EmptyState>
  }
  const dashboard = await getPublicDashboard(dashboards[0].key, range)
  return (
    <DashboardPage dashboard={dashboard} dashboards={dashboards} range={range} scope="public" linkBase="" pagePath="/" />
  )
}
