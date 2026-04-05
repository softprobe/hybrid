/**
 * Redis responseHook coverage: tags span attributes from command/response data.
 * Cassette persistence now happens at the executor boundary in replay.ts, where
 * request-scoped Softprobe runtime is still definitely available.
 */

import * as otelApi from '@opentelemetry/api';
import { AsyncHooksContextManager } from '@opentelemetry/context-async-hooks';
import { buildRedisResponseHook } from '../instrumentations/redis/capture';

describe('Redis capture (Task 10.5)', () => {
  beforeAll(() => {
    const contextManager = new AsyncHooksContextManager();
    contextManager.enable();
    otelApi.context.setGlobalContextManager(contextManager);
  });

  it('tags span attributes from command args and response payload', () => {
    const setAttribute = jest.fn<void, [string, unknown]>();
    const responseHook = buildRedisResponseHook();
    const stubResponse = { cached: true, id: 42 };
    const mockSpan = {
      setAttribute,
    };

    responseHook(mockSpan, 'GET', ['user:1'], stubResponse);

    expect(setAttribute).toHaveBeenCalledWith('softprobe.protocol', 'redis');
    expect(setAttribute).toHaveBeenCalledWith('softprobe.identifier', 'GET user:1');
    expect(setAttribute).toHaveBeenCalledWith('softprobe.request.body', JSON.stringify(['user:1']));
    expect(setAttribute).toHaveBeenCalledWith('softprobe.response.body', JSON.stringify(stubResponse));
  });

  it('handles missing response payload without writing response body attribute', () => {
    const setAttribute = jest.fn<void, [string, unknown]>();
    const responseHook = buildRedisResponseHook();
    const mockSpan = {
      setAttribute,
    };

    responseHook(mockSpan, 'PING', [], undefined);

    expect(setAttribute).toHaveBeenCalledWith('softprobe.protocol', 'redis');
    expect(setAttribute).toHaveBeenCalledWith('softprobe.identifier', 'PING');
    expect(setAttribute).not.toHaveBeenCalledWith('softprobe.response.body', expect.anything());
  });
});
