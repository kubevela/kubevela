CONTAINER_ID=$(docker exec kind-control-plane crictl ps | grep 'kubevela\s' | grep --regexp  '^.............' -o)
DOCKER_DIR=$(docker exec kind-control-plane crictl inspect --output go-template --template '{{range .info.runtimeSpec.mounts}}{{if (eq .destination "/workspace/data")}}{{.source}}{{end}}{{end}}' "${CONTAINER_ID}")
echo "${CONTAINER_ID}"
echo "${DOCKER_DIR}"

docker exec kind-control-plane crictl exec "${CONTAINER_ID}" kill -2 1

file=$DOCKER_DIR/e2e-profile.out
echo "$file"
n=1
while [ $n -le 60 ];do
    if_exist=$(docker exec kind-control-plane sh -c "test -f $file && echo 'ok'")
    echo "$if_exist"
    if [ -n  "$if_exist" ];then
        docker exec kind-control-plane cat "$file" > /tmp/e2e-profile.out
        break
    fi
    echo file not generated yet
    n="$(expr $n + 1)"
    sleep 1
done
