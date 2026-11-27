import { useCallback, useEffect, useRef, useState } from 'react';
import type { WsMessage } from './types';

/**
 * WebSocket com reconexão automática (backoff) e keep-alive.
 * `url` nulo desliga a conexão.
 */
export function useSocket(url: string | null, onMessage: (msg: WsMessage) => void, onOpen?: () => void) {
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const handlerRef = useRef(onMessage);
  const openRef = useRef(onOpen);
  handlerRef.current = onMessage;
  openRef.current = onOpen;

  useEffect(() => {
    if (!url) return;
    let stopped = false;
    let retry = 0;
    let timer: number | undefined;
    let ping: number | undefined;

    const connect = () => {
      const ws = new WebSocket(url);
      wsRef.current = ws;
      ws.onopen = () => {
        retry = 0;
        setConnected(true);
        openRef.current?.();
        ping = window.setInterval(() => ws.readyState === 1 && ws.send(JSON.stringify({ type: 'ping' })), 20000);
      };
      ws.onmessage = (ev) => {
        try {
          const msg = JSON.parse(ev.data) as WsMessage;
          if (msg.type !== 'pong') handlerRef.current(msg);
        } catch {
          /* ignora */
        }
      };
      ws.onclose = () => {
        setConnected(false);
        window.clearInterval(ping);
        if (stopped) return;
        retry = Math.min(retry + 1, 6);
        timer = window.setTimeout(connect, 500 * 2 ** retry);
      };
      ws.onerror = () => ws.close();
    };
    connect();

    const onVisible = () => {
      if (document.visibilityState === 'visible' && wsRef.current?.readyState === WebSocket.CLOSED) {
        window.clearTimeout(timer);
        connect();
      }
    };
    document.addEventListener('visibilitychange', onVisible);

    return () => {
      stopped = true;
      window.clearTimeout(timer);
      window.clearInterval(ping);
      document.removeEventListener('visibilitychange', onVisible);
      wsRef.current?.close();
    };
  }, [url]);

  const send = useCallback((msg: WsMessage) => {
    const ws = wsRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg));
  }, []);

  return { connected, send };
}
