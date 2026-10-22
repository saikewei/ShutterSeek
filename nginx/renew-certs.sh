#!/bin/sh
# Issue or renew the DNS-01 certificate and install it into nginx's cert dir.
# Safe to run repeatedly: acme.sh only renews when the certificate is due.
set -eu

: "${ACME_DOMAIN:?ACME_DOMAIN is required}"

ACME_HOME=${ACME_HOME:-/acme.sh}
CERT_DIR=${CERT_DIR:-/etc/nginx/certs}
ACME_SERVER=${ACME_SERVER:-letsencrypt}
ACME_EMAIL=${ACME_EMAIL:-}

META="$ACME_HOME/${ACME_DOMAIN}_ecc/${ACME_DOMAIN}.conf"

install_cert() {
    acme.sh --install-cert -d "$ACME_DOMAIN" --ecc \
        --key-file "$CERT_DIR/privkey.pem" \
        --fullchain-file "$CERT_DIR/fullchain.pem" \
        --reloadcmd "nginx -s reload || true" \
        --home "$ACME_HOME"
}

if [ -f "$META" ]; then
    echo "acme: checking renewal for ${ACME_DOMAIN}" >&2
    acme.sh --cron --home "$ACME_HOME"
    # A previous renewal may have installed successfully while the cert files
    # were removed during maintenance; reinstall without reissuing.
    if [ ! -s "$CERT_DIR/fullchain.pem" ] || [ ! -s "$CERT_DIR/privkey.pem" ]; then
        install_cert
    fi
    exit 0
fi

if [ -z "${Ali_Key:-}" ] || [ -z "${Ali_Secret:-}" ]; then
    echo "acme: Ali_Key/Ali_Secret are required to issue a new certificate" >&2
    exit 1
fi

echo "acme: issuing ${ACME_DOMAIN} via ${ACME_SERVER}" >&2
set -- --issue --dns dns_ali -d "$ACME_DOMAIN" \
    --server "$ACME_SERVER" \
    --keylength ec-256 \
    --home "$ACME_HOME" \
    --force
if [ -n "$ACME_EMAIL" ]; then
    set -- "$@" --accountemail "$ACME_EMAIL"
fi
acme.sh "$@"
install_cert
