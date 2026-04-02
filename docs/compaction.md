# Event Compaction

Package `compaction` provides utilities for managing event history size in
ADK-Go sessions.

## Overview

As agent sessions grow, event history can exceed LLM context windows or become
prohibitively expensive. Compaction reduces the event count while preserving
the information the agent needs to continue operating.

### When to Use

- Session event counts regularly exceed your model's context window.
- You want to control token costs by summarising older history.
- You need a simple "keep last N" truncation policy.
- You want to compact `[]*genai.Content` slices before sending them to an LLM.

## API Reference

### Strategy

```go
type Strategy interface {
    Compact(ctx context.Context, events []*session.Event) ([]*session.Event, error)
}
```

The pluggable interface for compaction algorithms. Two built-in implementations
are provided: `Truncation` and `Summarizer`.

### Config

```go
type Config struct {
    // Strategy called when compaction is triggered.
    Strategy Strategy

    // Maximum event count before compaction fires. 0 = disabled.
    MaxEvents int

    // Maximum estimated token count (chars / 4) before compaction fires. 0 = disabled.
    MaxTokens int

    // Hint to strategies about how many recent events to preserve.
    KeepRecent int
}
```

Compaction triggers when **either** `MaxEvents` or `MaxTokens` is exceeded.

### Apply

```go
func Apply(ctx context.Context, cfg Config, events []*session.Event) ([]*session.Event, error)
```

Checks thresholds and, if exceeded, calls `cfg.Strategy.Compact`. If no
threshold is exceeded the original slice is returned unchanged.

### CollectEvents

```go
func CollectEvents(events session.Events) []*session.Event
```

Converts the ADK `session.Events` interface into a plain `[]*session.Event`
slice, preserving iteration order.

### ApplyToContents

```go
func ApplyToContents(ctx context.Context, cfg Config, contents []*genai.Content) ([]*genai.Content, error)
```

Works on `[]*genai.Content` directly. Each `Content` is wrapped in a temporary
`session.Event`, the strategy is applied, and `Content` values are extracted
back. This lets you reuse the same strategies for both event-level and
content-level compaction.

## Truncation Strategy

`Truncation` keeps only the last `RetainCount` events, discarding all earlier
ones. It is the simplest strategy and is useful when only recent context matters.

### Constructor

```go
func NewTruncation(retainCount int) *Truncation
```

### Example

```go
tr := compaction.NewTruncation(20)
cfg := compaction.Config{
    Strategy:  tr,
    MaxEvents: 100,
}

events := compaction.CollectEvents(sess.Events())
compacted, err := compaction.Apply(ctx, cfg, events)
if err != nil {
    log.Fatal(err)
}
// compacted contains at most 20 events
```

## Summarizer Strategy

`Summarizer` asks an LLM to produce a concise summary of the events. The result
is a single `session.Event` authored by `"system"` containing the summary text.

### Constructor

```go
func NewSummarizer(m model.LLM, opts ...SummarizerOption) *Summarizer
```

### Options

```go
func WithInstruction(instruction string) SummarizerOption
```

Replaces the default summary prompt. The default is:
> "Summarize the following conversation concisely, preserving key decisions, facts, and action items."

### Example

```go
s := compaction.NewSummarizer(myLLM,
    compaction.WithInstruction("Summarize in three bullet points."),
)

cfg := compaction.Config{
    Strategy:  s,
    MaxEvents: 100,
    MaxTokens: 4000,
}

events := compaction.CollectEvents(sess.Events())
compacted, err := compaction.Apply(ctx, cfg, events)
if err != nil {
    log.Fatal(err)
}
// compacted is a single event with the summary text
```

## Integration Patterns

### Before-generate hook

Compact contents before every LLM call:

```go
import (
    "context"

    "github.com/ieshan/adk-go-pkg/compaction"
    "google.golang.org/genai"
)

cfg := compaction.Config{
    Strategy:  compaction.NewTruncation(30),
    MaxEvents: 50,
}

// Inside your before-generate callback:
req.Contents, err = compaction.ApplyToContents(ctx, cfg, req.Contents)
```

### Periodic session compaction

Compact the event store periodically:

```go
import (
    "context"

    "github.com/ieshan/adk-go-pkg/compaction"
    "google.golang.org/adk/session"
)

cfg := compaction.Config{
    Strategy:  compaction.NewTruncation(20),
    MaxEvents: 100,
}

events := compaction.CollectEvents(sess.Events())
compacted, err := compaction.Apply(ctx, cfg, events)
// Replace session events with compacted set
```

### Custom Strategy

Implement the `Strategy` interface for domain-specific compaction:

```go
type everyOther struct{}

func (everyOther) Compact(_ context.Context, events []*session.Event) ([]*session.Event, error) {
    out := make([]*session.Event, 0, len(events)/2)
    for i, e := range events {
        if i%2 == 0 {
            out = append(out, e)
        }
    }
    return out, nil
}
```
