import { connection } from "next/server"
import { getCustomCss, requestHost } from "@/lib/data"

// The site's CSS from the config file, linked after the built-in styles.
export async function GET() {
  await connection()
  let css = ""
  try {
    css = await getCustomCss(await requestHost())
  } catch {
    // An unreachable API shouldn't break the page's styles.
  }
  return new Response(css, {
    headers: { "content-type": "text/css; charset=utf-8", "cache-control": "public, max-age=60" },
  })
}
