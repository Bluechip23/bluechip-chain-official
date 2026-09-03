package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	keepertest "bluechipChain/testutil/keeper"
	"bluechipChain/x/liquidityvault/keeper"
	"bluechipChain/x/liquidityvault/types"
)

var (
	poolContract = sdk.AccAddress([]byte("pool-contract-1-----"))
	val2Addr     = sdk.ValAddress([]byte("validator-operator-2"))
	val2AccAddr  = sdk.AccAddress(val2Addr)
)

// setupPool registers a mock pool contract and funds the validator's vault.
func setupPool(t *testing.T) (keeper.Keeper, sdk.Context, *keepertest.MockStakingKeeper, *keepertest.MockBankKeeper, *keepertest.MockWasmKeeper, uint64) {
	t.Helper()
	k, ctx, stakingKeeper, bankKeeper, wasmKeeper := keepertest.LiquidityvaultKeeper(t)
	ctx = ctx.WithBlockTime(time.Now().UTC())

	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1_000))
	bankKeeper.SetBalance(valAccAddr, sdk.NewCoins(coin(1_000)))
	require.NoError(t, k.Deposit(ctx, valAddr, coin(1_000)))

	wasmKeeper.AddPool(poolContract)
	poolID, err := k.RegisterPool(ctx, poolContract.String(), "creator pool one")
	require.NoError(t, err)

	return k, ctx, stakingKeeper, bankKeeper, wasmKeeper, poolID
}

func TestRegisterPool(t *testing.T) {
	k, ctx, _, _, wasmKeeper := keepertest.LiquidityvaultKeeper(t)

	// Contract must exist.
	_, err := k.RegisterPool(ctx, poolContract.String(), "nope")
	require.ErrorIs(t, err, types.ErrPoolNotFound)

	wasmKeeper.AddPool(poolContract)
	poolID, err := k.RegisterPool(ctx, poolContract.String(), "creator pool one")
	require.NoError(t, err)
	require.Equal(t, uint64(1), poolID)

	pool, found := k.GetPool(ctx, poolID)
	require.True(t, found)
	require.True(t, pool.Enabled)
	require.Equal(t, poolContract.String(), pool.ContractAddress)

	// Duplicate contract is rejected.
	_, err = k.RegisterPool(ctx, poolContract.String(), "again")
	require.ErrorIs(t, err, types.ErrPoolAlreadyRegistered)

	// Ids increment.
	otherContract := sdk.AccAddress([]byte("pool-contract-2-----"))
	wasmKeeper.AddPool(otherContract)
	otherID, err := k.RegisterPool(ctx, otherContract.String(), "creator pool two")
	require.NoError(t, err)
	require.Equal(t, uint64(2), otherID)

	// Disable stops allocations only.
	require.NoError(t, k.SetPoolEnabled(ctx, poolID, false))
	pool, _ = k.GetPool(ctx, poolID)
	require.False(t, pool.Enabled)
	require.ErrorIs(t, k.SetPoolEnabled(ctx, 99, true), types.ErrPoolNotFound)
}

