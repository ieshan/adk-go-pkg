package config_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ieshan/adk-go-pkg/config"
	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/tool"
)

func TestLoad_Formats(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		file string
		data string
	}{
		{"json", "agent.json", `{"name":"json-agent","agent_class":"LlmAgent","model":"mock/fast"}`},
		{"yaml", "agent.yaml", "name: yaml-agent\nagent_class: LlmAgent\nmodel: mock/fast\n"},
		{"yml", "agent.yml", "name: yml-agent\nagent_class: LlmAgent\nmodel: mock/fast\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFile(t, dir, tc.file, tc.data)
			root := openRoot(t, dir)

			appCfg, err := config.Load(root, tc.file)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := appCfg.AgentConfig.Name(); got == "" {
				t.Error("Name: got empty, want non-empty")
			}
		})
	}
}

func TestLoad_Errors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "agent.txt", "name: x")
	root := openRoot(t, dir)

	if _, err := config.Load(root, "agent.txt"); err == nil {
		t.Error("Load(.txt): got nil, want error")
	}
	if _, err := config.Load(root, "missing.json"); err == nil {
		t.Error("Load(missing): got nil, want error")
	}
	if _, err := config.Load(nil, "agent.json"); err == nil {
		t.Error("Load(nil root): got nil, want error")
	}
}

func TestParse_UnsupportedFormat(t *testing.T) {
	t.Parallel()
	if _, err := config.Parse([]byte("{}"), "toml"); err == nil {
		t.Fatal("got nil, want error for unknown format")
	}
}

func TestParse_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		data   string
		format string
	}{
		{"empty yaml", "", "yaml"},
		{"empty json", "", "json"},
		{"comment-only yaml", "# nothing\n", "yaml"},
		{"invalid yaml", "name: [unclosed\n", "yaml"},
		{"invalid json", `{"name": "bad"`, "json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := config.Parse([]byte(tc.data), tc.format); err == nil {
				t.Fatal("got nil, want error")
			}
		})
	}
}

func TestParse_UnknownAgentClass(t *testing.T) {
	t.Parallel()
	_, err := config.Parse([]byte("name: x\nagent_class: UnknownAgent\n"), "yaml")
	if err == nil {
		t.Fatal("got nil, want error for unknown class")
	}
}

func TestParse_AgentClassDefaultsToLLM(t *testing.T) {
	t.Parallel()
	appCfg, err := config.Parse([]byte("name: bot\nmodel: mock/fast\n"), "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, ok := appCfg.AgentConfig.(*config.LLMAgentConfig); !ok {
		t.Fatalf("got %T, want *LLMAgentConfig", appCfg.AgentConfig)
	}
}

func TestParse_SubAgentEntries(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: root
agent_class: SequentialAgent
sub_agents:
  - name: child1
    agent_class: LlmAgent
    model: mock/fast
    sub_agents:
      - name: grandchild
        agent_class: LlmAgent
        model: mock/fast
  - child.yaml
  - config_path: other.yaml
  - code: myapp.agents.sub
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	entries := appCfg.AgentConfig.SubAgentEntries()
	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4", len(entries))
	}
	if entries[0].Inline == nil || entries[0].Inline.Name() != "child1" {
		t.Errorf("entry[0]: got %+v, want inline child1", entries[0])
	}
	if entries[1].Ref == nil || entries[1].Ref.ConfigPath != "child.yaml" {
		t.Errorf("entry[1]: got %+v, want config_path child.yaml", entries[1])
	}
	if entries[2].Ref == nil || entries[2].Ref.ConfigPath != "other.yaml" {
		t.Errorf("entry[2]: got %+v, want config_path other.yaml", entries[2])
	}
	if entries[3].Ref == nil || entries[3].Ref.Code != "myapp.agents.sub" {
		t.Errorf("entry[3]: got %+v, want code ref", entries[3])
	}
}

