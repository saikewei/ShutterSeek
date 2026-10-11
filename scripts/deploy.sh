#!/bin/sh
# Execute only through the release workflow; never source an environment file as shell code.
set -eu
release_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
project_dir=${SHUTTERSEEK_DEPLOY_PROJECT_DIR:-/volume1/docker/ShutterSeek}
: "${DEPLOY_SHA:?DEPLOY_SHA is required}"
case "$DEPLOY_SHA" in *[!0-9a-f]*|"") echo "Invalid release revision" >&2; exit 1;; esac
[ "${#DEPLOY_SHA}" -eq 40 ] || { echo "Invalid release revision" >&2; exit 1; }
export IMAGE_TAG="$DEPLOY_SHA"
compose() {
  docker compose --project-name shutterseek --project-directory "$project_dir" --env-file "$project_dir/.env" --file "$release_dir/docker-compose.yml" "$@"
}
docker compose up --help | grep -q -- --wait-timeout || {
  echo "Docker Compose with health waiting is required" >&2; exit 1;
}
environment=$(compose config --environment)
enabled=$(printf '%s\n' "$environment" | awk -F= '$1=="SHUTTERSEEK_CREATIONS_ENABLED" {print $2}')
if [ "$enabled" = true ]; then
  configured=$(printf '%s\n' "$environment" | awk -F= '$1=="SHUTTERSEEK_CREATIONS_MODEL_KEY" && length($0)>length($1)+1 {print "yes"}')
  [ "$configured" = yes ] || { echo "Production creation model configuration is missing" >&2; exit 1; }
  export COMPOSE_PROFILES=creations
  services="shutterseek embedding agent"
else
  services="shutterseek embedding"
fi
unset environment
compose config --quiet
# Explicit services activate the Agent profile whenever the Go feature is enabled.
compose pull $services
compose up --detach --wait --wait-timeout 240 $services
compose ps
printf '%s\n' "$DEPLOY_SHA" > "$release_dir/deployed-revision"
echo "Release $DEPLOY_SHA healthy"
