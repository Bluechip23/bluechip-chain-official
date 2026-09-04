package keeper

import (
	"context"
	"encoding/json"
	"fmt"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"bluechipChain/x/liquidityvault/types"
)

// moduleAccountAddress is the address holding all vault funds and pool
// positions at the contracts.
var moduleAccountAddress = authtypes.NewModuleAddress(types.ModuleName)

// maxMintTruncationLossDivisor bounds the value a share mint may lose to
// truncation: loss * divisor must not exceed the allocation's value (i.e.
// at most 0.1%). See the donation-attack guard in AllocateToPool.
var maxMintTruncationLossDivisor = math.NewInt(1000)

// PoolPositionValue queries a pool contract for the current value of the
// module's aggregate position, in the bond denom.
func (k Keeper) PoolPositionValue(ctx context.Context, pool types.RegisteredPool) (math.Int, error) {
	wasmKeeper, err := k.wasmKeeper()
	if err != nil {
		return math.Int{}, err
	}

	contractAddr, err := sdk.AccAddressFromBech32(pool.ContractAddress)
	if err != nil {
		return math.Int{}, err
	}

	query := types.PositionValueQuery{}
	query.PositionValue.Address = moduleAccountAddress.String()
	bz, err := json.Marshal(query)
	if err != nil {
		return math.Int{}, err
	}

	respBz, err := wasmKeeper.QuerySmart(ctx, contractAddr, bz)
	if err != nil {
		return math.Int{}, errorsmod.Wrapf(types.ErrPoolValueUnavailable, "pool %d: %v", pool.PoolId, err)
	}

	var resp types.PositionValueResponse
	if err := json.Unmarshal(respBz, &resp); err != nil {
		return math.Int{}, errorsmod.Wrapf(types.ErrPoolValueUnavailable, "pool %d: malformed response: %v", pool.PoolId, err)
	}
	value, ok := math.NewIntFromString(resp.Value)
	if !ok || value.IsNegative() {
		return math.Int{}, errorsmod.Wrapf(types.ErrPoolValueUnavailable, "pool %d: invalid value %q", pool.PoolId, resp.Value)
	}

	return value, nil
}

// positionValueFromShares converts internal shares to bond-denom value at
// the pool's current price: shares/totalShares of the module's position
// value.
func positionValueFromShares(shares, totalShares, poolValue math.Int) math.Int {
	if totalShares.IsZero() || shares.IsZero() {
		return math.ZeroInt()
	}
	return poolValue.Mul(shares).Quo(totalShares)
}

// ValidatorPositionsValue returns the total current value of a validator's
// pool positions in the bond denom. Shares queued for deallocation still
// count: they remain in their pools, earning, until the deallocation
// executes.
func (k Keeper) ValidatorPositionsValue(ctx context.Context, valAddr sdk.ValAddress) (math.Int, error) {
	total := math.ZeroInt()
	for _, position := range k.GetValidatorPositions(ctx, valAddr) {
		pool, found := k.GetPool(ctx, position.PoolId)
		if !found {
			return math.Int{}, errorsmod.Wrapf(types.ErrPoolNotFound, "position references pool %d", position.PoolId)
		}
		poolValue, err := k.PoolPositionValue(ctx, pool)
		if err != nil {
			return math.Int{}, err
		}
		total = total.Add(positionValueFromShares(position.Shares, k.GetPoolTotalShares(ctx, position.PoolId), poolValue))
	}
	return total, nil
}

