import { describe, expect, it } from 'vitest';
import type { ProtoInt64 } from '@/api/http';
import type { WsSubscribe } from './types';

describe('WebSocket contracts', () => {
  it('serializes Agent assetIds as ProtoInt64 strings', () => {
    const assetIds: ProtoInt64[] = ['9007199254740993', '9'];
    const frame: WsSubscribe = {
      action: 'input',
      channel: 'agent-chat',
      assetIds,
    };

    expect(JSON.parse(JSON.stringify(frame)).assetIds).toEqual(['9007199254740993', '9']);
  });
});
