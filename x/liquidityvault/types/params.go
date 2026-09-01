package types

import (
	"fmt"
	"time"

	"cosmossdk.io/math"
)

// Default parameter values
var (
	// DefaultStakeCap disables the stake cap until governance sets one.
	DefaultStakeCap = math.ZeroInt()

	// DefaultWithdrawalGracePeriod matches the LPV outline's repayment epoch
	// length of 3 days.
	DefaultWithdrawalGracePeriod = 72 * time.Hour
)

// NewParams creates a new Params instance
func NewParams(stakeCap math.Int, withdrawalGracePeriod time.Duration) Params {
	return Params{
		StakeCap:              stakeCap,
		WithdrawalGracePeriod: withdrawalGracePeriod,
	}
}

// DefaultParams returns a default set of parameters
func DefaultParams() Params {
	return NewParams(DefaultStakeCap, DefaultWithdrawalGracePeriod)
}

// Validate validates the set of params
func (p Params) Validate() error {
	if p.StakeCap.IsNil() {
		return fmt.Errorf("stake cap cannot be nil")
	}
	if p.StakeCap.IsNegative() {
		return fmt.Errorf("stake cap cannot be negative: %s", p.StakeCap)
	}
	if p.WithdrawalGracePeriod < 0 {
		return fmt.Errorf("withdrawal grace period cannot be negative: %s", p.WithdrawalGracePeriod)
	}
	return nil
}
