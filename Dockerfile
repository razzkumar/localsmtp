FROM node:24-alpine AS web-build
WORKDIR /app/web
COPY web/package*.json ./
RUN npm install
COPY web/ ./
RUN npm run build

FROM golang:1.25 AS go-build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
COPY README.md ./
COPY --from=web-build /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/localsmtp ./cmd/localsmtp

FROM alpine:3.23
RUN adduser -D -H -s /sbin/nologin localsmtp
RUN apk add --no-cache ca-certificates
RUN mkdir -p /data && chown localsmtp:localsmtp /data
WORKDIR /data
COPY --from=go-build /bin/localsmtp /usr/local/bin/localsmtp
USER localsmtp
EXPOSE 3025 2025
ENTRYPOINT ["localsmtp"]
