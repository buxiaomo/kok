#!/bin/sh
set -x
set -ex
export ID=${HOSTNAME##*[^0-9]}
export ETCDCTL_API=3
export HOSTNAME=$(hostname)
export MEMBER_COUNT=$(curl -s --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" "https://kubernetes.default.svc/apis/apps/v1/namespaces/${NAMESPACE}/statefulsets/etcd" | jq .spec.replicas)
export ETCD_CERT="--cacert /etc/kubernetes/pki/etcd/ca.crt --cert /etc/kubernetes/pki/etcd/healthcheck-client.crt --key /etc/kubernetes/pki/etcd/healthcheck-client.key"

ETCD_ENDPOINTS(){
  EPS=""
  for i in $(seq 0 $((${MEMBER_COUNT} - 1))); do
    EPS="${EPS}${EPS:+,}https://etcd-${i}.etcd.${NAMESPACE}:2379"
  done
  echo ${EPS}
}

moveLeader() {
	NEW_EP=$(etcdctl --endpoints $(ETCD_ENDPOINTS) ${ETCD_CERT} endpoint health | grep -v unhealthy | head -n 1 | awk '{print $1}')
	NEW_HASH=$(etcdctl --endpoints $(ETCD_ENDPOINTS) ${ETCD_CERT} member list | grep $NEW_EP | awk -F ', ' '{print $1}')
	etcdctl --endpoints $(ETCD_ENDPOINTS) ${ETCD_CERT} move-leader ${NEW_HASH}
}

isLeader() {
	etcdctl --endpoints https://${HOSTNAME}.etcd.${NAMESPACE}:2379 ${ETCD_CERT} endpoint status | awk -F ', ' '{print $5}'
}

leaderEndpoint() {
	etcdctl --endpoints https://${HOSTNAME}.etcd.${NAMESPACE}:2379 ${ETCD_CERT} endpoint status | awk -F ', ' '{print $5}'
}

selfHash() {
    etcdctl --endpoints https://${HOSTNAME}.etcd.${NAMESPACE}:2379 ${ETCD_CERT} member list | grep https://${HOSTNAME}.etcd.${NAMESPACE}:2380 | cut -d',' -f1
}

if [ -f /etc/kubernetes/pki/etcd/.etcd_member_count ];then
  source /etc/kubernetes/pki/etcd/.etcd_member_count
fi

if [ "$(isLeader)" == "true" ]; then
  echo "Re-elect the leader."
  moveLeader
fi

if [ ${MEMBER_NUMBER} -gt ${MEMBER_COUNT} ] && [ ${ID} -ne 0 ];then
  echo "Cluster reduction, Delete member."
  etcdctl --endpoints $(ETCD_ENDPOINTS) ${ETCD_CERT} member remove $(selfHash)
  rm -rf /etc/kubernetes/pki/etcd/.etcd_member_count /etc/kubernetes/pki/etcd/.envs
  mv /var/lib/etcd/member /var/lib/etcd/member.$(date '+%Y-%m-%dT%H:%M:%S').bak
fi
