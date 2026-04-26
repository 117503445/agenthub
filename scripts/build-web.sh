#!/usr/bin/env sh
set -eu

tmp_dir="$(mktemp -d)"

restore_dist() {
  rm -rf ./cmd/web/dist
  mkdir -p ./cmd/web/dist
  if [ -d "$tmp_dir/dist" ]; then
    cp -R "$tmp_dir/dist/." ./cmd/web/dist/
  fi
  rm -rf "$tmp_dir"
}

trap restore_dist EXIT

mkdir -p ./cmd/web/dist ./data/web
cp -R ./cmd/web/dist "$tmp_dir/dist"

rm -rf ./cmd/web/dist
mkdir -p ./cmd/web/dist
cp -R ./fe/dist/. ./cmd/web/dist/

go build -o ./data/web/web ./cmd/web
