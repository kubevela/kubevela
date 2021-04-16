---
title: Attach Sidecar
---

In this section, we will show you how to use `sidecar` trait to collect logs.

## Show the Usage of Sidecar

```shell
$ kubectl vela show sidecar
# Properties
+---------+-----------------------------------------+-----------------------+----------+---------+
|  NAME   |               DESCRIPTION               |         TYPE          | REQUIRED | DEFAULT |
+---------+-----------------------------------------+-----------------------+----------+---------+
| name    | Specify the name of sidecar container   | string                | true     |         |
| cmd     | Specify the commands run in the sidecar | []string              | false    |         |
| image   | Specify the image of sidecar container  | string                | true     |         |
| volumes | Specify the shared volume path          | [[]volumes](#volumes) | false    |         |
+---------+-----------------------------------------+-----------------------+----------+---------+


## volumes
+-----------+-------------+--------+----------+---------+
|   NAME    | DESCRIPTION |  TYPE  | REQUIRED | DEFAULT |
+-----------+-------------+--------+----------+---------+
| name      |             | string | true     |         |
| path      |             | string | true     |         |
+-----------+-------------+--------+----------+---------+
```

## Apply the Application

In this Application, component `log-gen-worker` and sidecar share the data volume that saves the logs.
The sidebar will re-output the log to stdout.

```yaml
# app.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: vela-app-with-sidecar
spec:
  components:
    - name: log-gen-worker
      type: worker
      properties:
        image: busybox
        cmd:
          - /bin/sh
          - -c
          - >
            i=0;
            while true;
            do
              echo "$i: $(date)" >> /var/log/date.log;
              i=$((i+1));
              sleep 1;
            done
        volumes:
          - name: varlog
            mountPath: /var/log
            type: emptyDir
      traits:
        - type: sidecar
          properties:
            name: count-log
            image: busybox
            cmd: [ /bin/sh, -c, 'tail -n+1 -f /var/log/date.log']
            volumes:
              - name: varlog
                path: /var/log
```

Apply this Application.

```shell
kubectl apply -f app.yaml
```

Check the workload generate by Application.

```shell
$ kubectl get pod
NAME                              READY   STATUS    RESTARTS   AGE
log-gen-worker-76945f458b-k7n9k   2/2     Running   0          90s
```

Check the output of sidecar. 

```shell
$ kubectl logs -f log-gen-worker-76945f458b-k7n9k count-log
0: Fri Apr 16 11:08:45 UTC 2021
1: Fri Apr 16 11:08:46 UTC 2021
2: Fri Apr 16 11:08:47 UTC 2021
3: Fri Apr 16 11:08:48 UTC 2021
4: Fri Apr 16 11:08:49 UTC 2021
5: Fri Apr 16 11:08:50 UTC 2021
6: Fri Apr 16 11:08:51 UTC 2021
7: Fri Apr 16 11:08:52 UTC 2021
8: Fri Apr 16 11:08:53 UTC 2021
9: Fri Apr 16 11:08:54 UTC 2021 
```