#! /bin/bash
DEF_PATH="charts/vela-core/templates/defwithtemplate"

function check_install() {
  res=`kubectl get namespace -A | grep vela-system`
  if [ -n "$res" ];then
    echo 'checking: vela-system namespace exist'
  else
    echo 'vela-system namespace do not exist'
    echo 'creating vela-system namespace ...'
    kubectl create namespace vela-system
  fi
  echo "applying definitions ..."
  cd "$DEF_PATH"

  for file in *.yaml ;
    do
      echo "Info: changing "$DEF_PATH"/""$file"
      sed -i.bak "s#namespace: {{ include \"systemDefinitionNamespace\" . }}#namespace: vela-system#g" "$file"
      kubectl apply -f "$file"
      rm "$file"
      mv "$file"".bak" "$file"
    done

  cd -
}

check_install

DEF_PATH="charts/vela-core/templates/definitions"

check_install

DEF_PATH="charts/vela-core/templates/velaql"

check_install