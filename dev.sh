#!/usr/bin/env bash
set -Eeuo pipefail # https://vaneyckt.io/posts/safer_bash_scripts_with_set_euxo_pipefail

trap "trap - SIGTERM && kill -- -$$ && echo '\\n\\n'" SIGINT SIGTERM EXIT # kill background jobs on exit

prefix() (
  prefix="$1"
  color="$2"
  colored_prefix="[$(tput setaf "$color")$prefix$(tput sgr0)] "

  # flushing awk: https://unix.stackexchange.com/a/83853
  awk -v prefix="$colored_prefix" '{ print prefix $0; system("") }'
)

echo "Checking for node..." && node --version
echo "Checking for yarn..." && yarn --version
echo "Checking for sbt..." && sbt --script-version

echo "Checking if ports can be opened..."
PORT_FRONTEND=12345
PORT_WS=8081
PORT_HTTP=8080
PORT_AUTH=8082

nc -z 127.0.0.1  $PORT_FRONTEND &>/dev/null && (echo "Port $PORT_FRONTEND is already in use"; exit 1)
nc -z 127.0.0.1  $PORT_HTTP     &>/dev/null && (echo "Port $PORT_HTTP is already in use";     exit 1)
nc -z 127.0.0.1  $PORT_WS       &>/dev/null && (echo "Port $PORT_WS is already in use";       exit 1)
nc -z 127.0.0.1  $PORT_AUTH     &>/dev/null && (echo "Port $PORT_AUTH is already in use";     exit 1)


yarn install --frozen-lockfile

(npx fun-local-env \
    --auth $PORT_AUTH \
    --ws $PORT_WS \
    --http $PORT_HTTP \
    --http-api lambda/target/scala-2.13/scalajs-bundler/main/lambda-fastopt.js httpApi \
    --http-rpc lambda/target/scala-2.13/scalajs-bundler/main/lambda-fastopt.js httpRpc \
    --ws-rpc lambda/target/scala-2.13/scalajs-bundler/main/lambda-fastopt.js wsRpc \
    --ws-event-authorizer lambda/target/scala-2.13/scalajs-bundler/main/lambda-fastopt.js wsEventAuth \
    | prefix "BACKEND" 4 || kill 0) &

sbt dev shell
printf "\n"


