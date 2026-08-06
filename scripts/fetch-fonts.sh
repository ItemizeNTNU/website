#!/usr/bin/env bash
#
# Vendor the web fonts into assets/static/fonts/.
#
# The fonts are self-hosted rather than loaded from a font CDN: serving them
# from a third party would hand every visitor's IP address to that third party
# on every page load, which is not something an information-security
# organisation should be doing. Once fetched they are committed, so this only
# needs running when a face is added or updated.
#
# No subsetting step is needed. Norwegian's æ, ø and å live at U+00E6, U+00F8
# and U+00E5, all inside the "latin" subset that Google already publishes.
#
# Usage: scripts/fetch-fonts.sh
set -euo pipefail

cd "$(dirname "$0")/.."
dest="assets/static/fonts"
mkdir -p "$dest"

# Chrome's UA is required: the API serves TTF to anything it does not
# recognise, and we want woff2.
UA='Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36'

fetch() {
	local out="$1" family="$2" weight="$3"
	if [[ -s "$dest/$out" ]]; then
		echo "  have  $out"
		return
	fi

	local css url
	css=$(curl -fsSL -A "$UA" \
		"https://fonts.googleapis.com/css2?family=${family}:wght@${weight}&display=swap&subset=latin")

	# Take the first woff2 in the latin block. The API emits the subsets in a
	# fixed order with latin last, so the last src wins; grep them all and take
	# the final one.
	url=$(grep -o 'https://[^)]*\.woff2' <<<"$css" | tail -1)
	if [[ -z "$url" ]]; then
		echo "  FAIL  $out — no woff2 in the stylesheet response" >&2
		return 1
	fi

	curl -fsSL -o "$dest/$out" "$url"
	echo "  got   $out ($(du -h "$dest/$out" | cut -f1))"
}

echo "Fetching fonts into $dest"
fetch vt323-latin-400.woff2 'VT323' 400
fetch jetbrains-mono-latin-400.woff2 'JetBrains+Mono' 400
fetch jetbrains-mono-latin-700.woff2 'JetBrains+Mono' 700
fetch ibm-plex-sans-latin-400.woff2 'IBM+Plex+Sans' 400
fetch ibm-plex-sans-latin-600.woff2 'IBM+Plex+Sans' 600

echo
echo "Done. Commit assets/static/fonts/ — the binary embeds whatever is there."
echo "Licences: VT323 and IBM Plex Sans are OFL-1.1; JetBrains Mono is OFL-1.1."
