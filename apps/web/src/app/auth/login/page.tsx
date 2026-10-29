import { Suspense } from "react"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import { env } from "@/lib/env"
import { safeNext } from "@/lib/origin"
import { connection } from "next/server"
import { LoginForm } from "./LoginForm"

const ERRORS: Record<string, string> = {
  oidc: "Sign-in with your identity provider failed. Try again.",
  expired: "The sign-in attempt expired. Try again.",
  forbidden: "Your account doesn't have access to this site. Ask an admin to add you.",
}

export default function LoginPage(props: PageProps<"/auth/login">) {
  return (
    <Suspense>
      <Login {...props} />
    </Suspense>
  )
}

async function Login({ searchParams }: PageProps<"/auth/login">) {
  await connection()
  const sp = await searchParams
  const next = safeNext(typeof sp.next === "string" ? sp.next : undefined)
  const error = typeof sp.error === "string" ? ERRORS[sp.error] : undefined
  const oidc = env.oidc()
  const admin = env.admin()

  return (
    <Card className="seer-login mx-auto mt-12 max-w-sm">
      <CardHeader>
        <CardTitle className="text-xl">Sign in</CardTitle>
        <CardDescription>Sign in to see internal dashboards.</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        {oidc && (
          <Button asChild className="w-full">
            <a href={`/auth/oidc?next=${encodeURIComponent(next)}`}>Continue with {oidc.name}</a>
          </Button>
        )}
        {oidc && admin && (
          <div className="flex items-center gap-3 text-xs text-muted-foreground">
            <Separator className="flex-1" /> or <Separator className="flex-1" />
          </div>
        )}
        {admin && <LoginForm next={next} />}
        {!oidc && !admin && (
          <Alert variant="destructive">
            <AlertDescription>
              No sign-in method is configured. Set SEER_OIDC_ISSUER, or SEER_ADMIN_USERNAME and
              SEER_ADMIN_PASSWORD_HASH.
            </AlertDescription>
          </Alert>
        )}
      </CardContent>
    </Card>
  )
}