func TestPoolMsgAuthority(t *testing.T) {
	k, ctx, _, _, wasmKeeper := keepertest.LiquidityvaultKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	wasmKeeper.AddPool(poolContract)

	_, err := ms.RegisterPool(ctx, &types.MsgRegisterPool{
		Authority:       valAccAddr.String(),
		ContractAddress: poolContract.String(),
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	resp, err := ms.RegisterPool(ctx, &types.MsgRegisterPool{
		Authority:       k.GetAuthority(),
		ContractAddress: poolContract.String(),
		Description:     "creator pool one",
	})
	require.NoError(t, err)

	_, err = ms.SetPoolEnabled(ctx, &types.MsgSetPoolEnabled{
		Authority: valAccAddr.String(),
		PoolId:    resp.PoolId,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	_, err = ms.SetPoolEnabled(ctx, &types.MsgSetPoolEnabled{
		Authority: k.GetAuthority(),
		PoolId:    resp.PoolId,
		Enabled:   false,
	})
	require.NoError(t, err)
}

func TestAllocateToPool(t *testing.T) {
	k, ctx, _, bankKeeper, wasmKeeper, poolID := setupPool(t)

	// First allocation mints shares 1:1 and moves funds to the contract.
	shares, err := k.AllocateToPool(ctx, valAddr, poolID, coin(400))
	require.NoError(t, err)
	require.Equal(t, math.NewInt(400), shares)

	vault, _ := k.GetVault(ctx, valAddr)
	require.Equal(t, math.NewInt(600), vault.Balance)
	require.Equal(t, math.NewInt(400), k.GetPoolTotalShares(ctx, poolID))
	require.Equal(t, sdk.NewCoins(coin(600)), bankKeeper.GetAllBalances(ctx, moduleAddr))
	require.Equal(t, sdk.NewCoins(coin(400)), bankKeeper.GetAllBalances(ctx, poolContract))

	// Pool value doubles; a second allocation mints pro-rata shares:
	// 400 * 400 / 800 = 200.
	wasmKeeper.SetPoolValue(poolContract, math.NewInt(800))
	shares, err = k.AllocateToPool(ctx, valAddr, poolID, coin(400))
	require.NoError(t, err)
	require.Equal(t, math.NewInt(200), shares)

	position, found := k.GetPosition(ctx, valAddr, poolID)
	require.True(t, found)
	require.Equal(t, math.NewInt(600), position.Shares)
	require.Equal(t, math.NewInt(600), k.GetPoolTotalShares(ctx, poolID))
}

func TestAllocateToPoolRejections(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, wasmKeeper, poolID := setupPool(t)

	// Unknown pool.
	_, err := k.AllocateToPool(ctx, valAddr, 99, coin(10))
	require.ErrorIs(t, err, types.ErrPoolNotFound)

	// Disabled pool.
	require.NoError(t, k.SetPoolEnabled(ctx, poolID, false))
	_, err = k.AllocateToPool(ctx, valAddr, poolID, coin(10))
	require.ErrorIs(t, err, types.ErrPoolDisabled)
	require.NoError(t, k.SetPoolEnabled(ctx, poolID, true))

	// No vault.
	stakingKeeper.SetValidatorTokens(val2Addr, math.NewInt(1))
	_, err = k.AllocateToPool(ctx, val2Addr, poolID, coin(10))
	require.ErrorIs(t, err, types.ErrVaultNotFound)

	// Wrong denom.
	_, err = k.AllocateToPool(ctx, valAddr, poolID, sdk.NewInt64Coin("other", 10))
	require.ErrorIs(t, err, types.ErrInvalidAllocation)

	// More than the active balance.
	_, err = k.AllocateToPool(ctx, valAddr, poolID, coin(2_000))
	require.ErrorIs(t, err, types.ErrInvalidAllocation)

	// Contract failure aborts and changes nothing.
	wasmKeeper.Pools[poolContract.String()].FailExecute = true
	_, err = k.AllocateToPool(ctx, valAddr, poolID, coin(10))
	require.ErrorIs(t, err, types.ErrInvalidAllocation)
	vault, _ := k.GetVault(ctx, valAddr)
	require.Equal(t, math.NewInt(1_000), vault.Balance)
	require.True(t, k.GetPoolTotalShares(ctx, poolID).IsZero())
	require.Equal(t, sdk.NewCoins(coin(1_000)), bankKeeper.GetAllBalances(ctx, moduleAddr))
}

func TestAllocationTooSmallRejected(t *testing.T) {
	// A deposit too small to mint a single share must be rejected, not
	// silently absorbed — this is also the guard against the classic
	// share-inflation attack (donating to the pool to push the share price
	// above victims' deposits burns the victims nothing; their txs fail).
	k, ctx, _, _, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)

	wasmKeeper.SetPoolValue(poolContract, math.NewInt(1_000_000))
	_, err = k.AllocateToPool(ctx, valAddr, poolID, coin(1))
	require.ErrorIs(t, err, types.ErrInvalidAllocation)

	// Nothing changed for the failed allocation.
	vault, _ := k.GetVault(ctx, valAddr)
	require.Equal(t, math.NewInt(400), vault.Balance)
	require.Equal(t, math.NewInt(600), k.GetPoolTotalShares(ctx, poolID))
}

func TestDeallocateFromPool(t *testing.T) {
	k, ctx, _, _, _, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)

	// No position in an unknown pool.
	_, err = k.DeallocateFromPool(ctx, valAddr, 99, math.NewInt(1))
	require.ErrorIs(t, err, types.ErrInsufficientShares)

	deallocation, err := k.DeallocateFromPool(ctx, valAddr, poolID, math.NewInt(200))
	require.NoError(t, err)
	require.Equal(t, ctx.BlockTime().Add(types.DefaultDeallocationGracePeriod), deallocation.CompleteTime)
	require.Equal(t, math.NewInt(200), k.GetPendingShares(ctx, valAddr, poolID))

	// Queued shares are reserved: only the remainder can be queued again.
	_, err = k.DeallocateFromPool(ctx, valAddr, poolID, math.NewInt(500))
	require.ErrorIs(t, err, types.ErrInsufficientShares)

	// Same-block second request merges into one queue entry.
	_, err = k.DeallocateFromPool(ctx, valAddr, poolID, math.NewInt(100))
	require.NoError(t, err)
	queue := k.GetPendingDeallocations(ctx, valAddr)
	require.Len(t, queue, 1)
	require.Equal(t, math.NewInt(300), queue[0].Shares)
	require.Equal(t, math.NewInt(300), k.GetPendingShares(ctx, valAddr, poolID))

	// The position itself is untouched until execution.
	position, _ := k.GetPosition(ctx, valAddr, poolID)
	require.Equal(t, math.NewInt(600), position.Shares)
}

func TestProcessMaturedDeallocations(t *testing.T) {
	k, ctx, _, bankKeeper, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)

	// The pool gains value: 600 -> 900.
	wasmKeeper.SetPoolValue(poolContract, math.NewInt(900))

	_, err = k.DeallocateFromPool(ctx, valAddr, poolID, math.NewInt(200))
	require.NoError(t, err)

	// Not matured yet: nothing happens.
	almostDone := ctx.BlockTime().Add(types.DefaultDeallocationGracePeriod - time.Second)
	require.NoError(t, k.ProcessMaturedDeallocations(ctx.WithBlockTime(almostDone)))
	require.Len(t, k.GetPendingDeallocations(ctx, valAddr), 1)

	// Matured: 200/600 of the 900 position goes to the validator's own
	// account (leaving the vault). The 18-decimal ratio truncation leaves
	// one unit of dust in the pool (299, not 300), favoring the remaining
	// shareholders; a later full exit recovers it.
	done := ctx.BlockTime().Add(types.DefaultDeallocationGracePeriod)
	require.NoError(t, k.ProcessMaturedDeallocations(ctx.WithBlockTime(done)))
	require.Empty(t, k.GetPendingDeallocations(ctx, valAddr))
	require.Equal(t, math.NewInt(299), bankKeeper.GetAllBalances(ctx, valAccAddr).AmountOf(keepertest.TestBondDenom))
	position, _ := k.GetPosition(ctx, valAddr, poolID)
	require.Equal(t, math.NewInt(400), position.Shares)
	require.Equal(t, math.NewInt(400), k.GetPoolTotalShares(ctx, poolID))
	require.True(t, k.GetPendingShares(ctx, valAddr, poolID).IsZero())
	require.Equal(t, math.NewInt(601), wasmKeeper.Pools[poolContract.String()].Value)

	// Deallocating everything deletes the position and, being a full exit
	// at ratio 1.0, recovers the dust: the validator ends with the whole
	// 900.
	_, err = k.DeallocateFromPool(ctx.WithBlockTime(done), valAddr, poolID, math.NewInt(400))
	require.NoError(t, err)
	require.NoError(t, k.ProcessMaturedDeallocations(ctx.WithBlockTime(done.Add(types.DefaultDeallocationGracePeriod))))
	_, found := k.GetPosition(ctx, valAddr, poolID)
	require.False(t, found)
	require.True(t, k.GetPoolTotalShares(ctx, poolID).IsZero())
	require.Equal(t, math.NewInt(900), bankKeeper.GetAllBalances(ctx, valAccAddr).AmountOf(keepertest.TestBondDenom))
}

func TestDeallocationRequeuesOnContractFailure(t *testing.T) {
	k, ctx, _, bankKeeper, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)
	_, err = k.DeallocateFromPool(ctx, valAddr, poolID, math.NewInt(600))
	require.NoError(t, err)

	// The pool is broken (e.g. paused) when the deallocation matures: the
	// entry is requeued, nothing else changes, and the block continues.
	wasmKeeper.Pools[poolContract.String()].FailExecute = true
	done := ctx.BlockTime().Add(types.DefaultDeallocationGracePeriod)
	require.NoError(t, k.ProcessMaturedDeallocations(ctx.WithBlockTime(done)))

	queue := k.GetPendingDeallocations(ctx, valAddr)
	require.Len(t, queue, 1)
	require.True(t, queue[0].CompleteTime.After(done))
	require.Equal(t, math.NewInt(600), k.GetPendingShares(ctx, valAddr, poolID))
	position, _ := k.GetPosition(ctx, valAddr, poolID)
	require.Equal(t, math.NewInt(600), position.Shares)
	require.True(t, bankKeeper.GetAllBalances(ctx, valAccAddr).AmountOf(keepertest.TestBondDenom).IsZero())

	// Once the pool recovers, the retry succeeds.
	wasmKeeper.Pools[poolContract.String()].FailExecute = false
	retryTime := queue[0].CompleteTime
	require.NoError(t, k.ProcessMaturedDeallocations(ctx.WithBlockTime(retryTime)))
	require.Empty(t, k.GetPendingDeallocations(ctx, valAddr))
	require.Equal(t, math.NewInt(600), bankKeeper.GetAllBalances(ctx, valAccAddr).AmountOf(keepertest.TestBondDenom))
}

