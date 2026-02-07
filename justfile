KUBECONFIG := "/tmp/k"
KIND_CLUSTER := "csi-test"
IMAGE := "justmount:dev"

kind-build:
  docker build -t {{IMAGE}} .

kind-load:
  kind load docker-image {{IMAGE}} --name {{KIND_CLUSTER}}

kind-deploy:
  KUBECONFIG={{KUBECONFIG}} kubectl apply -f examples/kind/justmount-node.yaml
  KUBECONFIG={{KUBECONFIG}} kubectl -n kube-system rollout status ds/justmount-node --timeout=120s

kind-test:
  KUBECONFIG={{KUBECONFIG}} kubectl apply -f examples/kind/tmpfs-static.yaml
  KUBECONFIG={{KUBECONFIG}} kubectl wait --for=condition=Ready pod/tmpfs-consumer --timeout=120s
  KUBECONFIG={{KUBECONFIG}} kubectl exec tmpfs-consumer -- cat /mnt/media/hello.txt

kind-clean:
  KUBECONFIG={{KUBECONFIG}} examples/kind/cleanup.sh

gen:
  go generate ./...
