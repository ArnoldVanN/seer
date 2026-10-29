import { connection, NextResponse, type NextRequest } from "next/server"
import * as client from "openid-client"
import { env } from "@/lib/env"
import { oidcConfig, OIDC_FLOW_COOKIE } from "@/lib/oidc"
import { requestOrigin, safeNext } from "@/lib/origin"

// Starts the authorization code flow with PKCE.
export async function GET(req: NextRequest) {
  // Request-time only; without this Cache Components prerenders the handler.
  await connection()
  const o = env.oidc()
  if (!o) return new Response("OIDC is not configured", { status: 404 })
  const config = await oidcConfig()
  const origin = requestOrigin(req)

  const verifier = client.randomPKCECodeVerifier()
  const state = client.randomState()
  const url = client.buildAuthorizationUrl(config, {
    redirect_uri: `${origin}/auth/callback`,
    scope: o.scopes,
    code_challenge: await client.calculatePKCECodeChallenge(verifier),
    code_challenge_method: "S256",
    state,
  })

  const res = NextResponse.redirect(url)
  res.cookies.set(OIDC_FLOW_COOKIE, JSON.stringify({ verifier, state, next: safeNext(req.nextUrl.searchParams.get("next")) }), {
    httpOnly: true,
    sameSite: "lax",
    secure: origin.startsWith("https://"),
    path: "/auth",
    maxAge: 600,
  })
  return res
}
