import { Button } from "@/components/ui/button"
import type { Viewer } from "@/lib/session"

// Shown in the header whenever there's a session, on public pages too.
export function AccountMenu({ viewer }: { viewer: Viewer }) {
  return (
    <div className="seer-account flex items-center gap-3 text-sm text-muted-foreground">
      <span>
        Signed in as <strong className="text-foreground">{viewer.name}</strong>
      </span>
      <form action="/auth/logout" method="post">
        <Button type="submit" variant="outline" size="sm">
          Sign out
        </Button>
      </form>
    </div>
  )
}
