import { connection } from "next/server"
import { Suspense } from "react"
import { DashboardPage, DashboardSkeleton } from "@/components/DashboardPage"
import { getPublicDashboard, listPublicDashboards } from "@/lib/data"
import { parseRange } from "@/lib/types"

export default function Page(props: PageProps<"/d/[key]">) {
  return (
    <Suspense fallback={<DashboardSkeleton />}>
      <PublicDashboard {...props} />
    </Suspense>
  )
}

async function PublicDashboard({ params, searchParams }: PageProps<"/d/[key]">) {
  await connection()
  const { key } = await params
  const range = parseRange((await searchParams).range)
  const [dashboards, dashboard] = await Promise.all([listPublicDashboards(), getPublicDashboard(key, range)])
  return (
    <DashboardPage
      dashboard={dashboard}
      dashboards={dashboards}
      pickedRange={range}
      scope="public"
      linkBase=""
      pagePath={`/d/${key}`}
    />
  )
}
