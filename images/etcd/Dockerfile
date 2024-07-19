FROM alpine:3.20.1 as builder
ARG ETCD_VERSION=v3.5.14
RUN wget https://github.com/etcd-io/etcd/releases/download/${ETCD_VERSION}/etcd-${ETCD_VERSION}-linux-amd64.tar.gz -O /tmp/etcd.tar.gz \
    && tar -zxf /tmp/etcd.tar.gz -C /tmp \
    && mv /tmp/etcd-${ETCD_VERSION}-linux-amd64 /tmp/etcd

FROM alpine:3.20.1
COPY --from=builder /tmp/etcd/etcd /usr/local/bin
COPY --from=builder /tmp/etcd/etcdctl /usr/local/bin
COPY entrypoint.sh /usr/local/bin
COPY prestop.sh /usr/local/bin
CMD ["entrypoint.sh"]