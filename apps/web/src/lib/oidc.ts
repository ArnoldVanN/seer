import "server-only"
import * as client from "openid-client"
import { env } from "./env"

let configPromise: Promise<client.Configuration> | null = null

// Discovery runs once per process; a failure is retried on the next login.
export function oidcConfig(): Promise<client.Configuration> {
  const o = env.oidc()
  if (!o) throw new Error("OIDC is not configured")
  if (!configPromise) {
    configPromise = client.discovery(new URL(o.issuer), o.clientId, o.clientSecret).catch((err) => {
      configPromise = null
      throw err
    })
  }
  return configPromise
}

export const OIDC_FLOW_COOKIE = "seer_oidc"

// The sign-in allowlist: a user gets in with any allowed group, or a verified
// email on an allowed domain (for providers like Google that send no groups).
// With neither list set, everyone the provider authenticates gets in.
export function oidcAllowed(
  o: { allowedGroups: string[]; allowedDomains: string[] },
  groups: string[],
  claims: Record<string, unknown>,
): boolean {
  if (o.allowedGroups.length === 0 && o.allowedDomains.length === 0) return true
  if (groups.some((g) => o.allowedGroups.includes(g))) return true
  // Unverified emails are ignored: some providers let users set any address.
  if (claims.email_verified !== true || typeof claims.email !== "string") return false
  const domain = claims.email.split("@").pop()?.toLowerCase()
  return !!domain && o.allowedDomains.includes(domain)
}
