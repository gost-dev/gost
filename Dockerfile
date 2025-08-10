FROM --platform=$BUILDPLATFORM tonistiigi/xx:1.6.1 AS xx

FROM --platform=$BUILDPLATFORM golang:1.24-alpine3.22 AS builder

# add upx for binary compression
RUN apk add --no-cache upx || echo "upx not found"

COPY --from=xx / /

ARG TARGETPLATFORM

RUN xx-info env

ENV CGO_ENABLED=0

ENV XX_VERIFY_STATIC=1

WORKDIR /app

COPY . .

RUN cd cmd/gost && \
    xx-go build -ldflags "-s -w" && \
    xx-verify gost && \
    { upx --best gost || true; }

FROM alpine:3.22

# add iptables for tun/tap
# RUN apk add --no-cache iptables
# bash is used for debugging, tzdata is used to add timezone information.
# Install ca-certificates to ensure no CA certificate errors.
#
# Do not try to add the "--no-cache" option when there are multiple "apk"
# commands, this will cause the build process to become very slow.
RUN set -ex \
    && apk upgrade \
    && apk add iptables bash tzdata ca-certificates \
    && rm -rf /var/cache/apk/*

WORKDIR /bin/

COPY --from=builder /app/cmd/gost/gost .

ENTRYPOINT ["/bin/gost"]
