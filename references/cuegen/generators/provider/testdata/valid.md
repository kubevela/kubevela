# test

## #Apply


### *Params*

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 cluster | The cluster to use. | string | true |  
 resource | The resource to get or apply. | map[string]_ | true |  
 options | The options to get or apply. | [options](#options) | true |  


#### options

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 threeWayMergePatch | The strategy of the resource. | [threeWayMergePatch](#threewaymergepatch) | true |  


##### threeWayMergePatch

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 enabled | The strategy to get or apply the resource. | bool | false | true 
 annotationPrefix | The annotation prefix to use for the three way merge patch. | string | false | resource 


### *Returns*

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 \- |  | {} | true |  
## #Get


### *Params*

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 cluster | The cluster to use. | string | true |  
 resource | The resource to get or apply. | map[string]_ | true |  
 options | The options to get or apply. | [options](#options) | true |  


#### options

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 threeWayMergePatch | The strategy of the resource. | [threeWayMergePatch](#threewaymergepatch) | true |  


##### threeWayMergePatch

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 enabled | The strategy to get or apply the resource. | bool | false | true 
 annotationPrefix | The annotation prefix to use for the three way merge patch. | string | false | resource 


### *Returns*

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 \- |  | {} | true |  
## #List


### *Params*

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 cluster | The cluster to use. | string | true |  
 filter | The filter to list the resources. | [filter](#filter) | false |  
 resource | The resource to list. | map[string]_ | true |  


#### filter

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 namespace | The namespace to list the resources. | string | false |  
 matchingLabels | The label selector to filter the resources. | map[string]string | false |  


### *Returns*

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 \- |  | {} | true |  
## #Patch


### *Params*

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 cluster | The cluster to use. | string | true |  
 resource | The resource to patch. | map[string]_ | true |  
 patch | The patch to be applied to the resource with kubernetes patch. | [patch](#patch) | true |  


#### patch

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 type | The type of patch being provided. | "merge" or "json" or "strategic" | true |  
 data |  | _ | true |  


### *Returns*

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 \- |  | {} | true |  
------

