# MQTT IOServer Performance Test 2026-05-28

## Test Scope

- Broker: `tcp://127.0.0.1:1883`
- Topic: `datachange_S_KIO_Project`
- QoS: `2`
- Protocol: KingIOT/KIO data change payload
- Purpose: verify IOServer MQTT update throughput, variable coverage, and receive latency.

## Result

The 10-minute checkpoint was reached successfully.

```text
elapsed: 600s
messages: 603
variable updates: 236734
unique variables: 432
average latency: 142.8 ms
p95 latency: 241.5 ms
max latency: 320.4 ms
stale variables > 5s: 1
bad quality count: 0
```

The test process was then stopped. Because command output was buffered, the process continued briefly before it was killed; the final observed line before termination was:

```text
elapsed: 960s
messages: 963
variable updates: 378511
unique variables: 432
average latency: 143.7 ms
p95 latency: 242.4 ms
max latency: 320.4 ms
stale variables > 5s: 1
bad quality count: 0
```

No continuing `mqttperf` process remained after termination.

## Observations

- IOServer MQTT publish rate was about 1 message per second.
- Each message carried hundreds of variable changes.
- The backend observed 432 unique variables during the test window.
- Latency remained stable around 140 ms average and about 240 ms P95.
- Quality code stayed good for all observed updates.
- No broad update stalls were observed. At individual checkpoints, `stale > 5s` was usually `0`, occasionally `1` or `2`.

## Interpretation

The IOServer MQTT path is sufficient for the current observed load. The only unresolved question is why the observed unique variable count is 432 instead of the expected 500. That may be because the current KIO source does not publish all 500 variables in the tested interval, or because some variables are static and do not appear in change payloads.
