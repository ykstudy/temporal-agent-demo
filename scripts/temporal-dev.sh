#!/usr/bin/env bash
# Independent Temporal service for repeatable integration checks.
set -euo pipefail

container_name="${TEMPORAL_TEST_CONTAINER:-temporal-agent-demo-integration}"
volume_name="${TEMPORAL_TEST_VOLUME:-temporal-agent-demo-integration-data}"
image_ref="${TEMPORAL_TEST_IMAGE:-temporalio/temporal@sha256:ad4c82c97bd12b417d1ea942610dbcd511afb250c4d5ed26c694009533df447e}"
grpc_port="${TEMPORAL_TEST_GRPC_PORT:-17333}"
ui_port="${TEMPORAL_TEST_UI_PORT:-18233}"

assert_owned() {
  local owner
  owner="$(docker inspect --format '{{index .Config.Labels "agentstudio.integration"}}' "$container_name")"
  if [[ "$owner" != true ]]; then
    echo "Refusing to operate on an unrelated container: $container_name" >&2
    exit 1
  fi
}

case "${1:-start}" in
  start)
    if docker container inspect "$container_name" >/dev/null 2>&1; then
      assert_owned
      docker start "$container_name" >/dev/null
    else
      docker pull "$image_ref"
      # Docker initializes a named volume as root; the official image runs as 1000.
      docker run --rm --user 0 -v "$volume_name:/data" --entrypoint sh "$image_ref" -c 'chown 1000:1000 /data'
      docker run -d --name "$container_name" --label agentstudio.integration=true \
        -p "127.0.0.1:$grpc_port:7233" -p "127.0.0.1:$ui_port:8233" \
        -v "$volume_name:/data" "$image_ref" server start-dev \
        --ip 0.0.0.0 --ui-ip 0.0.0.0 --db-filename /data/temporal.db --ui-disable-news-fetch
    fi
    for attempt in {1..30}; do
      if [[ "$(docker exec "$container_name" temporal --address 127.0.0.1:7233 operator cluster health 2>/dev/null || true)" == SERVING ]]; then
        echo "Temporal gRPC: 127.0.0.1:$grpc_port; UI: http://127.0.0.1:$ui_port/"
        exit 0
      fi
      sleep 1
    done
    docker logs --tail 30 "$container_name"
    exit 1
    ;;
  status)
    assert_owned
    docker exec "$container_name" temporal --address 127.0.0.1:7233 operator cluster health
    ;;
  stop)
    assert_owned
    docker stop "$container_name"
    echo "Retained persistent volume: $volume_name"
    ;;
  *) echo "Usage: $0 {start|status|stop}" >&2; exit 2 ;;
esac