// AllocateToPool moves amount from the validator's active vault balance into
// a registered pool, minting internal shares for the validator. Returns the
// minted shares.
func (k Keeper) AllocateToPool(ctx context.Context, valAddr sdk.ValAddress, poolID uint64, amount sdk.Coin) (math.Int, error) {
	vault, found := k.GetVault(ctx, valAddr)
	if !found {
		return math.Int{}, errorsmod.Wrap(types.ErrVaultNotFound, valAddr.String())
	}

	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return math.Int{}, err
	}
	if amount.Denom != bondDenom {
		return math.Int{}, errorsmod.Wrapf(types.ErrInvalidAllocation, "allocation must be in the bond denom %s, got %s", bondDenom, amount.Denom)
	}
	if vault.Balance.LT(amount.Amount) {
		return math.Int{}, errorsmod.Wrapf(types.ErrInvalidAllocation, "vault balance %s, requested %s", vault.Balance, amount.Amount)
	}

	pool, found := k.GetPool(ctx, poolID)
	if !found {
		return math.Int{}, errorsmod.Wrapf(types.ErrPoolNotFound, "pool %d", poolID)
	}
	if !pool.Enabled {
		return math.Int{}, errorsmod.Wrapf(types.ErrPoolDisabled, "pool %d", poolID)
	}

	totalShares := k.GetPoolTotalShares(ctx, poolID)

	// Value the position before and after the deposit and mint shares from
	// the MEASURED value delta, not the nominal amount: any slippage the
	// pool's internal swap/zap costs is charged to the allocator instead of
	// being socialized onto existing shareholders.
	valueBefore, err := k.PoolPositionValue(ctx, pool)
	if err != nil {
		return math.Int{}, err
	}
	if !totalShares.IsZero() && valueBefore.IsZero() {
		return math.Int{}, errorsmod.Wrapf(types.ErrPoolValueUnavailable, "pool %d reports zero position value with %s shares outstanding", poolID, totalShares)
	}

	wasmKeeper, err := k.wasmKeeper()
	if err != nil {
		return math.Int{}, err
	}
	contractAddr, err := sdk.AccAddressFromBech32(pool.ContractAddress)
	if err != nil {
		return math.Int{}, err
	}
	msgBz, err := json.Marshal(types.ProvideLiquidityMsg{})
	if err != nil {
		return math.Int{}, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if _, err := wasmKeeper.Execute(sdkCtx, contractAddr, moduleAccountAddress, msgBz, sdk.NewCoins(amount)); err != nil {
		return math.Int{}, errorsmod.Wrapf(types.ErrInvalidAllocation, "pool %d rejected the deposit: %v", poolID, err)
	}

	valueAfter, err := k.PoolPositionValue(ctx, pool)
	if err != nil {
		return math.Int{}, err
	}
	// Keep the fallback cache fresh: a pool that goes dark before the
	// vault's first value post should fall back to this observation, not
	// zero.
	if err := k.setCachedPoolValue(ctx, poolID, valueAfter); err != nil {
		return math.Int{}, err
	}
	delta := valueAfter.Sub(valueBefore)
	if !delta.IsPositive() {
		return math.Int{}, errorsmod.Wrapf(types.ErrInvalidAllocation, "pool %d position value did not increase (before %s, after %s)", poolID, valueBefore, valueAfter)
	}

	var shares math.Int
	if totalShares.IsZero() {
		shares = delta
	} else {
		// Guard against the ERC-4626 donation/inflation attack: an attacker
		// who takes a dust first allocation and then donates into the pool
		// position inflates the share price so that later allocators lose
		// up to one share's price to mint truncation — silently transferred
		// to existing shareholders. Rejecting any mint whose truncation
		// loss exceeds 0.1% of the allocation's measured value turns the
		// victim's loss into a harmless tx failure. The attacker gains
		// nothing (their donation is recoverable but transfers no victim
		// funds); the residual is a griefing vector — an inflated share
		// price rejects small allocations — which governance ends by
		// disabling the pool.
		numerator := totalShares.Mul(delta)
		shares = numerator.Quo(valueBefore)
		truncationLoss := numerator.Mod(valueBefore)
		if truncationLoss.Mul(maxMintTruncationLossDivisor).GT(numerator) {
			return math.Int{}, errorsmod.Wrapf(types.ErrInvalidAllocation,
				"pool %d share price is too high for this allocation: mint truncation would cost more than 0.1%% of the allocation's measured value; allocate a larger amount", poolID)
		}
	}
	if shares.IsZero() {
		return math.Int{}, errorsmod.Wrapf(types.ErrInvalidAllocation, "allocation of %s is too small to mint shares", amount)
	}

	vault.Balance = vault.Balance.Sub(amount.Amount)
	if err := k.SetVault(ctx, vault); err != nil {
		return math.Int{}, err
	}

	position, found := k.GetPosition(ctx, valAddr, poolID)
	if !found {
		position = types.PoolPosition{
			ValidatorAddress: vault.ValidatorAddress,
			PoolId:           poolID,
			Shares:           math.ZeroInt(),
		}
	}
	position.Shares = position.Shares.Add(shares)
	if err := k.setPosition(ctx, position); err != nil {
		return math.Int{}, err
	}
	if err := k.setPoolTotalShares(ctx, poolID, totalShares.Add(shares)); err != nil {
		return math.Int{}, err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeAllocation,
			sdk.NewAttribute(types.AttributeKeyValidator, vault.ValidatorAddress),
			sdk.NewAttribute(types.AttributeKeyPoolID, fmtPoolID(poolID)),
			sdk.NewAttribute(types.AttributeKeyAmount, amount.String()),
			sdk.NewAttribute(types.AttributeKeyShares, shares.String()),
		),
	)

	return shares, nil
}

