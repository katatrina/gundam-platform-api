#!/bin/sh

set -e

echo "run db migrations"
/app/migrate -path /app/migrations -database "$DATABASE_URL" -verbose up

echo "start the app"
exec "$@"