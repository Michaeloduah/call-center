package auth

import (
	"context"
	"fmt"
	"net/url"

	"callcenter/internal/platform/mailer"
)

// Notifier composes and sends auth-related emails.
type Notifier struct {
	mail       mailer.Mailer
	webBaseURL string
}

func NewNotifier(m mailer.Mailer, webBaseURL string) *Notifier {
	return &Notifier{mail: m, webBaseURL: webBaseURL}
}

func (n *Notifier) link(path, token string) string {
	return n.webBaseURL + path + "?token=" + url.QueryEscape(token)
}

func (n *Notifier) sendVerifyEmail(ctx context.Context, to, fullName, token string) error {
	return n.mail.Send(ctx, mailer.Message{
		To:      to,
		Subject: "Verify your email address",
		Body: fmt.Sprintf(`Hi %s,

Thanks for signing up. Please confirm your email address to activate your account:

%s

This link expires in 24 hours. If you did not sign up, you can ignore this email.
`, fullName, n.link("/verify-email", token)),
	})
}

func (n *Notifier) sendPasswordReset(ctx context.Context, to, fullName, token string) error {
	return n.mail.Send(ctx, mailer.Message{
		To:      to,
		Subject: "Reset your password",
		Body: fmt.Sprintf(`Hi %s,

We received a request to reset your password. Choose a new one here:

%s

This link expires in 1 hour. If you did not request this, you can ignore this email.
`, fullName, n.link("/reset-password", token)),
	})
}

// InviteEmail describes an invitation to send.
type InviteEmail struct {
	To         string
	FullName   string
	TenantName string // empty for platform admins
	Token      string
}

func (n *Notifier) sendInvite(ctx context.Context, in InviteEmail) error {
	org := "CallCenter"
	if in.TenantName != "" {
		org = in.TenantName
	}
	return n.mail.Send(ctx, mailer.Message{
		To:      in.To,
		Subject: "You've been invited to " + org,
		Body: fmt.Sprintf(`Hi %s,

You have been invited to join %s on CallCenter. Set your password to get started:

%s

This link expires in 7 days.
`, in.FullName, org, n.link("/accept-invite", in.Token)),
	})
}
