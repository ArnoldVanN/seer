// Where private dashboards live. Read by proxy.ts (to rewrite requests) and
// by pages (to build links), so it must stay free of server-only imports.

export const PRIVATE_ROUTE = "/private-view"

// Path prefix for private dashboards on every site, e.g. /internal.
export function privateBase(): string {
  return (process.env.SEER_PRIVATE_PATH_PREFIX || "/internal").replace(/\/$/, "")
}
