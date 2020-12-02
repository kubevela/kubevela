# 1. build container that includes vela binary
make cross-build
docker build -t oamdev/argo-tool:v1 -f Dockerfile .
kind load docker-image oamdev/argo-tool:v1

# 2. restarts argo server and insert plugin configMap, vela init container, vela configMap
kubectl -n argocd apply -f vela-cm.yaml
kubectl -n argocd patch cm/argocd-cm -p "$(cat argo-cm.yaml)"
kubectl -n argocd patch deploy/argocd-repo-server -p "$(cat deploy.yaml)"

# 3. apply argo app to test appfile gitops deployment
kubectl -n argocd apply -f app.yaml
