# Build stage
FROM golang:1.25.5 AS builder
WORKDIR /src
RUN apt-get update && apt-get install -y --no-install-recommends libopus-dev pkg-config && rm -rf /var/lib/apt/lists/*
ENV CGO_ENABLED=1
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /out/server ./cmd/server

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/server /server
COPY --from=builder /src/configs /configs
EXPOSE 8080 8081
USER nonroot:nonroot
ENTRYPOINT ["/server"]
