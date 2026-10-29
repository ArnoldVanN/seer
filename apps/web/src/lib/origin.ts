import "server-only"
import type { NextRequest } from "next/server"

// The origin the browser used, which differs from req.url behind a gateway.
export function requestOrigin(req: NextRequest): string {
  const proto = req.headers.get("x-forwarded-proto")?.split(",")[0]?.trim() || req.nextUrl.protocol.replace(":", "")
  const host = req.headers.get("x-forwarded-host")?.split(",")[0]?.trim() || req.headers.get("host") || req.nextUrl.host
  return `${proto}://${host}`
}

// Only same-site relative paths are allowed after login.
export function safeNext(next: string | null | undefined): string {
  return next && next.startsWith("/") && !next.startsWith("//") ? next : "/"
}
