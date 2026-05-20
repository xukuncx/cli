# eval-search local harness

This directory contains deterministic helper scripts for the `eval-search`
workflow.

## Sandbox executor setup

Use `eval-search-sandbox-run.sh` when the loader account can read the live eval
Base but the executor must be isolated from it.

The wrapper runs a strict two-phase flow:

1. Host lark-cli profile snapshots the live eval Base into `dataset.jsonl`.
2. Docker starts a fresh lark-cli config, authenticates a separate executor
   account, runs `eval-search-run.ts --dataset-file ...`, then optionally runs
   `eval-search-collect-search.ts`.
3. The host process runs `eval-search-cycle.ts`, which performs Judge,
   Optimizer, quality gate, and draft PR creation.

The executor account is still probed with `base +record-list`. If it can read
the eval Base, the sandbox run blocks before search preflight.

```bash
tests/eval-search/eval-search-sandbox-run.sh \
  --loader-profile default \
  --subset 3
```

First run creates `~/.eval-search-sandbox.env` if no env file exists. Fill:

```bash
EVAL_SEARCH_BASE_TOKEN=base_token_xxx
EVAL_SEARCH_TABLE_ID=tbl_xxx
EVAL_SEARCH_VIEW_ID=vew_xxx
LARKSUITE_CLI_APP_ID=cli_xxx
LARKSUITE_CLI_APP_SECRET=xxx
```

Set `LARKSUITE_CLI_USER_ACCESS_TOKEN` in that env file to skip interactive
device-code login. Otherwise the container prints an authorization URL.
By default the sandbox requests the domains needed by the multi-entity baseline:
`base,drive,docs,wiki,task,contact,im,minutes,vc,mail,calendar`.
Set `EVAL_SEARCH_EXPECTED_EXECUTOR_NAME` or
`EVAL_SEARCH_EXPECTED_EXECUTOR_OPEN_ID` to make the sandbox fail fast if the
authorized user is not the eval-set persona.

To reuse an existing snapshot:

```bash
tests/eval-search/eval-search-sandbox-run.sh \
  --dataset-file tests/eval-search/runs/<snapshot-run>/dataset.jsonl \
  --executor-run-id <snapshot-run>-sandbox
```

Use `--skip-cycle`, `--skip-optimizer`, or `--skip-pr` only for debugging a
specific stage. A plain "run once" should not pass those flags.

The dataset intentionally keeps only rows whose `是否采纳` field contains
`采纳`. The current view count is reported as `meta.json.raw_dataset_rows`;
full-table diagnostics are reported separately as `meta.json.all_table_diagnostics`.
