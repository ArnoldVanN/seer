// Kept free of server-only so instrumentation.ts can run it at startup.

const MIN_SECRET = 32
const MIN_PASSWORD = 12

// Refuses short secrets and admin passwords, which the example and demo
// values all are. SEER_ALLOW_WEAK_SECRETS=true turns this into a warning for
// local development and demos.
export function checkWeakSecrets() {
  const weak: string[] = []
  for (const name of ["SEER_SESSION_SECRET", "SEER_INTERNAL_JWT_SECRET"]) {
    const v = process.env[name]
    if (v && v.length < MIN_SECRET) weak.push(`${name} is shorter than ${MIN_SECRET} characters`)
  }
  const pw = process.env.SEER_ADMIN_PASSWORD
  if (pw && pw.length < MIN_PASSWORD) weak.push(`SEER_ADMIN_PASSWORD is shorter than ${MIN_PASSWORD} characters`)
  if (weak.length === 0) return

  const msg = `${weak.join("; ")}. Generate secrets with \`openssl rand -base64 32\`.`
  if (["1", "true"].includes((process.env.SEER_ALLOW_WEAK_SECRETS || "").toLowerCase())) {
    console.warn(`seer: ${msg} Allowed by SEER_ALLOW_WEAK_SECRETS.`)
    return
  }
  throw new Error(`${msg} Set SEER_ALLOW_WEAK_SECRETS=true to run anyway (development only).`)
}
