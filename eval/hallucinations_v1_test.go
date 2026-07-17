package eval

import (
	"context"
	"strings"
	"testing"

	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

func TestHallucinationSegmenterPrompt_HasPlaceholder(t *testing.T) {
	if !strings.Contains(HallucinationSegmenterPrompt, "{response}") {
		t.Error("expected {response} placeholder in segmenter prompt")
	}
}

func TestHallucinationValidatorPrompt_HasPlaceholders(t *testing.T) {
	if !strings.Contains(HallucinationValidatorPrompt, "{sentences}") {
		t.Error("expected {sentences} placeholder in validator prompt")
	}
	if !strings.Contains(HallucinationValidatorPrompt, "{context}") {
		t.Error("expected {context} placeholder in validator prompt")
	}
}

func TestParseValidationLabels(t *testing.T) {
	tests := []struct {
		name string
		resp string
		want []string
	}{
		{
			"single_supported",
			"sentence: Apples are red.\nlabel: supported\nrationale: Context states apples are red.\nsupporting_excerpt: Apples are red fruits.\ncontradicting_excerpt: null",
			[]string{"supported"},
		},
		{
			"mixed_labels",
			`sentence: Apples are red.
label: supported
rationale: Context states apples are red.
supporting_excerpt: Apples are red fruits.
contradicting_excerpt: null

sentence: Bananas are green.
label: contradictory
rationale: Context states bananas are yellow.
supporting_excerpt: null
contradicting_excerpt: Bananas are yellow fruits.

sentence: Enjoy your fruit!
label: not_applicable
rationale: General expression.
supporting_excerpt: null
contradicting_excerpt: null`,
			[]string{"supported", "contradictory", "not_applicable"},
		},
		{
			"no_blocks",
			"random text without validation blocks",
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseValidationLabels(tt.resp)
			if len(got) != len(tt.want) {
				t.Errorf("parseValidationLabels got %d labels, want %d (%v)", len(got), len(tt.want), got)
				return
			}
			for i, label := range got {
				if label != tt.want[i] {
					t.Errorf("parseValidationLabels[%d] = %q, want %q", i, label, tt.want[i])
				}
			}
		})
	}
}

func TestHallucinationsV1_ParseResponse(t *testing.T) {
	evaluator, err := NewHallucinationsV1Evaluator(EvalMetric{MetricName: string(HallucinationsV1)}, testutil.NewFakeLLM())
	if err != nil {
		t.Fatalf("NewHallucinationsV1Evaluator failed: %v", err)
	}

	t.Run("all_supported", func(t *testing.T) {
		resp := &model.LLMResponse{
			Content: &genai.Content{
				Parts: []*genai.Part{{Text: "sentence: A.\nlabel: supported\nrationale: yes.\nsupporting_excerpt: ctx.\ncontradicting_excerpt: null\nsentence: B.\nlabel: not_applicable\nrationale: greeting.\nsupporting_excerpt: null\ncontradicting_excerpt: null"}},
				Role:  "model",
			},
		}
		score := evaluator.parseResponse(resp)
		if score.Score == nil || *score.Score != 1.0 {
			t.Errorf("Score = %v, want 1.0", score.Score)
		}
	})

	t.Run("mixed", func(t *testing.T) {
		resp := &model.LLMResponse{
			Content: &genai.Content{
				Parts: []*genai.Part{{Text: "sentence: A.\nlabel: supported\nrationale: yes.\nsupporting_excerpt: ctx.\ncontradicting_excerpt: null\nsentence: B.\nlabel: unsupported\nrationale: no evidence.\nsupporting_excerpt: null\ncontradicting_excerpt: null\nsentence: C.\nlabel: not_applicable\nrationale: greeting.\nsupporting_excerpt: null\ncontradicting_excerpt: null"}},
				Role:  "model",
			},
		}
		score := evaluator.parseResponse(resp)
		if score.Score == nil || *score.Score != 2.0/3.0 {
			t.Errorf("Score = %v, want %.4f", score.Score, 2.0/3.0)
		}
	})

	t.Run("empty", func(t *testing.T) {
		resp := &model.LLMResponse{
			Content: &genai.Content{
				Parts: []*genai.Part{{Text: ""}},
				Role:  "model",
			},
		}
		score := evaluator.parseResponse(resp)
		if score.Score != nil {
			t.Errorf("Score = %v, want nil", score.Score)
		}
	})
}

func TestHallucinationsV1_SegmentResponse(t *testing.T) {
	segmenterResp := testutil.NewTextResponse("<sentence>First sentence.</sentence>\n<sentence>Second sentence.</sentence>\n<sentence>Third sentence.</sentence>")
	fakeLLM := testutil.NewFakeLLM(segmenterResp)

	evaluator, err := NewHallucinationsV1Evaluator(EvalMetric{MetricName: string(HallucinationsV1)}, fakeLLM)
	if err != nil {
		t.Fatalf("NewHallucinationsV1Evaluator failed: %v", err)
	}

	sentences, err := evaluator.segmentResponse(context.Background(), "segment this")
	if err != nil {
		t.Fatalf("segmentResponse failed: %v", err)
	}
	if len(sentences) != 3 {
		t.Fatalf("len(sentences) = %d, want 3", len(sentences))
	}
	if sentences[0] != "First sentence." {
		t.Errorf("sentences[0] = %q, want %q", sentences[0], "First sentence.")
	}
}

func TestHallucinationsV1_EvaluateInvocations(t *testing.T) {
	segmenterResp := testutil.NewTextResponse("<sentence>The capital is Paris.</sentence>")
	validatorResp := testutil.NewTextResponse("sentence: The capital is Paris.\nlabel: supported\nrationale: Tool response confirms Paris.\nsupporting_excerpt: capital: Paris\ncontradicting_excerpt: null")
	fakeLLM := testutil.NewFakeLLM(segmenterResp, validatorResp)

	threshold := 0.8
	evaluator, err := NewHallucinationsV1Evaluator(EvalMetric{
		MetricName: string(HallucinationsV1),
		Threshold:  &threshold,
	}, fakeLLM)
	if err != nil {
		t.Fatalf("NewHallucinationsV1Evaluator failed: %v", err)
	}

	actual := []Invocation{{
		FinalResponse: &genai.Content{
			Parts: []*genai.Part{{Text: "The capital is Paris."}},
			Role:  "model",
		},
		IntermediateData: &InvocationEventsData{
			Events: []InvocationEvent{
				{
					Author: "tool",
					Content: &genai.Content{
						Parts: []*genai.Part{{
							FunctionResponse: &genai.FunctionResponse{Name: "get_capital"},
						}},
						Role: "function",
					},
				},
			},
		},
	}}

	result, err := evaluator.EvaluateInvocations(context.Background(), actual, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.OverallScore == nil {
		t.Fatal("OverallScore is nil")
	}
	if *result.OverallScore != 1.0 {
		t.Errorf("OverallScore = %v, want 1.0", *result.OverallScore)
	}
	if result.OverallEvalStatus != EvalStatusPassed {
		t.Errorf("OverallEvalStatus = %v, want %v", result.OverallEvalStatus, EvalStatusPassed)
	}
}
