import { connection } from "next/server"
import { Suspense } from "react"
import { DashboardSkeleton } from "@/components/DashboardPage"
import { EndpointDetail } from "@/components/EndpointDetail"
import { getPublicEndpoint } from "@/lib/data"
import { DEFAULT_RANGE, parseRange } from "@/lib/types"

export default function Page(props: PageProps<"/e/[key]">) {
  return (
    <Suspense fallback={<DashboardSkeleton />}>
      <PublicEndpoint {...props} />
    </Suspense>
  )
}

async function PublicEndpoint({ params, searchParams }: PageProps<"/e/[key]">) {
  await connection()
  const { key } = await params
  const range = parseRange((await searchParams).range) ?? DEFAULT_RANGE
  const { endpoint, recent } = await getPublicEndpoint(key, range)
  return <EndpointDetail endpoint={endpoint} recent={recent} range={range} backHref="/" pagePath={`/e/${key}`} />
}
