package types

import (
	"fmt"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Validate performs basic registered pool field validation.
func (p RegisteredPool) Validate() error {
	if p.PoolId == 0 {
		return fmt.Errorf("pool id cannot be zero")
	}
	if _, err := sdk.AccAddressFromBech32(p.ContractAddress); err != nil {
		return errorsmod.Wrap(err, "invalid pool contract address")
	}
	return nil
}

// Validate performs basic pool position field validation.
func (p PoolPosition) Validate() error {
	if _, err := sdk.ValAddressFromBech32(p.ValidatorAddress); err != nil {
		return errorsmod.Wrap(err, "invalid position validator address")
	}
	if p.PoolId == 0 {
		return fmt.Errorf("position pool id cannot be zero")
	}
	if p.Shares.IsNil() || !p.Shares.IsPositive() {
		return fmt.Errorf("position shares must be positive: %s", p.Shares)
	}
	return nil
}

// Validate performs basic pending deallocation field validation.
func (d PendingDeallocation) Validate() error {
	if _, err := sdk.ValAddressFromBech32(d.ValidatorAddress); err != nil {
		return errorsmod.Wrap(err, "invalid deallocation validator address")
	}
	if d.PoolId == 0 {
		return fmt.Errorf("deallocation pool id cannot be zero")
	}
	if d.Shares.IsNil() || !d.Shares.IsPositive() {
		return fmt.Errorf("deallocation shares must be positive: %s", d.Shares)
	}
	if d.CompleteTime.IsZero() {
		return fmt.Errorf("deallocation complete time must be set")
	}
	return nil
}
