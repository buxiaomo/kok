#!/usr/bin/env bash
set -x
pushd $(dirname $0)
rm -rf ./assets/*
mkdir -p ./assets
pushd ./charts >/dev/null 2>&1
for dir in $(ls .);do
pushd ${dir} >/dev/null 2>&1
  set -e
  ls | xargs -I{} helm lint {} --values {}/values.yaml
  mkdir -p ../../assets/${dir}
  ls | xargs helm package --destination ../../assets/${dir}
popd >/dev/null 2>&1
done
popd >/dev/null 2>&1
helm repo index ./assets
