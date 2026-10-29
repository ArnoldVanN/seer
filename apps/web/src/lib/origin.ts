import "server-only"
import type { NextRequest } from "next/server"

// The origin the browser used, which differs from req.url behind a gateway.
// SEER_FORCE_HTTPS covers proxies that don't forward the browser's scheme.
export function headerOrigin(headers: Headers, fallback?: URL): string {
  const host = headers.get("x-forwarded-host")?.split(",")[0]?.trim() || headers.get("host") || fallback?.host || ""
  const proto = forceHttps()
    ? "https"
    : headers.get("x-forwarded-proto")?.split(",")[0]?.trim() || fallback?.protocol.replace(":", "") || "http"
  return `${proto}://${host}`
}

export function requestOrigin(req: NextRequest): string {
  return headerOrigin(req.headers, req.nextUrl)
}

function forceHttps(): boolean {
  return ["1", "true"].includes((process.env.SEER_FORCE_HTTPS || "").toLowerCase())
}

// Only same-site relative paths are allowed after login.
export function safeNext(next: string | null | undefined): string {
  return next && next.startsWith("/") && !next.startsWith("//") ? next : "/"
}
