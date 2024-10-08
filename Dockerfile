FROM golang:1.23.1-alpine3.20 AS builder
ENV GOPROXY "https://goproxy.cn,direct"
RUN apk add --no-cache g++ git curl bash openssl \
    && curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
WORKDIR /go/src/app
COPY go.mod go.sum /go/src/app/
RUN go mod download
COPY . /go/src/app/
RUN CGO_ENABLED=1 GO111MODULE=on GOOS=linux go build -o main main.go

FROM alpine:3.20.3
RUN apk add --no-cache curl \
    && adduser -D -h /app -u 1000 app
WORKDIR /app
COPY --from=builder /go/src/app/main ./main
COPY --from=builder /go/src/app/templates ./templates
COPY --from=builder /go/src/app/appmarket ./appmarket
COPY --from=builder /go/src/app/static ./static
COPY --from=builder /usr/local/bin/helm /usr/local/bin/helm
COPY entrypoint.sh /entrypoint.sh
VOLUME /app/data
EXPOSE 8080
USER 1000
CMD ["/entrypoint.sh"]