// DeallocateFromPool queues the removal of shares from a validator's pool
// position. The shares keep earning (and counting toward the composite
// score) until the universal deallocation grace period ends; the proceeds
// then go to the validator's own account, leaving the vault. Returns the
// queued deallocation.
func (k Keeper) DeallocateFromPool(ctx context.Context, valAddr sdk.ValAddress, poolID uint64, shares math.Int) (types.PendingDeallocation, error) {
	position, found := k.GetPosition(ctx, valAddr, poolID)
	if !found {
		return types.PendingDeallocation{}, errorsmod.Wrapf(types.ErrInsufficientShares, "validator %s has no position in pool %d", valAddr, poolID)
	}

	pending := k.GetPendingShares(ctx, valAddr, poolID)
	available := position.Shares.Sub(pending)
	if available.LT(shares) {
		return types.PendingDeallocation{}, errorsmod.Wrapf(types.ErrInsufficientShares,
			"position has %s shares with %s already queued; requested %s", position.Shares, pending, shares)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	deallocation := types.PendingDeallocation{
		ValidatorAddress: position.ValidatorAddress,
		PoolId:           poolID,
		Shares:           shares,
		CompleteTime:     sdkCtx.BlockTime().Add(k.GetParams(ctx).DeallocationGracePeriod),
	}
	if err := k.setPendingDeallocation(ctx, deallocation); err != nil {
		return types.PendingDeallocation{}, err
	}
	if err := k.setPendingShares(ctx, valAddr, poolID, pending.Add(shares)); err != nil {
		return types.PendingDeallocation{}, err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeDeallocationInitiated,
			sdk.NewAttribute(types.AttributeKeyValidator, position.ValidatorAddress),
			sdk.NewAttribute(types.AttributeKeyPoolID, fmtPoolID(poolID)),
			sdk.NewAttribute(types.AttributeKeyShares, shares.String()),
			sdk.NewAttribute(types.AttributeKeyCompleteTime, deallocation.CompleteTime.String()),
		),
	)

	return deallocation, nil
}

// executeWithdrawal calls withdraw_liquidity on the pool contract for the
// given fraction of the module's position and returns the bond-denom amount
// the contract sent back.
//
// The contract call — the only untrusted code — runs under its own bounded
// gas meter and a panic guard, so a misbehaving contract (infinite loop,
// out-of-gas, wasm panic) fails this call without taking down the end
// blocker; module bookkeeping around it deliberately stays outside the
// guard so genuine state corruption still surfaces.
//
// The ratio is truncated to 18 decimals, so a partial withdrawal can leave a
// few units of dust behind; the dust stays in the module's position,
// favoring the remaining shareholders. A full exit passes ratio 1.0 exactly
// and recovers everything.
func (k Keeper) executeWithdrawal(ctx sdk.Context, pool types.RegisteredPool, ratio math.LegacyDec, bondDenom string) (math.Int, error) {
	wasmKeeper, err := k.wasmKeeper()
	if err != nil {
		return math.Int{}, err
	}
	contractAddr, err := sdk.AccAddressFromBech32(pool.ContractAddress)
	if err != nil {
		return math.Int{}, err
	}

	msg := types.WithdrawLiquidityMsg{}
	msg.WithdrawLiquidity.Ratio = ratio.String()
	msgBz, err := json.Marshal(msg)
	if err != nil {
		return math.Int{}, err
	}

	balanceBefore := k.bankKeeper.GetAllBalances(ctx, moduleAccountAddress).AmountOf(bondDenom)

	if err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("pool %d withdrawal aborted: %v", pool.PoolId, r)
			}
		}()
		gasCtx := ctx.WithGasMeter(storetypes.NewGasMeter(contractCallGasLimit))
		_, err = wasmKeeper.Execute(gasCtx, contractAddr, moduleAccountAddress, msgBz, nil)
		return err
	}(); err != nil {
		return math.Int{}, fmt.Errorf("pool %d rejected the withdrawal: %w", pool.PoolId, err)
	}

	balanceAfter := k.bankKeeper.GetAllBalances(ctx, moduleAccountAddress).AmountOf(bondDenom)
	received := balanceAfter.Sub(balanceBefore)
	if received.IsNegative() {
		return math.Int{}, fmt.Errorf("pool %d withdrawal reduced the module balance", pool.PoolId)
	}
	return received, nil
}
