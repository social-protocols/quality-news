#!/bin/sh

set -euo pipefail

cd $(dirname "$0")

account="$(aws sts get-caller-identity | jq -r '."Account"')"
terraform_state_bucket="${account}-terraform-state"
terraform_state_lock_table="terraform-lock"

echo "Create s3 bucket for terraform state: ${terraform_state_bucket}"
aws s3 mb s3://${terraform_state_bucket} || true
aws s3api put-bucket-versioning --bucket ${terraform_state_bucket} --versioning-configuration Status=Enabled || true

echo "Create dynamodb table for terraform state lock: ${terraform_state_lock_table}"
aws dynamodb create-table --table-name ${terraform_state_lock_table} --attribute-definitions AttributeName=LockID,AttributeType=S --key-schema AttributeName=LockID,KeyType=HASH --billing-mode PAY_PER_REQUEST > /dev/null || true

cat <<EOF > terraform.tf
terraform {
  backend "s3" {
    encrypt        = true
    region         = "eu-central-1"
    key            = "example.tfstate"
    bucket         = "${terraform_state_bucket}"
    dynamodb_table = "${terraform_state_lock_table}"
  }
}

provider "aws" {
  region = "eu-central-1"
}
EOF
