import type { NextRequest } from "next/server"
import { env } from "@/lib/env"
import { passthrough } from "@/lib/passthrough"
import { SESSION_COOKIE, sessionId } from "@/lib/session"
import { mintInternalToken } from "@/lib/token"
import { sameOrigin } from "@/lib/origin"

// Browsers can't reach the private port, so private live updates come through
// here: check the session cookie, attach an internal token, stream the result.
// Only WatchStatus is exposed; private pages load everything else server-side.
const ALLOWED = new Set(["seer.private.v1.PrivateStatusService/WatchStatus"])

async function handle(req: NextRequest, ctx: RouteContext<"/private-rpc/[...path]">) {
  const { path } = await ctx.params
  const method = path.join("/")
  if (!ALLOWED.has(method)) return new Response("Not found", { status: 404 })

  if (!sameOrigin(req)) return new Response("Forbidden", { status: 403 })

  // The API checks the session itself, and ends the stream once it's revoked.
  const sid = await sessionId(req.cookies.get(SESSION_COOKIE)?.value)
  if (!sid) return new Response("Unauthenticated", { status: 401 })

  return passthrough(req, `${env.privateApiUrl()}/${method}`, {
    authorization: `Bearer ${await mintInternalToken(sid)}`,
  })
}

export const GET = handle
export const POST = handle