func TestParse_UnknownField(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
	}{
		{"top-level typo", "name: bot\nagent_class: LlmAgent\nintruction: x\n"},
		{"inline sub-agent typo", "name: root\nagent_class: SequentialAgent\nsub_agents:\n  - name: c\n    agent_class: LlmAgent\n    intruction: x\n"},
		{"tools entry typo", "name: bot\nagent_class: LlmAgent\ntools:\n  - nme: search\n"},
		{"runtime section typo", "name: bot\nagent_class: LlmAgent\nrun_config:\n  streamin_mode: sse\n"},
		{"live_run_config section", "name: bot\nagent_class: LlmAgent\nlive_run_config:\n  max_llm_calls: 5\n"},
		{"max_iterations on LLM", "name: bot\nagent_class: LlmAgent\nmax_iterations: 3\n"},
		{"sub_agents on Workflow", "name: wf\nagent_class: Workflow\nedges: []\nsub_agents: []\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Parse([]byte(tc.data), "yaml")
			if !errors.Is(err, config.ErrUnknownField) {
				t.Fatalf("got %v, want ErrUnknownField", err)
			}
		})
	}
}

func TestParse_RefEntryExtraKeys(t *testing.T) {
	t.Parallel()
	data := []byte("name: root\nagent_class: SequentialAgent\nsub_agents:\n  - config_path: a.yaml\n    name: b\n")
	_, err := config.Parse(data, "yaml")
	if !errors.Is(err, config.ErrUnknownField) {
		t.Fatalf("got %v, want ErrUnknownField", err)
	}
}

func TestParse_CallbackArgs(t *testing.T) {
	t.Parallel()
	data := []byte("name: bot\nagent_class: LlmAgent\nbefore_model_callbacks:\n  - name: cb\n    args:\n      x: 1\n")
	_, err := config.Parse(data, "yaml")
	if !errors.Is(err, config.ErrCallbackArgs) {
		t.Fatalf("got %v, want ErrCallbackArgs", err)
	}
}

func TestParse_UnsupportedFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
	}{
		{"static_instruction", "name: bot\nagent_class: LlmAgent\nstatic_instruction: x\n"},
		{"custom_metadata", "name: bot\nagent_class: LlmAgent\nrun_config:\n  custom_metadata:\n    k: v\n"},
		{"save_live_audio", "name: bot\nagent_class: LlmAgent\nrun_config:\n  save_live_audio: true\n"},
		{"speech_config", "name: bot\nagent_class: LlmAgent\nrun_config:\n  speech_config: {}\n"},
		{"create_http_options", "name: bot\nagent_class: LlmAgent\ncontext_cache_config:\n  create_http_options: {}\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Parse([]byte(tc.data), "yaml")
			if !errors.Is(err, config.ErrUnsupportedField) {
				t.Fatalf("got %v, want ErrUnsupportedField", err)
			}
		})
	}
}

func TestParse_IncludeContents(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"default", "none"} {
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			data := "name: bot\nagent_class: LlmAgent\ninclude_contents: " + v + "\n"
			if _, err := config.Parse([]byte(data), "yaml"); err != nil {
				t.Errorf("Parse: %v", err)
			}
		})
	}
	if _, err := config.Parse([]byte("name: bot\nagent_class: LlmAgent\ninclude_contents: bogus\n"), "yaml"); err == nil {
		t.Error("bogus include_contents: got nil, want error")
	}
	if _, err := config.Parse([]byte("name: s\nagent_class: SequentialAgent\ninclude_contents: none\n"), "yaml"); !errors.Is(err, config.ErrUnknownField) {
		t.Errorf("non-LLM include_contents: got %v, want ErrUnknownField", err)
	}
}

func TestParse_MaxIterations(t *testing.T) {
	t.Parallel()
	if _, err := config.Parse([]byte("name: l\nagent_class: LoopAgent\nmax_iterations: -1\n"), "yaml"); err == nil {
		t.Error("negative max_iterations: got nil, want error")
	}
	appCfg, err := config.Parse([]byte("name: l\nagent_class: LoopAgent\nmax_iterations: 3\n"), "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := appCfg.AgentConfig.(*config.LoopAgentConfig).MaxIterations; got != 3 {
		t.Errorf("MaxIterations: got %d, want 3", got)
	}
}

