FROM golang:1.24.4 AS builder

LABEL stage=gobuilder

ENV CGO_ENABLED 1

ENV GOPROXY https://goproxy.cn,direct

ENV GOSUMDB off

WORKDIR /build

COPY . .

RUN make build

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/share/zoneinfo/Asia/Shanghai /usr/share/zoneinfo/Asia/Shanghai
ENV TZ Asia/Shanghai

WORKDIR /app
COPY --from=builder /build/bin/main /app/server

CMD ["./server"]