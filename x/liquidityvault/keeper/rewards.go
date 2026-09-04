package keeper

import (
	"context"
	"encoding/json"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// Vault reward accounting (F1-lite).
//
// Each vault carries a cumulative RewardIndex: every reward collection adds
// delegator_share / validator_tokens to it. A delegator's entitlement for a
// window is (index_now - index_at_last_settlement) * their_stake, settled
// lazily: the delegation hooks close the window BEFORE any stake change, so
// no loop over delegators ever runs on-chain.
//
// All roundings favor the module: index increments truncate, settlements
// truncate, and claims pay the floor of the accrual (the fraction stays
// recorded). Slashing shrinks stake without a hook, which only ever lowers
// a settlement below the collected amount — the module can never owe more
// than it holds (enforced by the module-account invariant).

// GetDelegatorReward returns a delegator's reward record for a validator.
func (k Keeper) GetDelegatorReward(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) (types.DelegatorReward, bool) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := store.Get(types.DelegatorRewardKey(delAddr, valAddr))
	if bz == nil {
		return types.DelegatorReward{
			DelegatorAddress: delAddr.String(),
			ValidatorAddress: valAddr.String(),
			Index:            math.LegacyZeroDec(),
			Accrued:          math.LegacyZeroDec(),
		}, false
	}

	var reward types.DelegatorReward
	k.cdc.MustUnmarshal(bz, &reward)
	return reward, true
}

// SetDelegatorReward stores a delegator's reward record. Records are never
// deleted: a missing record defaults to index zero, which is only correct
// for delegations that predate the vault's first collection — deleting a
// record with a non-zero index would let the delegator re-accrue from zero.
func (k Keeper) SetDelegatorReward(ctx context.Context, reward types.DelegatorReward) error {
	delAddr, err := sdk.AccAddressFromBech32(reward.DelegatorAddress)
	if err != nil {
		return err
	}
	valAddr, err := sdk.ValAddressFromBech32(reward.ValidatorAddress)
	if err != nil {
		return err
	}

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz, err := k.cdc.Marshal(&reward)
	if err != nil {
		return err
	}
	store.Set(types.DelegatorRewardKey(delAddr, valAddr), bz)
	return nil
}

// GetAllDelegatorRewards returns every delegator reward record.
func (k Keeper) GetAllDelegatorRewards(ctx context.Context) []types.DelegatorReward {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iterator := storetypes.KVStorePrefixIterator(store, types.DelegatorRewardKeyPrefix)
	defer iterator.Close()

	var rewards []types.DelegatorReward
	for ; iterator.Valid(); iterator.Next() {
		var reward types.DelegatorReward
		k.cdc.MustUnmarshal(iterator.Value(), &reward)
		rewards = append(rewards, reward)
	}
	return rewards
}

// delegationTokens values a delegation's shares at the validator's current
// exchange rate; zero if either side is missing.
func (k Keeper) delegationTokens(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) math.LegacyDec {
	delegation, err := k.stakingKeeper.GetDelegation(ctx, delAddr, valAddr)
	if err != nil {
		return math.LegacyZeroDec()
	}
	validator, err := k.stakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		return math.LegacyZeroDec()
	}
	return validator.TokensFromShares(delegation.Shares)
}

// settleDelegatorReward closes the delegator's accrual window at the
// vault's current index: accrued += (index - last_index) * stake. Called by
// the delegation hooks BEFORE stake changes, and by claims/queries. A
// missing delegation settles nothing but still fast-forwards the index.
func (k Keeper) settleDelegatorReward(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) (types.DelegatorReward, error) {
	reward, found := k.GetDelegatorReward(ctx, delAddr, valAddr)

	vault, found := k.GetVault(ctx, valAddr)
	if !found {
		return reward, nil
	}

	indexUnchanged := reward.Index.Equal(vault.RewardIndex)
	if vault.RewardIndex.GT(reward.Index) {
		stake := k.delegationTokens(ctx, delAddr, valAddr)
		if stake.IsPositive() {
			reward.Accrued = reward.Accrued.Add(vault.RewardIndex.Sub(reward.Index).Mul(stake))
		}
	}
	reward.Index = vault.RewardIndex

	// Skip no-op writes: an existing record that didn't move, or a missing
	// record identical to the missing-key default (index and accrual both
	// zero). Never skip persisting a non-zero index pin — a missing record
	// defaults to index zero and would re-accrue from the beginning.
	if indexUnchanged && (found || vault.RewardIndex.IsZero()) {
		return reward, nil
	}

	if err := k.SetDelegatorReward(ctx, reward); err != nil {
		return types.DelegatorReward{}, err
	}
	return reward, nil
}

