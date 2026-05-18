// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDocs_UpdateDryRunSuppressesSemanticWarnings asserts the contract that
// docsUpdateWarnings is NOT invoked on the --dry-run path. The unit tests in
// shortcuts/doc/docs_update_check_test.go prove the helper emits warnings for
// replace_range + blank-line and for combined-emphasis markers; this E2E
// locks in that they never reach the user during dry-run planning, so a
// future refactor that moves warning emission into a shared code path can't
// silently regress.
//
// Input is intentionally crafted to trigger BOTH warnings the helper emits:
//   - mode=replace_range + markdown containing "\n\n" (blank-line warning)
//   - markdown containing `***combined***` (combined bold+italic warning)
//
// Neither string may appear in dry-run output.
func TestDocs_UpdateDryRunSuppressesSemanticWarnings(t *testing.T) {
	// Fake creds are enough — dry-run short-circuits before any real API call.
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	// "***combined***" is a triple-asterisk combined-emphasis shape; "\n\n"
	// is a paragraph break. Both would normally produce warnings when
	// Execute runs under --mode=replace_range; both must be absent here.
	markdown := "***combined***\n\nsecond paragraph"

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+update",
			"--doc", "doxcnDryRunE2E",
			"--mode", "replace_range",
			"--selection-with-ellipsis", "placeholder",
			"--markdown", markdown,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	// Neither warning prefix ("warning:") nor either specific warning body
	// may appear in dry-run output (stdout OR stderr).
	combined := result.Stdout + "\n" + result.Stderr
	for _, needle := range []string{
		"warning:",
		"does not split a block into multiple paragraphs",
		"combined bold+italic markers",
	} {
		if strings.Contains(combined, needle) {
			t.Errorf("dry-run output must not surface pre-write warning %q\nstdout:\n%s\nstderr:\n%s",
				needle, result.Stdout, result.Stderr)
		}
	}
}

// TestDocs_V2CreateXMLGuardDryRun verifies the v2 XML content guards on the
// docs +create dry-run path. A bare ampersand in XML content must cause a
// validation error (exit 2), while clean XML must succeed.
func TestDocs_V2CreateXMLGuardDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("bare ampersand rejected", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+create",
				"--content", `<text>R&D</text>`,
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)

		combined := result.Stdout + "\n" + result.Stderr
		if !strings.Contains(combined, "bare &") {
			t.Fatalf("expected bare-ampersand error, got:\nstdout: %s\nstderr: %s", result.Stdout, result.Stderr)
		}
	})

	t.Run("clean XML accepted", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+create",
				"--content", `<text>R&amp;D</text>`,
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		method := gjson.Get(result.Stdout, "api.0.method").String()
		require.Equal(t, "POST", method)
	})
}

// TestDocs_V2UpdateXMLGuardDryRun verifies the v2 XML content guards on the
// docs +update dry-run path. A bare ampersand must cause a validation error,
// while clean XML with a <quote-container> (non-fatal warning) must still
// succeed with exit 0 — warnings are only emitted on the execute path, not
// dry-run.
func TestDocs_V2UpdateXMLGuardDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("bare ampersand rejected", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+update",
				"--doc", "doxcnDryRunE2E",
				"--command", "overwrite",
				"--content", `<text>R&D</text>`,
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)

		combined := result.Stdout + "\n" + result.Stderr
		if !strings.Contains(combined, "bare &") {
			t.Fatalf("expected bare-ampersand error, got:\nstdout: %s\nstderr: %s", result.Stdout, result.Stderr)
		}
	})

	t.Run("warning-only content passes dry-run", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+update",
				"--doc", "doxcnDryRunE2E",
				"--command", "overwrite",
				"--content", `<quote-container><p>text</p></quote-container>`,
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// Warnings must NOT appear in dry-run output (only emitted on execute path).
		combined := result.Stdout + "\n" + result.Stderr
		if strings.Contains(combined, "warning:") {
			t.Errorf("dry-run must not emit XML warnings; got:\nstdout: %s\nstderr: %s", result.Stdout, result.Stderr)
		}
	})
}
