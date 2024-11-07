FROM alpine:3.18
WORKDIR /app

RUN apk update
RUN apk add --no-cache ca-certificates e2fsprogs findmnt bind-tools e2fsprogs-extra xfsprogs xfsprogs-extra blkid

COPY csi-utho-plugin /app/csi-utho-plugin

RUN mkdir -p /var/lib/csi/sockets/pluginproxy

ENTRYPOINT ["/app/csi-utho-plugin"]
