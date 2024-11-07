#!/usr/bin/env bash
set -x
git clone https://github.com/buxiaomo/appmarket.git /app/appmarket
./appmarket/index.sh
exec /app/main