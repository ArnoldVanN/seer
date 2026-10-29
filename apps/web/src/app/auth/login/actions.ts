"use server"

import bcrypt from "bcryptjs"
import { createHash, timingSafeEqual } from "node:crypto"
import { cookies, headers } from "next/headers"
import { redirect } from "next/navigation"
import { env } from "@/lib/env"
import { safeNext } from "@/lib/origin"
import { sealSession, sessionCookieOptions, SESSION_COOKIE } from "@/lib/session"

// username is echoed back so the form keeps it: React resets a form after its
// action runs, and refills fields from their defaultValue.
export type LoginState = { error?: string; username?: string }

export async function adminLogin(_: LoginState, form: FormData): Promise<LoginState> {
  const admin = env.admin()
  if (!admin) return { error: "Admin login is not enabled." }

  const username = String(form.get("username") ?? "")
  const password = String(form.get("password") ?? "")
  // Always check the password so a wrong username takes as long as a wrong one.
  const ok = await checkPassword(password, admin)
  if (!ok || username !== admin.username) return { error: "Wrong username or password.", username }

  const proto = (await headers()).get("x-forwarded-proto") ?? ""
  ;(await cookies()).set(
    SESSION_COOKIE,
    await sealSession({ sub: `admin:${admin.username}`, name: admin.username, groups: admin.groups }),
    sessionCookieOptions(proto === "https"),
  )
  redirect(safeNext(String(form.get("next") ?? "")))
}

// Plain passwords are compared as SHA-256 digests: equal lengths for
// timingSafeEqual, so timing reveals neither the password nor its length.
async function checkPassword(given: string, admin: { password?: string; passwordHash?: string }) {
  if (admin.passwordHash) return bcrypt.compare(given, admin.passwordHash)
  const digest = (v: string) => createHash("sha256").update(v).digest()
  return timingSafeEqual(digest(given), digest(admin.password ?? ""))
}
