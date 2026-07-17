package eval

import (
	"testing"

	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

func TestAddDefaultRetryOptionsIfNotPresent_NilRequest(t *testing.T) {
	AddDefaultRetryOptionsIfNotPresent(nil)
}

func TestAddDefaultRetryOptionsIfNotPresent_NilConfig(t *testing.T) {
	req := &model.LLMRequest{}
	AddDefaultRetryOptionsIfNotPresent(req)
	if req.Config == nil {
		t.Error("expected Config to be initialized")
	}
}

func TestAddDefaultRetryOptionsIfNotPresent_ExistingConfig(t *testing.T) {
	req := &model.LLMRequest{
		Config: &genai.GenerateContentConfig{Temperature: float32Ptr(0.5)},
	}
	AddDefaultRetryOptionsIfNotPresent(req)
	if req.Config == nil {
		t.Fatal("expected Config to remain non-nil")
	}
	if req.Config.Temperature == nil || *req.Config.Temperature != 0.5 {
		t.Error("expected existing config to be preserved")
	}
}

func TestEnsureRetryOptionsPlugin_BeforeModelCallback(t *testing.T) {
	plugin := &EnsureRetryOptionsPlugin{}
	req := &model.LLMRequest{}
	ctx := testutil.NewFakeCallbackContext()
	resp, err := plugin.BeforeModelCallback(ctx, req)
	if err != nil {
		t.Fatalf("BeforeModelCallback failed: %v", err)
	}
	if resp != nil {
		t.Error("expected nil response from before-callback")
	}
	if req.Config == nil {
		t.Error("expected Config to be initialized by AddDefaultRetryOptionsIfNotPresent")
	}
}

func float32Ptr(f float32) *float32 {
	return &f
}
