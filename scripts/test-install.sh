#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM
mkdir -p "$tmp/bin" "$tmp/source" "$tmp/out"
printf '#!/bin/sh\necho fixture\n' > "$tmp/source/specscore"
tar -czf "$tmp/specscore_1.2.3_linux_amd64.tar.gz" -C "$tmp/source" specscore
sha256sum "$tmp/specscore_1.2.3_linux_amd64.tar.gz" | awk '{print $1 "  specscore_1.2.3_linux_amd64.tar.gz"}' > "$tmp/checksums.txt"
cat > "$tmp/bin/uname" <<'MOCK'
#!/bin/sh
case "$1" in -s) echo Linux;; -m) echo x86_64;; esac
MOCK
cat > "$tmp/bin/curl" <<'MOCK'
#!/bin/sh
while [ "$#" -gt 0 ]; do
  case "$1" in -o) out=$2; shift 2;; https:*) url=$1; shift;; *) shift;; esac
done
case "$url" in
  https://github.com/specscore/specscore-cli/releases/download/v1.2.3/specscore_1.2.3_checksums.txt) [ "${MISSING_MANIFEST:-}" != 1 ] || exit 22; cp "$FIXTURE/checksums.txt" "$out";;
  https://github.com/specscore/specscore-cli/releases/download/v1.2.3/specscore_1.2.3_linux_amd64.tar.gz) cp "$FIXTURE/specscore_1.2.3_linux_amd64.tar.gz" "$out";;
  *) exit 22;;
esac
MOCK
chmod +x "$tmp/bin/uname" "$tmp/bin/curl"
export FIXTURE="$tmp" SPECSCORE_VERSION=v1.2.3 SPECSCORE_INSTALL_DIR="$tmp/out" PATH="$tmp/bin:$PATH"
sh "$root/scripts/install.sh" > /dev/null
[ -f "$tmp/out/specscore" ]
rm "$tmp/out/specscore"
if MISSING_MANIFEST=1 sh "$root/scripts/install.sh" > /dev/null 2>&1; then echo 'missing manifest accepted' >&2; exit 1; fi
printf '%064d  specscore_1.2.3_linux_amd64.tar.gz\n' 0 > "$tmp/checksums.txt"
if sh "$root/scripts/install.sh" > /dev/null 2>&1; then echo 'checksum mismatch accepted' >&2; exit 1; fi
[ ! -e "$tmp/out/specscore" ]
printf '%s\n' 'installer checksum tests passed'
