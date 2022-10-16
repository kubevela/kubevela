# How to config a dex connector

* Step 1: Make sure there is a dex-connector template

```bash
$ vela config-template list -A
NAMESPACE       NAME                    ALIAS                                   SCOPE   SENSITIVE       CREATED-TIME                 
vela-system     dex-connector           Dex Connector                           system  false           2022-10-12 23:48:05 +0800 CST
vela-system     helm-repository         Helm Repository                         project false           2022-10-14 12:04:58 +0800 CST
vela-system     image-registry          Image Registry                          project false           2022-10-13 15:39:37 +0800 CST

# View the document of the properties
$ vela config-template show dex-connector
```

If not exist, please enable the dex addon firstly.

* Step 2: Create a config

```bash
# Create a connector config
$ vela config create github-oauth --template dex-connector type=github github.clientID=*** github.clientSecret=*** github.redirectURI=***
```

Write a yaml file to create the config.

```bash
$ cat>github.yaml<<EOF

github:
  clientID: ***
  clientSecret: ***
  redirectURI: ***
EOF

$ vela config create github-oauth --template dex-connector type=github -f github.yaml
```
