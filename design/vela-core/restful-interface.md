# Vela RESTful API design

Below, I list the Vela backend RESTful API grouped by each resource.

The first version does not use authentication or authorization. We will use go structure to 
represent request and response body.

## Common properties

### Base URL
The base URL should be **`vela.oam.dev`** but in our first version, we assumed that it's `localhost`.

### Common Request Headers
Below are the common request headers. All fields are required.

| Name          | Type   | Values      | Description          |
|---------------|--------|--------------|----------------------|
| x-oam-version | String | "2020-08-15" | The REST API version  that user wants to use|
| x-oam-client-type| String | "CLI", "Dashboard" |  The type of client    |
| x-oam-request-timestamp | String | yyyy-MM-dd HH:mm:ss.SS | UTC time of the request|
| x-oam-client-id |  String  |    N/A        |   The unique client id |
| x-oam-client-request-id|String|  N/A          |   The unique client request id |

### Common Response Headers

 Name          | Type   | Values      | Description          |
|---------------|--------|--------------|----------------------|
| x-oam-client-request-id|String|  N/A          |   The unique client request id |
| x-oam-response-timestamp | String | yyyy-MM-dd HH:mm:ss.SS | UTC time of the response|


### Common Error Status
| Error code                 |  HTTP status code  | User message                                                                       |
|----------------------------|------------------|------------------------------------------------------------------------------------|
| Already Exists             | 409              | The specific resource already exists                                               |
| Not Found                  | 404              | The specific resource not found                                                    |
| InvalidHeaderValue         | 400              | The value provided for one of the HTTP headers was not in the correct format.      |
| InvalidQueryParameterValue | 400              | An invalid value was specified for one of the query parameters in the request URI. |
| InvalidInput               | 400              | One of the request inputs is not valid.                                            |
| InternalError              | 500              | The server encountered an internal error. Please retry the request.                |

### Common Query parameters
**updateMode**
```go
type updateMode string
const fullcontent updateMode = "fullContent"
const cueTemplate updateMode = "cueTemplate"
```

**qureyMode**
```go
type qureyMode string
const fullcontent qureyMode = "fullContent"
const cueTemplate qureyMode = "cueTemplate"
```

## Environment related API
Environment is a top level resource in the REST API. 
### Create 
**URL** : `/api/envs`

**Method** : `POST`

**Query Parameter** : `None`

**Body** :

```go
type env struct {
    envName   string       `json:"envName"` 
    namespace string       `json:"namespace"`    
}
```

**Responses Body** : None

### Get
**URL** : `/api/envs/${envName}`

**Method** : `GET`

**Query Parameter** : `None`

**Body** : 
```go
type env struct {
    namespace string       `json:"namespace"`      
}
```

**Responses Body** : `None` 

### List 
**URL** : `/api/envs`

**Method** : `GET`

**Query Parameter** : `None`

**Body** : None

**Responses Body** : 
```go
type env struct {
    envNames   []string       `json:"envNames"`  
}
```

### Delete 
**URL** : `/api/envs/${envName}`

**Method** : `Delete`

**Query Parameter** : `None`

**Body** : `None`

**Responses Body** : `None`

## ApplicationConfiguration related API
An app has to have an environment, so it is a sub-resource under an env.

### Create 
**URL** : `/api/envs/${envName}/apps`

**Method** : `POST`

**Query Parameter** : `appCreateMode`
```go
type appCreateMode string
const fullcontent appCreateMode = "parameters"
const cueTemplate appCreateMode = "appFile"
```

**Body** :
```go
type appConfigValue struct {
    appName string       `json:"appName"`
    definition runtime.RawExtension `json:"definition"` // the content
    definitionName string `json:"definitionName"` // use to find the definition
    definitionType string `json:"definitionType"`
}
```

**Responses Body** : None


### Update 
**URL** : `/api/envs/${envName}/apps/${appName}`

**Method** : `PUT`

**Query Parameter** : `appUpdateMode`
```go
type appUpdateMode string
const fullcontent appUpdateMode = "parameters"
const cueTemplate appUpdateMode = "appFile"
```
**Body** :

```go

type appConfigValue struct {
    definition runtime.RawExtension `json:"definition"` // the content
}
```

