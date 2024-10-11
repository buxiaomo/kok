#!/bin/sh
set -x
export HOSTNAME=$(hostname)
export ID=${HOSTNAME##*[^0-9]}
export ETCD_CERT="--cacert /etc/kubernetes/pki/etcd/ca.crt --cert /etc/kubernetes/pki/etcd/healthcheck-client.crt --key /etc/kubernetes/pki/etcd/healthcheck-client.key"
export ETCD_ARGS="--data-dir=/var/lib/etcd \
--listen-client-urls https://0.0.0.0:2379 \
--listen-peer-urls=https://0.0.0.0:2380 \
--advertise-client-urls=https://${HOSTNAME}.etcd.${NAMESPACE}:2379 \
--client-cert-auth=true \
--trusted-ca-file=/etc/kubernetes/pki/etcd/ca.crt \
--cert-file=/etc/kubernetes/pki/etcd/server.crt \
--key-file=/etc/kubernetes/pki/etcd/server.key \
--peer-client-cert-auth=true \
--peer-trusted-ca-file=/etc/kubernetes/pki/etcd/ca.crt \
--peer-cert-file=/etc/kubernetes/pki/etcd/peer.crt \
--peer-key-file=/etc/kubernetes/pki/etcd/peer.key \
--listen-metrics-urls=http://0.0.0.0:2381 \
--auto-compaction-retention=1 \
--max-request-bytes=33554432 \
--quota-backend-bytes=8589934592 \
--initial-cluster-token=kubernetes-etcd-cluster \
--enable-v2=false \
--snapshot-count=10000 \
--cipher-suites=TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_RSA_WITH_AES_128_CBC_SHA,TLS_RSA_WITH_AES_128_GCM_SHA256,TLS_RSA_WITH_AES_256_CBC_SHA,TLS_RSA_WITH_AES_256_GCM_SHA384"
export ETCDCTL_API=3

if [ ${ID} -eq 0 ]; then
  exec /usr/local/bin/etcd \
  --name=${HOSTNAME} \
  --initial-cluster=${HOSTNAME}=https://${HOSTNAME}.etcd.${NAMESPACE}:2380 \
  --initial-cluster-state=new \
  --initial-advertise-peer-urls=https://${HOSTNAME}.etcd.${NAMESPACE}:2380 ${ETCD_ARGS}
else
  if [ -f /var/lib/cache/envs ];then
    source /var/lib/cache/envs
    exec /usr/local/bin/etcd \
      --name=${ETCD_NAME} \
      --initial-cluster=${ETCD_INITIAL_CLUSTER} \
      --initial-cluster-state=${ETCD_INITIAL_CLUSTER_STATE} \
      --initial-advertise-peer-urls=${ETCD_INITIAL_ADVERTISE_PEER_URLS} ${ETCD_ARGS}
  fi

  etcdctl --endpoints https://etcd-0.etcd.${NAMESPACE}:2379 ${ETCD_CERT} member add ${HOSTNAME} --peer-urls=https://${HOSTNAME}.etcd.${NAMESPACE}:2380 | grep "^ETCD_" > /var/lib/cache/envs
  source /var/lib/cache/envs
  exec /usr/local/bin/etcd \
    --name=${ETCD_NAME} \
    --initial-cluster=${ETCD_INITIAL_CLUSTER} \
    --initial-cluster-state=${ETCD_INITIAL_CLUSTER_STATE} \
    --initial-advertise-peer-urls=${ETCD_INITIAL_ADVERTISE_PEER_URLS} ${ETCD_ARGS}
fi
