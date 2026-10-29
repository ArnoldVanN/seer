import "server-only"
import { createClient, type Interceptor } from "@connectrpc/connect"
import { createConnectTransport } from "@connectrpc/connect-node"
import { StatusService } from "@seer/protos/public"
import { PrivateStatusService } from "@seer/protos/private"
import { env } from "./env"
import { mintInternalToken } from "./token"
import type { Viewer } from "./session"

// Without a deadline a stalled API would hang page renders.
const TIMEOUT_MS = 10_000

// Which site a call is for: its key once known, or the hostname to resolve
// it from. The Go API scopes dashboards to that site.
export type SiteRef = { key: string } | { host: string }

function siteHeaders(site: SiteRef): Interceptor {
  return (next) => async (req) => {
    if ("key" in site) req.header.set("X-Seer-Site", site.key)
    else req.header.set("X-Forwarded-Host", site.host)
    return next(req)
  }
}

export function publicClient(site: SiteRef) {
  return createClient(
    StatusService,
    createConnectTransport({
      baseUrl: env.publicApiUrl(),
      httpVersion: "1.1",
      useHttpGet: true,
      defaultTimeoutMs: TIMEOUT_MS,
      interceptors: [siteHeaders(site)],
    }),
  )
}

export function privateClient(viewer: Viewer, site: SiteRef) {
  const auth: Interceptor = (next) => async (req) => {
    req.header.set("Authorization", `Bearer ${await mintInternalToken(viewer)}`)
    return next(req)
  }
  return createClient(
    PrivateStatusService,
    createConnectTransport({
      baseUrl: env.privateApiUrl(),
      httpVersion: "1.1",
      useHttpGet: true,
      defaultTimeoutMs: TIMEOUT_MS,
      interceptors: [auth, siteHeaders(site)],
    }),
  )
}
