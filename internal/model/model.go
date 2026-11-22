// Package model holds plain domain types shared by services and templates.
package model

type Role string

const (
	RoleAdmin        Role = "admin"
	RoleConsultant   Role = "consultant"
	RoleAgentNetwork Role = "agent_network"
)

func (r Role) Valid() bool {
	return r == RoleAdmin || r == RoleConsultant || r == RoleAgentNetwork
}

func (r Role) Label() string {
	switch r {
	case RoleAdmin:
		return "Admin"
	case RoleConsultant:
		return "Consultant"
	case RoleAgentNetwork:
		return "Agent network"
	}
	return string(r)
}

// User is the signed-in staff member (never carries the password hash).
type User struct {
	ID          int64
	Name        string
	Email       string
	Role        Role
	BranchID    *int16
	BranchName  string
	Active      bool
	AuthVersion int32
}

func (u *User) IsAdmin() bool { return u != nil && u.Role == RoleAdmin }

// RoleLabel is the pill text in the sidebar, e.g. "Consultant · Chennai".
func (u *User) RoleLabel() string {
	switch u.Role {
	case RoleAdmin:
		return "Admin · All branches"
	case RoleConsultant:
		return "Consultant · " + u.BranchName
	default:
		return "Agent network · B2B"
	}
}

type Branch struct {
	ID   int16
	Name string
}
