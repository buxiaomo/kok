#!/bin/sh
set -x
export HOSTNAME=$(hostname)
export ID=${HOSTNAME##*[^0-9]}
export ETCDCTL_API=3
export MEMBER_COUNT=$(curl -s --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" "https://kubernetes.default.svc/apis/apps/v1/namespaces/${NAMESPACE}/statefulsets/etcd" | jq .spec.replicas)
export ETCD_CERT="--cacert /etc/kubernetes/pki/etcd/ca.crt --cert /etc/kubernetes/pki/etcd/healthcheck-client.crt --key /etc/kubernetes/pki/etcd/healthcheck-client.key"
export ETCD_ARGS="--name=${HOSTNAME} \
--data-dir=/var/lib/etcd \
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

ETCD_ENDPOINTS() {
    EPS=""
    for i in $(seq 0 $((${MEMBER_COUNT} - ${ID}))); do
        EPS="${EPS}${EPS:+,}https://etcd-${i}.etcd.${NAMESPACE}:2379"
    done
    echo ${EPS}
}

if [ -f /var/lib/cache/start ]; then
  /var/lib/cache/start
fi

if [ ! -f /var/lib/cache/etcd_member_count ];then
  echo "MEMBER_NUMBER=${MEMBER_COUNT}" > /var/lib/cache/etcd_member_count
fi

if [ -f /var/lib/cache/envs ]; then
    source /var/lib/cache/envs
    echo exec /usr/local/bin/etcd \
        --initial-cluster=${ETCD_INITIAL_CLUSTER} \
        --initial-cluster-state=${ETCD_INITIAL_CLUSTER_STATE} \
        --initial-advertise-peer-urls=${ETCD_INITIAL_ADVERTISE_PEER_URLS} ${ETCD_ARGS} > /var/lib/cache/start
else
    if [ -d /var/lib/etcd/member ]; then
        echo exec /usr/local/bin/etcd \
            --initial-cluster=${HOSTNAME}=https://${HOSTNAME}.etcd.${NAMESPACE}:2380 \
            --initial-cluster-state=new \
            --initial-advertise-peer-urls=https://${HOSTNAME}.etcd.${NAMESPACE}:2380 ${ETCD_ARGS} > /var/lib/cache/start
    else
        if [ ${ID} -eq 0 ]; then
            echo exec /usr/local/bin/etcd \
                --initial-cluster=${HOSTNAME}=https://${HOSTNAME}.etcd.${NAMESPACE}:2380 \
                --initial-cluster-state=new \
                --initial-advertise-peer-urls=https://${HOSTNAME}.etcd.${NAMESPACE}:2380 ${ETCD_ARGS} > /var/lib/cache/start
        fi

        echo "Adding ${HOSTNAME} from etcd cluster"
        etcdctl --endpoints https://etcd-0.etcd.${NAMESPACE}:2379 ${ETCD_CERT} member add ${HOSTNAME} --peer-urls=https://${HOSTNAME}.etcd.${NAMESPACE}:2380 | grep "^ETCD_" >/var/lib/cache/envs
        source /var/lib/cache/envs
        echo exec /usr/local/bin/etcd \
            --initial-cluster=${ETCD_INITIAL_CLUSTER} \
            --initial-cluster-state=${ETCD_INITIAL_CLUSTER_STATE} \
            --initial-advertise-peer-urls=${ETCD_INITIAL_ADVERTISE_PEER_URLS} ${ETCD_ARGS} > /var/lib/cache/start
    fi
fi

chmod +x /var/lib/cache/start
/var/lib/cache/start