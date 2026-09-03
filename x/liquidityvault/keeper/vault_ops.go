package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// Deposit moves amount from the validator's own account into its vault. The
// validator must exist and the coin must be in the chain's bond denom.
func (k Keeper) Deposit(ctx context.Context, valAddr sdk.ValAddress, amount sdk.Coin) error {
	if _, err := k.stakingKeeper.GetValidator(ctx, valAddr); err != nil {
		return errorsmod.Wrap(types.ErrValidatorNotFound, valAddr.String())
	}

	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return err
	}
	if amount.Denom != bondDenom {
		return errorsmod.Wrapf(types.ErrInvalidDeposit, "deposit must be in the bond denom %s, got %s", bondDenom, amount.Denom)
	}

	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, sdk.AccAddress(valAddr), types.ModuleName, sdk.NewCoins(amount)); err != nil {
		return err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	vault, found := k.GetVault(ctx, valAddr)
	if !found {
		vault = types.NewVault(valAddr)
		// A new vault starts its value-post cadence.
		if err := k.scheduleNextValuePost(sdkCtx, valAddr); err != nil {
			return err
		}
	}
	vault.Balance = vault.Balance.Add(amount.Amount)
	if err := k.SetVault(ctx, vault); err != nil {
		return err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeDeposit,
			sdk.NewAttribute(types.AttributeKeyValidator, vault.ValidatorAddress),
			sdk.NewAttribute(types.AttributeKeyAmount, amount.String()),
		),
	)

	return nil
}

// InitiateWithdrawal removes amount from the vault's active balance (so it
// stops counting toward the composite score immediately) and queues it for
// release after the universal grace period. It returns the completion time.
func (k Keeper) InitiateWithdrawal(ctx context.Context, valAddr sdk.ValAddress, amount sdk.Coin) (types.PendingWithdrawal, error) {
	vault, found := k.GetVault(ctx, valAddr)
	if !found {
		return types.PendingWithdrawal{}, errorsmod.Wrap(types.ErrVaultNotFound, valAddr.String())
	}

	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return types.PendingWithdrawal{}, err
	}
	if amount.Denom != bondDenom {
		return types.PendingWithdrawal{}, errorsmod.Wrapf(types.ErrInvalidWithdrawal, "withdrawal must be in the bond denom %s, got %s", bondDenom, amount.Denom)
	}
	if vault.Balance.LT(amount.Amount) {
		return types.PendingWithdrawal{}, errorsmod.Wrapf(types.ErrInsufficientVaultBalance, "vault balance %s, requested %s", vault.Balance, amount.Amount)
	}

	vault.Balance = vault.Balance.Sub(amount.Amount)
	if err := k.SetVault(ctx, vault); err != nil {
		return types.PendingWithdrawal{}, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	withdrawal := types.PendingWithdrawal{
		ValidatorAddress: vault.ValidatorAddress,
		Amount:           amount.Amount,
		CompleteTime:     sdkCtx.BlockTime().Add(k.GetParams(ctx).WithdrawalGracePeriod),
	}
	if err := k.SetPendingWithdrawal(ctx, withdrawal); err != nil {
		return types.PendingWithdrawal{}, err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeWithdrawalInitiated,
			sdk.NewAttribute(types.AttributeKeyValidator, vault.ValidatorAddress),
			sdk.NewAttribute(types.AttributeKeyAmount, amount.String()),
			sdk.NewAttribute(types.AttributeKeyCompleteTime, withdrawal.CompleteTime.String()),
		),
	)

	return withdrawal, nil
}

// SetRewardShare sets the fraction of vault liquidity rewards passed through
// to the validator's delegators. The validator must exist; a vault is created
// on first use so the share can be set before the first deposit.
func (k Keeper) SetRewardShare(ctx context.Context, valAddr sdk.ValAddress, share math.LegacyDec) error {
	if err := types.ValidateRewardShare(share); err != nil {
		return err
	}
	if _, err := k.stakingKeeper.GetValidator(ctx, valAddr); err != nil {
		return errorsmod.Wrap(types.ErrValidatorNotFound, valAddr.String())
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	vault, found := k.GetVault(ctx, valAddr)
	if !found {
		vault = types.NewVault(valAddr)
		// A new vault starts its value-post cadence.
		if err := k.scheduleNextValuePost(sdkCtx, valAddr); err != nil {
			return err
		}
	}
	vault.DelegatorRewardShare = share
	if err := k.SetVault(ctx, vault); err != nil {
		return err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeSetRewardShare,
			sdk.NewAttribute(types.AttributeKeyValidator, vault.ValidatorAddress),
			sdk.NewAttribute(types.AttributeKeyDelegatorRewardShare, share.String()),
		),
	)

	return nil
}

// CompositeScore bundles a validator's score components.
type CompositeScore struct {
	// StakedTokens are the tokens delegated directly to the validator.
	StakedTokens math.Int
	// VaultBalance is the live active vault balance.
	VaultBalance math.Int
	// PositionValue is the live value of the validator's pool positions.
	PositionValue math.Int
	// MedianVaultValue is the median of the vault's value posts; before the
	// first post it falls back to the live vault value (balance +
	// positions).
	MedianVaultValue math.Int
	// Score is StakedTokens + MedianVaultValue — the composite score per
	// the LPV design document, with the median damping value swings.
	Score math.Int
}

// GetCompositeScore returns a validator's composite score. The vault
// component is the median of the vault's value posts (live value before the
// first post). Pending vault withdrawals do not count; pending pool
// deallocations still do (the liquidity is in the pool until the
// deallocation executes). Dark pools degrade to their last cached value —
// the score stays computable in exactly the broken-pool scenario the
// median exists to survive.
func (k Keeper) GetCompositeScore(ctx context.Context, valAddr sdk.ValAddress) (CompositeScore, error) {
	validator, err := k.stakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		return CompositeScore{}, errorsmod.Wrap(types.ErrValidatorNotFound, valAddr.String())
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	score := CompositeScore{
		StakedTokens: validator.GetTokens(),
		VaultBalance: math.ZeroInt(),
	}
	if vault, found := k.GetVault(ctx, valAddr); found {
		score.VaultBalance = vault.Balance
	}

	score.PositionValue, err = k.positionsValueWithFallback(sdkCtx, valAddr, false)
	if err != nil {
		return CompositeScore{}, err
	}

	score.MedianVaultValue = k.medianVaultValueOrLive(sdkCtx, valAddr, score.VaultBalance, score.PositionValue)
	score.Score = score.StakedTokens.Add(score.MedianVaultValue)
	return score, nil
}

// medianVaultValueOrLive is the single definition of the composite score's
// vault component: the median of the validator's value posts, or the live
// vault value (balance + positions) before the first post.
func (k Keeper) medianVaultValueOrLive(ctx sdk.Context, valAddr sdk.ValAddress, vaultBalance, positionValue math.Int) math.Int {
	if history := k.GetValuePostHistory(ctx, valAddr); len(history.Posts) > 0 {
		return MedianOfPosts(history.Posts)
	}
	return vaultBalance.Add(positionValue)
}
