#!/usr/bin/env bash
# Sobe o ambiente local: ./dev.sh        (só neste computador)
#                        ./dev.sh --lan  (urna acessível pelo celular na mesma rede)
set -euo pipefail
cd "$(dirname "$0")"

set -a; source .env.local; set +a

HOST=localhost
if [[ "${1:-}" == "--lan" ]]; then
  HOST=$(ipconfig getifaddr en0 || ipconfig getifaddr en1) || { echo "IP da rede não encontrado"; exit 1; }
fi
export FRONTEND_URL="http://$HOST:5173,http://localhost:5173"
export VITE_API_URL="http://$HOST:$PORT"

[[ -d frontend/node_modules ]] || (cd frontend && npm ci)

(cd backend && go build -o bin/api ./cmd/api && go run ./cmd/seed)

trap 'kill 0' EXIT
backend/bin/api &
(cd frontend && exec npm run dev -- --strictPort) &

echo
echo "  Aplicação: http://$HOST:5173"
echo "  Login:     $ADMIN_EMAIL / $ADMIN_PASSWORD"
echo "  Ctrl+C para parar"
echo
wait
