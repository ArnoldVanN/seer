import { NextResponse, type NextRequest } from "next/server"
import { PRIVATE_ROUTE, privateBase } from "./lib/routing"
import { siteAccess } from "./lib/site-lookup"

// Routes the private path prefix (/internal/...) to app/private-view, on
// whichever site the request is for, using the site's config:
//   - a site with no private dashboards has no private area: 404;
//   - on a private site everything is private, so other paths move under the
//     prefix.
// Pages check the session themselves; this only redirects early when there's
// no session cookie at all.

const SHARED = ["/auth", "/rpc", "/private-rpc", "/custom.css", "/healthz"]

// Any path no route matches, so Next renders the not-found page with a 404.
const NOT_FOUND = "/_seer/not-found"

export async function proxy(req: NextRequest) {
  const { pathname, search } = req.nextUrl
  const prefix = privateBase()

  // The private route tree is only reachable through the rewrite below.
  if (pathname === PRIVATE_ROUTE || pathname.startsWith(PRIVATE_ROUTE + "/")) {
    return new NextResponse("Not found", { status: 404 })
  }
  if (SHARED.some((p) => pathname === p || pathname.startsWith(p + "/"))) {
    return NextResponse.next()
  }

  // Null when the API is unreachable: carry on and let the page report it.
  const site = await siteAccess(requestHost(req))
  const isPrivatePath = pathname === prefix || pathname.startsWith(prefix + "/")

  if (!isPrivatePath) {
    if (!site?.private) return NextResponse.next()
    const url = req.nextUrl.clone()
    url.pathname = prefix + (pathname === "/" ? "" : pathname)
    return NextResponse.redirect(url)
  }
  if (site && !site.private && !site.hasPrivate) {
    return NextResponse.rewrite(new URL(NOT_FOUND, req.url))
  }

  if (!req.cookies.has("seer_session")) {
    const login = new URL("/auth/login", req.url)
    login.searchParams.set("next", pathname + search)
    return NextResponse.redirect(login)
  }
  const rest = pathname.slice(prefix.length) || "/"
  const url = req.nextUrl.clone()
  url.pathname = PRIVATE_ROUTE + (rest === "/" ? "" : rest)
  return NextResponse.rewrite(url)
}

// Normalized like the API does, so the lookup cache has one entry per site
// hostname.
function requestHost(req: NextRequest): string {
  const raw = req.headers.get("x-forwarded-host")?.split(",")[0] || req.headers.get("host") || ""
  return raw.trim().toLowerCase().replace(/:\d+$/, "")
}

export const config = {
  // Skips Next's assets and files from public/ (logos, icons), which must load
  // on the private host too.
  matcher: ["/((?!_next/static|_next/image|.*\\.(?:svg|png|jpe?g|gif|webp|ico|txt|xml)$).*)"],
}
