# lark-cli Error Contract

`errs/` defines a typed, RFC 7807–aligned error taxonomy for the CLI. Three
audiences depend on it: **AI agents and shell scripts** parsing the JSON
envelope on stderr; **protocol adapters** mapping CLI errors into MCP /
OAuth shapes; and **framework + business code** producing errors. This file
is the single source of truth for all three.

Migrating an `*output.ExitError` call site? See **Migration**. Something off
in production? See **Troubleshooting**.

## Invariants

1. Every error belongs to exactly one **Category**. The set is closed
   (`errs/category.go`); adding a member requires deliberate review.
2. Every **newly constructed** typed error has a **Subtype** — a stable
   lowercase-with-underscores identifier declared in `errs/subtypes*.go`.
   Undeclared subtypes fail CI. The constraint applies only to typed
   `*errs.*` literals; stage-1 legacy `*core.ConfigError` flows via the
   dispatcher's `asExitError` → legacy envelope path (not the typed
   taxonomy) and is unaffected. `errcompat.PromoteConfigError` is a
   stage-1 passthrough; its stage-2+ typed migration will subject the
   promoted typed error to this Subtype constraint at that time.
3. **`Category` + `Subtype`** are wire-stable identifiers consumers may
   branch on. Renaming either is a breaking change.
4. `Code` is the upstream numeric code when known (e.g. Lark API code).
   It is `omitempty` and never carries CLI-internal meaning.
5. Every typed error embeds `errs.Problem`. `CheckProblemEmbed` rejects
   exported `*Error` structs that do not.
6. Wrapping is idempotent: re-wrapping an already-typed error returns it
   unchanged across the `errors.As` / `errors.Unwrap` chain.
7. For the typed-envelope path, exit codes derive from `Category` only
   via `output.ExitCodeForCategory`. Two stage-1 exceptions:
   `SecurityPolicyError` always exits `1` (fixed by its legacy envelope),
   and unmigrated `*output.ExitError` producers carry a hand-set `Code`;
   both are retired in the legacy-removal stage.

## Wire format

Typed errors render to **stderr** as one JSON object per process exit:

```json
{
  "ok": false,
  "identity": "user",
  "error": {
    "type": "authorization",
    "subtype": "missing_scope",
    "code": 99991679,
    "message": "missing scope `calendar:event:create` for app cli_xxx",
    "hint": "run lark-cli auth login --scope calendar:event:create",
    "log_id": "20260520-0a1b2c3d",
    "missing_scopes": ["calendar:event:create"],
    "console_url": "https://open.feishu.cn/app/cli_xxx/auth?q=..."
  }
}
```

| Field | Stability | Notes |
|-------|-----------|-------|
| `ok` | wire-stable | always `false` for errors |
| `identity` | wire-stable | `user` \| `bot` — caller identity; omitted when not resolved |
| `error.type` | **wire-stable** | one of the 9 Categories |
| `error.subtype` | **wire-stable** | declared Subtype constant |
| `error.code` | wire-stable | upstream numeric code, omitted when zero |
| `error.message` | informational | not safe to branch on |
| `error.hint` | informational | actionable recovery guidance |
| `error.log_id` | informational | upstream request id (server-side trace) |
| `error.retryable` | wire-stable | `true` when present; omitted when `false` |
| per-Subtype extension fields | per-Subtype-stable | e.g. `missing_scopes`, `console_url`, `challenge_url` |

Carve-out: `SecurityPolicyError` keeps the legacy
`{type: "auth_error", code: "challenge_required"|"access_denied", ...}`
envelope until its consumers migrate. Removal is staged in **Migration**.

## Categories

