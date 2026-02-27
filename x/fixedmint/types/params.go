package types

// NOTE: After modifying params.proto, you must regenerate protobuf files by running:
//   ignite generate proto-go --yes
// The Params struct in params.pb.go will then include MintDenom, MintAmount, and MintEnabled fields.
// Until proto is regenerated, this file will not compile.

import (
	"fmt"

	"cosmossdk.io/math"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
)

var _ paramtypes.ParamSet = (*Params)(nil)

// Default parameter values
var (
	DefaultMintDenom   = "ubluechip"
	DefaultMintAmount  = math.NewInt(1000000) // 1 BLUECHIP per block
	DefaultMintEnabled = true
)

// ParamKeyTable the param key table for launch module
func ParamKeyTable() paramtypes.KeyTable {
	return paramtypes.NewKeyTable().RegisterParamSet(&Params{})
}

// NewParams creates a new Params instance
func NewParams(mintDenom string, mintAmount math.Int, mintEnabled bool) Params {
	return Params{
		MintDenom:   mintDenom,
		MintAmount:  mintAmount,
		MintEnabled: mintEnabled,
	}
}

// DefaultParams returns a default set of parameters
func DefaultParams() Params {
	return NewParams(DefaultMintDenom, DefaultMintAmount, DefaultMintEnabled)
}

// ParamSetPairs get the params.ParamSet
func (p *Params) ParamSetPairs() paramtypes.ParamSetPairs {
	return paramtypes.ParamSetPairs{}
}

// Validate validates the set of params
func (p Params) Validate() error {
	if err := validateMintDenom(p.MintDenom); err != nil {
		return err
	}
	if err := validateMintAmount(p.MintAmount); err != nil {
		return err
	}
	return nil
}

func validateMintDenom(denom string) error {
	if denom == "" {
		return fmt.Errorf("mint denom cannot be empty")
	}
	return nil
}

func validateMintAmount(amount math.Int) error {
	if amount.IsNegative() {
		return fmt.Errorf("mint amount cannot be negative: %s", amount)
	}
	return nil
}
