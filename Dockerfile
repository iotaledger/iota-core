ARG WITH_GO_WORK=0
# https://hub.docker.com/_/golang
FROM golang:1.20-bullseye AS env-with-go-work-0

ARG BUILD_TAGS=rocksdb

LABEL org.label-schema.description="IOTA core node"
LABEL org.label-schema.name="iotaledger/iota-core"
LABEL org.label-schema.schema-version="1.0"
LABEL org.label-schema.vcs-url="https://github.com/iotaledger/iota-core"

RUN mkdir /scratch /app

WORKDIR /scratch

# Here we assume our build context is the parent directory of iota-core
COPY ./iota-core ./iota-core

# We don't want go.work files to interfere in this build environment
RUN rm -f /scratch/iota-core/go.work /scratch/iota-core/go.work.sum

FROM env-with-go-work-0 AS env-with-go-work-1

COPY ./iota.go ./iota.go
COPY ./hive.go ./hive.go
COPY ./inx/go ./inx/go
COPY ./inx-app ./inx-app
COPY ./iota-core/go.work ./iota-core/
COPY ./iota-core/go.work.sum ./iota-core/
COPY ./go.work ./
COPY ./go.work.sum ./

FROM env-with-go-work-${WITH_GO_WORK} AS build
ARG WITH_GO_WORK=0

WORKDIR /scratch/iota-core

# Ensure ca-certificates are up to date
RUN update-ca-certificates

# Download go modules
RUN go mod download
# Do not verify modules if we have local modules coming from go.work
RUN if [ "${WITH_GO_WORK}" = "0" ]; then go mod verify; fi

# Build the binary
RUN go build -o /app/iota-core -a -tags="$BUILD_TAGS" -ldflags='-w -s'

# Copy the assets
COPY ./iota-core/config_defaults.json /app/config.json
COPY ./iota-core/peering.json /app/peering.json

############################
# Runtime Image
############################
# https://console.cloud.google.com/gcr/images/distroless/global/cc-debian11
# using distroless cc "nonroot" image, which includes everything in the base image (glibc, libssl and openssl)
FROM gcr.io/distroless/cc-debian11:nonroot

EXPOSE 15600/tcp
EXPOSE 14265/tcp

# Copy the app dir into distroless image
COPY --chown=nonroot:nonroot --from=build /app /app

WORKDIR /app
USER nonroot

ENTRYPOINT ["/app/iota-core"]
