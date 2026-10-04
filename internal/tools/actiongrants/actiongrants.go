package actiongrants

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"

// Table returns the generated table. It is the same table on every call, and
// nothing may write through it.
func Table() *finegrained.Table { return &table }

// Requirement returns the row of one canonical action ID, or nil when the
// table holds none, which is what an action added since the last generation
// gets.
func Requirement(id string) *finegrained.Requirement { return table.Requirement(id) }
