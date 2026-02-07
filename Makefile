KUBECONFIG ?= /tmp/k
KIND_CLUSTER ?= csi-test
IMAGE ?= justmount:dev

.PHONY: kind-build kind-load kind-deploy kind-test kind-clean

kind-build:
\tdocker build -t $(IMAGE) .

kind-load:
\tkind load docker-image $(IMAGE) --name $(KIND_CLUSTER)

kind-deploy:
\tKUBECONFIG=$(KUBECONFIG) kubectl apply -f examples/kind/justmount-node.yaml
\tKUBECONFIG=$(KUBECONFIG) kubectl -n kube-system rollout status ds/justmount-node --timeout=120s

kind-test:
\tKUBECONFIG=$(KUBECONFIG) kubectl apply -f examples/kind/tmpfs-static.yaml
\tKUBECONFIG=$(KUBECONFIG) kubectl wait --for=condition=Ready pod/tmpfs-consumer --timeout=120s
\tKUBECONFIG=$(KUBECONFIG) kubectl exec tmpfs-consumer -- cat /mnt/media/hello.txt

kind-clean:
\tKUBECONFIG=$(KUBECONFIG) examples/kind/cleanup.sh