// CollectPoolRewards pulls the accrued fees for the module's position in a
// pool and distributes them across the pool's validators pro rata by
// internal shares; each validator's cut is split by its vault's delegator
// reward share. The triggering validator must hold a position in the pool.
// Returns the total collected amount.
func (k Keeper) CollectPoolRewards(ctx context.Context, valAddr sdk.ValAddress, poolID uint64) (math.Int, error) {
	if _, found := k.GetPosition(ctx, valAddr, poolID); !found {
		return math.Int{}, errorsmod.Wrapf(types.ErrInsufficientShares, "validator %s has no position in pool %d", valAddr, poolID)
	}
	pool, found := k.GetPool(ctx, poolID)
	if !found {
		return math.Int{}, errorsmod.Wrapf(types.ErrPoolNotFound, "pool %d", poolID)
	}
	totalShares := k.GetPoolTotalShares(ctx, poolID)
	if totalShares.IsZero() {
		return math.ZeroInt(), nil
	}

	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return math.Int{}, err
	}

	wasmKeeper, err := k.wasmKeeper()
	if err != nil {
		return math.Int{}, err
	}
	contractAddr, err := sdk.AccAddressFromBech32(pool.ContractAddress)
	if err != nil {
		return math.Int{}, err
	}
	msgBz, err := json.Marshal(types.CollectRewardsMsg{})
	if err != nil {
		return math.Int{}, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	balanceBefore := k.bankKeeper.GetAllBalances(ctx, moduleAccountAddress).AmountOf(bondDenom)
	if _, err := wasmKeeper.Execute(sdkCtx, contractAddr, moduleAccountAddress, msgBz, nil); err != nil {
		return math.Int{}, errorsmod.Wrapf(types.ErrPoolValueUnavailable, "pool %d rejected the reward collection: %v", poolID, err)
	}
	balanceAfter := k.bankKeeper.GetAllBalances(ctx, moduleAccountAddress).AmountOf(bondDenom)
	collected := balanceAfter.Sub(balanceBefore)
	if collected.IsNegative() {
		return math.Int{}, errorsmod.Wrapf(types.ErrPoolValueUnavailable, "pool %d reward collection reduced the module balance", poolID)
	}
	if collected.IsZero() {
		return math.ZeroInt(), nil
	}

	// Distribute pro rata by shares. Truncation dust from the per-validator
	// split stays in the module account (covered by the >= invariant).
	for _, position := range k.GetAllPositions(ctx) {
		if position.PoolId != poolID {
			continue
		}
		validatorCut := collected.Mul(position.Shares).Quo(totalShares)
		if validatorCut.IsZero() {
			continue
		}
		positionValAddr, err := sdk.ValAddressFromBech32(position.ValidatorAddress)
		if err != nil {
			return math.Int{}, err
		}
		if err := k.distributeVaultReward(ctx, positionValAddr, validatorCut, bondDenom); err != nil {
			return math.Int{}, err
		}
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeRewardsCollected,
			sdk.NewAttribute(types.AttributeKeyPoolID, fmtPoolID(poolID)),
			sdk.NewAttribute(types.AttributeKeyAmount, collected.String()),
		),
	)

	return collected, nil
}

