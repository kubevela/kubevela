. ./hack/e2e/end_e2e_core.sh

OAM_CONTAINER_ID=$(docker exec k3d-k3s-default-server-0 crictl ps | grep oam-runtime | grep --regexp  '^.............' -o)
OAM_DOCKER_DIR=$(docker exec k3d-k3s-default-server-0 crictl inspect --output go-template --template '{{range .info.runtimeSpec.mounts}}{{if (eq .destination "/workspace/data")}}{{.source}}{{end}}{{end}}' "${OAM_CONTAINER_ID}")
echo "${OAM_CONTAINER_ID}"
echo "${OAM_DOCKER_DIR}"

docker exec k3d-k3s-default-server-0 crictl exec "${OAM_CONTAINER_ID}" kill -2 1

file=$OAM_DOCKER_DIR/e2e-profile.out
echo "$file"
n=1
while [ $n -le 60 ];do
    if_exist=$(docker exec k3d-k3s-default-server-0 sh -c "test -f $file && echo 'ok'")
    echo "$if_exist"
    if [ -n  "$if_exist" ];then
        docker exec k3d-k3s-default-server-0 cat "$file" > /tmp/oam-e2e-profile.out
        break
    fi
    echo file not generated yet
    n="$(expr $n + 1)"
    sleep 1
done