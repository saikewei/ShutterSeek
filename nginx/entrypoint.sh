#!/bin/sh
# Render the TLS vhost, ensure a certificate exists, then start nginx and keep
# a lightweight renewal loop in the background. TLS terminates here; the app
# only speaks HTTP on the private proxy network.
set -eu

: "${ACME_DOMAIN:?ACME_DOMAIN is required}"

case "$ACME_DOMAIN" in
    *[!A-Za-z0-9.-]*|.*|*.)
        echo "acme: invalid ACME_DOMAIN: $ACME_DOMAIN" >&2
        exit 1
        ;;
esac

ACME_HOME=${ACME_HOME:-/acme.sh}
CERT_DIR=${CERT_DIR:-/etc/nginx/certs}
APP_UPSTREAM=${APP_UPSTREAM:-shutterseek:8080}
ACME_RENEW_INTERVAL=${ACME_RENEW_INTERVAL:-43200}

case "$APP_UPSTREAM" in
    *[!A-Za-z0-9.:_/-]*|'')
        echo "acme: invalid APP_UPSTREAM: $APP_UPSTREAM" >&2
        exit 1
        ;;
esac

case "$ACME_RENEW_INTERVAL" in
    *[!0-9]*|'')
        echo "acme: invalid ACME_RENEW_INTERVAL: $ACME_RENEW_INTERVAL" >&2
        exit 1
        ;;
esac
[ "$ACME_RENEW_INTERVAL" -ge 60 ] || {
    echo "acme: ACME_RENEW_INTERVAL must be at least 60 seconds" >&2
    exit 1
}

mkdir -p "$ACME_HOME" "$CERT_DIR" /etc/nginx/conf.d
chmod 700 "$ACME_HOME" "$CERT_DIR" 2>/dev/null || true

sed -e "s|__ACME_DOMAIN__|$ACME_DOMAIN|g" \
    -e "s|__APP_UPSTREAM__|$APP_UPSTREAM|g" \
    /etc/nginx/templates/default.conf.template > /etc/nginx/conf.d/default.conf

if [ ! -s "$CERT_DIR/fullchain.pem" ] || [ ! -s "$CERT_DIR/privkey.pem" ]; then
    echo "acme: no usable certificate, issuing before nginx starts" >&2
    /usr/local/bin/renew-certs.sh
fi

nginx -t

# The first attempt runs shortly after nginx starts so an existing seeded cert
# can serve traffic immediately and a renewal reload cannot race startup.
(
    sleep 5
    while :; do
        /usr/local/bin/renew-certs.sh || echo "acme: renewal attempt failed" >&2
        sleep "$ACME_RENEW_INTERVAL"
    done
) &

exec "$@"
