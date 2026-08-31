// Command kanban-plugin serves the GoatFlow Kanban plugin via gRPC (go-plugin).
//
// Build: go build -ldflags="-s -w" -o kanban ./cmd/kanban-plugin
//
// Deploy:
//
//	plugins/goat-kanban/
//	  ├── plugin.yaml   # manifest
//	  └── kanban        # this binary
package main

import (
	"github.com/goatkit/goat-kanban/internal/kanban"
	"github.com/goatkit/goatflow/pkg/plugin/grpcutil"
)

func main() {
	grpcutil.ServePlugin(kanban.New())
}
