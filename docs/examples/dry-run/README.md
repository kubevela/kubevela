# Vela Dry run

```shell
$ vela system dry-run -f docs/examples/dry-run/app.yaml -d docs/examples/dry-run/definitions
---
# App application-sample -- Component myweb
---

apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.oam.dev/component: myweb
    app.oam.dev/name: application-sample
    workload.oam.dev/type: myworker
spec:
  selector:
    matchLabels:
      app.oam.dev/component: myweb
  template:
    metadata:
      labels:
        app.oam.dev/component: myweb
    spec:
      containers:
      - command:
        - sleep
        - "1000"
        image: busybox
        name: myweb

---
apiVersion: v1
kind: Service
metadata:
  labels:
    app.oam.dev/component: myweb
    app.oam.dev/name: application-sample
    trait.oam.dev/resource: service
    trait.oam.dev/type: myingress
  name: myweb
spec:
  ports:
  - port: 80
    targetPort: 80
  selector:
    app.oam.dev/component: myweb

---
apiVersion: networking.k8s.io/v1beta1
kind: Ingress
metadata:
  labels:
    app.oam.dev/component: myweb
    app.oam.dev/name: application-sample
    trait.oam.dev/resource: ingress
    trait.oam.dev/type: myingress
  name: myweb
spec:
  rules:
  - host: www.example.com
    http:
      paths:
      - backend:
          serviceName: myweb
          servicePort: 80
        path: /

---

```