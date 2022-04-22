#!/usr/bin/env bash

set -euo pipefail

basedir="./deploy"
# keydir="$(mktemp -d)"
keydir="./deploy"

# Read the PEM-encoded CA certificate, base64 encode it, and replace the `${CA_PEM_B64}` placeholder in the YAML
# template with it. Then, create the Kubernetes resources.
ca_pem_b64="$(openssl base64 -A <"${keydir}/ca.crt")"

set +euo pipefail

sed -e 's@${CA_PEM_B64}@'"$ca_pem_b64"'@g' < "${basedir}/deployment.yaml" | kubectl delete -f -

# Create the TLS secret for the generated keys.
kubectl -n default delete secret kubegames-operator-tls

echo "The operator server has been deployed and configured!"
