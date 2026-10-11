#!/bin/sh
# Writes the browser config from env vars so one image works for any API URL.
set -eu
if [ -z "${API_URL:-}" ]; then
    echo "sortly: WARNING: API_URL is not set; the site will show a 'not connected' warning" >&2
fi
api_url=$(printf '%s' "${API_URL:-}" | sed 's/[\\"]/\\&/g')
printf 'window.SORTLY_CONFIG = { apiUrl: "%s" };\n' "$api_url" > /usr/share/nginx/html/config.js
echo "sortly: API_URL=${API_URL:-<unset>}"
