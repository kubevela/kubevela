#!/bin/bash

echo "accessKeyID: ${ALICLOUD_ACCESS_KEY}\naccessKeySecret: ${ALICLOUD_SECRET_KEY}\nsecurityToken: ${ALICLOUD_SECURITY_TOKEN}" > alibaba-credentials.conf
kubectl create namespace vela-system
kubectl create secret generic alibaba-account-creds -n vela-system --from-file=credentials=alibaba-credentials.conf
rm -f alibaba-credentials.conf