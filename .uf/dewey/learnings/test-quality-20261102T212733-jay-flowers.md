---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-02T21:27:33Z
identity: test-quality-20261102T212733-jay-flowers
tier: draft
---

Deterministic HTTP-client testing seams used by dewey's openaicompat package (worth reusing for any retrying HTTP client): (1) inject the retry sleep — Client.waitFn is replaced in tests so 429-retry and Retry-After tests run in milliseconds while still asserting the delay values passed to the seam; (2) manipulate the probe cache under the client's mutex (client.mu.Lock(); client.lastCheck = time.Now().Add(-availabilityCacheTTL - time.Second)) instead of sleeping past the TTL; (3) use refused loopback ports (http://127.0.0.1:1) or TEST-NET addresses (http://192.0.2.1:1) for unavailability paths — failures are immediate or bounded by the one-second probe timeout, so tests stay fast on any network; (4) blank credential env vars with t.Setenv(name, "") at test start so ambient developer credentials (DEWEY_API_KEY, OPENROUTER_API_KEY) cannot flip assertions; (5) point stub-server endpoints at loopback hosts so cleartext-credential warnings stay deterministic regardless of ambient keys.
