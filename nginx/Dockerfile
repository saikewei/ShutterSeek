FROM nginx:1.27-alpine

# acme.sh is packaged by Alpine; curl/openssl are its runtime dependencies.
# GNU date provides nanosecond precision for acme.sh's Aliyun signature nonce:
# BusyBox date returns the same second for rapid requests, which Aliyun
# rejects with "Specified signature nonce was used already".
RUN apk add --no-cache acme.sh curl openssl tzdata coreutils \
 && mkdir -p /acme.sh /etc/nginx/certs /etc/nginx/templates \
    /etc/nginx/extra/locations /etc/nginx/extra/servers

COPY nginx.conf /etc/nginx/nginx.conf
COPY templates/default.conf.template /etc/nginx/templates/default.conf.template
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
COPY renew-certs.sh /usr/local/bin/renew-certs.sh
RUN chmod 0755 /usr/local/bin/entrypoint.sh /usr/local/bin/renew-certs.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
