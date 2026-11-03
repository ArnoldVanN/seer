import { NextResponse, type NextRequest } from "next/server"
import { requestOrigin, sameOrigin } from "@/lib/origin"
import { SESSION_COOKIE } from "@/lib/session"

export async function POST(req: NextRequest) {
  // Otherwise any site could sign viewers out.
  if (!sameOrigin(req)) return new Response("Forbidden", { status: 403 })
  const res = NextResponse.redirect(new URL("/", requestOrigin(req)), 303)
  res.cookies.delete(SESSION_COOKIE)
  return res
}
