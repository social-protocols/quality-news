# Example

This is your whole application. All code in scala. Infrastructure as terraform code. To start a new project, simply download or clone this example.

## Overview

Three scala projects:
- *lambda/*: backend code deployed as lambda functions behind api gateway
- *webapp/*: frontend code deployed to s3 bucket behind cloudfront
- *api/*: shared code between lambda and webapp

Terraform deployment:
- *terraform/*: terraform code to deploy backend and frontend to AWS.

## Requirements

- aws-cli
- sbt
- yarn
- node (>= 10.13.0)
- terraform (>= 1.0.0): https://www.terraform.io/downloads.html

The provided `default.nix` contains these dependencies, so if you are using `nix`, just run a `nix-shell` :-)

## Development

Watch and compile the application. Runs a webserver for the website, http and websocket servers for your backend lambdas, and a local auth server:
```sh
./dev.sh
```

You will see your locally running full-stack app at <http://localhost:12345>.

Changing any source file will automatically recompile and hot-reload the website and backends.

To know more about the details, have a look at [dev.sh](dev.sh)

If you just want to develop on your frontend without any backend:
```sh
sbt devf
```

#### Infos about webapp

Webpack configuration: `webapp/webpack.config.dev.js`, `webapp/webpack.config.prod.js`

Postcss configuration (picked up by webpack): `webapp/postcss.config.js`

Tailwind configuration (picked up by postcss): `webapp/tailwind.config.js`

Template for index.html and css (picked up by webpack): `webapp/src/main/html/index.html`, `webapp/src/main/css/`

Static assets folder (picked up by webpack): `webapp/assets/`

Local development configuration for your webapp: `webapp/local/app_config.js`

#### Infos about lambda

Webpack configuration: `lambda/webpack.config.dev.js`, `lambda/webpack.config.prod.js`

## Deployment

### Initial steps

You have to do these steps only once.

Create an s3-bucket and dynamodb table for the terraform state (generates a `terraform/terraform.tf` file):

```sh
# export AWS_PROFILE=<my-profile>
./terraform/initial_setup.sh
# git add terraform/terraform.tf
```

#### If you have a custom domain

Set your `domain` in `terraform/fun.tf`.

Create a hosted zone in AWS for this custom domain.
Either just register your domain in AWS directly - then you do not need to do anything else here.
Or create a hosted zone in AWS for your already owned domain:

```sh
aws route53 create-hosted-zone --name "example.com" --caller-reference $(date +%s)
```

For your already owned domain, you need point your domain registrar to the nameservers of this hosted zone. You need to do this where you bought the domain. Here is a command to print the nameserver IPs of the hosted zone again:

```sh
HOSTED_ZONE_ID=$(aws route53 list-hosted-zones-by-name --dns-name "example.com" | jq -r ".HostedZones[0].Id")
aws route53 get-hosted-zone --id $HOSTED_ZONE_ID | jq ".DelegationSet.NameServers"
```

### Deploy steps

First build the application:

```sh
sbt prod
```

Then go into the terraform directory. Set your `AWS_PROFILE`. And run terraform:

```sh
export AWS_PROFILE=...
cd terraform
terraform init -upgrade -reconfigure
terraform apply
```

Then the app is available under `https://example.com`.
Without a custom domain, you can see the endpoint in the outputs of the apply command.

### Environments

If you want to try something out without interrupting others, you can make your own terraform workspace and setup your own independent deployment:

```sh
terraform workspace new <my-workspace>
terraform workspace switch <my-workspace>
# run terraform as usual
```

If you are not on the `default` terraform workspace, the app is available under: `https://<my-workspace>.env.example.com`.

## Links

SDK library to communicate with the infrastructure in your code:
- Fun SDK Scala: [sdk-scala](https://github.com/fun-stack/sdk-scala)

Terraform module for the corresponding AWS infrastructure:
- Fun Terraform Module: [terraform-aws-fun](https://github.com/fun-stack/terraform-aws-fun)

See local development module for mocking the AWS infrastructure locally:
- Fun Local Environment: [local-env](https://github.com/fun-stack/local-env)

