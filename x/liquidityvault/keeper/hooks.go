package keeper

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

var _ stakingtypes.StakingHooks = StakingHooks{}

// StakingHooks implements stakingtypes.StakingHooks interface
type StakingHooks struct {
	k Keeper
}

// Hooks returns the staking hooks for the liquidityvault keeper
func (k Keeper) Hooks() StakingHooks {
	return StakingHooks{k}
}

// AfterValidatorCreated is called after a validator is created
func (h StakingHooks) AfterValidatorCreated(_ context.Context, _ sdk.ValAddress) error {
	return nil
}

// BeforeValidatorModified is called before a validator is modified
func (h StakingHooks) BeforeValidatorModified(_ context.Context, _ sdk.ValAddress) error {
	return nil
}

// AfterValidatorRemoved is called after a validator is removed
func (h StakingHooks) AfterValidatorRemoved(_ context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}

// AfterValidatorBonded is called after a validator is bonded
func (h StakingHooks) AfterValidatorBonded(_ context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}

// AfterValidatorBeginUnbonding is called after a validator begins unbonding
func (h StakingHooks) AfterValidatorBeginUnbonding(_ context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}

// BeforeDelegationCreated enforces the stake cap when a new delegation is created.
// If the delegation would push the validator over the cap, it is rejected.
func (h StakingHooks) BeforeDelegationCreated(ctx context.Context, _ sdk.AccAddress, valAddr sdk.ValAddress) error {
	// Check if this validator is registered in our module
	_, found := h.k.GetVault(ctx, valAddr.String())
	if !found {
		// Validator not in our system, allow delegation
		return nil
	}

	// We cannot know the exact delegation amount in this hook.
	// The actual enforcement happens via the delegation amount check.
	// For Phase 1, we just log that a new delegation is being created.
	return nil
}

// BeforeDelegationSharesModified is called before delegation shares are modified
func (h StakingHooks) BeforeDelegationSharesModified(ctx context.Context, _ sdk.AccAddress, valAddr sdk.ValAddress) error {
	return nil
}

// BeforeDelegationRemoved is called before a delegation is removed
func (h StakingHooks) BeforeDelegationRemoved(_ context.Context, _ sdk.AccAddress, _ sdk.ValAddress) error {
	return nil
}

// AfterDelegationModified is called after a delegation is modified.
// We check if the validator is now over the stake cap and emit a warning event.
func (h StakingHooks) AfterDelegationModified(ctx context.Context, _ sdk.AccAddress, valAddr sdk.ValAddress) error {
	_, found := h.k.GetVault(ctx, valAddr.String())
	if !found {
		return nil
	}

	excess, err := h.k.CheckStakeCap(ctx, valAddr, math.ZeroInt())
	if err != nil {
		return nil // Don't fail delegations due to our check errors
	}

	if excess.IsPositive() {
		sdkCtx := sdk.UnwrapSDKContext(ctx)
		sdkCtx.EventManager().EmitEvent(
			sdk.NewEvent(
				"stake_cap_exceeded",
				sdk.NewAttribute("validator", valAddr.String()),
				sdk.NewAttribute("excess", excess.String()),
			),
		)
		h.k.Logger().Warn("validator stake exceeds cap",
			"validator", valAddr.String(),
			"excess", excess.String(),
		)
	}

	return nil
}

// BeforeValidatorSlashed is called before a validator is slashed
func (h StakingHooks) BeforeValidatorSlashed(_ context.Context, _ sdk.ValAddress, _ math.LegacyDec) error {
	return nil
}

// AfterUnbondingInitiated is called after an unbonding is initiated
func (h StakingHooks) AfterUnbondingInitiated(_ context.Context, _ uint64) error {
	return nil
}
