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

func post(v int64) types.ValuePost {
	return types.ValuePost{Value: math.NewInt(v), PostTime: time.Now().UTC()}
}

func TestMedianOfPosts(t *testing.T) {
	require.True(t, keeper.MedianOfPosts(nil).IsZero())
	require.Equal(t, math.NewInt(5), keeper.MedianOfPosts([]types.ValuePost{post(5)}))
	require.Equal(t, math.NewInt(7), keeper.MedianOfPosts([]types.ValuePost{post(9), post(5), post(7)}))
	// Even count: average of the two middles, truncated.
	require.Equal(t, math.NewInt(6), keeper.MedianOfPosts([]types.ValuePost{post(9), post(4), post(5), post(8)}))
	// Outliers are damped: one manipulated spike doesn't move the median.
	require.Equal(t, math.NewInt(10), keeper.MedianOfPosts([]types.ValuePost{post(10), post(10), post(10), post(10), post(10), post(1_000_000)}))
}

func TestValuePostLifecycle(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, _ := keepertest.LiquidityvaultKeeper(t)
	start := time.Now().UTC()
	ctx = ctx.WithBlockTime(start)

	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1_000))
	bankKeeper.SetBalance(valAccAddr, sdk.NewCoins(coin(500)))

	// The first deposit creates the vault and schedules the first post
	// within [interval/2, interval*3/2).
	require.NoError(t, k.Deposit(ctx, valAddr, coin(500)))
	next, found := k.NextScheduledValuePost(ctx, valAddr)
	require.True(t, found)
	interval := types.DefaultValuePostInterval
	require.True(t, !next.PostTime.Before(start.Add(interval/2)))
	require.True(t, next.PostTime.Before(start.Add(interval*3/2)))

	// Before the due time nothing posts.
	require.NoError(t, k.ProcessDueValuePosts(ctx.WithBlockTime(next.PostTime.Add(-time.Second))))
	require.Empty(t, k.GetValuePostHistory(ctx, valAddr).Posts)

	// At the due time the vault's total value is posted and the next post
	// is scheduled.
	postCtx := ctx.WithBlockTime(next.PostTime)
	require.NoError(t, k.ProcessDueValuePosts(postCtx))
	history := k.GetValuePostHistory(ctx, valAddr)
	require.Len(t, history.Posts, 1)
	require.Equal(t, math.NewInt(500), history.Posts[0].Value)

	next2, found := k.NextScheduledValuePost(ctx, valAddr)
	require.True(t, found)
	require.True(t, next2.PostTime.After(next.PostTime))
}

func TestValuePostWindowTrimsToSix(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, _ := keepertest.LiquidityvaultKeeper(t)
	ctx = ctx.WithBlockTime(time.Now().UTC())
	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1))
	bankKeeper.SetBalance(valAccAddr, sdk.NewCoins(coin(100)))
	require.NoError(t, k.Deposit(ctx, valAddr, coin(100)))

	blockTime := ctx.BlockTime()
	for i := 0; i < 9; i++ {
		next, found := k.NextScheduledValuePost(ctx, valAddr)
		require.True(t, found)
		blockTime = next.PostTime
		require.NoError(t, k.ProcessDueValuePosts(ctx.WithBlockTime(blockTime)))
	}

	history := k.GetValuePostHistory(ctx, valAddr)
	require.Len(t, history.Posts, 6)
}

func TestCompositeScoreUsesMedianOncePostsExist(t *testing.T) {
	k, ctx, _, _, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)

	// Take one post at the current value (400 active + 600 position).
	next, found := k.NextScheduledValuePost(ctx, valAddr)
	require.True(t, found)
	require.NoError(t, k.ProcessDueValuePosts(ctx.WithBlockTime(next.PostTime)))

	// The pool then spikes to 60_000. The live position value follows, but
	// the median (single post at 1_000) keeps the score damped.
	wasmKeeper.SetPoolValue(poolContract, math.NewInt(60_000))
	score, err := k.GetCompositeScore(ctx, valAddr)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(60_000), score.PositionValue)
	require.Equal(t, math.NewInt(1_000), score.MedianVaultValue)
	require.Equal(t, math.NewInt(1_000+1_000), score.Score) // staked 1000 + median 1000
}

