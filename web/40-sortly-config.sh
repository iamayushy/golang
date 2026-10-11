#!/bin/sh
# Writes the browser config from env vars so one image works for any API URL.
set -eu
api_url=$(printf '%s' "${API_URL:-http://localhost:8080}" | sed 's/[\\"]/\\&/g')
printf 'window.SORTLY_CONFIG = { apiUrl: "%s" };\n' "$api_url" > /usr/share/nginx/html/config.js
