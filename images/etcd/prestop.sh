#!/bin/sh
set -x
export ETCDCTL_API=3
isLeader() {
	ETCDCTL_API=3 etcdctl --endpoints https://$(hostname).etcd.${NAMESPACE}:2379 \
		--cacert /etc/kubernetes/pki/etcd/ca.crt \
		--cert /etc/kubernetes/pki/etcd/healthcheck-client.crt \
		--key /etc/kubernetes/pki/etcd/healthcheck-client.key \
		endpoint status | awk -F ', ' '{print $5}'
}
member_hash() {
	etcdctl --endpoints https://etcd-0.etcd.${NAMESPACE}:2379 \
		--cacert /etc/kubernetes/pki/etcd/ca.crt \
		--cert /etc/kubernetes/pki/etcd/healthcheck-client.crt \
		--key /etc/kubernetes/pki/etcd/healthcheck-client.key \
		member list | grep https://$(hostname).etcd.${NAMESPACE}:2380 | cut -d',' -f1
}
if [ "$(isLeader)" == "true" ]; then
	ep=""
	for i in $(seq 0 $((9 - 1))); do
		ping -c 1 etcd-${i}.etcd.${NAMESPACE} >/dev/null 2>&1
		if [ $? -eq 0 ]; then
			ep="${ep}${ep:+,}https://etcd-${i}.etcd.${NAMESPACE}:2379"
		fi
	done

	NEW_EP=$(etcdctl --endpoints ${ep} \
		--cacert /etc/kubernetes/pki/etcd/ca.crt \
		--cert /etc/kubernetes/pki/etcd/healthcheck-client.crt \
		--key /etc/kubernetes/pki/etcd/healthcheck-client.key \
		endpoint health | grep healthy | head -n 1 | awk '{print $1}')

	NEW_HASH=$(etcdctl --endpoints ${ep} \
		--cacert /etc/kubernetes/pki/etcd/ca.crt \
		--cert /etc/kubernetes/pki/etcd/healthcheck-client.crt \
		--key /etc/kubernetes/pki/etcd/healthcheck-client.key \
		member list | grep $NEW_EP | awk -F ', ' '{print $1}')

	etcdctl --endpoints ${ep} \
		--cacert /etc/kubernetes/pki/etcd/ca.crt \
		--cert /etc/kubernetes/pki/etcd/healthcheck-client.crt \
		--key /etc/kubernetes/pki/etcd/healthcheck-client.key \
		move-leader ${NEW_HASH}
fi
HOSTNAME=$(hostname)
ID=${HOSTNAME##*[^0-9]}
if [ ${ID} -ne 0 ]; then
	echo "Removing ${HOSTNAME} from etcd cluster"
	etcdctl --endpoints https://etcd-0.etcd.${NAMESPACE}:2379 \
		--cacert /etc/kubernetes/pki/etcd/ca.crt \
		--cert /etc/kubernetes/pki/etcd/healthcheck-client.crt \
		--key /etc/kubernetes/pki/etcd/healthcheck-client.key \
		member remove $(member_hash)
	if [ $? -eq 0 ]; then
		# Remove everything otherwise the cluster will no longer scale-up
		rm -rf /var/lib/etcd/member
	fi
fi
