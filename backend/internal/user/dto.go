package user

import (
	"slices"

	"callcenter/internal/platform/reqctx"
	"callcenter/internal/platform/validate"
)

type CreateInput struct {
	FullName string      `json:"full_name"`
	Email    string      `json:"email"`
	Role     reqctx.Role `json:"role"`
}

func (in *CreateInput) Validate() error {
	v := validate.Errors{}
	in.FullName = v.RequiredString("full_name", in.FullName, 120)
	in.Email = v.Email("email", in.Email)
	validateRole(v, in.Role)
	return v.Err()
}

type UpdateInput struct {
	FullName string      `json:"full_name"`
	Role     reqctx.Role `json:"role"`
}

func (in *UpdateInput) Validate() error {
	v := validate.Errors{}
	in.FullName = v.RequiredString("full_name", in.FullName, 120)
	validateRole(v, in.Role)
	return v.Err()
}

func validateRole(v validate.Errors, r reqctx.Role) {
	if !r.IsTenantRole() {
		v.Add("role", "Choose a valid role.")
	}
}

// FilterFromQuery validates list filters; invalid values are ignored rather than rejected.
func FilterFromQuery(query, role, status string) Filter {
	f := Filter{Query: query}
	if r := reqctx.Role(role); r.IsTenantRole() {
		f.Role = r
	}
	if slices.Contains(statuses, status) {
		f.Status = status
	}
	if len([]rune(f.Query)) > 100 {
		f.Query = string([]rune(f.Query)[:100])
	}
	return f
}
