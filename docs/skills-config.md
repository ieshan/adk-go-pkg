# Agent Skills Configuration Guide

This guide covers configuring agent skills in `adk-go-pkg` using YAML/JSON configuration files.

## What are Agent Skills?

Agent Skills are specialized instruction sets that extend agent capabilities without modifying agent code. Skills follow the [Agent Skills Specification](https://agentskills.io) and consist of:

- **SKILL.md** (required): Contains YAML frontmatter with metadata and markdown instructions
- **references/** (optional): Additional documentation files
- **assets/** (optional): Templates, scripts, or other resources
- **scripts/** (optional): Executable scripts for automation

## Directory Structure

Skills are organized as subdirectories, each containing a SKILL.md file:

```
skills/
├── weather/
│   ├── SKILL.md
│   └── references/
│       └── api-docs.md
└── cooking/
    ├── SKILL.md
    ├── assets/
    │   └── recipe-template.txt
    └── scripts/
        └── convert-units.sh
```

## SKILL.md Format

Each SKILL.md file has YAML frontmatter followed by markdown instructions:

```yaml
---
name: my-skill
description: A brief description of what this skill does.
---

# Instructions

Detailed markdown instructions for the LLM on how to use this skill...
```

### Frontmatter Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique skill name (must match directory name) |
| `description` | Yes | Short description shown in skill listings |
| `license` | No | License identifier |
| `compatibility` | No | Compatibility notes |
| `metadata` | No | Key-value pairs for custom metadata |
| `allowed-tools` | No | List of tools this skill can use |

## Configuration Reference

### SkillsetRef Fields

```go
type SkillsetRef struct {
    Name              string
    Config            map[string]any
    Names             []string
    Preload           string
    SystemInstruction string
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `Name` | `string` | Yes | Registered SkillFactory identifier (e.g., "filesystem") |
| `Config` | `map[string]any` | No | Factory-specific configuration |
| `Names` | `[]string` | No | Specific skills to load (empty = all skills) |
| `Preload` | `string` | No | Preload strategy: "", "complete", or "frontmatters" |
| `SystemInstruction` | `string` | No | Custom instruction for skill usage |

### Built-in Skill Factories

| Factory | Config Keys | Description |
|---------|-------------|-------------|
| `filesystem` | `path` (string) | Loads skills from local filesystem |

### Preload Strategy Selection

Choose the right preload strategy based on your use case:

| Strategy | Memory | Latency | Best For |
|----------|--------|---------|----------|
| `""` (none) | Lowest | Highest | Large skill sets, memory-constrained environments |
| `"frontmatters"` | Low | Medium | Balanced, frequent skill listing |
| `"complete"` | Highest | Lowest | Small skill sets, fast response required |

**Decision Flowchart:**

1. Small skill set (<100MB) + need fast response? → Use `"complete"`
2. Large skill set + frequently list skills? → Use `"frontmatters"`
3. Very large skill set or memory constrained? → Use `""` (none)

## YAML Configuration Examples

### Basic Filesystem Skills

Load all skills from a directory:

```yaml
name: my-agent
type: llm
model: gemini/gemini-2.5-flash
instruction: "You are a helpful assistant with access to specialized skills."
skillsets:
  - name: filesystem
    config:
      path: "./skills"
```

### Specific Skills Only

Restrict agent to specific skills from a larger set:

```yaml
name: restricted-agent
type: llm
skillsets:
  - name: filesystem
    config:
      path: "./skills"  # Contains 20+ skills
    names:              # Agent only sees these 2:
      - "weather"
      - "cooking"
    preload: frontmatters
```

**Benefits:**
- **Security**: Agent cannot discover or use other skills
- **Performance**: Only selected skills are loaded (efficient with preload)
- **Clarity**: Explicit contract of what the agent can do

### Preloaded Skills

Load all skills into memory for fastest access:

```yaml
name: fast-agent
type: llm
skillsets:
  - name: filesystem
    config:
      path: "./skills"
    preload: complete
```

### Custom System Instruction

Override the default skill guidance:

```yaml
name: custom-skills-agent
type: llm
skillsets:
  - name: filesystem
    config:
      path: "./skills"
    systemInstruction: |
      You have access to domain-specific skills. When a user asks about
      weather or cooking, load the relevant skill before responding.
```

### Multiple Skill Sources

Combine skills from different sources:

```yaml
name: multi-source-agent
type: llm
skillsets:
  - name: filesystem
    config:
      path: "./local-skills"
    preload: frontmatters
  - name: gcs
    config:
      bucket: "org-skills"
      prefix: "shared/"
    preload: frontmatters
```

## Custom Skill Sources

Register custom skill factories for cloud storage, databases, etc.:

```go
package main

import (
    "github.com/ieshan/adk-go-pkg/config"
    "google.golang.org/adk/tool/skilltoolset/skill"
)

func main() {
    reg := config.NewRegistry()

    // Register custom S3 skill factory
    reg.RegisterSkill("s3", func(cfg map[string]any) (skill.Source, error) {
        bucket := cfg["bucket"].(string)
        prefix := cfg["prefix"].(string)
        
        // Create S3-based source
        source, err := createS3SkillSource(bucket, prefix)
        if err != nil {
            return nil, err
        }
        return source, nil
    })

    // Now you can use it in YAML:
    // skillsets:
    //   - name: s3
    //     config:
    //       bucket: "my-skills"
    //       prefix: "production/"
}
```

### Implementing a Custom Source

A custom source must implement the `skill.Source` interface:

```go
type Source interface {
    ListFrontmatters(ctx context.Context) ([]*Frontmatter, error)
    ListResources(ctx context.Context, name, subpath string) ([]string, error)
    LoadFrontmatter(ctx context.Context, name string) (*Frontmatter, error)
    LoadInstructions(ctx context.Context, name string) (string, error)
    LoadResource(ctx context.Context, name, resourcePath string) (io.ReadCloser, error)
}
```

## Go Usage Example

```go
package main

import (
    "log"

    "github.com/ieshan/adk-go-pkg/config"
)

func main() {
    // Registry already has filesystem factory registered
    reg := config.NewRegistry()

    // Load agent with skills from YAML
    agent, err := config.LoadAndBuild("agents/skills-agent.yaml", reg)
    if err != nil {
        log.Fatal(err)
    }

    // Agent now has access to skills defined in ./skills/
    // Use with runner or aguiadk bridge...
}
```

## Tips and Best Practices

1. **Start without preload** (`preload: ""`) when developing, then add preload for production if needed
2. **Use specific skill loading** (`names: [...]`) when you want to restrict what an agent can access
3. **Keep skill directories organized** - one skill per directory with clear naming
4. **Use references/** for large documentation files that shouldn't be loaded by default
5. **Monitor memory usage** with `"complete"` preload on large skill sets

## See Also

- [Config Agent Loader](config-agent.md) - Full config loader documentation
- [ADK-Go SkillToolset](https://pkg.go.dev/google.golang.org/adk/tool/skilltoolset) - ADK-Go skill package
- [Agent Skills Specification](https://agentskills.io) - Official skill specification
