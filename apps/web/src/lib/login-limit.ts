import "server-only"

// Failed admin sign-ins per client address, kept in memory: each web replica
// counts on its own, which still caps guessing at a few per minute.
const WINDOW_MS = 15 * 60_000
const MAX_FAILURES = 10
// Bounded so spoofed addresses can't grow it without limit.
const MAX_ENTRIES = 10_000

const failures = new Map<string, { count: number; resetAt: number }>()

export function loginBlocked(client: string): boolean {
  const f = failures.get(client)
  if (!f) return false
  if (f.resetAt <= Date.now()) {
    failures.delete(client)
    return false
  }
  return f.count >= MAX_FAILURES
}

export function recordLoginFailure(client: string) {
  const now = Date.now()
  const f = failures.get(client)
  if (f && f.resetAt > now) {
    f.count++
    return
  }
  if (failures.size >= MAX_ENTRIES) failures.delete(failures.keys().next().value!)
  failures.set(client, { count: 1, resetAt: now + WINDOW_MS })
}

export function clearLoginFailures(client: string) {
  failures.delete(client)
}
