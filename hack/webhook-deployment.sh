#!/bin/sh

script_dir=$(cd $(dirname $0); pwd)
base_dir=$(cd $(dirname $script_dir/..);pwd)

kubectl create -f ${base_dir}/deployments/service.yaml
cat ${base_dir}/deployments/service.yaml | ${base_dir}/hack/webhook-patch-ca-bundle.sh | kubectl create -f -
kubectl create -f ${base_dir}/deployments/deployment.yaml