| Category | When | Exit | Typed struct |
|----------|------|------|--------------|
| `validation` | malformed user input | 2 | `ValidationError` |
| `authentication` | no valid token / login required | 3 | `AuthenticationError` |
| `authorization` | token lacks scope / app permission denied | 3 | `PermissionError` |
| `config` | local config missing / unbound | 3 | `ConfigError` |
| `network` | DNS, refused, timeout, transport | 4 | `NetworkError` |
| `api` | server-side Lark error w/o specific bucket | 1 | `APIError` |
| `policy` | content safety / security challenge | 6 | `SecurityPolicyError`, `ContentSafetyError` |
| `internal` | SDK contract violation / decode failure | 5 | `InternalError` |
| `confirmation` | high-risk action needs `--yes` | 10 | `ConfirmationRequiredError` |

Canonical mapping: `internal/output/exitcode.go` `ExitCodeForCategory`.

> **Note on the `authorization` / `PermissionError` asymmetry.** The wire
> `type` field uses the RFC 7807 / taxonomy-formal name `"authorization"`,
> but the Go type is named `PermissionError`. This is deliberate, following
> the gRPC / Google APIs convention (`codes.Unauthenticated` +
> `codes.PermissionDenied`): each name is chosen to be **maximally
> distinct and readable on its own**, not to be perfectly symmetric.
> `AuthenticationError` and `AuthorizationError` differ visually only at
> the 5th character and are easy to confuse in code review;
> `AuthenticationError` and `PermissionError` cannot be confused. The wire
> field stays formal because it is the protocol-level taxonomy; the Go
> type favors call-site readability.

## Flow

```
  call site
     │ constructs typed error (e.g. *errs.ValidationError)
     ▼
  command runE returns err
     │
     ▼
  cmd/root.go handleRootError dispatches:
     ├─ *errs.SecurityPolicyError → legacy "auth_error" JSON envelope; exit 1
     ├─ typed (errs.ProblemOf)    → typed JSON envelope; exit = ExitCodeOf(err)
     ├─ *core.ConfigError         → asExitError adapts to legacy envelope ↓
     ├─ *output.ExitError         → legacy JSON envelope;  exit = exitErr.Code
     └─ untyped / Cobra error     → plain "Error: <msg>" (no envelope); exit 1
```

Only the typed and `*output.ExitError` branches emit a JSON envelope on
stderr. Untyped errors (including Cobra's "required flag missing" / unknown
subcommand messages) print plain text and exit `1` — consumers must
tolerate that fallback.

## Consumers

### Go (in-process)

```go
var pe *errs.PermissionError
if errors.As(err, &pe) {
    fmt.Println("missing:", pe.MissingScopes)
}
```

Predicates cover the common categories (`errs/predicates.go`):

```go
if errs.IsAuthentication(err)       { ... }
if errs.IsPermission(err) { ... }
if errs.IsValidation(err) { ... }
```

Type-agnostic field access:

```go
if p, ok := errs.ProblemOf(err); ok {
    log.Printf("cat=%s subtype=%s retryable=%t", p.Category, p.Subtype, p.Retryable)
}
exitCode := output.ExitCodeOf(err) // ExitInternal for non-typed errors
```

### Shell / AI

```bash
out=$(lark-cli ... 2>&1)
code=$?

# Untyped / Cobra errors print plain text — guard before jq.
if ! jq -e . >/dev/null 2>&1 <<<"$out"; then
    printf '%s\n' "$out" >&2
    exit "$code"
fi

case "$(jq -r '.error.type // empty' <<<"$out")" in
  authorization) jq -r '.error.missing_scopes[]' <<<"$out" ;;
  network)       echo "transport failure, safe to retry" ;;
  internal)      echo "bug — file an issue with log_id $(jq -r '.error.log_id // "n/a"' <<<"$out")" ;;
esac
```

Unknown fields are forward-compatible additions: ignore, don't fail.

### Protocol adapters

`errs/projection/`:

- `BuildBearerHeader(pe *errs.PermissionError) (string, bool)` — RFC 6750
  `WWW-Authenticate: Bearer scope="...", error="insufficient_scope"`.
  `ok=false` means no header should be emitted (e.g. the Subtype is not in
  the OAuth allowlist) — callers must check before sending.
