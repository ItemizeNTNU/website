#!/usr/bin/env bash
#
# Run the site locally with seeded test events, for eyeballing the
# /arrangementer page. Sets up everything the server refuses to start
# without: a throwaway MongoDB with a few events in it, and a stub OIDC
# discovery document — the real FusionAuth cannot be used here because it
# returns an issuer without the scheme, which go-oidc rejects.
#
# Nothing touches the compose stack or ./data; the database is a nameless
# volume inside a container that is removed again on `down`.
#
# Usage: scripts/preview-events.sh up     start and seed, then print the URL
#        scripts/preview-events.sh down   stop and remove everything
set -euo pipefail

cd "$(dirname "$0")/.."

state=/tmp/itemize-events-preview
mongo_ctr=itemize-events-preview-mongo
web_port=3123
oidc_port=9011

up() {
	mkdir -p "$state"

	echo "» mongo"
	docker rm -f "$mongo_ctr" >/dev/null 2>&1 || true
	docker run --rm -d -p 27017:27017 --name "$mongo_ctr" mongo:8 >/dev/null
	for _ in $(seq 1 30); do
		docker exec "$mongo_ctr" mongosh --quiet --eval 'db.runCommand({ping:1}).ok' >/dev/null 2>&1 && break
		sleep 1
	done

	echo "» seed events"
	docker exec "$mongo_ctr" mongosh --quiet website --eval '
		db.events.deleteMany({});
		const day = 24*3600*1000;
		db.events.insertMany([
			{
				name: "CTF-kveld", location: {name: "Savannen", url: ""},
				register_url: "", date: new Date(Date.now() + 2*day), duration: 2.0,
				end: new Date(Date.now() + 2*day + 2*3600*1000),
				ctf: {name: "picoCTF", url: "https://picoctf.org"},
				info: "Info", hidden: false, discord: false, created: new Date()
			},
			{
				name: "Kurs i lockpicking",
				location: {name: "R51", url: "https://use.mazemap.com/x"},
				register_url: "https://example.com/register",
				date: new Date(Date.now() + 4*day), duration: 3.0,
				end: new Date(Date.now() + 4*day + 3*3600*1000),
				ctf: {name: "", url: ""},
				info: "Linje en.\nLinje to.", hidden: false, discord: false, created: new Date()
			},
			{
				name: "Ferdig arrangement", location: {name: "Verden", url: ""},
				register_url: "", date: new Date(Date.now() - 7*day), duration: 2.0,
				end: new Date(Date.now() - 7*day + 2*3600*1000),
				ctf: {name: "", url: ""},
				info: "Fortid — synlig bak Vis tidligere.",
				hidden: false, discord: false, created: new Date()
			}
		]).insertedIds' >/dev/null

	echo "» oidc stub"
	mkdir -p "$state/oidc/.well-known"
	cat > "$state/oidc/.well-known/openid-configuration" <<-JSON
	{
	  "issuer": "http://localhost:$oidc_port",
	  "authorization_endpoint": "http://localhost:$oidc_port/oauth2/authorize",
	  "token_endpoint": "http://localhost:$oidc_port/oauth2/token",
	  "userinfo_endpoint": "http://localhost:$oidc_port/oauth2/userinfo",
	  "jwks_uri": "http://localhost:$oidc_port/.well-known/jwks.json",
	  "response_types_supported": ["code"],
	  "subject_types_supported": ["public"],
	  "id_token_signing_alg_values_supported": ["HS256", "RS256"]
	}
	JSON
	(cd "$state/oidc" && python3 -m http.server "$oidc_port" >/dev/null 2>&1) &
	echo $! > "$state/oidc.pid"

	echo "» server"
	BASE_URL="http://localhost:$web_port" \
	FUSION_AUTH_HOST="http://localhost:$oidc_port" \
	FUSION_AUTH_CLIENT_ID=dummy FUSION_AUTH_CLIENT_SECRET=dummysecret \
	FUSION_AUTH_SECRET=0123456789abcdef0123456789abcdef \
	MONGO_DB_URL=mongodb://localhost:27017/website \
	LISTEN="$web_port" \
		go run ./cmd/website -dev >"$state/server.log" 2>&1 &
	echo $! > "$state/server.pid"

	for _ in $(seq 1 30); do
		code=$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$web_port/arrangementer" || true)
		[[ "$code" == 200 ]] && break
		sleep 1
	done
	if [[ "${code:-}" != 200 ]]; then
		echo "server did not come up; see $state/server.log" >&2
		down
		exit 1
	fi

	echo
	echo "http://localhost:$web_port/arrangementer"
	echo "stop with: scripts/preview-events.sh down"
}

down() {
	for pidfile in "$state"/server.pid "$state"/oidc.pid; do
		if [[ -f "$pidfile" ]]; then
			pkill -P "$(cat "$pidfile")" 2>/dev/null || true
			kill "$(cat "$pidfile")" 2>/dev/null || true
			rm -f "$pidfile"
		fi
	done
	docker rm -f "$mongo_ctr" >/dev/null 2>&1 || true
	rm -rf "$state"
	echo "stopped"
}

case "${1:-}" in
up) up ;;
down) down ;;
*)
	echo "usage: $0 up|down" >&2
	exit 1
	;;
esac
