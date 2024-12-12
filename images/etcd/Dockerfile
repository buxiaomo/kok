FROM alpine:3.20.1 AS builder
ARG ETCD_VERSION=v3.5.14
RUN wget https://github.com/etcd-io/etcd/releases/download/${ETCD_VERSION}/etcd-${ETCD_VERSION}-linux-amd64.tar.gz -O /tmp/etcd.tar.gz \
    && tar -zxf /tmp/etcd.tar.gz -C /tmp \
    && mv /tmp/etcd-${ETCD_VERSION}-linux-amd64 /tmp/etcd

RUN wget https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-linux-amd64 -O /usr/local/bin/jq \
        && chmod +x /usr/local/bin/jq

FROM alpine:3.20.1
RUN apk add --no-cache curl
COPY --from=builder /tmp/etcd/etcd /usr/local/bin
COPY --from=builder /tmp/etcd/etcdctl /usr/local/bin
COPY --from=builder /usr/local/bin/jq /usr/local/bin
COPY entrypoint.sh /usr/local/bin
COPY prestop.sh /usr/local/bin
ENV TZ=Asia/Shanghai
CMD ["entrypoint.sh"]