#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
	printf 'usage: %s SOURCE.png OUTPUT.icns ICONSET_DIR\n' "$0" >&2
	exit 2
fi

source_png=$1
output_icns=$2
iconset_dir=$3
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
case "$source_png" in /*) ;; *) source_png="$repo_root/$source_png" ;; esac
case "$output_icns" in /*) ;; *) output_icns="$repo_root/$output_icns" ;; esac
case "$iconset_dir" in /*) ;; *) iconset_dir="$repo_root/$iconset_dir" ;; esac
work_dir="$(dirname "$iconset_dir")/$(basename "$iconset_dir" .iconset).icns-build"

mkdir -p "$(dirname "$output_icns")" "$iconset_dir" "$work_dir"
for size in 16 32 128 256 512; do
	double_size=$((size * 2))
	sips -z "$size" "$size" "$source_png" --out "$iconset_dir/icon_${size}x${size}.png" >/dev/null
	sips -z "$double_size" "$double_size" "$source_png" --out "$iconset_dir/icon_${size}x${size}@2x.png" >/dev/null
done

for size in 16 32 128 256 512; do
	sips -s format icns "$iconset_dir/icon_${size}x${size}.png" --out "$work_dir/icon-${size}.icns" >/dev/null
done
cd "$repo_root"
go run packaging/macos/combine-icns.go "$output_icns" \
	"$work_dir/icon-16.icns" \
	"$work_dir/icon-32.icns" \
	"$work_dir/icon-128.icns" \
	"$work_dir/icon-256.icns" \
	"$work_dir/icon-512.icns" \
	--png icp6 "$iconset_dir/icon_32x32@2x.png" \
	--png ic10 "$iconset_dir/icon_512x512@2x.png"
