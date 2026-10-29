# Build from the repo root: docker build -f apps/web/Dockerfile .
FROM node:24-bookworm-slim AS build
RUN corepack enable
WORKDIR /src

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY packages/protos/package.json packages/protos/
COPY apps/web/package.json apps/web/
RUN pnpm install --frozen-lockfile --filter web... --filter @seer/protos

COPY packages/protos/ packages/protos/
RUN cd packages/protos && rm -rf gen \
 && ./node_modules/.bin/buf generate --template buf.gen.ts.yaml \
 && pnpm build

COPY apps/web/ apps/web/
ENV NEXT_TELEMETRY_DISABLED=1
RUN cd apps/web && pnpm build

FROM node:24-bookworm-slim
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3000 HOSTNAME=0.0.0.0
COPY --from=build --chown=node:node /src/apps/web/.next/standalone ./
COPY --from=build --chown=node:node /src/apps/web/.next/static ./apps/web/.next/static
COPY --from=build --chown=node:node /src/apps/web/public ./apps/web/public
USER 1000:1000
EXPOSE 3000
CMD ["node", "apps/web/server.js"]
