import "server-only"
import { SignJWT } from "jose"
import { env } from "./env"
import type { Viewer } from "./session"

// Short-lived token for one call to the Go private port. The Go side checks
// the signature, audience and expiry, then applies dashboard group rules.
export async function mintInternalToken(viewer: Viewer): Promise<string> {
  return new SignJWT({ groups: viewer.groups })
    .setProtectedHeader({ alg: "HS256" })
    .setSubject(viewer.sub)
    .setAudience("seer-private")
    .setIssuedAt()
    .setExpirationTime("60s")
    .sign(new TextEncoder().encode(env.internalJwtSecret()))
}
