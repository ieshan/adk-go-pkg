# Session Rewind

Package `session/rewind` provides utilities for truncating (rewinding) ADK
sessions to a prior event.

## Overview

Rewinding rolls a session back to an earlier state by discarding all events
after a target and recalculating session state from the retained events'
`StateDelta` entries.

### Use Cases

- **Undo a bad tool call** -- roll back after a tool produced an undesirable result.
- **Conversation branching** -- rewind to a checkpoint and replay with different input.
- **Error recovery** -- discard events that followed a transient failure.
- **Interactive debugging** -- step backward through agent execution.

### How It Works

1. Fetch the session from the `session.Service`.
2. Locate the target event by ID or 0-based index.
3. Collect events `[0..target]` (inclusive).
4. Replay each retained event's `StateDelta` to rebuild session state (skipping
   `app:`, `user:`, and `temp:` prefixed keys, which are managed by the service).
5. Persist the truncated session via a create-before-delete swap to minimise
   data loss windows.
6. Return the new session.

## API Reference

### Rewind

```go
func Rewind(
    ctx context.Context,
    svc session.Service,
    appName, userID, sessionID, targetEventID string,
) (session.Session, error)
```

Truncates the session to the event with the given ID. Returns an error if the
event ID is not found.

### RewindToIndex

```go
func RewindToIndex(
    ctx context.Context,
    svc session.Service,
    appName, userID, sessionID string,
    targetIndex int,
) (session.Session, error)
```

Truncates the session to the event at the given 0-based index. `targetIndex`
must be in `[0, eventCount-1]`.

## Examples

### Rewind by Event ID

```go
import (
    "context"
    "fmt"
    "log"

    "github.com/ieshan/adk-go-pkg/session/rewind"
    "google.golang.org/adk/session"
)

ctx := context.Background()
svc := session.InMemoryService()

// Assume session "sess-1" has events with IDs: "e0", "e1", "e2", "e3", "e4"
// Rewind to "e2" discards "e3" and "e4".

rewound, err := rewind.Rewind(ctx, svc, "my-app", "user-1", "sess-1", "e2")
if err != nil {
    log.Fatal(err)
}
fmt.Println("events remaining:", rewound.Events().Len()) // 3
```

### Rewind by Index

```go
// Keep only the first 3 events (indices 0, 1, 2).
rewound, err := rewind.RewindToIndex(ctx, svc, "my-app", "user-1", "sess-1", 2)
if err != nil {
    log.Fatal(err)
}
fmt.Println("events remaining:", rewound.Events().Len()) // 3
```

### State Recalculation

Session state is rebuilt by replaying `StateDelta` entries from the retained
events in order:

```go
// Before rewind, session state might be:
//   {"counter": 5, "last_tool": "search"}
//
// If events 0-2 had deltas:
//   e0: {"counter": 1}
//   e1: {"counter": 2}
//   e2: {"counter": 3, "last_tool": "search"}
//
// After rewinding to index 2, state is:
//   {"counter": 3, "last_tool": "search"}
//
// The delta from e3 ({"counter": 5}) is discarded.

rewound, err := rewind.RewindToIndex(ctx, svc, "my-app", "user-1", "sess-1", 2)
if err != nil {
    log.Fatal(err)
}
// rewound.State() reflects only deltas from events 0-2
```

Note: App-scoped (`app:`), user-scoped (`user:`), and temporary (`temp:`)
state keys are not replayed by rewind because they are managed separately by the
session service.

### Complete Example with Session Creation

```go
import (
    "context"
    "fmt"
    "log"

    "github.com/ieshan/adk-go-pkg/session/rewind"
    "google.golang.org/adk/session"
)

func main() {
    ctx := context.Background()
    svc := session.InMemoryService()

    // Create a session via the ADK session service.
    resp, err := svc.Create(ctx, &session.CreateRequest{
        AppName: "my-app",
        UserID:  "user-1",
    })
    if err != nil {
        log.Fatal(err)
    }

    // ... append events to the session via your agent runner ...

    // Rewind to the third event (index 2), discarding everything after it.
    rewound, err := rewind.RewindToIndex(ctx, svc, "my-app", "user-1", resp.Session.ID(), 2)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("events remaining:", rewound.Events().Len())
}
```
