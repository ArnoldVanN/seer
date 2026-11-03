import type { NextRequest } from "next/server"
import { connection } from "next/server"
import { formatUptime } from "@/components/format"
import { BADGE_COLORS, badgeResponse, notFoundBadge, renderBadge } from "@/lib/badge"
import { getBadgeEndpoint, requestHost } from "@/lib/data"
import { DEFAULT_RANGE, parseRange } from "@/lib/types"

// Uptime over ?range= (the endpoint page's default when unset). ?label=
// replaces "uptime <range>".
export async function GET(req: NextRequest, ctx: RouteContext<"/badge/[key]/uptime">) {
  await connection()
  const { key } = await ctx.params
  const q = req.nextUrl.searchParams
  const range = parseRange(q.get("range") ?? undefined) ?? DEFAULT_RANGE
  const label = q.get("label") ?? `uptime ${range}`
  const e = await getBadgeEndpoint(await requestHost(), key, range)
  if (!e) return notFoundBadge(label)
  const checks = e.history.reduce((n, b) => n + b.checks, 0)
  return badgeResponse(renderBadge(label, checks ? formatUptime(e.uptime, checks) : "no data", uptimeColor(e.uptime, checks)))
}

function uptimeColor(u: number, checks: number): string {
  if (checks === 0) return BADGE_COLORS.unknown
  if (u >= 0.999) return BADGE_COLORS.up
  if (u >= 0.99) return BADGE_COLORS.partial
  return BADGE_COLORS.down
}