// distributeVaultReward splits one validator's reward cut by its vault's
// delegator reward share: the validator's part is paid to the operator
// account immediately, the delegators' part raises the vault's reward index
// (and stays in the module account until claimed). The whole cut goes to
// the validator when there is nothing to distribute to: no vault record
// (positions imply one, but a fabricated default must not divert funds),
// zero staked tokens (a pure liquidity validator has no delegators), or a
// cut too small to move the 18-decimal index (which would lock the funds
// as forever-unclaimable outstanding rewards).
func (k Keeper) distributeVaultReward(ctx context.Context, valAddr sdk.ValAddress, amount math.Int, bondDenom string) error {
	vault, found := k.GetVault(ctx, valAddr)

	tokens := math.ZeroInt()
	if validator, err := k.stakingKeeper.GetValidator(ctx, valAddr); err == nil {
		tokens = validator.GetTokens()
	}

	delegatorCut := math.ZeroInt()
	indexIncrement := math.LegacyZeroDec()
	if found && tokens.IsPositive() {
		delegatorCut = vault.DelegatorRewardShare.MulInt(amount).TruncateInt()
		indexIncrement = math.LegacyNewDecFromInt(delegatorCut).QuoInt(tokens)
		if indexIncrement.IsZero() {
			delegatorCut = math.ZeroInt()
		}
	}
	validatorCut := amount.Sub(delegatorCut)

	if validatorCut.IsPositive() {
		coins := sdk.NewCoins(sdk.NewCoin(bondDenom, validatorCut))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sdk.AccAddress(valAddr), coins); err != nil {
			return err
		}
	}

	if delegatorCut.IsPositive() {
		vault.RewardIndex = vault.RewardIndex.Add(indexIncrement)
		vault.OutstandingRewards = vault.OutstandingRewards.Add(math.LegacyNewDecFromInt(delegatorCut))
		if err := k.SetVault(ctx, vault); err != nil {
			return err
		}
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeRewardsDistributed,
			sdk.NewAttribute(types.AttributeKeyValidator, vault.ValidatorAddress),
			sdk.NewAttribute(types.AttributeKeyAmount, amount.String()),
			sdk.NewAttribute(types.AttributeKeyDelegatorCut, delegatorCut.String()),
		),
	)

	return nil
}

// ClaimVaultRewards settles and pays out a delegator's accrued vault
// rewards for one validator, returning the paid coin. The fractional
// remainder stays recorded for future claims.
func (k Keeper) ClaimVaultRewards(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) (sdk.Coin, error) {
	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return sdk.Coin{}, err
	}

	reward, err := k.settleDelegatorReward(ctx, delAddr, valAddr)
	if err != nil {
		return sdk.Coin{}, err
	}

	payout := reward.Accrued.TruncateInt()
	if !payout.IsPositive() {
		return sdk.NewCoin(bondDenom, math.ZeroInt()), nil
	}

	coins := sdk.NewCoins(sdk.NewCoin(bondDenom, payout))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, delAddr, coins); err != nil {
		return sdk.Coin{}, err
	}

	reward.Accrued = reward.Accrued.Sub(math.LegacyNewDecFromInt(payout))
	if err := k.SetDelegatorReward(ctx, reward); err != nil {
		return sdk.Coin{}, err
	}

	if vault, found := k.GetVault(ctx, valAddr); found {
		vault.OutstandingRewards = math.LegacyMaxDec(vault.OutstandingRewards.Sub(math.LegacyNewDecFromInt(payout)), math.LegacyZeroDec())
		if err := k.SetVault(ctx, vault); err != nil {
			return sdk.Coin{}, err
		}
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeRewardsClaimed,
			sdk.NewAttribute(types.AttributeKeyValidator, valAddr.String()),
			sdk.NewAttribute(types.AttributeKeyDelegator, delAddr.String()),
			sdk.NewAttribute(types.AttributeKeyAmount, payout.String()),
		),
	)

	return sdk.NewCoin(bondDenom, payout), nil
}

// ClaimableReward returns what a claim would pay right now: the settled
// accrual plus the unsettled projection at the current index, truncated.
// Read-only.
func (k Keeper) ClaimableReward(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) math.Int {
	reward, _ := k.GetDelegatorReward(ctx, delAddr, valAddr)
	claimable := reward.Accrued

	if vault, found := k.GetVault(ctx, valAddr); found && vault.RewardIndex.GT(reward.Index) {
		stake := k.delegationTokens(ctx, delAddr, valAddr)
		if stake.IsPositive() {
			claimable = claimable.Add(vault.RewardIndex.Sub(reward.Index).Mul(stake))
		}
	}

	return claimable.TruncateInt()
}
