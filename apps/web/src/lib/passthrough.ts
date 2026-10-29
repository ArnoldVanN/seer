import "server-only"
import type { NextRequest } from "next/server"
import { requestOrigin } from "./origin"

const FORWARD_HEADERS = [
  "content-type",
  "connect-protocol-version",
  "connect-timeout-ms",
  "connect-accept-encoding",
  "connect-content-encoding",
  "accept",
]

// Forwards a Connect call to the Go API and streams the response back as it
// arrives, which is what makes server-streaming RPCs work through Next.js.
export async function passthrough(
  req: NextRequest,
  target: string,
  extraHeaders: Record<string, string> = {},
): Promise<Response> {
  const headers = new Headers(extraHeaders)
  // The API picks the site from it, as it does for direct /rpc calls.
  headers.set("x-forwarded-host", new URL(requestOrigin(req)).host)
  for (const h of FORWARD_HEADERS) {
    const v = req.headers.get(h)
    if (v) headers.set(h, v)
  }
  const upstream = await fetch(target + req.nextUrl.search, {
    method: req.method,
    headers,
    body: req.method === "GET" || req.method === "HEAD" ? undefined : req.body,
    signal: req.signal,
    // @ts-expect-error -- Node's fetch needs this to send a streamed body
    duplex: "half",
  })
  const out = new Headers()
  upstream.headers.forEach((v, k) => {
    if (k !== "content-length" && k !== "transfer-encoding" && k !== "connection") out.set(k, v)
  })
  // Tell buffering proxies (nginx and friends) to pass chunks through.
  out.set("x-accel-buffering", "no")
  out.set("cache-control", "no-store")
  return new Response(upstream.body, { status: upstream.status, headers: out })
}
