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

	vault, found := k.GetVault(ctx, valAddr)
	if !found {
		vault = types.NewVault(valAddr)
	}
	vault.Balance = vault.Balance.Add(amount.Amount)
	if err := k.SetVault(ctx, vault); err != nil {
		return err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
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

	vault, found := k.GetVault(ctx, valAddr)
	if !found {
		vault = types.NewVault(valAddr)
	}
	vault.DelegatorRewardShare = share
	if err := k.SetVault(ctx, vault); err != nil {
		return err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeSetRewardShare,
			sdk.NewAttribute(types.AttributeKeyValidator, vault.ValidatorAddress),
			sdk.NewAttribute(types.AttributeKeyDelegatorRewardShare, share.String()),
		),
	)

	return nil
}

// GetCompositeScore returns a validator's composite score components: the
// tokens delegated directly to it on the chain, its active vault balance, and
// their sum. Pending withdrawals do not count.
func (k Keeper) GetCompositeScore(ctx context.Context, valAddr sdk.ValAddress) (stakedTokens, vaultBalance, compositeScore math.Int, err error) {
	validator, err := k.stakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		return math.Int{}, math.Int{}, math.Int{}, errorsmod.Wrap(types.ErrValidatorNotFound, valAddr.String())
	}

	stakedTokens = validator.GetTokens()

	vaultBalance = math.ZeroInt()
	if vault, found := k.GetVault(ctx, valAddr); found {
		vaultBalance = vault.Balance
	}

	return stakedTokens, vaultBalance, stakedTokens.Add(vaultBalance), nil
}
