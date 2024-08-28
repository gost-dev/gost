FROM --platform=$BUILDPLATFORM golang:1.22-alpine as builder

# Convert TARGETPLATFORM to GOARCH format
# https://github.com/tonistiigi/xx
COPY --from=tonistiigi/xx:golang / /

ARG TARGETPLATFORM

RUN apk add --no-cache musl-dev git gcc

ADD . /src

WORKDIR /src

ENV GO111MODULE=on

RUN cd cmd/gost && go env && go build

FROM alpine:latest

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

COPY --from=builder /src/cmd/gost/gost .

ENTRYPOINT ["/bin/gost"]
