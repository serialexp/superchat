#!/bin/sh
set -e

# Start the archiver in the background
# It listens for archive protocol connections and writes HTML to nginx's serve directory
superchat-archiver \
  --listen "${ARCHIVER_LISTEN:-:6470}" \
  --db "${ARCHIVER_DB:-/data/archive.db}" \
  --output /usr/share/nginx/html/archive \
  --html-interval "${ARCHIVER_HTML_INTERVAL:-300}" \
  --word-filter="${ARCHIVER_WORD_FILTER:-true}" \
  --word-filter-backfill="${ARCHIVER_WORD_FILTER_BACKFILL:-true}" \
  --word-filter-extra "${ARCHIVER_WORD_FILTER_EXTRA:-}" \
  --word-filter-remove "${ARCHIVER_WORD_FILTER_REMOVE:-}" \
  --word-filter-allow "${ARCHIVER_WORD_FILTER_ALLOW:-}" &

ARCHIVER_PID=$!

# Trap signals to gracefully stop both processes
trap "kill $ARCHIVER_PID; wait $ARCHIVER_PID; exit 0" SIGTERM SIGINT

# Start nginx in the foreground
nginx -g 'daemon off;' &
NGINX_PID=$!

# Wait for either process to exit
wait -n $ARCHIVER_PID $NGINX_PID
EXIT_CODE=$?

# If one exits, stop the other
kill $ARCHIVER_PID $NGINX_PID 2>/dev/null || true
wait $ARCHIVER_PID $NGINX_PID 2>/dev/null || true

exit $EXIT_CODE
