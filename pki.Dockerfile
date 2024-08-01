FROM golang:1.22.5-alpine3.20 AS builder
ENV GOPROXY "https://goproxy.cn,direct"
WORKDIR /go/src/app
COPY go.mod go.sum /go/src/app/
RUN go mod download
COPY . /go/src/app/
RUN apk add --no-cache g++
RUN CGO_ENABLED=1 GO111MODULE=on GOOS=linux go build -o main pki.go

FROM alpine:3.20.1
WORKDIR /app
COPY --from=builder /go/src/app/main ./main
CMD ["/app/main"]
