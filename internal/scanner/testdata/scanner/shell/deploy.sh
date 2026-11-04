#!/usr/bin/env bash
set -euo pipefail
# Uses $FAKE_IN_COMMENT
TARGET="production"
DEPLOY_ENV=${DEPLOY_ENV:-staging}
echo 'Literal $FAKE_IN_SINGLE_QUOTES'
echo "Deploying ${REAL_APP_NAME} to $TARGET with $REAL_REGISTRY"
curl -H "Authorization: Bearer ${REAL_DEPLOY_TOKEN:?missing token}" "$REAL_API_URL"
for HOST in a b; do echo "$HOST"; done
echo "$RANDOM $HOME"
