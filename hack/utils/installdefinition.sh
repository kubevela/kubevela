#! /bin/bash
DEFPATH="../../charts/vela-core/templates/defwithtemplate"
function scan_def(){
res=`kubectl get namespace -A | grep vela-system`
if [ -n "$res" ];then
  echo 'vela-system namespace exist'
else
  echo 'vela-system namespace do not exist'
  echo 'creating vela-system namespace ... '
  kubectl create namespace vela-system
fi
echo "applying definitions ..."
cd $DEFPATH
# shellcheck disable=SC2045
for file in `ls .`
  do
    echo "Info: changing "$DEFPATH"/"$file
    sed -i '' "s#{{.Values.systemDefinitionNamespace}}#vela-system#g" $file
    kubectl apply -f $file
    sed -i '' "s#vela-system#{{.Values.systemDefinitionNamespace}}#g" $file
  done
}

scan_def