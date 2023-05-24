. ./hack/e2e/end_e2e_core.sh

DOCKER_CONTAINER=kind-control-plane
OAM_CONTAINER_ID=$(docker exec $DOCKER_CONTAINER crictl ps | grep oam-runtime | grep --regexp  '^.............' -o)
OAM_DOCKER_DIR=$(docker exec $DOCKER_CONTAINER crictl inspect --output go-template --template '{{range .info.runtimeSpec.mounts}}{{if (eq .destination "/workspace/data")}}{{.source}}{{end}}{{end}}' "${OAM_CONTAINER_ID}")
echo "${OAM_CONTAINER_ID}"
echo "${OAM_DOCKER_DIR}"

docker exec $DOCKER_CONTAINER crictl exec "${OAM_CONTAINER_ID}" kill -2 1

file=$OAM_DOCKER_DIR/e2e-profile.out
echo "$file"
n=1
while [ $n -le 60 ];do
    if_exist=$(docker exec $DOCKER_CONTAINER sh -c "test -f $file && echo 'ok'")
    echo "$if_exist"
    if [ -n  "$if_exist" ];then
        docker exec $DOCKER_CONTAINER cat "$file" > /tmp/oam-e2e-profile.out
        break
    fi
    echo file not generated yet
    n="$(expr $n + 1)"
    sleep 1
done