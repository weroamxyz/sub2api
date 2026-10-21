#!/usr/bin/env bash
set -Eeuo pipefail

: "${RELEASE_IMAGE:?RELEASE_IMAGE is required}"
: "${RELEASE_VERSION:?RELEASE_VERSION is required}"
: "${COMPOSE_DIR:=/root/edson/sub2api}"
: "${COMPOSE_SERVICE:=sub2api}"
: "${APP_CONTAINER:=sub2api}"
: "${POSTGRES_CONTAINER:=sub2api-postgres}"
: "${POSTGRES_USER:=sub2api}"
: "${POSTGRES_DB:=sub2api}"
: "${REDIS_CONTAINER:=sub2api-redis}"
: "${HEALTH_URL:=http://127.0.0.1:8088/health}"
: "${BACKUP_DIR:=${COMPOSE_DIR}/backups}"
: "${OVERRIDE_FILE:=${COMPOSE_DIR}/docker-compose.override.yml}"

[[ "$RELEASE_IMAGE" =~ ^ghcr\.io/[a-z0-9._/-]+:[0-9]+\.[0-9]+\.[0-9]+$ ]]
[[ "$RELEASE_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
command -v docker >/dev/null
command -v flock >/dev/null

install -d -m 700 "$BACKUP_DIR"
exec 9>"${COMPOSE_DIR}/.production-deploy.lock"
flock -n 9 || { echo "another production deployment is running" >&2; exit 1; }

cd "$COMPOSE_DIR"
docker compose config -q
docker image inspect "$RELEASE_IMAGE" >/dev/null
image_version="$(docker run --rm --entrypoint /app/sub2api "$RELEASE_IMAGE" --version 2>&1)"
grep -Fq "Sub2API ${RELEASE_VERSION}" <<<"$image_version"

current_image="$(docker inspect -f '{{.Config.Image}}' "$APP_CONTAINER")"
stamp="$(date +%Y%m%d-%H%M%S)"
backup="${BACKUP_DIR}/pre-v${RELEASE_VERSION}-${stamp}"
install -d -m 700 "$backup"
docker exec "$POSTGRES_CONTAINER" pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc >"${backup}/postgres.dump"
docker exec -i "$POSTGRES_CONTAINER" pg_restore -l <"${backup}/postgres.dump" >"${backup}/postgres.list"
test -s "${backup}/postgres.list"
docker exec "$REDIS_CONTAINER" env -u REDISCLI_AUTH redis-cli SAVE >/dev/null
docker cp "${REDIS_CONTAINER}:/data/dump.rdb" "${backup}/redis-dump.rdb" >/dev/null
cp "$OVERRIDE_FILE" "${backup}/docker-compose.override.yml"
cp docker-compose.yml "${backup}/docker-compose.yml"
[[ ! -f .env ]] || cp .env "${backup}/.env"
docker inspect "$APP_CONTAINER" >"${backup}/app.inspect.json"
docker inspect "$POSTGRES_CONTAINER" >"${backup}/postgres.inspect.json"
docker inspect "$REDIS_CONTAINER" >"${backup}/redis.inspect.json"
printf '%s\n' "$current_image" >"${backup}/old-image.txt"
docker save "$current_image" | gzip -1 >"${backup}/old-image.tar.gz"
sha256sum "${backup}"/* >"${backup}/SHA256SUMS"
chmod -R go-rwx "$backup"

pg_id="$(docker inspect -f '{{.Id}}' "$POSTGRES_CONTAINER")"
pg_started="$(docker inspect -f '{{.State.StartedAt}}' "$POSTGRES_CONTAINER")"
redis_id="$(docker inspect -f '{{.Id}}' "$REDIS_CONTAINER")"
redis_started="$(docker inspect -f '{{.State.StartedAt}}' "$REDIS_CONTAINER")"

rollback() {
  rc=$?
  trap - ERR
  docker logs "$APP_CONTAINER" >"${backup}/failed-release.log" 2>&1 || true
  cp "${backup}/docker-compose.override.yml" "$OVERRIDE_FILE"
  docker compose config -q
  docker compose up -d --no-deps --force-recreate "$COMPOSE_SERVICE"
  echo "release v${RELEASE_VERSION} failed; restored ${current_image}" >&2
  exit "$rc"
}
trap rollback ERR

image_lines="$(grep -Ec '^[[:space:]]+image:[[:space:]]+' "$OVERRIDE_FILE")"
[[ "$image_lines" == 1 ]]
next_override="$(mktemp "${COMPOSE_DIR}/.override.XXXXXX")"
sed -E "s#^([[:space:]]+image:)[[:space:]]+.*#\1 ${RELEASE_IMAGE}#" "$OVERRIDE_FILE" >"$next_override"
install -m 600 "$next_override" "$OVERRIDE_FILE"
rm -f -- "$next_override"
docker compose config -q

deploy_since="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
docker compose up -d --no-deps --force-recreate "$COMPOSE_SERVICE"
healthy=false
for _ in $(seq 1 120); do
  if [[ "$(docker inspect -f '{{.State.Health.Status}}' "$APP_CONTAINER" 2>/dev/null || true)" == healthy ]]; then
    healthy=true
    break
  fi
  sleep 1
done
[[ "$healthy" == true ]]
[[ "$(docker inspect -f '{{.Config.Image}}' "$APP_CONTAINER")" == "$RELEASE_IMAGE" ]]
[[ "$(docker inspect -f '{{.RestartCount}}' "$APP_CONTAINER")" == 0 ]]
[[ "$(docker inspect -f '{{.Id}}' "$POSTGRES_CONTAINER")" == "$pg_id" ]]
[[ "$(docker inspect -f '{{.State.StartedAt}}' "$POSTGRES_CONTAINER")" == "$pg_started" ]]
[[ "$(docker inspect -f '{{.Id}}' "$REDIS_CONTAINER")" == "$redis_id" ]]
[[ "$(docker inspect -f '{{.State.StartedAt}}' "$REDIS_CONTAINER")" == "$redis_started" ]]
for _ in $(seq 1 20); do curl -fsS --max-time 5 "$HEALTH_URL" >/dev/null; done

error_count="$(docker logs --since "$deploy_since" "$APP_CONTAINER" 2>&1 | grep -Eic '"level":"(error|fatal|panic)"|(^|[[:space:]])(ERROR|FATAL|PANIC)([[:space:]]|$)' || true)"
[[ "$error_count" == 0 ]]
trap - ERR
echo "deployed v${RELEASE_VERSION}; image=${RELEASE_IMAGE}; backup=${backup}"
