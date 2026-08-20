# syntax=docker/dockerfile:1
# Single static Go binary with the React build embedded (web/dist via embed.FS)
# and the migrations + seed CSVs. No CGO, so it runs on a scratch/distroless base.
# Target arch is driven by buildx (§11.7 deploys linux/arm64 on Graviton):
#   docker buildx build --platform linux/arm64 -t mc-sales:latest .

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build
ARG TARGETOS TARGETARCH
WORKDIR /src

# cached module layer
COPY go.mod go.sum ./
RUN go mod download

# build (web/dist is committed, so no JS toolchain is needed here)
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-arm64} \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# minimal, non-root runtime
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /home/nonroot
COPY --from=build /out/server /usr/local/bin/server
ENV PORT=8080 UPLOAD_DIR=/home/nonroot/uploads
EXPOSE 8080
ENTRYPOINT ["server"]
CMD ["serve"]
