#!/usr/bin/env bash
set -e

DOCKER_REGISTRY=${DOCKER_REGISTRY:-"quay.io/niranjan94"}
DOCKER_TAG=${1:-"latest"}
ARM64_BUILDER=${ARM64_BUILDER:-default}
AMD64_BUILDER=${AMD64_BUILDER:-default}

function build() {
  echo "Building"
  DOCKER_FQDN="$DOCKER_REGISTRY/tsnet-relay"

  docker image rm "$DOCKER_FQDN:$DOCKER_TAG-amd64" || true
  docker buildx build --provenance=false --load --builder "$AMD64_BUILDER" --platform linux/amd64 -t "$DOCKER_FQDN:$DOCKER_TAG-amd64" .
  if [[ -z "${SKIP_PUSH}" ]]; then
    docker push "$DOCKER_FQDN:$DOCKER_TAG-amd64"
  fi

  docker image rm "$DOCKER_FQDN:$DOCKER_TAG-arm64" || true
  docker buildx build --provenance=false --load --builder "$ARM64_BUILDER" --platform linux/arm64 -t "$DOCKER_FQDN:$DOCKER_TAG-arm64" .
  if [[ -z "${SKIP_PUSH}" ]]; then
    docker push "$DOCKER_FQDN:$DOCKER_TAG-arm64"
  fi

  docker manifest rm "$DOCKER_FQDN:$DOCKER_TAG" || true
  docker manifest create "$DOCKER_FQDN:$DOCKER_TAG" --amend "$DOCKER_FQDN:$DOCKER_TAG-amd64" --amend "$DOCKER_FQDN:$DOCKER_TAG-arm64"
  if [[ -z "${SKIP_PUSH}" ]]; then
    docker manifest push "$DOCKER_FQDN:$DOCKER_TAG"
  fi
}

build