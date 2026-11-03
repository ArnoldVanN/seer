import { adminCredentials } from "./lib/admin-credentials"
import { checkWeakSecrets } from "./lib/weak-secrets"

// Fails startup on config that would otherwise only error at login.
export function register() {
  adminCredentials()
  checkWeakSecrets()
}