- `MCPCodeFor(Category, Subtype) int` — JSON-RPC outer code for the MCP host.

Wire shapes are pinned by `errs/projection/*_test.go`.

## Producers

### Quick reference

| Situation | Use |
|-----------|-----|
| Bad user input | `&errs.ValidationError{...}` or `output.ErrValidation(msg)` |
| Login required | `&errs.AuthenticationError{...}` |
| Token lacks scope | `errclass.BuildAPIError(resp, ctx)` |
| Local config missing | `&errs.ConfigError{...}` |
| Transport failure | `&errs.NetworkError{...}` |
| Lark API error | `errclass.BuildAPIError(resp, ctx)` |
| SDK / decode bug | `&errs.InternalError{Problem: errs.Problem{Category: errs.CategoryInternal, Subtype: errs.SubtypeSDKFailure, ...}}` |
| Policy block | `&errs.SecurityPolicyError{...}` or `&errs.ContentSafetyError{...}` |
| Needs `--yes` | `&errs.ConfirmationRequiredError{...}` |

### Construct

```go
return &errs.ValidationError{
    Problem: errs.Problem{
        Category: errs.CategoryValidation,
        Subtype:  errs.SubtypeInvalidArg,
        Message:  fmt.Sprintf("--data must be a valid JSON object: %v", parseErr),
    },
    Param: "--data",
}
```

Use the helpers (`output.ErrAuth`, `output.ErrValidation`, `output.ErrNetwork`)
when only `Message` varies; reach for the struct literal when extension
fields matter.

### Wrap an upstream error

`errs.WrapInternal(err)` lifts an untyped error to `*InternalError`.
Already-typed errors pass through unchanged.

At the SDK boundary use `client.WrapDoAPIError(err)` — it routes transport
failures to `*NetworkError` and everything else to `*InternalError`.

### Add a Subtype

1. Add a constant in `errs/subtypes.go` (framework) or
   `errs/subtypes_service_<name>.go` (service).
2. If it maps from a Lark code, register the mapping in
   `internal/errclass/codemeta_<service>.go`.
3. Add a dispatch test in `internal/errclass/classify_test.go`.
4. Reference the constant from a producer; bare `ad_hoc_*` strings expire
   in one week (`CheckAdHocSubtype` labels them for follow-up).
5. `go run ./cmd/lintcheck .` — `CheckDeclaredSubtype` fails until the
   constant is wired through.

### Add a typed Error struct

Rare; the existing structs cover the 9 Categories with room. If you must:

1. Add the struct in `errs/types.go` embedding `errs.Problem`, with a
   nil-receiver-safe `Unwrap()` if it carries `Cause`.
2. Add an `IsXxx` predicate in `errs/predicates.go`.
3. Add a wire-format pin in `errs/marshal_test.go`.
4. Add a projection if the type maps to MCP / OAuth.

`CheckProblemEmbed` enforces the `Problem` embed at lint time.

## CI guards

| Check | Enforces | Where |
|-------|----------|-------|
| forbidigo | business path (`shortcuts/**`, `cmd/service/**`) must not call legacy `output.*` error constructors — route through the typed classifier | `.golangci.yml` |
| `CheckProblemEmbed` | every exported `*Error` embeds `errs.Problem` | `internal/lintcheck/` AST |
| `CheckNoRegistrar` | no `mergeCodeMeta` / `RegisterServiceMap` from service code | `internal/lintcheck/` AST |
| `CheckAdHocSubtype` | `ad_hoc_*` Subtypes labeled for promotion (warn) | `internal/lintcheck/` AST |
| `CheckDeclaredSubtype` | every `Subtype:` value is a declared constant or `ad_hoc_*` | `internal/lintcheck/` AST |
| `CheckTypedErrorCompleteness` | every `*errs.<X>Error{Problem: errs.Problem{...}}` literal must set `Category`, `Subtype`, and `Message` | `internal/lintcheck/` AST |

