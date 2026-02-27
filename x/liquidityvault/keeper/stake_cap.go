package keeper

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// CheckStakeCap checks if adding additional stake to a validator would exceed the stake cap.
// Returns the excess amount (zero if within cap), or error.
func (k Keeper) CheckStakeCap(ctx context.Context, valAddr sdk.ValAddress, additionalStake math.Int) (math.Int, error) {
	params := k.GetParams(ctx)
	stakeCap := params.StakeCap

	// If stake cap is zero, no cap is enforced
	if stakeCap.IsZero() {
		return math.ZeroInt(), nil
	}

	validator, err := k.stakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		// Validator not found - allow (it's being created)
		return math.ZeroInt(), nil
	}

	currentStake := validator.GetBondedTokens()
	newTotal := currentStake.Add(additionalStake)

	if newTotal.GT(stakeCap) {
		excess := newTotal.Sub(stakeCap)
		return excess, nil
	}

	return math.ZeroInt(), nil
}

// EnforceStakeCap checks if a validator is over the stake cap.
// Returns an error if the validator would be over the cap with the additional amount.
func (k Keeper) EnforceStakeCap(ctx context.Context, valAddr sdk.ValAddress, additionalStake math.Int) error {
	excess, err := k.CheckStakeCap(ctx, valAddr, additionalStake)
	if err != nil {
		return err
	}

	if excess.IsPositive() {
		return types.ErrStakeCapExceeded
	}

	return nil
}
