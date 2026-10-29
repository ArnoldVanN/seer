import { connection } from "next/server"
import { currentSite } from "@/lib/data"

const footerClass = "seer-footer mx-auto max-w-page space-y-2 px-4 pb-8 text-center text-xs text-muted-foreground"
const linkClass = "underline underline-offset-2 hover:text-foreground"

export async function SiteFooter() {
  await connection()
  const site = await currentSite().catch(() => null)
  return (
    <footer className={footerClass}>
      {site?.footer && <p className="seer-footer-text whitespace-pre-line">{site.footer}</p>}
      {site?.footerLinks.length ? (
        <nav className="seer-footer-links flex flex-wrap justify-center gap-x-4 gap-y-1" aria-label="Footer">
          {site.footerLinks.map((l) => (
            <a key={l.url} href={l.url} className={linkClass}>
              {l.name}
            </a>
          ))}
        </nav>
      ) : null}
      {!site?.hidePoweredBy && <PoweredBy />}
    </footer>
  )
}

export function SiteFooterFallback() {
  return (
    <footer className={footerClass}>
      <PoweredBy />
    </footer>
  )
}

function PoweredBy() {
  return (
    <p className="seer-powered-by">
      Powered by{" "}
      <a href="https://github.com/ArnoldVanN/seer" className={linkClass}>
        Seer
      </a>
    </p>
  )
}
