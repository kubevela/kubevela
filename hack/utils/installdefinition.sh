#! /bin/bash
export PATH=/bin:/usr/bin:$PATH
DEFPATH="../charts/vela-core/templates/defwithtemplate"
function read_dir(){
# shellcheck disable=SC2045
for file in `ls $DEFPATH `
  do
    if [ -d $DEFPATH"/"$file ]
    then
    read_dir $DEFPATH"/"$file
    else
      echo $DEFPATH"/"$file
      sed -i "s#{{.Values.systemDefinitionNamespace}}#vela-system#g" $DEFPATH"/"$file
      kubectl apply -f  $DEFPATH"/"$file
      sed -i "s#vela-system#{{.Values.systemDefinitionNamespace}}#g" $DEFPATH"/"$file
    fi
  done
}

read_dir