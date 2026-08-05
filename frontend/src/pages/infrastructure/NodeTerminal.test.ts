import { describe, expect, it } from 'vitest';
import { terminalWebSocketURL } from './NodeTerminal';

describe('terminalWebSocketURL', () => {
  it('converts a same-origin HTTPS terminal ticket to WSS', () => {
    expect(terminalWebSocketURL(
      '/api/v1/nodes/node-1/ssh/terminal?session=opaque-ticket',
      'node-1',
      { origin: 'https://cp.example', protocol: 'https:' },
    )).toBe('wss://cp.example/api/v1/nodes/node-1/ssh/terminal?session=opaque-ticket');
  });

  it('converts a same-origin HTTP terminal ticket to WS', () => {
    expect(terminalWebSocketURL(
      'http://127.0.0.1:8080/api/v1/nodes/node-1/ssh/terminal?session=opaque-ticket',
      'node-1',
      { origin: 'http://127.0.0.1:8080', protocol: 'http:' },
    )).toBe('ws://127.0.0.1:8080/api/v1/nodes/node-1/ssh/terminal?session=opaque-ticket');
  });

  it('rejects a terminal URL on another origin', () => {
    expect(() => terminalWebSocketURL(
      'https://attacker.example/api/v1/nodes/node-1/ssh/terminal?session=stolen',
      'node-1',
      { origin: 'https://cp.example', protocol: 'https:' },
    )).toThrow('terminal URL must use the control-plane origin');
  });

  it('rejects a ticket for another node or without a session', () => {
    const browserLocation = { origin: 'https://cp.example', protocol: 'https:' };
    expect(() => terminalWebSocketURL(
      '/api/v1/nodes/node-2/ssh/terminal?session=opaque-ticket',
      'node-1',
      browserLocation,
    )).toThrow('terminal URL does not match the selected node');
    expect(() => terminalWebSocketURL(
      '/api/v1/nodes/node-1/ssh/terminal',
      'node-1',
      browserLocation,
    )).toThrow('terminal URL does not match the selected node');
  });
});
