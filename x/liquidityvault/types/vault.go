package types

import (
	"fmt"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// DefaultDelegatorRewardShare is the fraction of vault liquidity rewards
// passed to delegators until the validator sets one. The LPV outline's worked
// example assumes a 50/50 split.
var DefaultDelegatorRewardShare = math.LegacyNewDecWithPrec(5, 1) // 0.5

// NewVault creates a vault for a validator with a zero balance and the
// default delegator reward share.
func NewVault(valAddr sdk.ValAddress) Vault {
	return Vault{
		ValidatorAddress:     valAddr.String(),
		Balance:              math.ZeroInt(),
		DelegatorRewardShare: DefaultDelegatorRewardShare,
	}
}

// Validate performs basic vault field validation.
func (v Vault) Validate() error {
	if _, err := sdk.ValAddressFromBech32(v.ValidatorAddress); err != nil {
		return errorsmod.Wrap(err, "invalid validator address")
	}
	if v.Balance.IsNil() || v.Balance.IsNegative() {
		return fmt.Errorf("vault balance must be a non-negative integer: %s", v.Balance)
	}
	return ValidateRewardShare(v.DelegatorRewardShare)
}

// Validate performs basic pending withdrawal field validation.
func (w PendingWithdrawal) Validate() error {
	if _, err := sdk.ValAddressFromBech32(w.ValidatorAddress); err != nil {
		return errorsmod.Wrap(err, "invalid validator address")
	}
	if w.Amount.IsNil() || !w.Amount.IsPositive() {
		return fmt.Errorf("pending withdrawal amount must be positive: %s", w.Amount)
	}
	if w.CompleteTime.IsZero() {
		return fmt.Errorf("pending withdrawal complete time must be set")
	}
	return nil
}

// ValidateRewardShare checks that a delegator reward share is a valid
// fraction in [0, 1].
func ValidateRewardShare(share math.LegacyDec) error {
	if share.IsNil() || share.IsNegative() || share.GT(math.LegacyOneDec()) {
		return errorsmod.Wrapf(ErrInvalidRewardShare, "got %s", share)
	}
	return nil
}
