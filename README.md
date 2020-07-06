# RudrX

RudrX is a command-line tool to use OAM based micro-app engine.

## Use with command-line

1. Install Template  CRD into your cluster 
```shell script
make install
```

2. Install template object 

```shell script
kubectl apply -f config/samples/
```

3. rudrx run

```bash
rudrx run containerized frontend -p 80 -i oam-dev/demo:v1
```
