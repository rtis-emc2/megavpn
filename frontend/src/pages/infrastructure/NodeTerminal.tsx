import { Eraser, PlugZap, Unplug } from 'lucide-react';
import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { nodeSshTerminalPath } from '../../shared/api/endpoints';
import { Button, StatusBadge, Toolbar } from '../../shared/ui';

const maxTerminalOutputChars = 256 * 1024;
const maxTerminalBufferedBytes = 1024 * 1024;

export type NodeTerminalSession = {
  nodeId: string;
  url: string;
  expiresAt?: string;
};

type BrowserLocation = Pick<Location, 'origin' | 'protocol'>;

type TerminalMessage = {
  type?: string;
  data?: string;
  message?: string;
};

export function terminalWebSocketURL(rawURL: string, nodeId: string, browserLocation: BrowserLocation = window.location): string {
  const parsed = new URL(rawURL, browserLocation.origin);
  if (parsed.origin !== browserLocation.origin) {
    throw new Error('terminal URL must use the control-plane origin');
  }
  const expectedPath = nodeSshTerminalPath(nodeId);
  if (parsed.pathname !== expectedPath || !parsed.searchParams.get('session')) {
    throw new Error('terminal URL does not match the selected node');
  }
  parsed.protocol = browserLocation.protocol === 'https:' ? 'wss:' : 'ws:';
  return parsed.toString();
}

function terminalKeyData(event: KeyboardEvent<HTMLElement>): string {
  if (event.ctrlKey && !event.metaKey && !event.altKey && event.key.length === 1) {
    const code = event.key.toUpperCase().charCodeAt(0);
    if (code >= 64 && code <= 95) return String.fromCharCode(code - 64);
  }
  if (event.metaKey || event.altKey) return '';
  switch (event.key) {
    case 'Enter': return '\r';
    case 'Backspace': return '\x7f';
    case 'Tab': return '\t';
    case 'Escape': return '\x1b';
    case 'ArrowUp': return '\x1b[A';
    case 'ArrowDown': return '\x1b[B';
    case 'ArrowRight': return '\x1b[C';
    case 'ArrowLeft': return '\x1b[D';
    case 'Home': return '\x1b[H';
    case 'End': return '\x1b[F';
    case 'Delete': return '\x1b[3~';
    case 'PageUp': return '\x1b[5~';
    case 'PageDown': return '\x1b[6~';
    default: return event.key.length === 1 ? event.key : '';
  }
}

export function NodeTerminal({ session, onDisconnect }: { session: NodeTerminalSession; onDisconnect: () => void }) {
  const { t } = useTranslation();
  const socketRef = useRef<WebSocket | null>(null);
  const surfaceRef = useRef<HTMLDivElement | null>(null);
  const [status, setStatus] = useState<'pending' | 'active' | 'failed' | 'stopped'>('pending');
  const [output, setOutput] = useState('');
  const connection = useMemo(() => {
    try {
      return { url: terminalWebSocketURL(session.url, session.nodeId), error: '' };
    } catch (error) {
      return { url: '', error: error instanceof Error ? error.message : String(error) };
    }
  }, [session.nodeId, session.url]);

  const appendOutput = (value: string) => {
    if (!value) return;
    setOutput((current) => (current + value).slice(-maxTerminalOutputChars));
  };

  useEffect(() => {
    if (!connection.url) return undefined;
    const socket = new WebSocket(connection.url);
    socketRef.current = socket;

    socket.addEventListener('open', () => {
      if (socketRef.current !== socket) return;
      setStatus('active');
      surfaceRef.current?.focus();
    });
    socket.addEventListener('message', (event) => {
      if (socketRef.current !== socket || typeof event.data !== 'string') return;
      let message: TerminalMessage;
      try {
        message = JSON.parse(event.data) as TerminalMessage;
      } catch {
        setStatus('failed');
        appendOutput(`${t('nodes.terminalInvalidResponse')}\n`);
        socket.close(1002, 'invalid terminal response');
        return;
      }
      if (message.type === 'output') appendOutput(String(message.data || ''));
      if (message.type === 'status') setStatus('active');
      if (message.type === 'error') {
        setStatus('failed');
        appendOutput(`\n${String(message.message || t('nodes.terminalConnectionFailed'))}\n`);
      }
      if (message.type === 'exit') {
        setStatus('stopped');
        appendOutput(`\n${String(message.message || t('nodes.terminalClosed'))}\n`);
      }
    });
    socket.addEventListener('error', () => {
      if (socketRef.current !== socket) return;
      setStatus('failed');
      appendOutput(`\n${t('nodes.terminalConnectionFailed')}\n`);
    });
    socket.addEventListener('close', () => {
      if (socketRef.current !== socket) return;
      socketRef.current = null;
      setStatus((current) => current === 'failed' ? current : 'stopped');
    });

    return () => {
      if (socketRef.current === socket) socketRef.current = null;
      socket.close();
    };
  }, [connection.url, t]);

  useEffect(() => {
    if (!session.expiresAt) return undefined;
    const expiresAt = Date.parse(session.expiresAt);
    if (!Number.isFinite(expiresAt)) return undefined;
    const delay = Math.max(0, Math.min(expiresAt - Date.now(), 2_147_483_647));
    const timeout = window.setTimeout(() => {
      socketRef.current?.close(1000, 'terminal ticket expired');
      setStatus('stopped');
    }, delay);
    return () => window.clearTimeout(timeout);
  }, [session.expiresAt]);

  useEffect(() => {
    const surface = surfaceRef.current;
    if (surface) surface.scrollTop = surface.scrollHeight;
  }, [output]);

  const send = (data: string) => {
    const socket = socketRef.current;
    if (!data || !socket || socket.readyState !== WebSocket.OPEN) return;
    if (socket.bufferedAmount > maxTerminalBufferedBytes) {
      setStatus('failed');
      appendOutput(`\n${t('nodes.terminalBackpressure')}\n`);
      socket.close(1013, 'terminal input backpressure');
      return;
    }
    socket.send(JSON.stringify({ type: 'input', data }));
  };

  const disconnect = () => {
    const socket = socketRef.current;
    if (socket?.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify({ type: 'close' }));
    }
    socket?.close(1000, 'operator disconnected');
    onDisconnect();
  };

  return (
    <div className="page-stack">
      <Toolbar>
        <StatusBadge status={connection.error ? 'failed' : status} />
        <Button icon={<Unplug size={16} />} onClick={disconnect}>{t('nodes.terminalDisconnect')}</Button>
        <Button icon={<Eraser size={16} />} onClick={() => setOutput('')}>{t('nodes.terminalClear')}</Button>
      </Toolbar>
      <div
        ref={surfaceRef}
        className="web-terminal"
        role="application"
        tabIndex={0}
        aria-label={t('nodes.terminalSurface')}
        onClick={() => surfaceRef.current?.focus()}
        onKeyDown={(event) => {
          const data = terminalKeyData(event);
          if (!data) return;
          event.preventDefault();
          send(data);
        }}
        onPaste={(event) => {
          const data = event.clipboardData.getData('text/plain');
          if (!data) return;
          event.preventDefault();
          send(data);
        }}
      >
        <pre className="web-terminal-output">{connection.error || output || t('nodes.terminalConnecting')}</pre>
      </div>
      <p className="muted"><PlugZap size={14} aria-hidden="true" /> {t('nodes.terminalInputHint')}</p>
    </div>
  );
}
