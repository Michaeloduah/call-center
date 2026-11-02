package widget

import (
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"callcenter/internal/platform/validate"
)

const maxAllowedOrigins = 20

var colorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

type Input struct {
	Name           string     `json:"name"`
	DepartmentID   uuid.UUID  `json:"department_id"`
	AllowedOrigins []string   `json:"allowed_origins"`
	IsActive       *bool      `json:"is_active"`
	ButtonLabel    string     `json:"button_label"`
	AccentColor    string     `json:"accent_color"`
	Greeting       string     `json:"greeting"`
	CallerForm     CallerForm `json:"caller_form"`
}

func (in *Input) Validate() error {
	v := validate.Errors{}
	in.Name = v.RequiredString("name", in.Name, 80)
	if in.DepartmentID == uuid.Nil {
		v.Add("department_id", "Choose a department.")
	}
	in.AllowedOrigins = validateOrigins(v, in.AllowedOrigins)
	if in.IsActive == nil {
		v.Add("is_active", "This field is required.")
	}
	in.ButtonLabel = v.RequiredString("button_label", in.ButtonLabel, 40)
	in.AccentColor = strings.ToLower(strings.TrimSpace(in.AccentColor))
	if !colorPattern.MatchString(in.AccentColor) {
		v.Add("accent_color", "Use a hex colour such as #4f46e5.")
	}
	in.Greeting = v.OptionalString("greeting", in.Greeting, 200)
	for field, mode := range map[string]FieldMode{
		"caller_form.name": in.CallerForm.Name, "caller_form.phone": in.CallerForm.Phone, "caller_form.email": in.CallerForm.Email,
	} {
		if !mode.Valid() {
			v.Add(field, "Choose hidden, optional or required.")
		}
	}
	return v.Err()
}

// validateOrigins normalises each entry to scheme://host[:port] and de-duplicates.
// HTTPS is required except for local development hosts.
func validateOrigins(v validate.Errors, raw []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		origin, ok := normaliseOrigin(r)
		if !ok {
			v.Add("allowed_origins", "“"+truncate(r, 60)+"” is not a valid website origin, e.g. https://www.example.com")
			continue
		}
		if !seen[origin] {
			seen[origin] = true
			out = append(out, origin)
		}
	}
	switch {
	case len(out) == 0:
		v.Add("allowed_origins", "Add at least one website where the widget will be embedded.")
	case len(out) > maxAllowedOrigins:
		v.Add("allowed_origins", "At most 20 websites are allowed.")
	}
	return out
}

func normaliseOrigin(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if scheme != "https" && !(scheme == "http" && isLocalHost(host)) {
		return "", false
	}
	if strings.ContainsAny(host, "*") {
		return "", false
	}
	origin := scheme + "://" + host
	if port := u.Port(); port != "" {
		origin += ":" + port
	}
	return origin, true
}

func isLocalHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".test") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// FilterFromQuery validates list filters.
func FilterFromQuery(query string) Filter {
	f := Filter{Query: strings.TrimSpace(query)}
	if r := []rune(f.Query); len(r) > 100 {
		f.Query = string(r[:100])
	}
	return f
}
