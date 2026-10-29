import { Suspense } from "react"
import { DashboardPage, DashboardSkeleton, EmptyState } from "@/components/DashboardPage"
import { getPrivateDashboard, listPrivateDashboards } from "@/lib/data"
import { privateBase } from "@/lib/routing"
import { requireViewer } from "@/lib/session"
import { parseRange } from "@/lib/types"

// Reached only through the proxy.ts rewrite from the private prefix.
export default function Page(props: PageProps<"/private-view">) {
  return (
    <Suspense fallback={<DashboardSkeleton />}>
      <PrivateHome {...props} />
    </Suspense>
  )
}

async function PrivateHome({ searchParams }: PageProps<"/private-view">) {
  const base = privateBase()
  const viewer = await requireViewer(base)
  const range = parseRange((await searchParams).range)
  const dashboards = await listPrivateDashboards(viewer)
  // Prefer a private dashboard; the public ones are on the public page.
  const first = dashboards.find((d) => d.visibility === "private") ?? dashboards[0]
  if (!first) return <EmptyState>You don&apos;t have access to any dashboards.</EmptyState>
  const dashboard = await getPrivateDashboard(viewer, first.key, range)
  return (
    <>
      <DashboardPage
        dashboard={dashboard}
        dashboards={dashboards}
        pickedRange={range}
        scope="private"
        linkBase={base}
        pagePath={base}
      />
    </>
  )
}
