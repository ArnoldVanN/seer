import { connection, NextResponse, type NextRequest } from "next/server"
import * as client from "openid-client"
import { env } from "@/lib/env"
import { oidcAllowed, oidcConfig, OIDC_FLOW_COOKIE } from "@/lib/oidc"
import { requestOrigin, safeNext } from "@/lib/origin"
import { sealSession, sessionCookieOptions, SESSION_COOKIE } from "@/lib/session"

export async function GET(req: NextRequest) {
  // Request-time only; without this Cache Components prerenders the handler.
  await connection()
  const o = env.oidc()
  if (!o) return new Response("OIDC is not configured", { status: 404 })
  const origin = requestOrigin(req)

  let flow: { verifier: string; state: string; next: string }
  try {
    flow = JSON.parse(req.cookies.get(OIDC_FLOW_COOKIE)?.value ?? "")
  } catch {
    return NextResponse.redirect(new URL("/auth/login?error=expired", origin))
  }

  const config = await oidcConfig()
  // The URL the provider redirected to, as the browser saw it.
  const current = new URL(req.nextUrl.pathname + req.nextUrl.search, origin)
  let claims: Record<string, unknown>
  try {
    const tokens = await client.authorizationCodeGrant(config, current, {
      pkceCodeVerifier: flow.verifier,
      expectedState: flow.state,
    })
    claims = { ...(tokens.claims() ?? {}) }
    // Some providers only put groups in userinfo.
    if (!(o.groupsClaim in claims) && claims.sub) {
      const info = await client.fetchUserInfo(config, tokens.access_token, String(claims.sub))
      claims = { ...info, ...claims }
    }
  } catch (err) {
    console.error("oidc callback failed", err)
    return NextResponse.redirect(new URL("/auth/login?error=oidc", origin))
  }

  const rawGroups = claims[o.groupsClaim]
  const groups = Array.isArray(rawGroups) ? rawGroups.map(String) : typeof rawGroups === "string" ? [rawGroups] : []
  const name = String(claims.name ?? claims.preferred_username ?? claims.email ?? claims.sub)

  if (!oidcAllowed(o, groups, claims)) {
    console.warn("oidc sign-in denied: not in SEER_OIDC_ALLOWED_GROUPS/DOMAINS", { sub: claims.sub, groups })
    const res = NextResponse.redirect(new URL("/auth/login?error=forbidden", origin))
    res.cookies.delete({ name: OIDC_FLOW_COOKIE, path: "/auth" })
    return res
  }

  const res = NextResponse.redirect(new URL(safeNext(flow.next), origin))
  res.cookies.set(
    SESSION_COOKIE,
    await sealSession({ sub: `oidc:${claims.sub}`, name, groups }),
    sessionCookieOptions(origin.startsWith("https://")),
  )
  res.cookies.delete({ name: OIDC_FLOW_COOKIE, path: "/auth" })
  return res
}
