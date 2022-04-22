#!/usr/bin/env bash

# Create the `kubegames` namespace. This cannot be part of the YAML file as we first need to create the TLS secret,
# which would fail otherwise.
echo "Creating Kubernetes objects ..."

set -euo pipefail

basedir="./deploy"
# keydir="$(mktemp -d)"
keydir="./deploy"

# Generate keys into a temporary directory.
echo "Generating TLS keys ..."

"${basedir}/generate-keys.sh" "$keydir"

# Create the TLS secret for the generated keys.
kubectl -n default create secret tls kubegames-operator-tls \
    --cert "${keydir}/kubegames-operator-tls.crt" \
    --key "${keydir}/kubegames-operator-tls.key"

# Read the PEM-encoded CA certificate, base64 encode it, and replace the `${CA_PEM_B64}` placeholder in the YAML
# template with it. Then, create the Kubernetes resources.
ca_pem_b64="$(openssl base64 -A <"${keydir}/ca.crt")"

sed -e 's@${CA_PEM_B64}@'"$ca_pem_b64"'@g' < "${basedir}/deployment.yaml" \
    | kubectl create -f -

# Delete the key directory to prevent abuse (DO NOT USE THESE KEYS ANYWHERE ELSE).
# rm -rf "$keydir"
echo "The operator server has been deployed and configured!"
