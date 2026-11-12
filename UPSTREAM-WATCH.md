# Upstream-blocker watchlist

Capabilities that are broken by upstream (grok.com / console.x.ai / Grok Build)
changes, not by our code or config. Last checked: 2026-11-12 on
`v3.1.8+cheapai.1` (`59680e62`).

Every canary reads the client key from the environment and prints only the
status line or a short verdict:

```bash
export G=https://grok.cheapaiapi.org/v1   # or the farm under test
export K=...                              # a client key, never committed
```

Rules: run canaries one at a time. Never run a pool-wide or concurrent quota
refresh to "help" a canary: bursts get every Web egress IP throttled for 10
minutes (incident 2026-11-12).

| Capability | Symptom | Our error code | Upstream | Fixed when |
|---|---|---|---|---|
| Web `grok-imagine-image-2.0`, `grok-imagine-image` (quality) | Imagine WebSocket returns an `error` frame right after submit | 502 `upstream_unavailable` (log: `Imagine WebSocket 返回错误`) | [#1102](https://github.com/chenyme/grok2api/issues/1102), [#1104](https://github.com/chenyme/grok2api/issues/1104) (`castle_request_token`, `grok-site-verification` removed, statsig signer NXDOMAIN) | canary 1 prints `200` and a `data[0].url` |
| Web image edits `grok-imagine-image-edit` | Basic accounts report `imageEdit`/`imagePro` remaining 0, so selection refuses before submit | 503 `upstream_not_submitted` | same as above | canary 2 prints `200` |
| Web video `grok-imagine-video`, `grok-imagine-video-live` | `quota_info` reports `video720p` 0 on every Basic account; a submitted job fails with `Grok Web 媒体上游返回 429: 8: Too many requests` | job `status=failed`, `service_unavailable` | no issue yet; free-tier Imagine allowance is per-account and was cut for many accounts after 2026-09 | canary 3 ends with `completed` |
| Console quota sync, and every Console model: `grok-4.5`, `grok-4.3`, `grok-4.20-*`, `grok-build-0.1`, `grok-imagine-*` Console routes, `grok-voice-*` (TTS), `grok-stt` (STT) | All Console accounts show quota 0 / waiting reset; admin refresh-quota fails | 429 `upstream_quota_exhausted` | [#1098](https://github.com/chenyme/grok2api/issues/1098) | canaries 4–6 print `200` |
| Build pool (`grok-4.6`, `grok-composer-2.5-fast`) | The only Build account needs re-login; route has no accounts | 404 `model_not_found` | [#1078](https://github.com/chenyme/grok2api/issues/1078) (device flow / `castle_request_token`) | canary 7 prints `200` after re-login |

## Canaries (one line each)

```bash
# 1 Web image 2.0
curl -s -o /tmp/c1.json -w '%{http_code}\n' -H "Authorization: Bearer $K" -H 'Content-Type: application/json' $G/images/generations -d '{"model":"grok-imagine-image-2.0","prompt":"a red cube on a white table","n":1}'; jq -r '.data[0].url // .error.code' /tmp/c1.json
# 2 Web image edit (input: any public image URL)
curl -s -o /tmp/c2.json -w '%{http_code}\n' -H "Authorization: Bearer $K" -H 'Content-Type: application/json' $G/images/edits -d '{"model":"grok-imagine-image-edit","prompt":"make it blue","image":{"url":"https://upload.wikimedia.org/wikipedia/commons/thumb/1/15/Red_Apple.jpg/320px-Red_Apple.jpg"}}'; jq -r '.data[0].url // .error.code' /tmp/c2.json
# 3 Web video (create, then poll for ~3 minutes)
id=$(curl -s -H "Authorization: Bearer $K" -H 'Content-Type: application/json' $G/videos/generations -d '{"model":"grok-imagine-video","prompt":"a candle flame flickering","duration":6}' | jq -r .request_id); for i in $(seq 18); do s=$(curl -s -H "Authorization: Bearer $K" $G/videos/$id | jq -r .status); echo $s; case $s in completed|failed) break;; esac; sleep 10; done
# 4 Console chat
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $K" -H 'Content-Type: application/json' $G/chat/completions -d '{"model":"grok-4.5","messages":[{"role":"user","content":"Reply: ok"}],"max_tokens":8}'
# 5 Console TTS
curl -s -o /tmp/c5.mp3 -w '%{http_code}\n' -H "Authorization: Bearer $K" -H 'Content-Type: application/json' $G/audio/speech -d '{"model":"grok-voice-latest","input":"ok","voice":"ara"}'
# 6 Console STT (uses the canary-5 audio)
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $K" -F model=grok-stt -F file=@/tmp/c5.mp3 $G/audio/transcriptions
# 7 Build
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $K" -H 'Content-Type: application/json' $G/chat/completions -d '{"model":"grok-4.6","messages":[{"role":"user","content":"Reply: ok"}],"max_tokens":8}'
```

Known-good baselines (should stay `200`): `grok-chat-fast` chat,
`/v1/responses` with `web_search` (expect `x_search_call` items with x.com
sources), and `grok-imagine-image-lite`.

## Procedure

On every upstream merge, run all canaries above against the candidate
deployment. Update this file: remove fixed rows, add new breakages, and refresh
the "Last checked" line.
