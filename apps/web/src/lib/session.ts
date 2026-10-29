import "server-only"
import { createHash } from "node:crypto"
import { EncryptJWT, jwtDecrypt } from "jose"
import { cookies } from "next/headers"
import { redirect } from "next/navigation"
import { env } from "./env"

export type Viewer = {
  sub: string
  name: string
  groups: string[]
}

export const SESSION_COOKIE = "seer_session"
const SESSION_TTL_SECONDS = 8 * 60 * 60

function key(): Uint8Array {
  return createHash("sha256").update(env.sessionSecret()).digest()
}

export async function sealSession(v: Viewer): Promise<string> {
  return new EncryptJWT({ name: v.name, groups: v.groups })
    .setProtectedHeader({ alg: "dir", enc: "A256GCM" })
    .setSubject(v.sub)
    .setIssuedAt()
    .setExpirationTime(`${SESSION_TTL_SECONDS}s`)
    .encrypt(key())
}

export async function openSession(token: string | undefined): Promise<Viewer | null> {
  if (!token) return null
  try {
    const { payload } = await jwtDecrypt(token, key())
    if (!payload.sub) return null
    return {
      sub: payload.sub,
      name: typeof payload.name === "string" ? payload.name : payload.sub,
      groups: Array.isArray(payload.groups) ? payload.groups.map(String) : [],
    }
  } catch {
    return null
  }
}

export const sessionCookieOptions = (secure: boolean) => ({
  httpOnly: true,
  sameSite: "lax" as const,
  secure,
  path: "/",
  maxAge: SESSION_TTL_SECONDS,
})

export async function getViewer(): Promise<Viewer | null> {
  return openSession((await cookies()).get(SESSION_COOKIE)?.value)
}

// For private pages: the signed-in viewer, or a redirect to login.
export async function requireViewer(next: string): Promise<Viewer> {
  const v = await getViewer()
  if (!v) redirect(`/auth/login?next=${encodeURIComponent(next)}`)
  return v
}
