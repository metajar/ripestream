FROM node:22-alpine AS ui

WORKDIR /src/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build


FROM golang:1.25-alpine AS backend

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=ui /src/web/dist ./web/dist

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ripestream .


FROM alpine:3.22

RUN apk add --no-cache ca-certificates wget \
    && addgroup -S ripestream \
    && adduser -S -G ripestream ripestream \
    && mkdir -p /data \
    && chown ripestream:ripestream /data

COPY --from=backend /out/ripestream /usr/local/bin/ripestream
COPY --chmod=755 scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

USER ripestream
WORKDIR /data

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
