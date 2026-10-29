import { Suspense } from "react"
import { DashboardSkeleton } from "@/components/DashboardPage"
import { EndpointDetail } from "@/components/EndpointDetail"
import { getPrivateEndpoint } from "@/lib/data"
import { privateBase } from "@/lib/routing"
import { requireViewer } from "@/lib/session"
import { DEFAULT_RANGE, parseRange } from "@/lib/types"

export default function Page(props: PageProps<"/private-view/e/[key]">) {
  return (
    <Suspense fallback={<DashboardSkeleton />}>
      <PrivateEndpoint {...props} />
    </Suspense>
  )
}

async function PrivateEndpoint({ params, searchParams }: PageProps<"/private-view/e/[key]">) {
  const { key } = await params
  const base = privateBase()
  const viewer = await requireViewer(`${base}/e/${key}`)
  const range = parseRange((await searchParams).range) ?? DEFAULT_RANGE
  const { endpoint, recent } = await getPrivateEndpoint(viewer, key, range)
  return (
    <>
      <EndpointDetail
        endpoint={endpoint}
        recent={recent}
        range={range}
        backHref={base}
        pagePath={`${base}/e/${key}`}
      />
    </>
  )
}
