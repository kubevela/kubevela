# How to write the config to Nacos server

* Step 1: Make sure there are a nacos-server and nacos-config templates

```bash
$ vela config-template list -A
NAMESPACE       NAME                    ALIAS                                   SCOPE   SENSITIVE       CREATED-TIME                 
vela-system     nacos-config            Nacos Configuration                     system  false           2022-10-13 15:39:44 +0800 CST
vela-system     nacos-server            Nacos Server                            system  false           2022-10-13 15:39:47 +0800 CST

# View the document of the properties
$ vela config-template show nacos-server
```

If not exist, please enable the VelaUX addon firstly.

* Step 2: Create a config to added a Nacos server

```bash

# Create a nacos server config, the config name must be "nacos"
$ vela config create nacos --template nacos-server servers[0].ipAddr=127.0.0.1 servers[0].port=8849
```

* Step 3: Create a config to the Nacos server

```bash
# Use the default template, you could define custom template.
$ vela config create db-config --template nacos-config dataId=db group="DEFAULT_GROUP" contentType="properties" content.host=127.0.0.1 content.port=3306 content.username=root 
```

Then, the content will be written to the Nacos server.

```properties
host = 127.0.0.1
port = 3306
username = root
```
