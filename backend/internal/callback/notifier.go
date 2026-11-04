package callback

import (
	"context"
	"fmt"
	"log/slog"

	"callcenter/internal/platform/mailer"
)

// EmailNotifier emails tenant administrators about new callback requests.
type EmailNotifier struct {
	mail       mailer.Mailer
	webBaseURL string
	logger     *slog.Logger
}

func NewEmailNotifier(m mailer.Mailer, webBaseURL string, logger *slog.Logger) *EmailNotifier {
	return &EmailNotifier{mail: m, webBaseURL: webBaseURL, logger: logger}
}

func (n *EmailNotifier) NewCallback(ctx context.Context, tenantName string, admins []Admin, cb Notification) {
	// Caller-supplied text only goes into the plain-text body, never into headers.
	body := func(admin Admin) string {
		return fmt.Sprintf(`Hi %s,

Someone asked to be called back (%s, %s).

Name:   %s
Phone:  %s
Email:  %s
Reason: %s

Open the request: %s/callbacks/%s
`, admin.Name, tenantName, cb.DepartmentName, cb.Name, orDash(cb.Phone), orDash(cb.Email), orDash(cb.Reason), n.webBaseURL, cb.ID)
	}
	for _, admin := range admins {
		if err := n.mail.Send(ctx, mailer.Message{To: admin.Email, Subject: "New callback request", Body: body(admin)}); err != nil {
			n.logger.Warn("send callback notification", "callback_id", cb.ID, "error", err)
		}
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
