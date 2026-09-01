package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"bluechipChain/x/liquidityvault/types"
)

// Hooks enforces the validator stake cap through the staking module's hook
// interface. Enforcing here (rather than in an ante decorator) covers every
// delegation path, including messages nested inside authz MsgExec.
//
// The Before* hooks snapshot the validator's tokens; AfterDelegationModified
// compares against the snapshot and rejects the operation when it INCREASED
// the validator's tokens beyond the cap. Operations that decrease tokens
// (undelegations) are never blocked, even if the validator already sits above
// a newly lowered cap. If no snapshot exists (paths that skip the Before
// hooks, such as genesis import), the cap is not enforced.
type Hooks struct {
	k Keeper
}

var _ stakingtypes.StakingHooks = Hooks{}

// StakingHooks returns the staking hooks that enforce the stake cap.
func (k Keeper) StakingHooks() Hooks {
	return Hooks{k: k}
}

// BeforeDelegationCreated snapshots the validator's tokens before a new
// delegation is added.
func (h Hooks) BeforeDelegationCreated(ctx context.Context, _ sdk.AccAddress, valAddr sdk.ValAddress) error {
	return h.k.snapshotValidatorTokens(ctx, valAddr)
}

// BeforeDelegationSharesModified snapshots the validator's tokens before an
// existing delegation changes.
func (h Hooks) BeforeDelegationSharesModified(ctx context.Context, _ sdk.AccAddress, valAddr sdk.ValAddress) error {
	return h.k.snapshotValidatorTokens(ctx, valAddr)
}

// AfterDelegationModified enforces the stake cap against the snapshot taken
// by the Before* hooks.
func (h Hooks) AfterDelegationModified(ctx context.Context, _ sdk.AccAddress, valAddr sdk.ValAddress) error {
	return h.k.enforceStakeCap(ctx, valAddr)
}

func (h Hooks) AfterValidatorCreated(context.Context, sdk.ValAddress) error   { return nil }
func (h Hooks) BeforeValidatorModified(context.Context, sdk.ValAddress) error { return nil }
func (h Hooks) AfterValidatorRemoved(context.Context, sdk.ConsAddress, sdk.ValAddress) error {
	return nil
}
func (h Hooks) AfterValidatorBonded(context.Context, sdk.ConsAddress, sdk.ValAddress) error {
	return nil
}
func (h Hooks) AfterValidatorBeginUnbonding(context.Context, sdk.ConsAddress, sdk.ValAddress) error {
	return nil
}
func (h Hooks) BeforeDelegationRemoved(context.Context, sdk.AccAddress, sdk.ValAddress) error {
	return nil
}
func (h Hooks) BeforeValidatorSlashed(context.Context, sdk.ValAddress, math.LegacyDec) error {
	return nil
}
func (h Hooks) AfterUnbondingInitiated(context.Context, uint64) error { return nil }

// snapshotValidatorTokens records the validator's current tokens so the
// following AfterDelegationModified can tell whether the operation increased
// them. A validator that does not exist yet (MsgCreateValidator) snapshots as
// zero.
func (k Keeper) snapshotValidatorTokens(ctx context.Context, valAddr sdk.ValAddress) error {
	tokens := math.ZeroInt()
	if validator, err := k.stakingKeeper.GetValidator(ctx, valAddr); err == nil {
		tokens = validator.GetTokens()
	}

	bz, err := tokens.Marshal()
	if err != nil {
		return err
	}

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	store.Set(types.TokensSnapshotKey(valAddr), bz)
	return nil
}

// enforceStakeCap fails the surrounding delegation operation when it pushed
// the validator's tokens above the stake cap. The snapshot is consumed either
// way; without one the cap is not enforced (see Hooks).
func (k Keeper) enforceStakeCap(ctx context.Context, valAddr sdk.ValAddress) error {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	key := types.TokensSnapshotKey(valAddr)
	bz := store.Get(key)
	if bz == nil {
		return nil
	}
	store.Delete(key)

	stakeCap := k.GetParams(ctx).StakeCap
	if stakeCap.IsNil() || !stakeCap.IsPositive() {
		return nil
	}

	validator, err := k.stakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		return nil
	}
	tokens := validator.GetTokens()

	var previousTokens math.Int
	if err := previousTokens.Unmarshal(bz); err != nil {
		return err
	}

	if tokens.GT(stakeCap) && tokens.GT(previousTokens) {
		return errorsmod.Wrapf(types.ErrStakeCapExceeded,
			"validator %s tokens %s would exceed stake cap %s; capital above the cap can be committed to the validator's liquidity vault instead",
			valAddr, tokens, stakeCap)
	}

	return nil
}
