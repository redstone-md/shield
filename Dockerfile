FROM golang:1.26-alpine AS build

ARG GIT_BRANCH
ARG GITHUB_SHA
ARG CI

WORKDIR /build

RUN apk add --no-cache ca-certificates git

# Copy go.mod and go.sum first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

RUN go version

# Build with cache mounts for faster rebuilds
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    if [ -z "$CI" ] ; then \
      echo "runs outside of CI"; \
      if git rev-parse --git-dir > /dev/null 2>&1; then \
        version=$(git rev-parse --abbrev-ref HEAD)-$(git log -1 --format=%h)-$(date +%Y%m%dT%H:%M:%S); \
      else \
        version=local-$(date +%Y%m%dT%H:%M:%S); \
      fi; \
    else version=${GIT_BRANCH}-${GITHUB_SHA:0:7}-$(date +%Y%m%dT%H:%M:%S); fi && \
    echo "version=$version" && \
    cd app && go build -o /build/shield -ldflags "-X main.revision=${version} -s -w"


FROM alpine:3.24
# enables automatic changelog generation by tools like Dependabot
LABEL org.opencontainers.image.source="https://github.com/redstone-md/shield" \
      org.opencontainers.image.licenses="MIT"
ENV SHIELD_IN_DOCKER=1
RUN apk add --no-cache tzdata ffmpeg
COPY --from=build /build/shield /srv/shield
COPY LICENSE /srv/LICENSE

COPY data /srv/preset
COPY data/.not_mounted /srv/data/.not_mounted
COPY entrypoint.sh /srv/entrypoint.sh

RUN \
 adduser -s /bin/sh -D -u 1000 app && chown -R app:app /home/app && \
 chown -R app:app /srv/preset /srv/data && \
 chmod -R 777 /srv/preset && \
 chmod -R 775 /srv/data && \
 chmod +x /srv/entrypoint.sh && \
 ls -la /srv/preset

USER app
WORKDIR /srv

RUN \
 /srv/shield --convert=only --files.dynamic=/srv/preset --files.samples=/srv/preset && \
 sh -c 'for f in /srv/preset/*.txt.loaded; do mv -vf "$f" "${f%.loaded}"; done' && \
 echo "preset files converted" && \
 ls -la /srv/preset && \
 ls -la /srv/data

EXPOSE 8080
ENTRYPOINT ["/srv/entrypoint.sh"]
