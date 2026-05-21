# VC Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).

## Key catalog (2)

| EventKey | Purpose |
|---|---|
| `vc.meeting.participant_meeting_ended_v1` | A meeting the current user participates in has ended |
| `vc.note.generated_v1` | A note has been generated (meeting, realtime recording, local file upload, etc.) |

Both keys use a **Custom schema** (flat output at `.xxx`) and carry a **PreConsume hook** that auto-subscribes / unsubscribes via OAPI on first / last consumer.

## Scopes & auth

| EventKey | Scope | Auth |
|---|---|---|
| `vc.meeting.participant_meeting_ended_v1` | `vc:meeting.meetingevent:read` | user |
| `vc.note.generated_v1` | `vc:note:read` | user |

Both require `--as user`.

## `vc.meeting.participant_meeting_ended_v1`

### Output fields

| Field | Type | Description |
|---|---|---|
| `type` | string | Event type; always `vc.meeting.participant_meeting_ended_v1` |
| `event_id` | string | Globally unique event ID; safe for deduplication |
| `timestamp` | string (timestamp_ms) | Event delivery time (ms timestamp string) |
| `meeting_id` | string | Meeting ID |
| `topic` | string | Meeting topic |
| `meeting_no` | string | Meeting number |
| `start_time` | string | Meeting start time in RFC3339, converted to the local timezone |
| `end_time` | string | Meeting end time in RFC3339, converted to the local timezone |
| `calendar_event_id` | string | Calendar event ID associated with the meeting |

### Gotchas

- `start_time` / `end_time` are **not** the raw unix-seconds from OAPI — the Process hook converts them to local-timezone RFC3339. If the raw value is empty or non-numeric, the field is left empty.
- No detail API call is made; all fields come from the event payload itself.

### Example

```bash
lark-cli event consume vc.meeting.participant_meeting_ended_v1 --as user

# Project meeting topic and end time only
lark-cli event consume vc.meeting.participant_meeting_ended_v1 --as user \
  --jq '{meeting: .meeting_id, topic: .topic, ended: .end_time}'
```

## `vc.note.generated_v1`

### Output fields

| Field | Type | Description |
|---|---|---|
| `type` | string | Event type; always `vc.note.generated_v1` |
| `event_id` | string | Globally unique event ID; safe for deduplication |
| `timestamp` | string (timestamp_ms) | Event delivery time (ms timestamp string) |
| `note_id` | string | Note ID |
| `note_token` | string | Generated note document token (enriched via detail API) |
| `verbatim_token` | string | Generated verbatim document token (enriched via detail API) |
| `note_source` | object | Note source metadata (enriched via detail API); only present when the source is a meeting |
| `note_source.source_type` | string | Source type; only present when the source is a meeting (value: `meeting`) |
| `note_source.source_entity_id` | string | Source entity ID (meeting ID); only present when the source is a meeting |

### Enrichment & degradation

The Process hook calls `GET /open-apis/vc/v1/notes/{note_id}` to enrich `note_token`, `verbatim_token`, and `note_source`. If the detail API fails, these fields are left empty — the base fields (`type`, `event_id`, `timestamp`, `note_id`) are always present.

### Source type semantics

This event fires for **all** note generation scenarios, not just meetings:

| `source_type` | Trigger |
|---|---|
| `meeting` | Note generated from a meeting |
| `realtime_recording` | Note generated from a realtime recording |
| `upload_local_file` | Note generated from an uploaded local file |

### Example

```bash
lark-cli event consume vc.note.generated_v1 --as user

# Only note tokens, skip events where enrichment failed
lark-cli event consume vc.note.generated_v1 --as user \
  --jq 'select(.note_token != null) | {note_id, note_token, verbatim_token}'

# Filter by source type
lark-cli event consume vc.note.generated_v1 --as user \
  --jq 'select(.note_source.source_type == "meeting") | {note_id, topic: .note_source.source_entity_id}'
```
