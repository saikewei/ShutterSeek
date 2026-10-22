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
configuration=$(compose --profile creations config)
enabled=$(printf '%s\n' "$configuration" | awk '$1=="SHUTTERSEEK_CREATIONS_ENABLED:" {value=$2; gsub(/["\047]/,"",value); print value; exit}')
case "$enabled" in true|false) ;; *) echo "Cannot resolve production creation switch" >&2; exit 1;; esac
if [ "$enabled" = true ]; then
  configured=$(printf '%s\n' "$configuration" | awk '$1=="SHUTTERSEEK_CREATIONS_MODEL_KEY:" {value=$0; sub(/^[^:]*:[[:space:]]*/,"",value); gsub(/["\047]/,"",value); if (length(value)>0 && value!="null") print "yes"; exit}')
  [ "$configured" = yes ] || { echo "Production creation model configuration is missing" >&2; exit 1; }
  export COMPOSE_PROFILES=creations
  services="shutterseek embedding agent nginx"
else
  services="shutterseek embedding nginx"
fi
unset configuration
compose config --quiet
# Explicit services activate the Agent profile whenever the Go feature is enabled.
compose pull $services
compose up --detach --wait --wait-timeout 240 $services
compose ps
if [ "$enabled" = true ]; then
  agent_logs=$(compose logs --no-color --since 5m agent)
  printf '%s\n' "$agent_logs" | grep -q 'event=agent_ready' || {
    echo "Agent startup log unavailable" >&2; exit 1;
  }
  printf '%s\n' "$agent_logs" | grep -q 'event=heartbeat' || {
    echo "Agent heartbeat log unavailable" >&2; exit 1;
  }
  printf '%s\n' "$agent_logs" | grep -E 'event=agent_ready|event=heartbeat' | tail -n 3
  unset agent_logs
fi
printf '%s\n' "$DEPLOY_SHA" > "$release_dir/deployed-revision"
echo "Release $DEPLOY_SHA healthy"
