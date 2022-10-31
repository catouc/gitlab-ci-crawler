#!/usr/bin/env bash

set -euo pipefail

if [ -z "$1" ]; then
	echo "Usage:"
	echo "update-go-version.sh <new-go-version>"
	exit 1
fi

NEW_VERSION="$1"
CURRENT_VERSION_DOCKER="$(grep 'golang:.*' Dockerfile | cut -d ' ' -f 2 | cut -d ':' -f 2)"
CURRENT_VERSION_GO_MOD="$(grep 'go [1-9].[0-9]*$' go.mod | cut -d ' ' -f 2)"

if [ "$CURRENT_VERSION_DOCKER" != "$CURRENT_VERSION_GO_MOD" ]; then
	echo "Got version mismatch between Docker (${CURRENT_VERSION_DOCKER}) and go.mod (${CURRENT_VERSION_GO_MOD})"
	exit 1
fi

CURRENT_VERSION="$CURRENT_VERSION_GO_MOD"
sed -i "s/${CURRENT_VERSION}/${NEW_VERSION}/g" Dockerfile go.mod
