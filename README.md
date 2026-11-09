# LFU Cache

A generic Least Frequently Used (LFU) cache written in Go. When the cache is full,
it removes the least frequently used item. If several items have the same
frequency, the least recently used one is removed first.

## Features

- Generic keys and values.
- Constant-time `Get` and `Put` operations.
- Configurable capacity (default: 5).
- Iteration from most to least frequently used items.
- Cache merging with `Merge`.

## Requirements

Use the Go version specified in [go.mod](go.mod) (1.27.1).


## Tests (take at least 15s)

```sh
go test ./...
```

