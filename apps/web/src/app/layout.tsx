import type { Metadata } from "next"
import { Geist } from "next/font/google"
import { connection } from "next/server"
import { Suspense } from "react"
import { SiteHeader, SiteHeaderFallback } from "@/components/SiteHeader"
import { TooltipProvider } from "@/components/ui/tooltip"
import { currentSite } from "@/lib/data"
import "./globals.css"

const geist = Geist({ subsets: ["latin"], variable: "--font-geist" })

export async function generateMetadata(): Promise<Metadata> {
  await connection()
  const site = await currentSite().catch(() => null)
  return {
    title: site?.title ?? "Status",
    description: site?.description || undefined,
    icons: [{ url: site?.faviconUrl ?? "/seer-icon.svg" }],
  }
}

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className={geist.variable}>
      <head>
        {/* After the built-in styles so user rules win at equal specificity.
            It's runtime config from the API, so it can't be imported. */}
        {/* eslint-disable-next-line @next/next/no-css-tags */}
        <link rel="stylesheet" href="/custom.css" precedence="high" />
      </head>
      <body className="seer min-h-screen">
        <TooltipProvider delayDuration={100}>
          <Suspense fallback={<SiteHeaderFallback />}>
            <SiteHeader />
          </Suspense>
          <main className="seer-main mx-auto max-w-page px-4 pb-12">{children}</main>
          <footer className="seer-footer mx-auto max-w-page px-4 pb-8 text-center text-xs text-muted-foreground">
            Powered by{" "}
            <a href="https://github.com/ArnoldVanN/seer" className="underline underline-offset-2 hover:text-foreground">
              Seer
            </a>
          </footer>
        </TooltipProvider>
      </body>
    </html>
  )
}
