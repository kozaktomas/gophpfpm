FROM golang:1.21 AS builder
ENV CGO_ENABLED 0
ADD . /app
WORKDIR /app
RUN go build -ldflags "-s -w" -v -o gophpfpm .

FROM alpine:3
RUN apk update && \
    apk add openssl tzdata && \
    rm -rf /var/cache/apk/* \
    && mkdir /app

WORKDIR /app

ADD Dockerfile /Dockerfile

COPY --from=builder /app/gophpfpm /app/gophpfpm

RUN chown nobody /app/gophpfpm \
    && chmod 500 /app/gophpfpm

USER nobody

ENTRYPOINT ["/app/gophpfpm"]
