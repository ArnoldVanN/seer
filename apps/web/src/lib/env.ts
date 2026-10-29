import "server-only"
import { adminCredentials } from "./admin-credentials"
import { publicApiUrl } from "./api-url"

function list(name: string): string[] {
  return (process.env[name] || "")
    .split(",")
    .map((v) => v.trim())
    .filter(Boolean)
}

function required(name: string): string {
  const v = process.env[name]
  if (!v) throw new Error(`${name} is not set`)
  return v
}

export const env = {
  // In-cluster URLs of the Go API's public and private ports.
  publicApiUrl,
  privateApiUrl: () => process.env.SEER_PRIVATE_API_URL || "http://localhost:8081",
  internalJwtSecret: () => required("SEER_INTERNAL_JWT_SECRET"),
  sessionSecret: () => required("SEER_SESSION_SECRET"),

  oidc: () => {
    const issuer = process.env.SEER_OIDC_ISSUER
    if (!issuer) return null
    return {
      issuer,
      clientId: required("SEER_OIDC_CLIENT_ID"),
      clientSecret: process.env.SEER_OIDC_CLIENT_SECRET,
      scopes: process.env.SEER_OIDC_SCOPES || "openid profile email",
      groupsClaim: process.env.SEER_OIDC_GROUPS_CLAIM || "groups",
      name: process.env.SEER_OIDC_NAME || "SSO",
      // Who may sign in at all. Both empty lets in anyone the provider
      // authenticates.
      allowedGroups: list("SEER_OIDC_ALLOWED_GROUPS"),
      allowedDomains: list("SEER_OIDC_ALLOWED_DOMAINS").map((d) => d.toLowerCase().replace(/^@/, "")),
    }
  },

  admin: () => {
    const a = adminCredentials()
    if (!a) return null
    const groups = process.env.SEER_ADMIN_GROUPS ? list("SEER_ADMIN_GROUPS") : ["admin"]
    return { ...a, groups }
  },
}
