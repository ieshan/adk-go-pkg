package eval_test

import (
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

func TestAddDefaultRetryOptionsIfNotPresent_NilRequest(t *testing.T) {
	eval.AddDefaultRetryOptionsIfNotPresent(nil)
}

func TestAddDefaultRetryOptionsIfNotPresent_NilConfig(t *testing.T) {
	req := &model.LLMRequest{}
	eval.AddDefaultRetryOptionsIfNotPresent(req)
	if req.Config == nil {
		t.Error("got nil Config, want initialized")
	}
}

func TestAddDefaultRetryOptionsIfNotPresent_ExistingConfig(t *testing.T) {
	req := &model.LLMRequest{
		Config: &genai.GenerateContentConfig{Temperature: float32Ptr(0.5)},
	}
	eval.AddDefaultRetryOptionsIfNotPresent(req)
	if req.Config == nil {
		t.Fatal("got nil Config, want non-nil")
	}
	if req.Config.Temperature == nil || *req.Config.Temperature != 0.5 {
		t.Error("got config overwritten, want preserved")
	}
}

func TestEnsureRetryOptionsPlugin_BeforeModelCallback(t *testing.T) {
	plugin := &eval.EnsureRetryOptionsPlugin{}
	req := &model.LLMRequest{}
	ctx := testutil.NewFakeCallbackContext()
	resp, err := plugin.BeforeModelCallback(ctx, req)
	if err != nil {
		t.Fatalf("BeforeModelCallback failed: %v", err)
	}
	if resp != nil {
		t.Error("got non-nil response from before-callback, want nil")
	}
	if req.Config == nil {
		t.Error("got nil Config, want initialized by AddDefaultRetryOptionsIfNotPresent")
	}
}

func float32Ptr(f float32) *float32 {
	return &f
}
