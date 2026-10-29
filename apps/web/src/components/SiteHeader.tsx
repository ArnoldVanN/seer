import Link from "next/link"
import { connection } from "next/server"
import { AccountMenu } from "@/components/AccountMenu"
import { currentSite } from "@/lib/data"
import { getViewer } from "@/lib/session"

const headerClass = "seer-header mx-auto flex max-w-page flex-wrap items-center justify-between gap-4 px-4 pt-7 pb-5"

export async function SiteHeader() {
  await connection()
  const [site, viewer] = await Promise.all([currentSite().catch(() => null), getViewer()])
  const brand = (
    <>
      {site?.logo && <Logo {...site.logo} />}
      <span className="seer-title text-xl font-semibold">{site?.title ?? "Status"}</span>
    </>
  )
  const brandClass = "seer-brand flex items-center gap-2.5 text-foreground"
  return (
    <header className={headerClass}>
      {/* With ui.homeUrl the brand leads back to the main site, like Gatus' ui.link. */}
      {site?.homeUrl ? (
        <a href={site.homeUrl} className={brandClass}>
          {brand}
        </a>
      ) : (
        <Link href="/" className={brandClass}>
          {brand}
        </Link>
      )}
      {viewer && <AccountMenu viewer={viewer} />}
    </header>
  )
}

export function SiteHeaderFallback() {
  return (
    <header className={headerClass}>
      <span className="seer-brand h-7" />
    </header>
  )
}

// Both variants are rendered and CSS shows the one for the current scheme,
// so there's no flash or hydration mismatch. With one logo, only one img.
function Logo({ light, dark }: { light: string; dark: string }) {
  /* eslint-disable @next/next/no-img-element -- any URL from config; next/image would need its host allowlisted */
  if (light === dark) return <img src={light} alt="" className="seer-logo h-8 w-auto" />
  return (
    <>
      <img src={light} alt="" className="seer-logo h-8 w-auto dark:hidden" data-scheme="light" />
      <img src={dark} alt="" className="seer-logo hidden h-8 w-auto dark:block" data-scheme="dark" />
    </>
  )
  /* eslint-enable @next/next/no-img-element */
}
