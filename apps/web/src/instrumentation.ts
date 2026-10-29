import { adminCredentials } from "./lib/admin-credentials"

// Fails startup on config that would otherwise only error at login.
export function register() {
  adminCredentials()
}
