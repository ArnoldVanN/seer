import { NextResponse, type NextRequest } from "next/server"
import { requestOrigin } from "@/lib/origin"
import { SESSION_COOKIE } from "@/lib/session"

export async function POST(req: NextRequest) {
  const res = NextResponse.redirect(new URL("/", requestOrigin(req)), 303)
  res.cookies.delete(SESSION_COOKIE)
  return res
}
