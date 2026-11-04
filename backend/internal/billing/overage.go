package billing

import "callcenter/internal/limits"

// Overage is the extra usage of a period and what it costs.
type Overage struct {
	Minutes int
	GB      int
	Amount  int64 // minor units
}

// ComputeOverage prices the usage beyond a plan's caps. Only caps whose policy is "overage" are
// billed, and the excess is rounded up to whole minutes and whole GB.
func ComputeOverage(callSeconds, storageBytes int64, caps limits.Caps, pol limits.Policies, price Price) Overage {
	var o Overage
	if caps.MaxCallMinutes != nil && pol.Minutes == limits.PolicyOverage {
		if excess := callSeconds - int64(*caps.MaxCallMinutes)*limits.SecondsPerMinute; excess > 0 {
			o.Minutes = int(ceilDiv(excess, limits.SecondsPerMinute))
		}
	}
	if caps.MaxStorageBytes != nil && pol.Storage == limits.PolicyOverage {
		if excess := storageBytes - *caps.MaxStorageBytes; excess > 0 {
			o.GB = int(ceilDiv(excess, BytesPerGB))
		}
	}
	o.Amount = int64(o.Minutes)*price.OveragePerMinuteMinor + int64(o.GB)*price.OveragePerGBMinor
	return o
}

func ceilDiv(a, b int64) int64 { return (a + b - 1) / b }
