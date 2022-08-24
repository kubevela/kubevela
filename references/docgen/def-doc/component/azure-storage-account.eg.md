```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: storage-account-dev
spec:
  components:
    - name: storage-account-dev
      type: azure-storage-account
      properties:
        create_rsg: false
        resource_group_name: "weursgappdev01"
        location: "West Europe"
        name: "appdev01"
        tags: |
          {
            ApplicationName       = "Application01"
            Terraform             = "Yes"
          } 
        static_website: |
          [{
            index_document = "index.html"
            error_404_document = "index.html"
          }]

        writeConnectionSecretToRef:
          name: storage-account-dev
          namespace: vela-system
```