CI runs `cmd/lintcheck` on every PR. Locally: `go run ./cmd/lintcheck .`

## Anti-patterns

- **Don't** wrap a typed error with `output.ErrAPI(err, ...)` — it strips
  the typed envelope. Return the typed error directly; the wire layer
  detects it via `errs.ProblemOf`.
- **Don't** keep an `ad_hoc_*` Subtype longer than one week — it exists to
  unblock observation, not as a permanent identifier.
- **Don't** put secrets, tokens, or PII in `Message` or `Hint`. Both are
  surfaced to stderr and may be logged.
- **Don't** pick an exit code by hand in new typed producers — let
  `ExitCodeForCategory` derive it from `Category`. New Category → new
  mapping entry → new exit code. (Legacy `*output.ExitError` and
  `SecurityPolicyError` keep their hand-set codes during stage 1.)
- **Don't** construct `*output.ExitError` in new code. It is documented
  `// Deprecated:` and exists only for unmigrated call sites.
- **Don't** invent new top-level envelope fields. Per-Subtype data goes
  into the typed Error struct as a documented extension field.
- **Don't** branch consumer logic on `error.message` substrings. Only
  `type`, `subtype`, `code`, and typed extension fields are stable.
- **Don't** return a typed-nil pointer (`var pe *errs.PermissionError; return pe`).
  Return the interface `nil` — typed-nil defeats `errors.As`.

## Stability

| Tier | Surface | Change policy |
|------|---------|---------------|
| Wire-stable | `error.type`, `error.subtype`, `error.code`, `error.retryable`, declared extension fields, `Category` enum values | breaking change ⇒ semver major; deprecation window required |
| Additive | new Category, new declared Subtype, new extension field on an existing struct | minor release; consumers ignore unknown fields by contract |
| Experimental | `ad_hoc_*` Subtypes; fields documented as such in `errs/types.go` | may change or be promoted/removed within one release |

The deprecated `*output.ExitError` surface is outside these tiers — it
will be removed once business migration completes.

## Migration

The error-contract refactor lands in stages. This PR is **stage 1**, and
its scope is **strictly framework-only**: every production wire shape
matches pre-PR byte-for-byte (additive fields only where the legacy slot
had no subtype emission). Stage 1 ships infrastructure; behavioural
migration of any specific path lives in later stages.

Stages:

1. **Framework slice — this PR.** Ships the `errs/` typed taxonomy,
   classifier (`internal/errclass`), promotion stub (`internal/errcompat`,
   passthrough in stage 1), projection adapters (`errs/projection`),
   dispatcher hook (`WriteTypedErrorEnvelope`), and six lint guards
   (forbidigo + five AST checks). Wire shapes preserved byte-for-byte
   versus pre-PR, with **one intentional semantic fix**: config-class
   errors (`*core.ConfigError`) now exit `3` instead of `2`, aligning
   with `ExitCodeForCategory` (config errors share the auth exit slot
   per the taxonomy). The classifier and promote helpers are *shipped
   but unused* in production paths — they exist so stage 2+ migrations
   can plug in without re-architecting.
2. **`SecurityPolicyError` typed envelope** — replace the legacy
   `type: "auth_error"` carve-out with the typed shape.
3. **Business-domain migration**, one PR per domain in declared order:
   `task → drive → calendar → im → mail → whiteboard → contact`. Each PR
   moves the domain's `output.ErrAPI(...)` / `output.ErrAuth(...)` /
   `output.ErrWithHint(...)` call sites to typed constructors or
   `BuildAPIError`, removes its Deprecated annotations, and announces the
   wire change explicitly.
4. **Framework-boundary migration**: `client.WrapDoAPIError` and
   `client.WrapJSONResponseParseError` flip to typed wrap;
   `client.CheckResponse` adopts `errclass.BuildAPIError`;
   `internal/client/client.go resolveAccessToken` adopts the typed
   `NeedAuthorizationError → *errs.AuthenticationError` recognition;
   `cmd/auth/scopes.go` and `cmd/service/service.go` adopt typed
   `*errs.PermissionError`; `errcompat.PromoteConfigError` lifts the
   `Type="config"` (and later `Type="auth"`) branches to typed.
