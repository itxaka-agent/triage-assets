#!/bin/sh
unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY
# CASE env: ADMIN_HOST (server host in admin.conf), MANIFEST=1 writes the static pod with the annotation, DUMMY=1 adds 192.168.77.10/24 on dummy0 (no gateway)
E=/envtest; KC=/usr/bin/kubectl
echo "--- case: ADMIN_HOST=$ADMIN_HOST MANIFEST=$MANIFEST MANIFEST_ADDR=$MANIFEST_ADDR NET=$NET"
echo "--- ip route:"; ip route; echo "--- default routes in /proc/net/route:"; awk 'NR>1 && $2=="00000000"' /proc/net/route | wc -l
echo "127.0.0.1 cp.example.internal" >> /etc/hosts
$E/etcd --data-dir /tmp/etcd --listen-client-urls http://127.0.0.1:2379 --advertise-client-urls http://127.0.0.1:2379 --listen-peer-urls http://127.0.0.1:2380 >/tmp/etcd.log 2>&1 &
sleep 2
$E/kube-apiserver --etcd-servers http://127.0.0.1:2379 --bind-address 0.0.0.0 --advertise-address 10.0.0.1 --secure-port 6443 \
  --tls-cert-file /pki/srv.crt --tls-private-key-file /pki/srv.key --client-ca-file /pki/ca.crt \
  --service-account-key-file /pki/sa.pub --service-account-signing-key-file /pki/sa.key --service-account-issuer https://kubernetes.default.svc \
  --authorization-mode RBAC --enable-bootstrap-token-auth --endpoint-reconciler-type none --service-cluster-ip-range 10.0.0.0/24 >/tmp/apiserver.log 2>&1 &
rm -rf /root2; mkdir -p /root2/etc/kubernetes/pki; cp /qa/etc/kubernetes/pki/ca.crt /root2/etc/kubernetes/pki/
sed "s#server: .*#server: https://$ADMIN_HOST:6443#" /qa/etc/kubernetes/admin.conf > /root2/etc/kubernetes/admin.conf
if [ "$MANIFEST" = 1 ]; then mkdir -p /root2/etc/kubernetes/manifests; cat > /root2/etc/kubernetes/manifests/kube-apiserver.yaml <<M
apiVersion: v1
kind: Pod
metadata:
  annotations:
    kubeadm.kubernetes.io/kube-apiserver.advertise-address.endpoint: ${MANIFEST_ADDR:-192.168.77.10:6443}
  name: kube-apiserver
  namespace: kube-system
spec:
  containers:
  - name: kube-apiserver
    image: registry.k8s.io/kube-apiserver:v1.37.0
M
fi
for i in $(seq 1 60); do $KC --kubeconfig /root2/etc/kubernetes/admin.conf get --raw /readyz >/dev/null 2>&1 && break; sleep 1; done
echo "--- apiserver ready after ${i}s"
for v in base pr; do
  $KC --kubeconfig /root2/etc/kubernetes/admin.conf -n kube-system delete secret kubeadm-certs --ignore-not-found >/dev/null 2>&1
  # keep the transient kubeadm config for inspection
  echo "=========== $v: mint-join --role controlplane"
  ( while :; do for f in /run/kubeadm-*.yaml; do [ -f "$f" ] && cp "$f" /tmp/cfg-$v.yaml; done; done ) & W=$!
  /bins/bin-$v mint-join --role controlplane --root-path /root2 > /tmp/out-$v.txt 2>/tmp/err-$v.txt; echo "exit=$?"; kill $W
  echo "stderr:"; grep -v 'dl.k8s.io\|falling back to the local client' /tmp/err-$v.txt | tail -8
  echo "stdout (secrets redacted):"; sed -E 's/[a-z0-9]{6}\.[a-z0-9]{16}/<token>/g; s/[a-f0-9]{64}/<hex64>/g' /tmp/out-$v.txt | head -20
  echo "localAPIEndpoint in the transient upload-certs config:"; grep -A2 localAPIEndpoint /tmp/cfg-$v.yaml 2>/dev/null || echo "(config not captured)"
  echo -n "kubeadm-certs secret: "; $KC --kubeconfig /root2/etc/kubernetes/admin.conf -n kube-system get secret kubeadm-certs -o go-template="created {{.metadata.creationTimestamp}}{{range \$k,\$v := .data}} {{\$k}}={{len \$v}}B{{end}}" 2>&1; echo
done
