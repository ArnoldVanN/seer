// Kept free of server-only so instrumentation.ts can validate it at startup.

export type AdminCredentials =
  | { username: string; password: string; passwordHash?: never }
  | { username: string; passwordHash: string; password?: never }

// The admin account takes either a plain password or a bcrypt hash. Both set
// is an error rather than a guess at which one was meant.
export function adminCredentials(): AdminCredentials | null {
  const username = process.env.SEER_ADMIN_USERNAME
  const password = process.env.SEER_ADMIN_PASSWORD
  const passwordHash = process.env.SEER_ADMIN_PASSWORD_HASH
  if (password && passwordHash) {
    throw new Error("Set SEER_ADMIN_PASSWORD or SEER_ADMIN_PASSWORD_HASH, not both")
  }
  if (!username || !(password || passwordHash)) return null
  return password ? { username, password } : { username, passwordHash: passwordHash! }
}
