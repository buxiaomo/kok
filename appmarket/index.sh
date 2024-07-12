#!/usr/bin/env bash
rm -rf assets/*.tgz
for dir in $(ls charts);do
pushd charts/${dir} >/dev/null 2>&1
  ls | xargs helm package --destination ../../assets
popd >/dev/null 2>&1
done
