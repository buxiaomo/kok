# kok

This project is deploy kubernetes control-plane on k8s.

## quick start

```shell
kind create cluster --name=kok-demo  --image=docker.m.moby.org.cn/kindest/node:v1.29.4

kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.14.4/config/manifests/metallb-native.yaml
cat <<EOF | kubectl apply -f -
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: address-pool
  namespace: metallb-system
spec:
  addresses:
  - 172.16.200.1-172.16.200.10
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: example
  namespace: metallb-system
spec:
  ipAddressPools:
  - address-pool
EOF

helm upgrade -i kok ./kok -n kok --create-namespace

# get EXTERNAL-IP and redeploy
helm upgrade -i kok ./kok -n kok --create-namespace \
--set webhookUrl=http://172.16.200.1:8080 
```

Now you can open the link to create the cluster
* http://<EXTERNAL-IP>:8080/console/cluster