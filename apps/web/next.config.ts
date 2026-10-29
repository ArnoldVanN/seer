import path from "node:path"
import type { NextConfig } from "next"

const nextConfig: NextConfig = {
  cacheComponents: true,
  partialPrefetching: true,
  // Self-contained server bundle for the Docker image. Tracing from the repo
  // root pulls in the workspace @seer/protos package.
  output: "standalone",
  outputFileTracingRoot: path.join(import.meta.dirname, "../.."),
  // Logos come from user config, so any host is allowed.
  images: { unoptimized: true },
}

export default nextConfig