func TestParse_ModelExclusion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
	}{
		{"model and model_code", "name: b\nagent_class: LlmAgent\nmodel: mock/fast\nmodel_code:\n  name: x\n"},
		{"model_args and model_code", "name: b\nagent_class: LlmAgent\nmodel_args:\n  temperature: 0.5\nmodel_code:\n  name: x\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := config.Parse([]byte(tc.data), "yaml"); err == nil {
				t.Fatal("got nil, want error")
			}
		})
	}
}

func TestParse_MissingName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
	}{
		{"top-level", "agent_class: LlmAgent\n"},
		{"tools entry", "name: b\nagent_class: LlmAgent\ntools:\n  - args: {}\n"},
		{"skill_sets entry", "name: b\nagent_class: LlmAgent\nskill_sets:\n  - config: {}\n"},
		{"model_code", "name: b\nagent_class: LlmAgent\nmodel_code:\n  args: {}\n"},
		{"callback", "name: b\nagent_class: LlmAgent\nbefore_agent_callbacks:\n  - {}\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := config.Parse([]byte(tc.data), "yaml"); err == nil {
				t.Fatal("got nil, want error")
			}
		})
	}
}

func TestParse_SubAgentRefErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		item string
	}{
		{"both ref keys", "  - config_path: a.yaml\n    code: b\n"},
		{"empty config_path", "  - config_path: \"\"\n"},
		{"non-string scalar", "  - 42\n"},
		{"null", "  - null\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data := "name: root\nagent_class: SequentialAgent\nsub_agents:\n" + tc.item
			if _, err := config.Parse([]byte(data), "yaml"); err == nil {
				t.Fatal("got nil, want error")
			}
		})
	}
}

func TestParse_ScalarShorthand(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: bot
agent_class: LlmAgent
model: mock/fast
tools:
  - search
before_agent_callbacks:
  - my.cb
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	llm := appCfg.AgentConfig.(*config.LLMAgentConfig)
	if len(llm.Tools) != 1 || llm.Tools[0].Name != "search" {
		t.Errorf("Tools: got %+v, want [{search}]", llm.Tools)
	}
	if len(llm.BeforeAgentCallbacks) != 1 || llm.BeforeAgentCallbacks[0].Name != "my.cb" {
		t.Errorf("BeforeAgentCallbacks: got %+v", llm.BeforeAgentCallbacks)
	}
}

