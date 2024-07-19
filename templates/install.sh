#!/bin/bash
#set -x
# apt-get install iptables ipvsadm ipset -y
command_exists() {
	command -v "$@" > /dev/null 2>&1
}

if ! command_exists iptables; then
  cat >&2 <<-'EOF'
Error: Kubernetes worker node needs this command ,please install 'iptables' first and try again.
EOF
  exit 1
fi

if ! command_exists ipvsadm; then
  cat >&2 <<-'EOF'
Error: Kubernetes worker node needs this command ,please install 'ipvsadm' first and try again.
EOF
  exit 1
fi

if ! command_exists ipset; then
  cat >&2 <<-'EOF'
Error: Kubernetes worker node needs this command ,please install 'ipset' first and try again.
EOF
  exit 1
fi

while [ $# -gt 0 ]; do
  case "$1" in
  --name)
    export NAME="$2"
    shift
    ;;
  --*)
    echo "Illegal option $1"
    ;;
  esac
  shift $(($# > 0 ? 1 : 0))
done




if command_exists firewalld; then
  echo "-> Close Firewalld."
  systemctl stop firewalld.service
  systemctl disable firewalld.service
fi

if command_exists ufw; then
  echo "-> Close ufw."
  systemctl stop ufw.service
  systemctl disable ufw.service
fi

#set -e
if [ -f /etc/selinux/config ]; then
  echo "-> Close SELinux."
  sed -i "s/^SELINUX=.*/SELINUX=disabled/g" /etc/selinux/config
fi

echo "-> Setup sysctl."
cat >/etc/modules-load.d/99-kubernetes.conf <<EOF
br_netfilter
nf_conntrack
overlay
xt_REDIRECT
xt_owner
ip_tables
ip6table_filter
ip_vs
ip_vs_rr
ip_vs_wrr
ip_vs_sh
EOF

cat >/etc/sysctl.d/99-kubernetes.conf <<EOF
fs.file-max=52706963
fs.inotify.max_user_instances=524288
fs.inotify.max_user_watches=524288
fs.may_detach_mounts=1
fs.nr_open=52706963
kernel.pid_max=65535
net.bridge.bridge-nf-call-arptables=1
net.bridge.bridge-nf-call-ip6tables=1
net.bridge.bridge-nf-call-iptables=1
net.core.default_qdisc=fq
net.core.netdev_max_backlog=10000
net.core.rmem_max=2500000
net.ipv4.conf.all.arp_announce=2
net.ipv4.conf.all.rp_filter=0
net.ipv4.conf.default.arp_announce=2
net.ipv4.conf.default.forwarding=1
net.ipv4.conf.default.rp_filter=0
net.ipv4.conf.lo.arp_announce=2
net.ipv4.ip_forward=1
net.ipv4.neigh.default.gc_stale_time=120
net.ipv4.neigh.default.gc_thresh1=1024
net.ipv4.neigh.default.gc_thresh2=4096
net.ipv4.neigh.default.gc_thresh3=8192
net.ipv4.tcp_congestion_control=bbr
net.ipv4.tcp_keepalive_intvl=30
net.ipv4.tcp_keepalive_probes=10
net.ipv4.tcp_keepalive_time=600
net.ipv4.tcp_max_syn_backlog=1024
net.ipv4.tcp_max_tw_buckets=5000
net.ipv4.tcp_synack_retries=2
net.ipv4.tcp_syncookies=1
net.ipv6.conf.all.disable_ipv6=0
net.ipv6.conf.all.forwarding=1
net.ipv6.conf.default.disable_ipv6=0
net.ipv6.conf.lo.disable_ipv6=0
net.ipv6.ip_forward=1
net.netfilter.nf_conntrack_buckets=655360
net.netfilter.nf_conntrack_max=10485760
net.netfilter.nf_conntrack_tcp_timeout_close_wait=3600
net.netfilter.nf_conntrack_tcp_timeout_established=300
vm.max_map_count=262144
vm.overcommit_memory=1
vm.panic_on_oom=0
vm.swappiness=0
EOF
sysctl --system > /dev/null 2>&1
echo "-> Close swap."
swapoff -a

echo "-> Install CNI."
# Install CNI
mkdir -p /opt/cni/bin
wget https://github.com/containernetworking/plugins/releases/download/v1.5.1/cni-plugins-linux-amd64-v1.5.1.tgz -O /usr/local/src/cni-plugins-linux-amd64-v1.5.1.tgz
tar -zxf /usr/local/src/cni-plugins-linux-amd64-v1.5.1.tgz --exclude LICENSE --exclude README.md -C /opt/cni/bin

echo "-> Install CNI."
# Install runc
wget https://github.com/opencontainers/runc/releases/download/v{{ .Runc }}/runc.amd64 -O /usr/local/bin/runc
chmod +x /usr/local/bin/runc

echo "-> Install Containerd."
# Install containerd
mkdir -p /etc/containerd
wget https://github.com/containerd/containerd/releases/download/v{{ .Containerd }}/containerd-{{ .Containerd }}-linux-amd64.tar.gz -O /usr/local/src/containerd-{{ .Containerd }}-linux-amd64.tar.gz
tar -zxf /usr/local/src/containerd-{{ .Containerd }}-linux-amd64.tar.gz --strip-components=1 -C /usr/local/bin
cat >/etc/containerd/config.toml <<EOF
version = 2
root = "/data/containerd"
state = "/run/containerd"
temp = ""
disabled_plugins = []
imports = []
oom_score = 0
plugin_dir = ""
required_plugins = []


[cgroup]
 path = ""

[debug]
 address = "/run/containerd/debug.sock"
 level = "info"
 format = "json"
 gid = 0
 uid = 0

[grpc]
 address = "/run/containerd/containerd.sock"
 gid = 0
 max_recv_message_size = 16777216
 max_send_message_size = 16777216
 tcp_address = ""
 tcp_tls_ca = ""
 tcp_tls_cert = ""
 tcp_tls_key = ""
 uid = 0

[metrics]
 address = "127.0.0.1:1338"
 grpc_histogram = false

[plugins]

 [plugins."io.containerd.gc.v1.scheduler"]
   deletion_threshold = 0
   mutation_threshold = 100
   pause_threshold = 0.02
   schedule_delay = "0s"
   startup_delay = "100ms"

 [plugins."io.containerd.grpc.v1.cri"]
   cdi_spec_dirs = ["/etc/cdi", "/var/run/cdi"]
   device_ownership_from_security_context = false
   disable_apparmor = false
   disable_cgroup = false
   disable_hugetlb_controller = true
   disable_proc_mount = false
   disable_tcp_service = true
   drain_exec_sync_io_timeout = "0s"
   enable_cdi = false
   enable_selinux = false
   enable_tls_streaming = false
   enable_unprivileged_icmp = false
   enable_unprivileged_ports = false
   ignore_deprecation_warnings = []
   ignore_image_defined_volumes = false
   image_pull_progress_timeout = "5m0s"
   image_pull_with_sync_fs = false
   max_concurrent_downloads = 3
   max_container_log_line_size = 16384
   netns_mounts_under_state_dir = false
   restrict_oom_score_adj = false
   sandbox_image = "{{ .Registry }}/pause:{{ .Pause }}"
   selinux_category_range = 1024
   stats_collect_period = 10
   stream_idle_timeout = "4h0m0s"
   stream_server_address = "127.0.0.1"
   stream_server_port = "0"
   systemd_cgroup = false
   tolerate_missing_hugetlb_controller = true
   unset_seccomp_profile = ""

   [plugins."io.containerd.grpc.v1.cri".cni]
	 bin_dir = "/opt/cni/bin"
	 conf_dir = "/etc/cni/net.d"
	 conf_template = ""
	 ip_pref = ""
	 max_conf_num = 1
	 setup_serially = false

   [plugins."io.containerd.grpc.v1.cri".containerd]
	 default_runtime_name = "runc"
	 disable_snapshot_annotations = true
	 discard_unpacked_layers = false
	 ignore_blockio_not_enabled_errors = false
	 ignore_rdt_not_enabled_errors = false
	 no_pivot = false
	 snapshotter = "overlayfs"

	 [plugins."io.containerd.grpc.v1.cri".containerd.default_runtime]
	   base_runtime_spec = ""
	   cni_conf_dir = ""
	   cni_max_conf_num = 0
	   container_annotations = []
	   pod_annotations = []
	   privileged_without_host_devices = false
	   privileged_without_host_devices_all_devices_allowed = false
	   runtime_engine = ""
	   runtime_path = ""
	   runtime_root = ""
	   runtime_type = ""
	   sandbox_mode = ""
	   snapshotter = ""

	   [plugins."io.containerd.grpc.v1.cri".containerd.default_runtime.options]

	 [plugins."io.containerd.grpc.v1.cri".containerd.runtimes]

	   [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
		 base_runtime_spec = ""
		 cni_conf_dir = ""
		 cni_max_conf_num = 0
		 container_annotations = []
		 pod_annotations = []
		 privileged_without_host_devices = false
		 privileged_without_host_devices_all_devices_allowed = false
		 runtime_engine = ""
		 runtime_path = ""
		 runtime_root = ""
		 runtime_type = "io.containerd.runc.v2"
		 sandbox_mode = "podsandbox"
		 snapshotter = ""

		 [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
		   BinaryName = ""
		   CriuImagePath = ""
		   CriuPath = ""
		   CriuWorkPath = ""
		   IoGid = 0
		   IoUid = 0
		   NoNewKeyring = false
		   NoPivotRoot = false
		   Root = ""
		   ShimCgroup = ""
		   SystemdCgroup = true

	 [plugins."io.containerd.grpc.v1.cri".containerd.untrusted_workload_runtime]
	   base_runtime_spec = ""
	   cni_conf_dir = ""
	   cni_max_conf_num = 0
	   container_annotations = []
	   pod_annotations = []
	   privileged_without_host_devices = false
	   privileged_without_host_devices_all_devices_allowed = false
	   runtime_engine = ""
	   runtime_path = ""
	   runtime_root = ""
	   runtime_type = ""
	   sandbox_mode = ""
	   snapshotter = ""

	   [plugins."io.containerd.grpc.v1.cri".containerd.untrusted_workload_runtime.options]

   [plugins."io.containerd.grpc.v1.cri".image_decryption]
	 key_model = "node"

   [plugins."io.containerd.grpc.v1.cri".registry]
	 config_path = ""

	 [plugins."io.containerd.grpc.v1.cri".registry.auths]

	 [plugins."io.containerd.grpc.v1.cri".registry.configs]

	 [plugins."io.containerd.grpc.v1.cri".registry.headers]

	 [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
      [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
          endpoint = [ "https://docker.m.moby.org.cn" ]
      [plugins."io.containerd.grpc.v1.cri".registry.mirrors."registry.k8s.io"]
          endpoint = [ "https://k8s.m.moby.org.cn" ]
      [plugins."io.containerd.grpc.v1.cri".registry.mirrors."quay.io"]
          endpoint = [ "https://quay.m.moby.org.cn" ]
   [plugins."io.containerd.grpc.v1.cri".x509_key_pair_streaming]
	 tls_cert_file = ""
	 tls_key_file = ""

 [plugins."io.containerd.internal.v1.opt"]
   path = "/opt/containerd"

 [plugins."io.containerd.internal.v1.restart"]
   interval = "10s"

 [plugins."io.containerd.internal.v1.tracing"]
   sampling_ratio = 1.0
   service_name = "containerd"

 [plugins."io.containerd.metadata.v1.bolt"]
   content_sharing_policy = "shared"

 [plugins."io.containerd.monitor.v1.cgroups"]
   no_prometheus = false

 [plugins."io.containerd.nri.v1.nri"]
   disable = true
   disable_connections = false
   plugin_config_path = "/etc/nri/conf.d"
   plugin_path = "/opt/nri/plugins"
   plugin_registration_timeout = "5s"
   plugin_request_timeout = "2s"
   socket_path = "/var/run/nri/nri.sock"

 [plugins."io.containerd.runtime.v1.linux"]
   no_shim = false
   runtime = "runc"
   runtime_root = ""
   shim = "containerd-shim"
   shim_debug = false

 [plugins."io.containerd.runtime.v2.task"]
   platforms = ["linux/amd64"]
   sched_core = false

 [plugins."io.containerd.service.v1.diff-service"]
   default = ["walking"]

 [plugins."io.containerd.service.v1.tasks-service"]
   blockio_config_file = ""
   rdt_config_file = ""

 [plugins."io.containerd.snapshotter.v1.aufs"]
   root_path = ""

 [plugins."io.containerd.snapshotter.v1.blockfile"]
   fs_type = ""
   mount_options = []
   root_path = ""
   scratch_file = ""

 [plugins."io.containerd.snapshotter.v1.btrfs"]
   root_path = ""

 [plugins."io.containerd.snapshotter.v1.devmapper"]
   async_remove = false
   base_image_size = ""
   discard_blocks = false
   fs_options = ""
   fs_type = ""
   pool_name = ""
   root_path = ""

 [plugins."io.containerd.snapshotter.v1.native"]
   root_path = ""

 [plugins."io.containerd.snapshotter.v1.overlayfs"]
   mount_options = []
   root_path = ""
   sync_remove = false
   upperdir_label = false

 [plugins."io.containerd.snapshotter.v1.zfs"]
   root_path = ""

 [plugins."io.containerd.tracing.processor.v1.otlp"]
   endpoint = ""
   insecure = false
   protocol = ""

 [plugins."io.containerd.transfer.v1.local"]
   config_path = ""
   max_concurrent_downloads = 3
   max_concurrent_uploaded_layers = 3

   [[plugins."io.containerd.transfer.v1.local".unpack_config]]
	 differ = ""
	 platform = "linux/amd64"
	 snapshotter = "overlayfs"

[proxy_plugins]

[stream_processors]

 [stream_processors."io.containerd.ocicrypt.decoder.v1.tar"]
   accepts = ["application/vnd.oci.image.layer.v1.tar+encrypted"]
   args = ["--decryption-keys-path", "/etc/containerd/ocicrypt/keys"]
   env = ["OCICRYPT_KEYPROVIDER_CONFIG=/etc/containerd/ocicrypt/ocicrypt_keyprovider.conf"]
   path = "ctd-decoder"
   returns = "application/vnd.oci.image.layer.v1.tar"

 [stream_processors."io.containerd.ocicrypt.decoder.v1.tar.gzip"]
   accepts = ["application/vnd.oci.image.layer.v1.tar+gzip+encrypted"]
   args = ["--decryption-keys-path", "/etc/containerd/ocicrypt/keys"]
   env = ["OCICRYPT_KEYPROVIDER_CONFIG=/etc/containerd/ocicrypt/ocicrypt_keyprovider.conf"]
   path = "ctd-decoder"
   returns = "application/vnd.oci.image.layer.v1.tar+gzip"

[timeouts]
 "io.containerd.timeout.bolt.open" = "0s"
 "io.containerd.timeout.metrics.shimstats" = "2s"
 "io.containerd.timeout.shim.cleanup" = "5s"
 "io.containerd.timeout.shim.load" = "5s"
 "io.containerd.timeout.shim.shutdown" = "3s"
 "io.containerd.timeout.task.state" = "2s"

[ttrpc]
 address = ""
 gid = 0
 uid = 0
EOF

cat >/etc/systemd/system/containerd.service <<EOF
# Copyright The containerd Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

[Unit]
Description=containerd container runtime
Documentation=https://containerd.io
After=network.target local-fs.target

[Service]
Slice=podruntime.slice
#uncomment to enable the experimental sbservice (sandboxed) version of containerd/cri integration
#Environment="ENABLE_CRI_SANDBOXES=sandboxed"
ExecStartPre=-/sbin/modprobe overlay
ExecStartPre=/usr/bin/env iptables -P FORWARD ACCEPT
ExecStart=/usr/local/bin/containerd

Type=notify
Delegate=yes
KillMode=process
Restart=always
RestartSec=5
# Having non-zero Limit*s causes performance problems due to accounting overhead
# in the kernel. We recommend using cgroups to do container-local accounting.
LimitNPROC=infinity
LimitCORE=infinity
LimitNOFILE=infinity
# Comment TasksMax if your systemd version does not supports it.
# Only systemd 226 and above support this version.
TasksMax=infinity
OOMScoreAdjust=-999

[Install]
WantedBy=multi-user.target
EOF

# Install kubelet
echo "-> Install kubelet."
mkdir -p /var/lib/kubelet /etc/kubernetes/pki /etc/kubernetes/manifests
wget https://dl.k8s.io/{{ .Kubernetes }}/bin/linux/amd64/kubelet -O /usr/local/bin/kubelet
chmod +x /usr/local/bin/kubelet
cat >/etc/kubernetes/pki/ca.crt <<EOF
{{ .Ca }}EOF

cat >/etc/kubernetes/pki/ca.key <<EOF
{{ .Key }}EOF

if [ -f /run/systemd/resolve/resolv.conf ]; then
  RESOLVCONF="/run/systemd/resolve/resolv.conf"
else
  RESOLVCONF="/etc/resolv.conf"
fi
pushd /etc/kubernetes/pki
openssl genrsa -out kubelet.key 2048
openssl req -new -key kubelet.key -subj "/CN=system:node:$(hostname -f)/O=system:nodes" -out kubelet.csr
openssl x509 -req -in kubelet.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 10000 -out kubelet.crt \
-extfile <(printf "keyUsage=digitalSignature,keyEncipherment\nextendedKeyUsage=clientAuth\nsubjectAltName=DNS:$(hostname -f),IP:$(hostname -I | awk '{print $1}')")
popd

cat >/var/lib/kubelet/config.yaml <<EOF
address: "0.0.0.0"
healthzBindAddress: 0.0.0.0
apiVersion: kubelet.config.k8s.io/v1beta1
authentication:
  anonymous:
    enabled: false
  webhook:
    cacheTTL: 2m0s
    enabled: true
  x509:
    clientCAFile: /etc/kubernetes/pki/ca.crt
authorization:
  mode: AlwaysAllow
  webhook:
    cacheAuthorizedTTL: 5m0s
    cacheUnauthorizedTTL: 30s
cgroupDriver: systemd
cgroupsPerQOS: true
clusterDNS:
- {{ .ClusterDNS }}
clusterDomain: cluster.local
configMapAndSecretChangeDetectionStrategy: Watch
containerLogMaxFiles: 5
containerLogMaxSize: 10Mi
contentType: application/vnd.kubernetes.protobuf
cpuCFSQuota: true
cpuCFSQuotaPeriod: 100ms
cpuManagerPolicy: none
cpuManagerReconcilePeriod: 10s
enableControllerAttachDetach: true
enableDebuggingHandlers: true
enableSystemLogHandler: true
enforceNodeAllocatable:
- pods
eventBurst: 100
eventRecordQPS: 50
evictionHard:
  imagefs.available: 10%
  memory.available: 500Mi
  nodefs.available: 10%
  nodefs.inodesFree: 5%
evictionMaxPodGracePeriod: 30
evictionPressureTransitionPeriod: 5m
evictionSoft:
  imagefs.available: 15%
  memory.available: 512Mi
  nodefs.available: 15%
  nodefs.inodesFree: 10%
evictionSoftGracePeriod:
  imagefs.available: 3m
  memory.available: 1m
  nodefs.available: 3m
  nodefs.inodesFree: 1m
kubeReserved:
  cpu: 400m
  memory: 1Gi
  ephemeral-storage: 500Mi
systemReserved:
  cpu: 100m
  memory: 200Mi
  ephemeral-storage: 1Gi
kubeReservedCgroup: /podruntime.slice
systemReservedCgroup: /system.slice
failSwapOn: true
fileCheckFrequency: 10s
hairpinMode: promiscuous-bridge
healthzPort: 10248
httpCheckFrequency: 0s
imageGCHighThresholdPercent: 85
imageGCLowThresholdPercent: 20
imageMinimumGCAge: 2m0s
iptablesDropBit: 15
iptablesMasqueradeBit: 14
kind: KubeletConfiguration
kubeAPIBurst: 10
kubeAPIQPS: 50
makeIPTablesUtilChains: true
maxOpenFiles: 1000000
maxPods: 110
nodeLeaseDurationSeconds: 40
nodeStatusReportFrequency: 1m0s
nodeStatusUpdateFrequency: 10s
oomScoreAdj: -999
podPidsLimit: -1
port: 10250
readOnlyPort: 0
registryBurst: 20
registryPullQPS: 10
resolvConf: ${RESOLVCONF}
rotateCertificates: true
runtimeRequestTimeout: 2m0s
serializeImagePulls: true
staticPodPath: /etc/kubernetes/manifests
streamingConnectionIdleTimeout: 20m0s
syncFrequency: 1m0s
volumeStatsAggPeriod: 1m0s
tlsCertFile: /etc/kubernetes/pki/kubelet.crt
tlsPrivateKeyFile: /etc/kubernetes/pki/kubelet.key
volumePluginDir: /usr/libexec/kubernetes/kubelet-plugins/volume/exec/
tlsCipherSuites:
  - TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
  - TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
  - TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305
  - TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
  - TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305
  - TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
  - TLS_RSA_WITH_AES_256_GCM_SHA384
  - TLS_RSA_WITH_AES_128_GCM_SHA256
EOF
cat >/etc/kubernetes/kubelet.kubeconfig <<EOF
apiVersion: v1
clusters:
  - cluster:
      certificate-authority: /etc/kubernetes/pki/ca.crt
      server: https://{{ .LoadBalancer }}:6443
    name: kubernetes
contexts:
  - context:
      cluster: kubernetes
      user: system:node:$(hostname -f)
    name: system:node:$(hostname -f)@kubernetes
current-context: system:node:$(hostname -f)@kubernetes
kind: Config
preferences: {}
users:
  - name: system:node:$(hostname -f)
    user:
      client-certificate: /etc/kubernetes/pki/kubelet.crt
      client-key: /etc/kubernetes/pki/kubelet.key
EOF

cat >/etc/systemd/system/kubelet.service <<EOF
[Unit]
Description=kubelet: The Kubernetes Node Agent
Documentation=https://kubernetes.io/docs/home/
Wants=network-online.target

[Service]
Slice=podruntime.slice
ExecStart=/usr/local/bin/kubelet \\
 --kubeconfig=/etc/kubernetes/kubelet.kubeconfig \\
 --config=/var/lib/kubelet/config.yaml \\
 --pod-infra-container-image="{{ .Registry }}/pause:{{ .Pause }}" \\
 --runtime-request-timeout=15m \\
 --container-runtime-endpoint=unix:///run/containerd/containerd.sock \\
 --container-runtime=remote \\
 --anonymous-auth=false \\
 --authorization-mode=Webhook \\
 --allowed-unsafe-sysctls=net.* \\
 --v=1

Restart=on-failure
StartLimitInterval=0
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# Install kube-proxy
echo "-> Install kube-proxy."
wget https://dl.k8s.io/{{ .Kubernetes }}/bin/linux/amd64/kube-proxy -O /usr/local/bin/kube-proxy
chmod +x /usr/local/bin/kube-proxy
pushd /etc/kubernetes/pki
openssl genrsa -out kube-proxy.key 2048
openssl req -new -key kube-proxy.key -subj "/CN=system:kube-proxy/O=system:node-proxier" -out kube-proxy.csr
openssl x509 -req -in kube-proxy.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 10000 -out kube-proxy.crt \
-extfile <(printf "keyUsage=digitalSignature,keyEncipherment\nextendedKeyUsage=clientAuth\nsubjectAltName=DNS:$(hostname -f),IP:$(hostname -I | awk '{print $1}')")
popd
cat >/etc/systemd/system/kube-proxy.service <<EOF
[Unit]
Description=Kubernetes Kube Proxy
Documentation=https://github.com/kubernetes/kubernetes
After=network.target

[Service]
ExecStart=/usr/local/bin/kube-proxy \\
 --config=/etc/kubernetes/kube-proxy.yaml \\
 --ipvs-scheduler=wrr \\
 --ipvs-min-sync-period=5s \\
 --ipvs-sync-period=5s \\
 --v=1

Restart=always
RestartSec=10s
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
cat >/etc/kubernetes/kube-proxy.yaml <<EOF
apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
bindAddress: "0.0.0.0"
metricsBindAddress: "0.0.0.0:10249"
clientConnection:
  acceptContentTypes: ""
  burst: 10
  contentType: application/vnd.kubernetes.protobuf
  kubeconfig: /etc/kubernetes/kube-proxy.kubeconfig
  qps: 5
clusterCIDR: "{{ .ServiceSubnet }}"
configSyncPeriod: 15m0s
conntrack:
  max: null
  maxPerCore: 32768
  min: 131072
  tcpCloseWaitTimeout: 1h0m0s
  tcpEstablishedTimeout: 24h0m0s
enableProfiling: false
healthzBindAddress: 127.0.0.1:10256
iptables:
  masqueradeAll: true
  masqueradeBit: 14
  minSyncPeriod: 0s
  syncPeriod: 30s
ipvs:
  strictARP: true
  excludeCIDRs: null
  minSyncPeriod: 5s
  scheduler: "wrr"
  syncPeriod: 30s
mode: "ipvs"
nodePortAddresses: null
oomScoreAdj: -999
portRange: ""
resourceContainer: /kube-proxy
udpIdleTimeout: 250ms
EOF
cat >/etc/kubernetes/kube-proxy.kubeconfig <<EOF
apiVersion: v1
clusters:
  - cluster:
      certificate-authority: /etc/kubernetes/pki/ca.crt
      server: https://{{ .LoadBalancer }}:6443
    name: kubernetes
contexts:
  - context:
      cluster: kubernetes
      user: kube-proxy
    name: kubernetes
current-context: kubernetes
kind: Config
preferences: {}
users:
  - name: kube-proxy
    user:
      client-certificate: /etc/kubernetes/pki/kube-proxy.crt
      client-key: /etc/kubernetes/pki/kube-proxy.key
EOF
echo "-> Start service, please run 'kubectl get no' command on master check the node status."
systemctl daemon-reload
systemctl restart kube-proxy.service kubelet.service  containerd.service
systemctl enable kube-proxy.service kubelet.service  containerd.service
