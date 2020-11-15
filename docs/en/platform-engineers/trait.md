# Extending Traits in KubeVela

In the following tutorial, you will learn how to add a new trait and expose it to users via Appfile.

### Step 1: Install KubeWatch via Cap Center

Add cap center that contains KubeWatch:

```bash
$ vela cap center config my-center https://github.com/oam-dev/catalog/tree/master/registry
successfully sync 2/2 from my-center remote center
Successfully configured capability center my-center and sync from remote

$ vela cap center sync my-center
successfully sync 2/2 from my-center remote center
sync finished
```

Install KubeWatch:

```bash
$ vela cap install my-center/kubewatch
Installing trait capability kubewatch
...
Successfully installed chart (kubewatch) with release name (kubewatch)
Successfully installed capability kubewatch from my-center


```

### Step 2: Verify Kubewatch Trait Added


```bash
$ vela traits
Synchronizing capabilities from cluster⌛ ...
Sync capabilities successfully ✅ (no changes)
TYPE          CATEGORY    DESCRIPTION
kubewatch     trait       Add a watch for resource
...
```

### Step 3: Adding Kubewatch Trait to The App

Write an Appfile:

```bash
$ cat << EOF > vela.yaml
name: testapp
services:
  testsvc:
    type: webservice
    image: crccheck/hello-world
    port: 8000
    route:
      domain: testsvc.example.com
EOF
```

Deploy it:

```bash
$ vela up
```

Now add `kubewatch` config to Appfile:

```bash
$ cat << EOF >> vela.yaml
    kubewatch:
      webhook: https://hooks.slack.com/<your-token>
EOF
```

Update deployment:

```
$ vela up
```

Check your Slack channel to verify the nofitications:

![Image of Kubewatch](../../resources/kubewatch-notif.jpg)

