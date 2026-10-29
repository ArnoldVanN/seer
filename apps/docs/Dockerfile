# Build from the repo root: docker build -f apps/docs/Dockerfile .
FROM node:24-bookworm-slim AS build
RUN corepack enable
WORKDIR /src

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/docs/package.json apps/docs/
RUN pnpm install --frozen-lockfile --filter docs

COPY apps/docs/ apps/docs/
ARG SITE_URL=https://seer.arnovannieuwenhuyzen.com
ENV NEXT_TELEMETRY_DISABLED=1 SITE_URL=$SITE_URL
RUN cd apps/docs && pnpm build

FROM nginxinc/nginx-unprivileged:alpine
COPY --from=build /src/apps/docs/out /usr/share/nginx/html
COPY apps/docs/nginx.conf /etc/nginx/nginx.conf
EXPOSE 8080
CMD ["nginx", "-g", "daemon off;"]
