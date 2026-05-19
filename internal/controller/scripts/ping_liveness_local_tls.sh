#!/bin/sh
set -e

TIMEOUT_SECONDS="${1:-5}"
case "$TIMEOUT_SECONDS" in
	''|*[!0-9]*)
		echo "Invalid timeout: $TIMEOUT_SECONDS" >&2
		exit 1
		;;
esac

if [ ! -z "$VALKEY_PASSWORD" ]; then export REDISCLI_AUTH=$VALKEY_PASSWORD; fi;

response=$(
	timeout --foreground -s 15 "$TIMEOUT_SECONDS" \
	valkey-cli \
		-h localhost \
		-p $VALKEY_TLS_PORT_NUMBER \
		--tls \
		--cacert $VALKEY_TLS_CA_FILE \
		--cert $VALKEY_TLS_CERT_FILE \
		--key $VALKEY_TLS_KEY_FILE \
		ping
)

if [ "$?" -eq "124" ]; then
	echo "Timed out"
	exit 1
fi

responseFirstWord=$(echo $response | head -n1 | awk '{print $1;}')
if [ "$response" != "PONG" ] && [ "$responseFirstWord" != "LOADING" ] && [ "$responseFirstWord" != "MASTERDOWN" ]; then
	echo "$response"
	exit 1
fi
