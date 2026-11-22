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

type Option struct {
	Value string
	Label string
}

type University struct {
	ID      int64
	Name    string
	Country string
}

type Consultancy struct {
	ID   int64
	Name string
}

// Statuses in display order.
var Statuses = []Option{
	{"applied", "Applied"}, {"admitted", "Admitted"}, {"reserve", "Reserve"}, {"rejected", "Rejected"},
	{"withdrawn", "Withdrawn"}, {"fees_paid", "Fees paid"}, {"visa_granted", "Visa granted"}, {"visa_rejected", "Visa rejected"},
}

var Levels = []Option{{"masters", "Master’s"}, {"bachelors", "Bachelor’s"}, {"top_up", "Top-up"}}
var Intakes = []Option{{"autumn", "Autumn"}, {"spring", "Spring"}}
var TuitionPer = []Option{{"semester", "Semester"}, {"year", "Year"}, {"programme", "Programme"}}
var Genders = []Option{{"Female", "Female"}, {"Male", "Male"}, {"Other", "Other"}}
var Currencies = []string{"EUR", "SEK", "DKK", "NOK", "GBP", "CHF", "PLN", "CZK", "HUF", "INR"}
var Countries = []string{"Austria", "Belgium", "Czechia", "Denmark", "Estonia", "Finland", "France", "Germany", "Hungary",
	"Ireland", "Italy", "Latvia", "Lithuania", "Luxembourg", "Malta", "Netherlands", "Norway", "Poland", "Portugal",
	"Spain", "Sweden", "Switzerland", "UK"}

// ProgressSteps is the happy path shown as dots on each application.
var ProgressSteps = []Option{{"applied", "Applied"}, {"admitted", "Admitted"}, {"fees_paid", "Fees paid"}, {"visa_granted", "Visa granted"}}

func label(opts []Option, v string) string {
	for _, o := range opts {
		if o.Value == v {
			return o.Label
		}
	}
	return v
}

func ValidOption(opts []Option, v string) bool {
	for _, o := range opts {
		if o.Value == v {
			return true
		}
	}
	return false
}

func StatusLabel(v string) string {
	if v == "" {
		return "No status"
	}
	return label(Statuses, v)
}
func LevelLabel(v string) string  { return label(Levels, v) }
func IntakeLabel(v string) string { return label(Intakes, v) }
func PerLabel(v string) string    { return label(TuitionPer, v) }

// StatusClass is the badge CSS class for a status value.
func StatusClass(v string) string {
	if v == "" {
		return "s-none"
	}
	out := []byte("s-")
	for i := 0; i < len(v); i++ {
		if v[i] != '_' {
			out = append(out, v[i])
		}
	}
	return string(out)
}
