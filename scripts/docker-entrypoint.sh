#!/bin/sh

set -eu

: "${GEOLITE_DB_URL:?Set GEOLITE_DB_URL to the HTTPS URL of GeoLite2-ASN.mmdb}"

destination="${GEOLITE_DB_PATH:-/data/geolite/GeoLite2-ASN.mmdb}"
destination_dir="$(dirname "$destination")"
temporary="${destination}.download.$$"

cleanup() {
    rm -f "$temporary"
}
trap cleanup EXIT HUP INT TERM

if [ -n "${GEOLITE_DB_CF_ACCESS_CLIENT_ID:-}" ] || [ -n "${GEOLITE_DB_CF_ACCESS_CLIENT_SECRET:-}" ]; then
    : "${GEOLITE_DB_CF_ACCESS_CLIENT_ID:?Set both Cloudflare Access service-token values or neither}"
    : "${GEOLITE_DB_CF_ACCESS_CLIENT_SECRET:?Set both Cloudflare Access service-token values or neither}"
fi

mkdir -p "$destination_dir"

echo "Downloading GeoLite2-ASN database"
download_database() {
    set -- wget --https-only --quiet --output-document "$temporary"

    if [ -n "${GEOLITE_DB_CF_ACCESS_CLIENT_ID:-}" ]; then
        set -- "$@" \
            --header "CF-Access-Client-Id: ${GEOLITE_DB_CF_ACCESS_CLIENT_ID}" \
            --header "CF-Access-Client-Secret: ${GEOLITE_DB_CF_ACCESS_CLIENT_SECRET}"
    fi

    if [ -n "${GEOLITE_DB_AUTHORIZATION:-}" ]; then
        set -- "$@" --header "Authorization: ${GEOLITE_DB_AUTHORIZATION}"
    fi

    "$@" "$GEOLITE_DB_URL"
}

download_database

if [ ! -s "$temporary" ]; then
    echo "GeoLite2-ASN download was empty" >&2
    exit 1
fi

if [ -n "${GEOLITE_DB_SHA256:-}" ]; then
    expected="$(printf '%s' "$GEOLITE_DB_SHA256" | tr '[:upper:]' '[:lower:]')"
    actual="$(sha256sum "$temporary" | awk '{print $1}')"
    if [ "$actual" != "$expected" ]; then
        echo "GeoLite2-ASN SHA-256 mismatch" >&2
        exit 1
    fi
fi

chmod 0444 "$temporary"
mv -f "$temporary" "$destination"
trap - EXIT HUP INT TERM

echo "GeoLite2-ASN database ready at $destination"
exec /usr/local/bin/ripestream "$@"
