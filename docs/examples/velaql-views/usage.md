

### Usage

You can use velaQL with a syntax similar to promeQL.

The syntax format of velaQL is as follows:

```sql
view{parameter1=value1}.statusKey
```

1. `view` represents different query views, we have built a few views: `component-pod-view`,`pod-view`,`resource-view`
2. `parameter1=value1` represents query configuration items
3. `statusKey`  represents the aggregate result of the query, default is `status`

### component-pod-view

#### describe

List the pods created by specified component

#### parameter

```
parameter: {
	appName:    string // application name
	appNs:      string // application namespace
	name:       string // component name
	cluster?:   string // cluster name(Optional)
	clusterNs?: string // cluster namespace(Optional)
}
```

#### statusKey

`status`

#### query result

```
// query successful
status: {
  podList: [{
    cluster: string
    worload: {
      apiVersion: string
      kind:       string
    }
    metadata: {
      name:         string
      namespace:    string
      creationTime: string
      version: {
        revision:       string
        publishVersion: string
        deployVersion:  string
      }
    }
    status: {
      phase:    "Pending" | "Running" | "Succeeded" | "Failed" | "Unknown"
      // if phase == "Pending" or "Unknown": podIP, hostIP, nodeName will be empty
      podIP:    string
      hostIP:   string
      nodeName: string
    }
  }]
}

// query failed
status: {
  error: string
}
```

#### demo

```sql
component-pod-view{appName=demo,appNs=default,cluster=prod,clusterNs=default,name=web}.status
```

### pod-view

#### describe

Query the pods detail information

#### parameter

```
parameter: {
	name:      string // pod name
	namespace: string // pod namespace
	cluster?:  string // cluster name(Optional)
}
```

#### statusKey

`status`

#### query result

```
// query successful
status: {
	containers: [{
		name:  string
		image: string
		resources: {
			limits: {
				cpu:    string
				memory: string
			}
			requests: {
				cpu:    string
				memory: string
			}
			usage: {
				cpu:    string
				memory: string
			}
		}
		status: {
		    // state holds a possible state of container. 
		    // only one of its members may be specified.
			state: {
				running: {...}
				waiting: {...}
				terminated: {...}
			}
			restartCount: string
		}
	}]
	events: [...v1.Event]
}

// query failed
status: {
  error: string
}
```

#### demo

```
pod-view{name=demo,namespace=default,cluster=prod}.status
```

### resource-view

#### describe

List resources

#### parameter

```
parameter: {
	type:      "ns" | "secret" | "configMap" | "pvc" | "storageClass"
	namespace: *"" | string // Optional
	cluster:   *"" | string // Optional
}
```

#### statusKey

`status`

#### query result

```
// query successful
status: {
	list: [...k8sObject]
}

// query failed
status: {
	error: string
}
```

#### demo

```
resource-view{type=ns,cluster=prod}.status
```


