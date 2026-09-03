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

	// DefaultDeallocationGracePeriod is the universal "Liquidity Providing
	// Change" waiting period before liquidity leaves a pool.
	DefaultDeallocationGracePeriod = 72 * time.Hour
)

// NewParams creates a new Params instance
func NewParams(stakeCap math.Int, withdrawalGracePeriod, deallocationGracePeriod time.Duration) Params {
	return Params{
		StakeCap:                stakeCap,
		WithdrawalGracePeriod:   withdrawalGracePeriod,
		DeallocationGracePeriod: deallocationGracePeriod,
	}
}

// DefaultParams returns a default set of parameters
func DefaultParams() Params {
	return NewParams(DefaultStakeCap, DefaultWithdrawalGracePeriod, DefaultDeallocationGracePeriod)
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
	if p.DeallocationGracePeriod < 0 {
		return fmt.Errorf("deallocation grace period cannot be negative: %s", p.DeallocationGracePeriod)
	}
	return nil
}
