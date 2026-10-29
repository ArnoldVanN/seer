import type { NextRequest } from "next/server"
import { env } from "@/lib/env"
import { passthrough } from "@/lib/passthrough"

// Browser calls to the public API. In Kubernetes the HTTPRoute sends /rpc
// straight to the Go public port, so this only handles setups without that
// route (local dev, docker compose).
async function handle(req: NextRequest, ctx: RouteContext<"/rpc/[...path]">) {
  const { path } = await ctx.params
  return passthrough(req, `${env.publicApiUrl()}/${path.join("/")}`)
}

export const GET = handle
export const POST = handle