func TestParse_GlobalInstruction(t *testing.T) {
	t.Parallel()
	appCfg, err := config.Parse([]byte("name: bot\nagent_class: LlmAgent\nglobal_instruction: global\n"), "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	llm := appCfg.AgentConfig.(*config.LLMAgentConfig)
	if llm.GlobalInstruction != "global" {
		t.Errorf("GlobalInstruction: got %q, want global", llm.GlobalInstruction)
	}
}

func TestParse_SchemaRefPrecedence(t *testing.T) {
	t.Parallel()
	data := []byte("name: bot\nagent_class: LlmAgent\ninput_schema:\n  name: x\n  type: object\n")
	_, err := config.Parse(data, "yaml")
	if !errors.Is(err, config.ErrUnknownField) {
		t.Fatalf("got %v, want ErrUnknownField (type on a named ref)", err)
	}
}

func TestParse_InlineSchemaSnakeCase(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: bot
agent_class: LlmAgent
input_schema:
  type: object
  properties:
    user_name:
      type: string
      property_ordering: [a, b]
    count:
      type: integer
      min_items: 1
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	llm := appCfg.AgentConfig.(*config.LLMAgentConfig)
	if llm.InputSchema == nil || llm.InputSchema.Inline == nil {
		t.Fatal("InputSchema.Inline is nil")
	}
	prop, ok := llm.InputSchema.Inline.Properties["user_name"]
	if !ok {
		t.Fatalf("properties[user_name] missing: %v", llm.InputSchema.Inline.Properties)
	}
	if len(prop.PropertyOrdering) != 2 {
		t.Errorf("PropertyOrdering: got %v, want [a b]", prop.PropertyOrdering)
	}
	count := llm.InputSchema.Inline.Properties["count"]
	if count == nil || count.MinItems == nil || *count.MinItems != 1 {
		t.Errorf("MinItems: got %v, want 1", count)
	}
}

func TestParse_ModelCodeArgs(t *testing.T) {
	t.Parallel()
	data := []byte("name: b\nagent_class: LlmAgent\nmodel_code:\n  name: x\n  args:\n    temperature: 0.5\n")
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mc := appCfg.AgentConfig.(*config.LLMAgentConfig).ModelCode
	if mc == nil || mc.Name != "x" || mc.Args["temperature"] != 0.5 {
		t.Errorf("ModelCode: got %+v, want {Name: x, Args: {temperature: 0.5}}", mc)
	}
}

func TestParse_RunConfig(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: bot
agent_class: LlmAgent
run_config:
  streaming_mode: sse
  save_live_blob: true
  save_input_blobs_as_artifacts: true
  max_llm_calls: 0
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rc := appCfg.RunConfig
	if rc == nil {
		t.Fatal("RunConfig is nil")
	}
	if rc.StreamingMode != config.StreamingModeSSE {
		t.Errorf("StreamingMode: got %q", rc.StreamingMode)
	}
	if !rc.SaveLiveBlob || !rc.SaveInputBlobsAsArtifacts {
		t.Errorf("blob flags: got live=%v input=%v", rc.SaveLiveBlob, rc.SaveInputBlobsAsArtifacts)
	}
	if rc.MaxLLMCalls == nil || *rc.MaxLLMCalls != 0 {
		t.Errorf("MaxLLMCalls: got %v, want pointer to 0", rc.MaxLLMCalls)
	}
}

func TestParse_RunConfigValidation(t *testing.T) {
	t.Parallel()
	if _, err := config.Parse([]byte("name: b\nagent_class: LlmAgent\nrun_config:\n  streaming_mode: bogus\n"), "yaml"); err == nil {
		t.Error("invalid streaming_mode: got nil, want error")
	}
	if _, err := config.Parse([]byte("name: b\nagent_class: LlmAgent\nrun_config:\n  streaming_mode: 5\n"), "yaml"); err == nil {
		t.Error("non-string streaming_mode: got nil, want error")
	}
	if _, err := config.Parse([]byte(`{"name":"b","agent_class":"LlmAgent","run_config":{"max_llm_calls":1.5}}`), "json"); err == nil {
		t.Error("fractional max_llm_calls: got nil, want error")
	}
	if _, err := config.Parse([]byte("name: b\nagent_class: LlmAgent\nrun_config:\n  max_llm_calls: -5\n"), "yaml"); err != nil {
		t.Errorf("negative max_llm_calls should parse: %v", err)
	}
}

func TestParse_ContextCacheConfig(t *testing.T) {
	t.Parallel()
	appCfg, err := config.Parse([]byte("name: b\nagent_class: LlmAgent\ncontext_cache_config:\n  min_tokens: 50\n"), "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	c := appCfg.ContextCacheConfig
	if c == nil {
		t.Fatal("ContextCacheConfig is nil")
	}
	if c.CacheIntervals != 10 || c.TTLSeconds != 1800 || c.MinTokens != 50 {
		t.Errorf("ContextCacheConfig: got %+v", c)
	}
	if _, err := config.Parse([]byte("name: b\nagent_class: LlmAgent\ncontext_cache_config:\n  cache_intervals: 101\n"), "yaml"); err == nil {
		t.Error("out-of-range cache_intervals: got nil, want error")
	}
}

func TestParse_RuntimeConfigAbsent(t *testing.T) {
	t.Parallel()
	appCfg, err := config.Parse([]byte("name: b\nagent_class: LlmAgent\n"), "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if appCfg.RunConfig != nil {
		t.Errorf("RunConfig: got %+v, want nil", appCfg.RunConfig)
	}
	if appCfg.ContextCacheConfig != nil {
		t.Errorf("ContextCacheConfig: got %+v, want nil", appCfg.ContextCacheConfig)
	}
}

func TestParse_SkillsetFields(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: bot
agent_class: LlmAgent
skill_sets:
  - name: filesystem
    config:
      path: ./skills
    preload: complete
    names: [weather, cooking]
    system_instruction: use skills
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	s := appCfg.AgentConfig.(*config.LLMAgentConfig).Skillsets[0]
	if s.Name != "filesystem" || s.Preload != "complete" {
		t.Errorf("Skillset: got %+v", s)
	}
	if len(s.Names) != 2 || s.Names[0] != "weather" {
		t.Errorf("Names: got %v", s.Names)
	}
	if s.Config["path"] != "./skills" {
		t.Errorf("Config[path]: got %v", s.Config["path"])
	}
}

func TestParse_UnknownFormatJSON(t *testing.T) {
	t.Parallel()
	appCfg, err := config.Parse([]byte(`{"name":"bot","agent_class":"LlmAgent","model":"mock/fast"}`), "JSON")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if appCfg.AgentConfig.Name() != "bot" {
		t.Errorf("Name: got %q", appCfg.AgentConfig.Name())
	}
}

func TestParse_JSONDuplicateKey(t *testing.T) {
	t.Parallel()
	_, err := config.Parse([]byte(`{"name":"a","name":"b"}`), "json")
	if err == nil {
		t.Fatal("got nil, want error for duplicate JSON key")
	}
}

// TestParse_DescriptionOnNonLLM verifies description is accepted on all classes.
func TestParse_DescriptionOnNonLLM(t *testing.T) {
	t.Parallel()
	for _, class := range []string{"SequentialAgent", "ParallelAgent", "LoopAgent"} {
		t.Run(class, func(t *testing.T) {
			t.Parallel()
			data := "name: a\nagent_class: " + class + "\ndescription: hello\n"
			appCfg, err := config.Parse([]byte(data), "yaml")
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if appCfg.AgentConfig.Description() != "hello" {
				t.Errorf("Description: got %q", appCfg.AgentConfig.Description())
			}
		})
	}
}

// TestSpecCorpus_Parses verifies the golden spec corpus parses without error.
func TestSpecCorpus_Parses(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(filepath.Join("testdata", "spec"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(filepath.Join("testdata", "spec", e.Name()))
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if _, err := config.Parse(data, "yaml"); err != nil {
				t.Fatalf("Parse(%s): %v", e.Name(), err)
			}
		})
	}
}

// TestSpecCorpus_Build builds the multi-agent corpus and checks the tree shape.
func TestSpecCorpus_Build(t *testing.T) {
	t.Parallel()
	reg := newTestRegistry()
	reg.RegisterTool("roll_die", func(map[string]any) (tool.Tool, error) {
		return testutil.NewFakeTool("roll_die"), nil
	})
	root := openRoot(t, filepath.Join("testdata", "spec"))

	a, _, _, _, err := config.LoadAndBuild(context.Background(), root, "root_agent.yaml", reg)
	if err != nil {
		t.Fatalf("LoadAndBuild: %v", err)
	}
	subs := a.SubAgents()
	if len(subs) != 2 {
		t.Fatalf("root sub-agents: got %d, want 2", len(subs))
	}
	names := []string{subs[0].Name(), subs[1].Name()}
	if names[0] != "prime_agent" || names[1] != "roll_agent" {
		t.Errorf("sub-agent names: got %v, want [prime_agent roll_agent]", names)
	}
}

// FuzzParse exercises Parse through the public API for both decoder paths.
func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"name: bot\nagent_class: LlmAgent\n",
		"name: bot\nagent_class: LlmAgent\ngenerate_content_config:\n  temperature: 0.7\n",
		`{"name":"bot","agent_class":"LlmAgent"}`,
		":\n  :\n   bad\n",
		"",
	} {
		f.Add([]byte(seed), "yaml")
		f.Add([]byte(seed), "json")
	}
	specDir := filepath.Join("testdata", "spec")
	if entries, err := os.ReadDir(specDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(specDir, e.Name()))
			if err != nil {
				continue
			}
			f.Add(data, "yaml")
		}
	}

	f.Fuzz(func(t *testing.T, data []byte, format string) {
		if format != "yaml" && format != "json" {
			format = "yaml"
		}
		_, _ = config.Parse(data, format)
	})
}
