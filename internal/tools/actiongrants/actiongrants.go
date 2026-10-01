package actiongrants

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"

// Table returns the generated table. It is the same table on every call, and
// nothing may write through it.
func Table() *finegrained.Table { return &table }

// Requirement returns the row of one canonical action ID, or nil when the
// table holds none, which is what an action added since the last generation
// gets.
func Requirement(id string) *finegrained.Requirement { return table.Requirement(id) }

// Build returns what a credential may do as a fine-grained personal access
// token, for the client that carries it, or nil for any other credential.
//
// What a fine-grained token is given is judged from what was read about it
// against the generated table ([finegrained.Judge]): phase B, only what its
// grant reaches, when the grant was read and the instance reports the
// release the table was recorded from; phase A otherwise, which withholds
// exactly what no fine-grained token can reach, whatever its grant, and
// leaves the rest to GitLab, saying why the grant was not evaluated. A classic
// token gets nil and is decided by nothing here, which keeps its path and its
// cost what they were.
//
// It is called once per pool entry in HTTP mode and once per process on stdio,
// never per request, so the authority it returns is shared by every request
// of that credential.
func Build(fineGrained bool, reading finegrained.Reading) *finegrained.Authority {
	if !fineGrained {
		return nil
	}
	return finegrained.Judge(&table, reading)
}
