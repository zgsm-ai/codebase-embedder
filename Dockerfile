FROM golang:1.24.4 AS builder

LABEL stage=gobuilder

# ENV CGO_ENABLED 1

ENV GOPROXY https://goproxy.cn,direct

ENV GOSUMDB off

WORKDIR /build

COPY . .

RUN go env -w CGO_ENABLED=0 && \
    go env -w GO111MODULE=on && \
    go env -w GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy,direct

RUN make build

# FROM alpine:latest

# 

FROM alpine:3.21 AS runtime

# FROM golang:1.24.4 AS STANDARD

# RUN apk --no-cache add ca-certificates tzdata
# RUN apk add --no-cache bash


COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/share/zoneinfo/Asia/Shanghai /usr/share/zoneinfo/Asia/Shanghai
ENV TZ Asia/Shanghai

WORKDIR /app
COPY --from=builder /build/bin/main /app/server
RUN chmod +x /app/server

CMD ["./server"]