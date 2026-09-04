package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	keepertest "bluechipChain/testutil/keeper"
	"bluechipChain/x/liquidityvault/keeper"
	liquidityvault "bluechipChain/x/liquidityvault/module"
	"bluechipChain/x/liquidityvault/types"
)

// TestFullStageOneLifecycle drives the complete LPV stage-1 journey through
// the keeper — deposit, allocation, value posts, reward collection and
// claims, deallocation, withdrawal — checking both module invariants and a
// genesis export/import round trip along the way.
func TestFullStageOneLifecycle(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, wasmKeeper, poolID := setupPool(t)
	hooks := k.StakingHooks()

	checkInvariants := func(step string) {
		t.Helper()
		msg, broken := keeper.ModuleAccountInvariant(k)(ctx)
		require.False(t, broken, "module-account invariant broken after %s: %s", step, msg)
		msg, broken = keeper.PoolSharesInvariant(k)(ctx)
		require.False(t, broken, "pool-shares invariant broken after %s: %s", step, msg)
	}

	// The validator (1000 staked) allocates 600 of its 1000-deposit vault.
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)
	checkInvariants("allocation")

	// A delegator stakes 400 of the validator's 1000 tokens.
	require.NoError(t, hooks.BeforeDelegationCreated(ctx, delegator1, valAddr))
	stakingKeeper.SetDelegationTokens(delegator1, valAddr, math.NewInt(400))
	require.NoError(t, hooks.AfterDelegationModified(ctx, delegator1, valAddr))

	// The pool appreciates and takes a value post: 400 active + 900
	// position = 1300.
	wasmKeeper.SetPoolValue(poolContract, math.NewInt(900))
	next, found := k.NextScheduledValuePost(ctx, valAddr)
	require.True(t, found)
	ctx = ctx.WithBlockTime(next.PostTime)
	require.NoError(t, k.ProcessDueValuePosts(ctx))
	score, err := k.GetCompositeScore(ctx, valAddr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_300), score.MedianVaultValue)
	require.Equal(t, math.NewInt(2_300), score.Score)
	checkInvariants("value post")

	// Fees accrue and are collected: 1000 -> 500 to the operator, 500 to
	// delegators at 0.5/token.
	wasmKeeper.AccrueFees(poolContract, math.NewInt(1_000))
	collected, err := k.CollectPoolRewards(ctx, valAddr, poolID)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000), collected)
	require.Equal(t, math.NewInt(500), balanceOf(bankKeeper, ctx, valAccAddr))
	checkInvariants("reward collection")

	// The delegator claims 400 * 0.5 = 200.
	paid, err := k.ClaimVaultRewards(ctx, delegator1, valAddr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(200), paid.Amount)
	checkInvariants("reward claim")

	// The validator exits the pool entirely; after the grace period the
	// whole 900 position lands in the operator account.
	_, err = k.DeallocateFromPool(ctx, valAddr, poolID, math.NewInt(600))
	require.NoError(t, err)
	checkInvariants("deallocation queued")
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(types.DefaultDeallocationGracePeriod))
	require.NoError(t, k.ProcessMaturedDeallocations(ctx))
	require.Equal(t, math.NewInt(500+900), balanceOf(bankKeeper, ctx, valAccAddr))
	checkInvariants("deallocation settled")

	// The remaining active balance is withdrawn from the vault.
	_, err = k.InitiateWithdrawal(ctx, valAddr, coin(400))
	require.NoError(t, err)
	checkInvariants("withdrawal queued")
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(types.DefaultWithdrawalGracePeriod))
	require.NoError(t, k.ProcessMaturedWithdrawals(ctx))
	require.Equal(t, math.NewInt(500+900+400), balanceOf(bankKeeper, ctx, valAccAddr))
	checkInvariants("withdrawal settled")

	vault, _ := k.GetVault(ctx, valAddr)
	require.True(t, vault.Balance.IsZero())

	// The exported state at this point survives a round trip into a fresh
	// keeper and still validates.
	exported := liquidityvault.ExportGenesis(ctx, k)
	require.NoError(t, exported.Validate())

	k2, ctx2, _, _, _ := keepertest.LiquidityvaultKeeper(t)
	require.NotPanics(t, func() { liquidityvault.InitGenesis(ctx2, k2, *exported) })
	reExported := liquidityvault.ExportGenesis(ctx2, k2)
	require.Equal(t, exported.Params, reExported.Params)
	require.ElementsMatch(t, exported.Vaults, reExported.Vaults)
	require.ElementsMatch(t, exported.DelegatorRewards, reExported.DelegatorRewards)
	require.ElementsMatch(t, exported.ValuePostHistories, reExported.ValuePostHistories)
}

// TestUpgradeStoreKeyMatchesModule guards the upgrade wiring: the store key
// added by the lpv-stage-1 store loader must be the module's actual store
// key.
func TestUpgradeStoreKeyMatchesModule(t *testing.T) {
	require.Equal(t, "liquidityvault", types.StoreKey)
}

// TestEndBlockerNeverErrorsOnBrokenPool simulates the module's full
// EndBlock work against a completely broken pool contract: nothing may
// bubble an error (which would halt the chain).
func TestEndBlockerNeverErrorsOnBrokenPool(t *testing.T) {
	k, ctx, _, _, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)
	_, err = k.DeallocateFromPool(ctx, valAddr, poolID, math.NewInt(600))
	require.NoError(t, err)

	wasmKeeper.Pools[poolContract.String()].FailExecute = true
	wasmKeeper.Pools[poolContract.String()].FailQuery = true

	// Run a week of end-blocker work at 6h steps with the pool dark the
	// whole time: withdrawals, deallocations (failing + requeueing +
	// eventually abandoning), and value posts (cached fallback) must all
	// keep returning nil.
	blockTime := ctx.BlockTime()
	for i := 0; i < 28; i++ {
		blockTime = blockTime.Add(6 * time.Hour)
		stepCtx := ctx.WithBlockTime(blockTime)
		require.NoError(t, k.ProcessMaturedWithdrawals(stepCtx))
		require.NoError(t, k.ProcessMaturedDeallocations(stepCtx))
		require.NoError(t, k.ProcessDueValuePosts(stepCtx))
	}

	// The position is intact whatever happened to the queue entry.
	position, found := k.GetPosition(ctx, valAddr, poolID)
	require.True(t, found)
	require.Equal(t, math.NewInt(600), position.Shares)
}
