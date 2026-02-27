package types

import (
	"fmt"

	"cosmossdk.io/math"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
)

var _ paramtypes.ParamSet = (*Params)(nil)

// Default parameter values
var (
	DefaultStakeCap                     = math.NewInt(1_000_000_000_000) // 1M BLUECHIP (in ubluechip)
	DefaultSimpleCheckInterval          = uint64(14400)                  // ~24h at 6s blocks
	DefaultComplexCheckInterval         = uint64(72000)                  // ~5 days at 6s blocks
	DefaultValuePostsPerComplexInterval = uint64(6)
	DefaultDelegatorRewardPercent       = math.LegacyNewDecWithPrec(50, 0) // 50%
)

// ParamKeyTable the param key table for launch module
func ParamKeyTable() paramtypes.KeyTable {
	return paramtypes.NewKeyTable().RegisterParamSet(&Params{})
}

// NewParams creates a new Params instance
func NewParams(
	stakeCap math.Int,
	simpleCheckInterval uint64,
	complexCheckInterval uint64,
	valuePostsPerComplexInterval uint64,
	defaultDelegatorRewardPercent math.LegacyDec,
) Params {
	return Params{
		StakeCap:                     stakeCap,
		SimpleCheckInterval:          simpleCheckInterval,
		ComplexCheckInterval:         complexCheckInterval,
		ValuePostsPerComplexInterval: valuePostsPerComplexInterval,
		DefaultDelegatorRewardPercent: defaultDelegatorRewardPercent,
	}
}

// DefaultParams returns a default set of parameters
func DefaultParams() Params {
	return NewParams(
		DefaultStakeCap,
		DefaultSimpleCheckInterval,
		DefaultComplexCheckInterval,
		DefaultValuePostsPerComplexInterval,
		DefaultDelegatorRewardPercent,
	)
}

// ParamSetPairs get the params.ParamSet
func (p *Params) ParamSetPairs() paramtypes.ParamSetPairs {
	return paramtypes.ParamSetPairs{}
}

// Validate validates the set of params
func (p Params) Validate() error {
	if p.StakeCap.IsNegative() {
		return fmt.Errorf("stake cap cannot be negative: %s", p.StakeCap)
	}
	if p.SimpleCheckInterval == 0 {
		return fmt.Errorf("simple check interval must be positive")
	}
	if p.ComplexCheckInterval == 0 {
		return fmt.Errorf("complex check interval must be positive")
	}
	if p.ValuePostsPerComplexInterval == 0 {
		return fmt.Errorf("value posts per complex interval must be positive")
	}
	if p.DefaultDelegatorRewardPercent.IsNegative() || p.DefaultDelegatorRewardPercent.GT(math.LegacyNewDec(100)) {
		return fmt.Errorf("default delegator reward percent must be between 0 and 100")
	}
	return nil
}
