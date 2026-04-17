package plugin

import (
	"strings"
	"testing"
)

// TestPipelinePlugin_Run_RecoversPanic pins SEC-WASM-003: a plugin whose Run
// handler panics must not unwind the caller. The pipeline should receive a
// normal (handled=false, err!=nil) return so it can log the failure and
// continue cleanup on subsequent plugins.
func TestPipelinePlugin_Run_RecoversPanic(t *testing.T) {
	t.Parallel()

	p := NewPipelinePlugin(
		"panicker",
		PhasePreProxy,
		100,
		func(ctx *PipelineContext) (bool, error) { panic("boom from Run") },
		nil,
	)

	handled, err := p.Run(&PipelineContext{})
	if err == nil {
		t.Fatal("expected panic to be converted to error, got nil")
	}
	if handled {
		t.Errorf("handled must be false after recovered panic, got true")
	}
	if !strings.Contains(err.Error(), "panicker") {
		t.Errorf("error should mention plugin name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "boom from Run") {
		t.Errorf("error should include panic value, got: %v", err)
	}
}

// TestPipelinePlugin_AfterProxy_RecoversPanic pins the post-proxy path: a
// panic during AfterProxy must not propagate, because the cleanup caller
// needs to continue flushing other plugins and the audit log.
func TestPipelinePlugin_AfterProxy_RecoversPanic(t *testing.T) {
	t.Parallel()

	p := NewPipelinePlugin(
		"afterpanicker",
		PhasePostProxy,
		100,
		nil,
		func(ctx *PipelineContext, proxyErr error) { panic("boom from AfterProxy") },
	)

	// If the recover is missing, this call will panic and crash the test
	// runner. That IS the assertion.
	p.AfterProxy(&PipelineContext{}, nil)
}

// TestPipeline_Execute_ContinuesAfterPluginPanic asserts that when one plugin
// in the chain panics, the Pipeline.Execute result surfaces an error rather
// than taking down the caller — i.e. the recover cooperates with the
// pipeline's normal error-handling branch.
func TestPipeline_Execute_ContinuesAfterPluginPanic(t *testing.T) {
	t.Parallel()

	bad := NewPipelinePlugin(
		"bad",
		PhasePreProxy,
		100,
		func(ctx *PipelineContext) (bool, error) { panic("intentional") },
		nil,
	)
	pipeline := NewPipeline([]PipelinePlugin{bad})

	handled, err := pipeline.Execute(&PipelineContext{})
	if err == nil {
		t.Fatal("expected Pipeline.Execute to return the converted panic as error")
	}
	if handled {
		t.Errorf("handled must be false after a panicked plugin, got true")
	}
}

// TestPipeline_ExecutePostProxy_SurvivesPanic asserts ExecutePostProxy keeps
// iterating subsequent plugins even if one of them panics.
func TestPipeline_ExecutePostProxy_SurvivesPanic(t *testing.T) {
	t.Parallel()

	var secondRan bool
	bad := NewPipelinePlugin(
		"bad",
		PhasePostProxy,
		100,
		nil,
		func(ctx *PipelineContext, proxyErr error) { panic("boom") },
	)
	good := NewPipelinePlugin(
		"good",
		PhasePostProxy,
		100,
		nil,
		func(ctx *PipelineContext, proxyErr error) { secondRan = true },
	)
	pipeline := NewPipeline([]PipelinePlugin{bad, good})
	pipeline.ExecutePostProxy(&PipelineContext{}, nil)

	if !secondRan {
		t.Fatal("a panicking plugin must not prevent subsequent AfterProxy callbacks from running")
	}
}