func TestDeallocationAbandonedAfterMaxRetries(t *testing.T) {
	k, ctx, _, bankKeeper, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)
	_, err = k.DeallocateFromPool(ctx, valAddr, poolID, math.NewInt(200))
	require.NoError(t, err)

	// The pool never recovers: after the retry budget is exhausted the
	// entry is abandoned, the reservation is released, and the shares stay
	// in the position — nothing is wedged and nothing is lost.
	wasmKeeper.Pools[poolContract.String()].FailExecute = true
	blockTime := ctx.BlockTime().Add(types.DefaultDeallocationGracePeriod)
	for i := 0; i < 30; i++ {
		require.NoError(t, k.ProcessMaturedDeallocations(ctx.WithBlockTime(blockTime)))
		queue := k.GetPendingDeallocations(ctx, valAddr)
		if len(queue) == 0 {
			break
		}
		blockTime = queue[0].CompleteTime
	}

	require.Empty(t, k.GetPendingDeallocations(ctx, valAddr))
	require.True(t, k.GetPendingShares(ctx, valAddr, poolID).IsZero())
	position, found := k.GetPosition(ctx, valAddr, poolID)
	require.True(t, found)
	require.Equal(t, math.NewInt(600), position.Shares)
	require.True(t, bankKeeper.GetAllBalances(ctx, valAccAddr).AmountOf(keepertest.TestBondDenom).IsZero())

	// The shares are usable again: a fresh deallocation can be queued.
	_, err = k.DeallocateFromPool(ctx.WithBlockTime(blockTime), valAddr, poolID, math.NewInt(600))
	require.NoError(t, err)
}

