import type { NextRequest } from "next/server"
import { connection } from "next/server"
import { BADGE_COLORS, badgeResponse, notFoundBadge, renderBadge } from "@/lib/badge"
import { getBadgeEndpoint, requestHost } from "@/lib/data"

// The endpoint's current status. ?label= replaces the endpoint name.
export async function GET(req: NextRequest, ctx: RouteContext<"/badge/[key]">) {
  await connection()
  const { key } = await ctx.params
  const label = req.nextUrl.searchParams.get("label")
  // The shortest range: only the status is needed, not the history.
  const e = await getBadgeEndpoint(await requestHost(), key, "24h")
  if (!e) return notFoundBadge(label ?? "status")
  return badgeResponse(renderBadge(label ?? e.name, e.status, BADGE_COLORS[e.status]))
}
