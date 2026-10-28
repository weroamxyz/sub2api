#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
script="$root/deploy/production-tag-deploy.sh"
workflow="$root/.github/workflows/release.yml"

bash -n "$script"
grep -Fq 'docker compose up -d --no-deps --force-recreate' "$script"
grep -Fq 'pg_restore -l' "$script"
grep -Fq 'trap rollback ERR' "$script"
grep -Fq 'initial_deploy=true' "$script"
grep -Fq 'docker compose rm -sf' "$script"
grep -Fq '${HOME}/edson/sub2api' "$script"
grep -Fq 'needs: [prepare, release]' "$workflow"
grep -Fq "github.event_name == 'push'" "$workflow"
grep -Fq 'refs/tags/v' "$workflow"
grep -Fq 'secrets.PROD_SSH_PASSWORD' "$workflow"
grep -Fq 'secrets.PROD_SSH_PRIVATE_KEY' "$workflow"
grep -Fq 'IdentitiesOnly=yes' "$workflow"
grep -Fq 'PasswordAuthentication=no' "$workflow"
grep -Fq 'org.opencontainers.image.revision' "$workflow"

if grep -Eiq '(password|secret|token)[[:space:]]*=' "$script"; then
  echo "production deploy script must not contain embedded credentials" >&2
  exit 1
fi

echo "production tag deployment checks passed"
