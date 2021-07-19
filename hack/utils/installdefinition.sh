#! /bin/bash
DEF_PATH="charts/vela-core/templates/defwithtemplate"

function check_install() {
  res=`kubectl get namespace -A | grep vela-system`
  if [ -n "$res" ];then
    echo 'vela-system namespace exist'
  else
    echo 'vela-system namespace do not exist'
    echo 'creating vela-system namespace ...'
    kubectl create namespace vela-system
  fi
  echo "applying definitions ..."
  cd $DEF_PATH

  for file in *.yaml ;
    do
      echo "Info: changing "$DEF_PATH"/"$file
      sed -i.bak "s#{{.Values.systemDefinitionNamespace}}#vela-system#g" $file
      kubectl apply -f $file
      rm $file
      mv $file".bak" $file
    done
}

check_install