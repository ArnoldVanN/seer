import "server-only"
import { SignJWT } from "jose"
import { env } from "./env"

// Short-lived tokens for calls to the Go private port, signed with the shared
// internal secret. The Go side checks the signature, audience and expiry.

// For one call made for a viewer. It names their session; the API checks
// it's still active and reads their groups from it.
export async function mintInternalToken(sid: string): Promise<string> {
  return mint("seer-private", "viewer", { sid })
}

// For the web app's own calls to the session service.
export async function mintWebToken(): Promise<string> {
  return mint("seer-web", "web", {})
}

function mint(audience: string, subject: string, claims: Record<string, string>): Promise<string> {
  return new SignJWT(claims)
    .setProtectedHeader({ alg: "HS256" })
    .setSubject(subject)
    .setAudience(audience)
    .setIssuedAt()
    .setExpirationTime("60s")
    .sign(new TextEncoder().encode(env.internalJwtSecret()))
}
