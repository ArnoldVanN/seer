import type { NextRequest } from "next/server"
import { env } from "@/lib/env"
import { passthrough } from "@/lib/passthrough"

// Push endpoint pings. Like /rpc, the HTTPRoute sends these straight to the
// Go public port when directRpc is on; this covers every other setup.
async function handle(req: NextRequest, ctx: RouteContext<"/push/[key]">) {
  const { key } = await ctx.params
  // Only this route forwards Authorization: it carries the push token.
  const auth = req.headers.get("authorization")
  return passthrough(
    req,
    `${env.publicApiUrl()}/push/${encodeURIComponent(key)}`,
    auth ? { authorization: auth } : {},
  )
}

export const GET = handle
export const POST = handle