func TestValuePostFallsBackToCachedPoolValue(t *testing.T) {
	k, ctx, _, _, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)

	// First post succeeds and caches the pool value (600).
	next, _ := k.NextScheduledValuePost(ctx, valAddr)
	require.NoError(t, k.ProcessDueValuePosts(ctx.WithBlockTime(next.PostTime)))
	history := k.GetValuePostHistory(ctx, valAddr)
	require.Equal(t, math.NewInt(1_000), history.Posts[0].Value) // 400 active + 600 position

	// The pool goes dark; the next post uses the cached 600 instead of
	// zeroing the position or aborting the end blocker.
	wasmKeeper.Pools[poolContract.String()].FailQuery = true
	next, _ = k.NextScheduledValuePost(ctx, valAddr)
	require.NoError(t, k.ProcessDueValuePosts(ctx.WithBlockTime(next.PostTime)))
	history = k.GetValuePostHistory(ctx, valAddr)
	require.Len(t, history.Posts, 2)
	require.Equal(t, math.NewInt(1_000), history.Posts[1].Value)
}

func TestFirstPostUsesAllocationCachedValueWhenPoolDark(t *testing.T) {
	// The allocation itself caches the observed pool value, so a pool that
	// goes dark before the vault's FIRST post still gets valued from that
	// observation instead of zero.
	k, ctx, _, _, wasmKeeper, poolID := setupPool(t)
	_, err := k.AllocateToPool(ctx, valAddr, poolID, coin(600))
	require.NoError(t, err)

	wasmKeeper.Pools[poolContract.String()].FailQuery = true
	next, found := k.NextScheduledValuePost(ctx, valAddr)
	require.True(t, found)
	require.NoError(t, k.ProcessDueValuePosts(ctx.WithBlockTime(next.PostTime)))

	history := k.GetValuePostHistory(ctx, valAddr)
	require.Len(t, history.Posts, 1)
	require.Equal(t, math.NewInt(1_000), history.Posts[0].Value) // 400 active + cached 600
}

func TestSetRankingShadowQuery(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, _, poolID := setupPool(t)
	_ = poolID

	// val1: 1000 staked, 1000 vault. val2: 1000 staked, 200 vault.
	// val3: 5000 staked, no vault.
	stakingKeeper.SetValidatorTokens(val2Addr, math.NewInt(1_000))
	bankKeeper.SetBalance(val2AccAddr, sdk.NewCoins(coin(200)))
	require.NoError(t, k.Deposit(ctx, val2Addr, coin(200)))

	val3 := sdk.ValAddress([]byte("validator-operator-3"))
	stakingKeeper.SetValidatorTokens(val3, math.NewInt(5_000))

	resp, err := k.SetRanking(ctx, &types.QuerySetRankingRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Validators, 3)

	// Highest stake first; the equal-stake pair is broken by composite
	// score (vault value), per the design document's complex check.
	require.Equal(t, val3.String(), resp.Validators[0].ValidatorAddress)
	require.Equal(t, valAddr.String(), resp.Validators[1].ValidatorAddress)
	require.Equal(t, math.NewInt(2_000), resp.Validators[1].CompositeScore)
	require.Equal(t, val2Addr.String(), resp.Validators[2].ValidatorAddress)
	require.Equal(t, math.NewInt(1_200), resp.Validators[2].CompositeScore)
}

func TestValuePostsQuery(t *testing.T) {
	k, ctx, stakingKeeper, bankKeeper, _ := keepertest.LiquidityvaultKeeper(t)
	ctx = ctx.WithBlockTime(time.Now().UTC())
	stakingKeeper.SetValidatorTokens(valAddr, math.NewInt(1))
	bankKeeper.SetBalance(valAccAddr, sdk.NewCoins(coin(300)))
	require.NoError(t, k.Deposit(ctx, valAddr, coin(300)))

	next, _ := k.NextScheduledValuePost(ctx, valAddr)
	require.NoError(t, k.ProcessDueValuePosts(ctx.WithBlockTime(next.PostTime)))

	resp, err := k.ValuePosts(ctx, &types.QueryValuePostsRequest{ValidatorAddress: valAddr.String()})
	require.NoError(t, err)
	require.Len(t, resp.Posts, 1)
	require.Equal(t, math.NewInt(300), resp.Median)
	require.False(t, resp.NextPostTime.IsZero())
}
