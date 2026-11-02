package tenant

import (
	"slices"
	"strings"

	"callcenter/internal/platform/validate"
)

type CreateInput struct {
	Name       string `json:"name"`
	AdminName  string `json:"admin_name"`
	AdminEmail string `json:"admin_email"`
}

func (in *CreateInput) Validate() error {
	v := validate.Errors{}
	in.Name = v.RequiredString("name", in.Name, 120)
	in.AdminName = v.RequiredString("admin_name", in.AdminName, 120)
	in.AdminEmail = v.Email("admin_email", in.AdminEmail)
	return v.Err()
}

type UpdateInput struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (in *UpdateInput) Validate() error {
	v := validate.Errors{}
	in.Name = v.RequiredString("name", in.Name, 120)
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	if !ValidSlug(in.Slug) {
		v.Add("slug", "Use 3–60 lowercase letters, numbers and single hyphens.")
	}
	return v.Err()
}

// FilterFromQuery validates list filters; invalid values are ignored rather than rejected.
func FilterFromQuery(query, status string) Filter {
	f := Filter{Query: strings.TrimSpace(query)}
	if len([]rune(f.Query)) > 100 {
		f.Query = string([]rune(f.Query)[:100])
	}
	if slices.Contains(statuses, status) {
		f.Status = status
	}
	return f
}
