FROM alpine:3.23

# enables automatic changelog generation by tools like Dependabot
LABEL org.opencontainers.image.source="https://github.com/redstone-md/shield" \
      org.opencontainers.image.licenses="MIT"

ENV UPDATER_IN_DOCKER=1
RUN apk add --no-cache --update git tzdata ca-certificates rsync
COPY LICENSE /srv/LICENSE
COPY updater/update-git.sh /srv/update-git.sh
RUN chmod +x /srv/update-git.sh

WORKDIR /srv
ENTRYPOINT ["/srv/update-git.sh"]
