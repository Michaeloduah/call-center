package billing

import (
	"context"
	"fmt"
	"log/slog"

	"callcenter/internal/platform/mailer"
)

// Admin is a tenant administrator to email.
type Admin struct {
	Email string
	Name  string
}

// Notice is who to tell and about which tenant.
type Notice struct {
	TenantName string
	Admins     []Admin
	PlanName   string
}

// Notifier emails tenant administrators about billing events.
type Notifier interface {
	PaymentFailed(ctx context.Context, n Notice)
	FellBackToFree(ctx context.Context, n Notice)
	UsageThreshold(ctx context.Context, n Notice, cap string, percent int)
}

// EmailNotifier sends those emails through the mailer.
type EmailNotifier struct {
	mail       mailer.Mailer
	webBaseURL string
	logger     *slog.Logger
}

func NewEmailNotifier(m mailer.Mailer, webBaseURL string, logger *slog.Logger) *EmailNotifier {
	return &EmailNotifier{mail: m, webBaseURL: webBaseURL, logger: logger}
}

func (n *EmailNotifier) send(ctx context.Context, notice Notice, subject, text string) {
	for _, a := range notice.Admins {
		body := fmt.Sprintf("Hi %s,\n\n%s\n\nBilling: %s/admin/billing\n", a.Name, text, n.webBaseURL)
		if err := n.mail.Send(ctx, mailer.Message{To: a.Email, Subject: subject, Body: body}); err != nil {
			n.logger.Warn("send billing email", "subject", subject, "error", err)
		}
	}
}

func (n *EmailNotifier) PaymentFailed(ctx context.Context, notice Notice) {
	n.send(ctx, notice, "Your payment failed",
		fmt.Sprintf("We could not collect payment for %s on the %s plan. Please update your payment method within 14 days; after that your organisation moves to the free plan (nothing is deleted).", notice.TenantName, notice.PlanName))
}

func (n *EmailNotifier) FellBackToFree(ctx context.Context, notice Notice) {
	n.send(ctx, notice, "Your organisation is now on the free plan",
		fmt.Sprintf("%s has moved to the free plan. Nothing was deleted, but you cannot create more than the free plan allows. Choose a plan again to restore your limits.", notice.TenantName))
}

func (n *EmailNotifier) UsageThreshold(ctx context.Context, notice Notice, cap string, percent int) {
	what := "recording storage"
	if cap == "minutes" {
		what = "call minutes this month"
	}
	text := fmt.Sprintf("%s has used %d%% of its %s.", notice.TenantName, percent, what)
	if percent >= 100 {
		text += " Depending on your settings, new calls are now blocked or billed as overage."
	}
	n.send(ctx, notice, fmt.Sprintf("You have used %d%% of your plan's %s", percent, what), text)
}
