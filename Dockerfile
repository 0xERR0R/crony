FROM golang:1-alpine AS build-env

RUN apk --no-cache add build-base gcc musl-dev

WORKDIR ${GOPATH}/src/github.com/0xERR0R/crony

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o /go/bin/crony .

FROM alpine

LABEL org.opencontainers.image.source="https://github.com/0xERR0R/crony" \
      org.opencontainers.image.url="https://github.com/0xERR0R/crony" \
      org.opencontainers.image.title="Docker cron task scheduler"

COPY --from=build-env /go/bin/crony /app/crony

EXPOSE 8080

ENTRYPOINT ["/app/crony"]
