#!/bin/sh

set -eu

: "${R2_ACCOUNT_ID:?Set R2_ACCOUNT_ID to the Cloudflare account ID}"
: "${R2_ACCESS_KEY_ID:?Set R2_ACCESS_KEY_ID to the R2 Access Key ID}"
: "${R2_SECRET_ACCESS_KEY:?Set R2_SECRET_ACCESS_KEY to the R2 Secret Access Key}"
: "${R2_BUCKET:?Set R2_BUCKET to the private R2 bucket name}"
: "${R2_OBJECT_KEY:?Set R2_OBJECT_KEY to the GeoLite2-ASN.mmdb object key}"

case "$R2_ACCOUNT_ID" in
    ''|*[!a-fA-F0-9]*)
        echo "R2_ACCOUNT_ID must be the hexadecimal Cloudflare account ID" >&2
        exit 1
        ;;
esac

case "${R2_JURISDICTION:-default}" in
    default)
        r2_endpoint="https://${R2_ACCOUNT_ID}.r2.cloudflarestorage.com"
        ;;
    eu|fedramp)
        r2_endpoint="https://${R2_ACCOUNT_ID}.${R2_JURISDICTION}.r2.cloudflarestorage.com"
        ;;
    *)
        echo "R2_JURISDICTION must be default, eu, or fedramp" >&2
        exit 1
        ;;
esac

destination="${GEOLITE_DB_PATH:-/data/geolite/GeoLite2-ASN.mmdb}"
destination_dir="$(dirname "$destination")"
temporary="${destination}.download.$$"

cleanup() {
    rm -f "$temporary"
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$destination_dir"

echo "Downloading GeoLite2-ASN database from private Cloudflare R2 bucket"
download_database() {
    export RCLONE_CONFIG_R2_TYPE=s3
    export RCLONE_CONFIG_R2_PROVIDER=Cloudflare
    export RCLONE_CONFIG_R2_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID"
    export RCLONE_CONFIG_R2_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"
    export RCLONE_CONFIG_R2_ENDPOINT="$r2_endpoint"
    export RCLONE_CONFIG_R2_REGION=auto

    object_key="${R2_OBJECT_KEY#/}"
    rclone copyto --quiet "r2:${R2_BUCKET}/${object_key}" "$temporary"
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
