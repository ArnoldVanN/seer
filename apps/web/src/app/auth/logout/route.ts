import { NextResponse, type NextRequest } from "next/server"
import { requestOrigin, sameOrigin } from "@/lib/origin"
import { revokeSession, SESSION_COOKIE } from "@/lib/session"

export async function POST(req: NextRequest) {
  // Otherwise any site could sign viewers out.
  if (!sameOrigin(req)) return new Response("Forbidden", { status: 403 })
  try {
    await revokeSession(req.cookies.get(SESSION_COOKIE)?.value)
  } catch (err) {
    // Still clear the cookie; the session ends on its own when it expires.
    console.error("revoking session failed", err)
  }
  const res = NextResponse.redirect(new URL("/", requestOrigin(req)), 303)
  res.cookies.delete(SESSION_COOKIE)
  return res
}