func TestRegisterPoolProbesAdapterInterface(t *testing.T) {
	k, ctx, _, _, wasmKeeper := keepertest.LiquidityvaultKeeper(t)

	// A contract that exists but cannot answer position_value is rejected:
	// it would accept allocations and strand every later withdrawal.
	pool := wasmKeeper.AddPool(poolContract)
	pool.FailQuery = true
	_, err := k.RegisterPool(ctx, poolContract.String(), "broken adapter")
	require.ErrorIs(t, err, types.ErrPoolValueUnavailable)

	pool.FailQuery = false
	_, err = k.RegisterPool(ctx, poolContract.String(), "working adapter")
	require.NoError(t, err)
}

func TestAllocationChargesSlippageToAllocator(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, wasmKeeper, poolID := setupPool(t)

	// Validator 1 allocates 500 with no slippage.
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(500))
	require.NoError(t, err)

	// Validator 2 allocates 500 through a pool whose internal swap costs
	// 20%: the measured value delta is 400, so only 400 shares are minted.
	// Validator 1's stake stays worth 500 — the slippage lands entirely on
	// validator 2.
	wasmKeeper.Pools[poolContract.String()].DepositFeeBps = 2_000
	stakingKeeper.SetValidatorTokens(val2Addr, math.NewInt(1))
	bankKeeper.SetBalance(val2AccAddr, sdk.NewCoins(coin(500)))
	require.NoError(t, k.Deposit(ctx, val2Addr, coin(500)))
	shares, err := k.AllocateToPool(ctx, val2Addr, poolID, coin(500))
	require.NoError(t, err)
	require.Equal(t, math.NewInt(400), shares)

	// Pool value is now 900; val1 holds 500/900 shares -> 500, val2
	// 400/900 -> 400.
	score, err := k.GetCompositeScore(ctx, valAddr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(500), score.PositionValue)
	score, err = k.GetCompositeScore(ctx, val2Addr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(400), score.PositionValue)
}

