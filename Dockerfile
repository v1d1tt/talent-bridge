# syntax=docker/dockerfile:1
# Three stages: build the page with Node, build the server with Go, then copy
# only the two outputs into a tiny image with no shell and no package manager.

# 1. Frontend: Solid + Vite -> web/dist
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. Backend: one static Go binary
FROM golang:1.27-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY db/ db/
COPY internal/ internal/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/server ./cmd/server

# 3. Runtime: distroless "static" has CA certificates and a non-root user, nothing else.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=server /out/server /app/server
COPY --from=web /src/web/dist /app/web/dist
ENV STATIC_DIR=/app/web/dist
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]
