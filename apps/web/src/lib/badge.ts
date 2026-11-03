// Shields-style SVG badges for embedding in READMEs and other sites. They're
// shown outside the status page, so colors are fixed hex values (the Tailwind
// shades the --seer-* defaults use) rather than the theme's CSS variables.

export const BADGE_COLORS = {
  up: "#16a34a",
  partial: "#f59e0b",
  down: "#dc2626",
  unknown: "#a1a1a1",
  label: "#555",
} as const

const MAX_TEXT = 64

// Rough Verdana 11px advance widths: close enough to size the badge without
// a font table.
function textWidth(s: string): number {
  let w = 0
  for (const c of s) {
    if ("iIjl.,:;!|'` ".includes(c)) w += 3.5
    else if ("fJrt()[]{}/-".includes(c)) w += 4.5
    else if ("mwMW%@".includes(c)) w += 10
    else if (c >= "A" && c <= "Z") w += 7.5
    else w += 6.5
  }
  return Math.ceil(w)
}

function escape(s: string): string {
  return s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`)
}

export function renderBadge(label: string, value: string, color: string): string {
  label = label.slice(0, MAX_TEXT)
  value = value.slice(0, MAX_TEXT)
  const lw = textWidth(label) + 12
  const vw = textWidth(value) + 12
  const w = lw + vw
  const l = escape(label)
  const v = escape(value)
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="20" role="img" aria-label="${l}: ${v}"><title>${l}: ${v}</title><linearGradient id="s" x2="0" y2="100%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient><clipPath id="r"><rect width="${w}" height="20" rx="3" fill="#fff"/></clipPath><g clip-path="url(#r)"><rect width="${lw}" height="20" fill="${BADGE_COLORS.label}"/><rect x="${lw}" width="${vw}" height="20" fill="${color}"/><rect width="${w}" height="20" fill="url(#s)"/></g><g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11"><text x="${lw / 2}" y="15" fill="#010101" fill-opacity=".3">${l}</text><text x="${lw / 2}" y="14">${l}</text><text x="${lw + vw / 2}" y="15" fill="#010101" fill-opacity=".3">${v}</text><text x="${lw + vw / 2}" y="14">${v}</text></g></svg>`
}

// Badges are images, so a missing endpoint still answers with one (and a 404)
// instead of a broken image.
export function badgeResponse(svg: string, status = 200): Response {
  return new Response(svg, {
    status,
    headers: {
      "content-type": "image/svg+xml; charset=utf-8",
      "cache-control": "public, max-age=30",
    },
  })
}

export function notFoundBadge(label: string): Response {
  return badgeResponse(renderBadge(label, "not found", BADGE_COLORS.unknown), 404)
}
