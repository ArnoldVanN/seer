import type { NextRequest } from "next/server"
import { env } from "@/lib/env"
import { passthrough } from "@/lib/passthrough"
import { openSession, SESSION_COOKIE } from "@/lib/session"
import { mintInternalToken } from "@/lib/token"
import { requestOrigin } from "@/lib/origin"

// Browsers can't reach the private port, so private live updates come through
// here: check the session cookie, attach an internal token, stream the result.
// Only WatchStatus is exposed; private pages load everything else server-side.
const ALLOWED = new Set(["seer.private.v1.PrivateStatusService/WatchStatus"])

async function handle(req: NextRequest, ctx: RouteContext<"/private-rpc/[...path]">) {
  const { path } = await ctx.params
  const method = path.join("/")
  if (!ALLOWED.has(method)) return new Response("Not found", { status: 404 })

  // Cookies ride along on cross-site requests; only accept our own origin.
  const origin = req.headers.get("origin")
  if (origin && origin !== requestOrigin(req)) return new Response("Forbidden", { status: 403 })

  const viewer = await openSession(req.cookies.get(SESSION_COOKIE)?.value)
  if (!viewer) return new Response("Unauthenticated", { status: 401 })

  return passthrough(req, `${env.privateApiUrl()}/${method}`, {
    authorization: `Bearer ${await mintInternalToken(viewer)}`,
  })
}

export const GET = handle
export const POST = handle
