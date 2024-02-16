# https://hub.docker.com/_/golang
FROM golang:1.22-bookworm AS build

ARG BUILD_TAGS=rocksdb
ARG BUILD_VERSION=v1.0.0-develop

LABEL org.label-schema.description="IOTA core node"
LABEL org.label-schema.name="iotaledger/iota-core"
LABEL org.label-schema.schema-version="1.0"
LABEL org.label-schema.vcs-url="https://github.com/iotaledger/iota-core"

# Ensure ca-certificates are up to date
RUN update-ca-certificates

# Set the current Working Directory inside the container
RUN mkdir /scratch
WORKDIR /scratch

# Prepare the folder where we are putting all the files
RUN mkdir /app

WORKDIR /scratch

# Copy everything from the current directory to the PWD(Present Working Directory) inside the container
COPY . .

# Download go modules
RUN go mod download
RUN go mod verify

# Build the binary
RUN go build -o /app/iota-core -tags="$BUILD_TAGS" -ldflags="-w -s -X=github.com/iotaledger/iota-core/components/app.Version=${BUILD_VERSION}"

# Copy the assets
RUN cp ./config_defaults.json /app/config.json
RUN cp ./peering.json /app/peering.json

############################
# Runtime Image
############################
# https://console.cloud.google.com/gcr/images/distroless/global/cc-debian12
# using distroless cc "nonroot" image, which includes everything in the base image (glibc, libssl and openssl)
FROM gcr.io/distroless/cc-debian12:nonroot

HEALTHCHECK --interval=10s --timeout=5s --retries=30 CMD ["/app/iota-core", "tools", "node-info"]

# Copy the app dir into distroless image
COPY --chown=nonroot:nonroot --from=build /app /app

WORKDIR /app
USER nonroot

ENTRYPOINT ["/app/iota-core"]
