// Package access is the ONLY place that decides which students a user can see.
//
//	Admin:         everything
//	Consultant:    internal students of their own branch
//	Agent network: all B2B students
//
// Applications inherit visibility from their student.
package access

import (
	"fmt"

	"github.com/ChandraShekar2001/application-tracker/internal/model"
)

type Scope struct {
	Role     model.Role
	BranchID *int16
	UserID   int64
}

func ScopeOf(u *model.User) Scope {
	return Scope{Role: u.Role, BranchID: u.BranchID, UserID: u.ID}
}

func (s Scope) IsAdmin() bool { return s.Role == model.RoleAdmin }

// CanCreateStudents: consultants (into their branch) and the agent network (B2B).
func (s Scope) CanCreateStudents() bool {
	return s.Role == model.RoleConsultant || s.Role == model.RoleAgentNetwork
}

// NewStudentSource is the source a student created by this user gets.
func (s Scope) NewStudentSource() (source string, branchID *int16) {
	if s.Role == model.RoleConsultant {
		return "internal", s.BranchID
	}
	return "b2b", nil
}

// CanSee mirrors StudentFilter for a student already loaded into memory
// (used for the duplicate-email message, never as the primary check).
func (s Scope) CanSee(source string, branchID *int16) bool {
	switch s.Role {
	case model.RoleAdmin:
		return true
	case model.RoleConsultant:
		return source == "internal" && branchID != nil && s.BranchID != nil && *branchID == *s.BranchID
	case model.RoleAgentNetwork:
		return source == "b2b"
	}
	return false
}

// Args accumulates positional query arguments ($1, $2, ...).
type Args struct{ vals []any }

// Add appends v and returns its placeholder.
func (a *Args) Add(v any) string {
	a.vals = append(a.vals, v)
	return fmt.Sprintf("$%d", len(a.vals))
}

func (a *Args) Values() []any { return a.vals }

// StudentFilter returns a SQL boolean expression restricting the students
// table (aliased as alias) to what scope may see. Unknown roles see nothing.
func StudentFilter(s Scope, alias string, a *Args) string {
	switch s.Role {
	case model.RoleAdmin:
		return "TRUE"
	case model.RoleConsultant:
		if s.BranchID == nil {
			return "FALSE"
		}
		return fmt.Sprintf("(%[1]s.source = 'internal' AND %[1]s.branch_id = %[2]s)", alias, a.Add(*s.BranchID))
	case model.RoleAgentNetwork:
		return fmt.Sprintf("(%s.source = 'b2b')", alias)
	}
	return "FALSE"
}
