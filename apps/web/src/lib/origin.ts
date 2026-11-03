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

// The client's address: the last X-Forwarded-For entry, the one our own
// proxy appended. Earlier entries come from the client, which could pick a
// fresh one per login attempt. Behind several proxies it's the next proxy's
// address, so clients share a login-limit bucket, which errs on the strict
// side.
export function clientAddress(headers: Headers): string {
  return (
    headers.get("x-forwarded-for")?.split(",").pop()?.trim() ||
    headers.get("x-real-ip")?.trim() ||
    "unknown"
  )
}

// Cookies ride along on cross-site requests; state-changing and private
// routes only accept our own origin. Browsers send Origin on POSTs and on
// fetches, so a missing one is a non-browser client without our cookies.
export function sameOrigin(req: NextRequest): boolean {
  const origin = req.headers.get("origin")
  return !origin || origin === requestOrigin(req)
}

// Only same-site relative paths are allowed after login. Parsed rather than
// prefix-checked: URLs treat "\" as "/" and drop tabs and newlines, so
// "/\evil.com" would otherwise slip through as "//evil.com".
export function safeNext(next: string | null | undefined): string {
  if (!next?.startsWith("/")) return "/"
  const base = "http://seer.invalid"
  try {
    const url = new URL(next, base)
    return url.origin === base ? url.pathname + url.search + url.hash : "/"
  } catch {
    return "/"
  }
}