func TestCompositeScoreWithPositions(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, wasmKeeper, poolID := setupPool(t)

	// Two validators share the pool: val1 allocates 600, val2 allocates 300.
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)

	stakingKeeper.SetValidatorTokens(val2Addr, math.NewInt(2_000))
	bankKeeper.SetBalance(val2AccAddr, sdk.NewCoins(coin(300)))
	require.NoError(t, k.Deposit(ctx, val2Addr, coin(300)))
	_, err = k.AllocateToPool(ctx, val2Addr, poolID, coin(300))
	require.NoError(t, err)

	// The pool value rises to 1800: val1 owns 2/3 (1200), val2 1/3 (600).
	wasmKeeper.SetPoolValue(poolContract, math.NewInt(1800))

	score, err := k.GetCompositeScore(ctx, valAddr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000), score.StakedTokens)
	require.Equal(t, math.NewInt(400), score.VaultBalance)
	require.Equal(t, math.NewInt(1_200), score.PositionValue)
	require.Equal(t, math.NewInt(2_600), score.Score)

	score, err = k.GetCompositeScore(ctx, val2Addr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(600), score.PositionValue)
	require.Equal(t, math.NewInt(2_600), score.Score)

	// A dark pool degrades to its last cached value (900, observed at
	// val2's allocation) instead of erroring: the score stays computable
	// with the pool's pre-spike valuation. val1 owns 600/900 shares -> 600.
	wasmKeeper.Pools[poolContract.String()].FailQuery = true
	score, err = k.GetCompositeScore(ctx, valAddr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(600), score.PositionValue)
	require.Equal(t, math.NewInt(2_000), score.Score) // 1000 staked + 400 balance + 600 cached position
}

func TestPoolSharesInvariant(t *testing.T) {
	k, ctx, _, _, _, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)
	_, err = k.DeallocateFromPool(ctx, valAddr, poolID, math.NewInt(100))
	require.NoError(t, err)

	invariant := keeper.PoolSharesInvariant(k)
	_, broken := invariant(ctx)
	require.False(t, broken)
}

func TestPositionsQuery(t *testing.T) {
	k, ctx, _, _, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)
	_, err = k.DeallocateFromPool(ctx, valAddr, poolID, math.NewInt(100))
	require.NoError(t, err)
	wasmKeeper.SetPoolValue(poolContract, math.NewInt(1_200))

	resp, err := k.Positions(ctx, &types.QueryPositionsRequest{ValidatorAddress: valAddr.String()})
	require.NoError(t, err)
	require.Len(t, resp.Positions, 1)
	require.Equal(t, math.NewInt(600), resp.Positions[0].Position.Shares)
	require.Equal(t, math.NewInt(1_200), resp.Positions[0].Value)
	require.Len(t, resp.PendingDeallocations, 1)

	poolsResp, err := k.Pools(ctx, &types.QueryPoolsRequest{})
	require.NoError(t, err)
	require.Len(t, poolsResp.Pools, 1)
}