5. **Legacy removal** — once `git grep '\*output\.ExitError'` returns no
   production hits, delete `Errorf`, `ErrAPI`, `ErrAuth`, `ErrWithHint`,
   `ErrBare`, `ClassifyLarkError`, `ErrDetail`, `ExitError`,
   `ErrorEnvelope` and their projections.

During migration, helper assertions accept both shapes (see
`shortcuts/mail/mail_shortcut_validation_test.go` `assertValidationError`)
so the build stays green domain-by-domain.

Before / after at a call site (illustrative — actually performed in
stage 3):

```go
// before (legacy)
return output.ErrAPI(larkCode, "create event failed", resp.RawBody())

// after (typed) — cc carries Brand / AppID / Identity from the caller's context
return errclass.BuildAPIError(parsedResp, cc)
```

## Troubleshooting

**Envelope shows `type=api subtype=api_generic` for what should be a more
specific category.** The Lark code is unknown to `LookupCodeMeta` and fell
through to the generic bucket (`internal/errclass/classify.go`). Add the
code to `internal/errclass/codemeta_<service>.go` with the right Category
and Subtype, plus a dispatch test in `classify_test.go`.

**Envelope shows `type=internal subtype=sdk_failure`.** Origin is
`client.WrapDoAPIError` taking the non-transport branch
(`internal/client/api_errors.go`). Check: did the SDK fail to decode the
response (look for `subtype=json_parse` in the wrapped chain)? Was the
transport detection too narrow for this error (e.g. a `*url.Error` with an
inner that does not satisfy `net.Error`)? Either widen the transport
predicate or add an explicit typed wrap upstream.

**`CheckDeclaredSubtype` rejects my Subtype.** The constant must be
declared in `errs/subtypes*.go` *and* referenced from the dispatch path.
Bare string literals trip `CheckDeclaredSubtype` unless they match the
`ad_hoc_*` prefix; `ad_hoc_*` then trips `CheckAdHocSubtype` as a
follow-up warning.

**`errors.As(&typedErr)` panics with a nil-pointer receiver.** A typed-nil
slipped through. All typed errors define nil-safe `Unwrap()`, but
returning a typed-nil pointer up the stack still defeats `errors.As`.
Return interface `nil` from constructors, never a typed-nil pointer.

**Exit code is 5 (internal) when I expected 3 (auth).** The error was not
typed before reaching `handleRootError`. Wrap at the boundary
(`client.WrapDoAPIError` or a typed constructor) — the bare `error.Error()`
string cannot be classified retroactively.

## Security & privacy

- `log_id` is a server-side trace token. Safe to surface; it does not
  carry user content.
- `missing_scopes` is app configuration, not user data.
- `Message` and `Hint` must not contain tokens, JWTs, or personally
  identifying values. CI does not catch this — producer responsibility.
- Wrapped `Cause` is **not** serialized to the wire (`json:"-"`). It is
  retained for in-process `errors.Is` / `errors.Unwrap` traversal and
  optional debug logging only.

## Pointers (task-driven)

- *Which struct to construct?* → **Producers / Quick reference**
- *Add a new condition?* → **Add a Subtype**
- *Consume from a shell script?* → **Consumers / Shell / AI**
- *Expose to MCP / OAuth?* → **Consumers / Protocol adapters**
- *Understand or fix a CI failure?* → **CI guards**
- *Migrate a legacy `ExitError` call site?* → **Migration** + the
  Deprecated note on the symbol being replaced.
- *Read source.* → `errs/doc.go` → `errs/category.go` → `errs/types.go`
  → `errs/predicates.go` → `internal/errclass/` →
  `cmd/root.go` `handleRootError`.
