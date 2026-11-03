import "server-only"
import { createHash } from "node:crypto"
import { Code, ConnectError } from "@connectrpc/connect"
import { EncryptJWT, jwtDecrypt } from "jose"
import { cookies } from "next/headers"
import { redirect } from "next/navigation"
import { cache } from "react"
import { sessionClient } from "./api"
import { env } from "./env"
import { clientAddress } from "./origin"

// Sessions live in the Go API's database, so signing out ends them for good.
// The cookie holds only the session's ID, encrypted so a cookie we didn't
// issue is rejected without asking the API.

export type Viewer = {
  // The session ID, which private API tokens carry. The API reads the
  // viewer's groups from the session, not from the token.
  sid: string
  sub: string
  name: string
  groups: string[]
}

export const SESSION_COOKIE = "seer_session"
// Matches the API's session lifetime.
const SESSION_TTL_SECONDS = 8 * 60 * 60

function key(): Uint8Array {
  return createHash("sha256").update(env.sessionSecret()).digest()
}

async function seal(sid: string): Promise<string> {
  return new EncryptJWT({ sid })
    .setProtectedHeader({ alg: "dir", enc: "A256GCM" })
    .setIssuedAt()
    .setExpirationTime(`${SESSION_TTL_SECONDS}s`)
    .encrypt(key())
}

// The session ID in a cookie, or null for a missing or foreign one.
export async function sessionId(cookie: string | undefined): Promise<string | null> {
  if (!cookie) return null
  try {
    const { payload } = await jwtDecrypt(cookie, key())
    return typeof payload.sid === "string" ? payload.sid : null
  } catch {
    return null
  }
}

// Starts a session for a signed-in user and returns the cookie value. Email
// and username are stored so an admin can tell who a session belongs to; pass
// email only when the provider verified it.
export async function createSession(
  user: { sub: string; name: string; groups: string[]; email?: string; username?: string },
  headers: Headers,
): Promise<string> {
  const { id } = await sessionClient().createSession({
    subject: user.sub,
    name: user.name,
    groups: user.groups,
    email: user.email ?? "",
    username: user.username ?? "",
    userAgent: headers.get("user-agent") ?? "",
    ip: clientAddress(headers),
  })
  return seal(id)
}

// Ends the session a cookie names. Unknown sessions are fine.
export async function revokeSession(cookie: string | undefined) {
  const id = await sessionId(cookie)
  if (id) await sessionClient().revokeSession({ id })
}

export const sessionCookieOptions = (secure: boolean) => ({
  httpOnly: true,
  sameSite: "lax" as const,
  secure,
  path: "/",
  maxAge: SESSION_TTL_SECONDS,
})

// The signed-in viewer, or null when signed out or the session ended. Cached
// per request, since the header and the page both ask.
export const getViewer = cache(async (): Promise<Viewer | null> => {
  const sid = await sessionId((await cookies()).get(SESSION_COOKIE)?.value)
  if (!sid) return null
  try {
    const { session } = await sessionClient().getSession({ id: sid })
    if (!session) return null
    return { sid, sub: session.subject, name: session.name, groups: session.groups }
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.NotFound) return null
    throw err
  }
})

// For private pages: the signed-in viewer, or a redirect to login.
export async function requireViewer(next: string): Promise<Viewer> {
  const v = await getViewer()
  if (!v) redirect(`/auth/login?next=${encodeURIComponent(next)}`)
  return v
}
