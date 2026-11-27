#!/bin/sh
# Gera /env-config.js em runtime com as variáveis VITE_* do container,
# permitindo usar a mesma imagem em qualquer ambiente.
set -e
TARGET=/usr/share/nginx/html/env-config.js
esc() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }
{
  echo "window.__ENV__ = {"
  echo "  VITE_API_URL: \"$(esc "${VITE_API_URL:-}")\","
  echo "};"
} > "$TARGET"
echo "env-config.js gerado (VITE_API_URL=${VITE_API_URL:-<vazio>})"