**Responses Body** : None

### Get

**URL** : `/api/envs/${envName}/apps/${appName}`

**Method** : `Get`

**Query Parameter** : `appQuerymode` 
```go
type appQuerymode string
const fullcontent appQuerymode = "fullContent"
const parameterOnly appQuerymode = "parameterDef"
const statusOnly appQuerymode = "appFile"
const statusOnly appQuerymode = "statusOnly"
```
**Body** : None

**Responses Body** : 
```go
type appConfigValue struct {
    appName string  `json:"appName"` // the app name
    definition runtime.RawExtension `json:"definition"` // the definition
}
```

### List

**URL** : `/api/envs/${envName}/apps`

**Method** : `Get`

**Query Parameter** : `None`

**Body** : None

**Responses Body** : 
```go
type appConfigNames struct {
    appConfigName string[] `json:"appConfigName"` // the appconfig name list, no next Token
}
```

### Delete

**URL** : `/api/envs/${envName}/apps/${appName}`

**Method** : `Delete`

**Query Parameter** : `dryrun` 

**Body** : None

**Responses Body** : None 

## Definition related API

We will add ${definitionType} in the path to differentiate if it's a workload/trait/scope definition.
It can be either "workloadDefinition","traitDefinition", or "scopeDefinition" as they are three different
types of resources.
 
### Create 

**URL** : `/api/${definitionType}`

**Method** : `POST`

**Query Parameter** : `None`

**Body** :

```go

type oamDefinition struct {
	definitionName string `json:"definitionName"`
    definition runtime.RawExtension `json:"definition"` // the real definition data
}
```

### Update 

**URL** : `/api/${definitionType}/${definitionName}`

**Method** : `PUT`

**Query Parameter** : `updateMode` 

**Body** :

```go

type oamDefinition struct {
    definition runtime.RawExtension `json:"definition"` // the definition data
}
```

**Responses Body** : None

### GET

**URL** : `/api/${definitionType}/${definitionName}`

**Method** : `Get`

**Query Parameter** : `qureyMode` 

**Body** : None

**Responses Body** : 
```go
type definition struct {
    definition runtime.RawExtension `json:"definition"` // either the full definition or the cueTemplate
}
```

### List

**URL** : `/api/${definitionType}`

**Method** : `Get`

**Query Parameter** : `None`

**Body** : None

**Responses Body** : 
```go
type definitionName struct {
    definitionNames string[] `json:"definitionNames"` // the definition name list, no next Token
}
```

### Delete

**URL** : `/api/${definitionType}/${definitionName}`

**Method** : `Delete`

**Query Parameter** : `dryrun` 

**Body** : None

**Responses Body** : None 


## Repo related API
These API operate on definition files stored in a git repo. It could be a git repo on the local file system. 

The only valid `categoryName` are `workload`,`trait` and `scope` 

They all have a `repoURL` query parameter that stores the git repo's URL so that the server is stateless. 
 
### Update 

**URL** : `/api/category/${categoryName}/${definitionName}`

**Method** : `PUT`

**Query Parameter** : `repoURL`, `updateMode` 

**Body** :

```go

type oamDefinition struct {
    definition runtime.RawExtension `json:"definition"` // the definition data
}
```

**Responses Body** : None

### GET

**URL** : `/api/category/${categoryName}/${definitionName}`

**Method** : `Get`

**Query Parameter** : `repoURL`, `qureyMode` 

**Body** : None

**Responses Body** : 
```go
type definition struct {
    definition runtime.RawExtension `json:"definition"` // either the full definition or the cueTemplate
}
```

### List

**URL** : `/api/category/${categoryName}`

**Method** : `Get`

**Query Parameter** : `repoURL`, `None`

**Body** : None

**Responses Body** : 
```go
type definitionName struct {
    definitionNames string[] `json:"definitionNames"` // the definition name list, no next Token
}
```

## Version API
This is debatable, we are not sure if this is really needed.

### GET

**URL** : `/api/version`

**Method** : `Get`

**Query Parameter** : None 

**Body** : None

**Responses Body** : 
```go
type definition struct {
    velaServerVersion string `json:"velaServerVersion"`
    k8sServerVersion string `json:"k8sServerVersion"` 
}
```