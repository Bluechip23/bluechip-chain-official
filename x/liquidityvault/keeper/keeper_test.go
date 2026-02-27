package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	keepertest "bluechipChain/testutil/keeper"
	"bluechipChain/x/liquidityvault/keeper"
	"bluechipChain/x/liquidityvault/types"
)

// ---------------------------------------------------------------------------
// 1. Params
// ---------------------------------------------------------------------------

func TestGetParams(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	// The test helper already calls SetParams(DefaultParams()), so GetParams
	// should return defaults immediately.
	params := k.GetParams(ctx)

	require.Equal(t, types.DefaultStakeCap, params.StakeCap)
	require.Equal(t, types.DefaultSimpleCheckInterval, params.SimpleCheckInterval)
	require.Equal(t, types.DefaultComplexCheckInterval, params.ComplexCheckInterval)
	require.Equal(t, types.DefaultValuePostsPerComplexInterval, params.ValuePostsPerComplexInterval)
	require.True(t, types.DefaultDelegatorRewardPercent.Equal(params.DefaultDelegatorRewardPercent))
}

func TestSetParams(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	custom := types.NewParams(
		math.NewInt(999),
		uint64(100),
		uint64(500),
		uint64(12),
		math.LegacyNewDecWithPrec(75, 0),
	)

	err := k.SetParams(ctx, custom)
	require.NoError(t, err)

	got := k.GetParams(ctx)
	require.Equal(t, custom.StakeCap, got.StakeCap)
	require.Equal(t, custom.SimpleCheckInterval, got.SimpleCheckInterval)
	require.Equal(t, custom.ComplexCheckInterval, got.ComplexCheckInterval)
	require.Equal(t, custom.ValuePostsPerComplexInterval, got.ValuePostsPerComplexInterval)
	require.True(t, custom.DefaultDelegatorRewardPercent.Equal(got.DefaultDelegatorRewardPercent))
}

func TestSetParamsRoundTrip(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	tests := []struct {
		name   string
		params types.Params
	}{
		{
			name:   "default params",
			params: types.DefaultParams(),
		},
		{
			name: "custom params with small values",
			params: types.NewParams(
				math.NewInt(1),
				uint64(1),
				uint64(1),
				uint64(1),
				math.LegacyNewDec(0),
			),
		},
		{
			name: "custom params with large values",
			params: types.NewParams(
				math.NewInt(1_000_000_000_000_000),
				uint64(1_000_000),
				uint64(5_000_000),
				uint64(100),
				math.LegacyNewDec(100),
			),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := k.SetParams(ctx, tc.params)
			require.NoError(t, err)

			got := k.GetParams(ctx)
			require.Equal(t, tc.params.StakeCap, got.StakeCap)
			require.Equal(t, tc.params.SimpleCheckInterval, got.SimpleCheckInterval)
			require.Equal(t, tc.params.ComplexCheckInterval, got.ComplexCheckInterval)
			require.Equal(t, tc.params.ValuePostsPerComplexInterval, got.ValuePostsPerComplexInterval)
			require.True(t, tc.params.DefaultDelegatorRewardPercent.Equal(got.DefaultDelegatorRewardPercent))
		})
	}
}

// ---------------------------------------------------------------------------
// 2. Vault CRUD
// ---------------------------------------------------------------------------

func TestVaultCRUD(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	valAddr := "cosmosvaloper1abc"

	// --- Create vault ---
	vault := types.Vault{
		ValidatorAddress:       valAddr,
		TotalDeposited:         sdk.NewCoin("ubluechip", math.NewInt(1000)),
		DelegatorRewardPercent: math.LegacyNewDec(50),
		Positions:              []types.PoolPosition{},
		ValidatorType:          types.ValidatorType_VALIDATOR_TYPE_FULL,
	}
	err := k.SetVault(ctx, vault)
	require.NoError(t, err)

	// --- Get vault ---
	got, found := k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.Equal(t, valAddr, got.ValidatorAddress)
	require.Equal(t, vault.TotalDeposited.Denom, got.TotalDeposited.Denom)
	require.True(t, vault.TotalDeposited.Amount.Equal(got.TotalDeposited.Amount))
	require.True(t, vault.DelegatorRewardPercent.Equal(got.DelegatorRewardPercent))
	require.Equal(t, vault.ValidatorType, got.ValidatorType)
	require.Empty(t, got.Positions)

	// --- Update vault ---
	got.TotalDeposited = sdk.NewCoin("ubluechip", math.NewInt(5000))
	got.DelegatorRewardPercent = math.LegacyNewDec(75)
	err = k.SetVault(ctx, got)
	require.NoError(t, err)

	updated, found := k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.True(t, updated.TotalDeposited.Amount.Equal(math.NewInt(5000)))
	require.True(t, updated.DelegatorRewardPercent.Equal(math.LegacyNewDec(75)))

	// --- Delete vault ---
	k.DeleteVault(ctx, valAddr)
	_, found = k.GetVault(ctx, valAddr)
	require.False(t, found)
}

func TestGetVaultNotFound(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	_, found := k.GetVault(ctx, "cosmosvaloper1nonexistent")
	require.False(t, found)
}

func TestGetAllVaults(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	// Initially no vaults
	vaults := k.GetAllVaults(ctx)
	require.Empty(t, vaults)

	// Add three vaults
	addrs := []string{"cosmosvaloper1aaa", "cosmosvaloper1bbb", "cosmosvaloper1ccc"}
	for _, addr := range addrs {
		err := k.SetVault(ctx, types.Vault{
			ValidatorAddress:       addr,
			TotalDeposited:         sdk.NewCoin("ubluechip", math.ZeroInt()),
			DelegatorRewardPercent: math.LegacyNewDec(50),
			Positions:              []types.PoolPosition{},
			ValidatorType:          types.ValidatorType_VALIDATOR_TYPE_FULL,
		})
		require.NoError(t, err)
	}

	vaults = k.GetAllVaults(ctx)
	require.Len(t, vaults, 3)

	// Collect addresses to verify all are present (order may vary with KV iteration)
	gotAddrs := make(map[string]bool)
	for _, v := range vaults {
		gotAddrs[v.ValidatorAddress] = true
	}
	for _, addr := range addrs {
		require.True(t, gotAddrs[addr], "expected vault for %s", addr)
	}
}

func TestIterateVaults(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	addrs := []string{"cosmosvaloper1aaa", "cosmosvaloper1bbb", "cosmosvaloper1ccc"}
	for _, addr := range addrs {
		err := k.SetVault(ctx, types.Vault{
			ValidatorAddress:       addr,
			TotalDeposited:         sdk.NewCoin("ubluechip", math.ZeroInt()),
			DelegatorRewardPercent: math.LegacyNewDec(50),
			Positions:              []types.PoolPosition{},
			ValidatorType:          types.ValidatorType_VALIDATOR_TYPE_FULL,
		})
		require.NoError(t, err)
	}

	// Iterate with early break (return true to stop)
	var count int
	k.IterateVaults(ctx, func(vault types.Vault) bool {
		count++
		return count >= 2 // stop after 2
	})
	require.Equal(t, 2, count)

	// Iterate all (never return true)
	count = 0
	k.IterateVaults(ctx, func(vault types.Vault) bool {
		count++
		return false
	})
	require.Equal(t, 3, count)
}

func TestCreateVaultDuplicate(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	valAddr := "cosmosvaloper1dup"
	err := k.CreateVault(ctx, valAddr, types.ValidatorType_VALIDATOR_TYPE_FULL)
	require.NoError(t, err)

	// Creating the same vault again should error
	err = k.CreateVault(ctx, valAddr, types.ValidatorType_VALIDATOR_TYPE_FULL)
	require.ErrorIs(t, err, types.ErrVaultAlreadyExists)
}

func TestCreateVaultSetsFieldsCorrectly(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	valAddr := "cosmosvaloper1new"
	err := k.CreateVault(ctx, valAddr, types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY)
	require.NoError(t, err)

	vault, found := k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.Equal(t, valAddr, vault.ValidatorAddress)
	require.Equal(t, "ubluechip", vault.TotalDeposited.Denom)
	require.True(t, vault.TotalDeposited.Amount.IsZero())
	require.True(t, types.DefaultDelegatorRewardPercent.Equal(vault.DelegatorRewardPercent))
	require.Empty(t, vault.Positions)
	require.Equal(t, types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY, vault.ValidatorType)

	// Also verifies that SetValidatorType is called
	vt, found := k.GetValidatorType(ctx, valAddr)
	require.True(t, found)
	require.Equal(t, types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY, vt)
}

// ---------------------------------------------------------------------------
// 3. Validator Type
// ---------------------------------------------------------------------------

func TestValidatorType(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	tests := []struct {
		name    string
		valAddr string
		valType types.ValidatorType
	}{
		{
			name:    "FULL type",
			valAddr: "cosmosvaloper1full",
			valType: types.ValidatorType_VALIDATOR_TYPE_FULL,
		},
		{
			name:    "LIQUIDITY type",
			valAddr: "cosmosvaloper1liq",
			valType: types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
		},
		{
			name:    "UNSPECIFIED type",
			valAddr: "cosmosvaloper1unspec",
			valType: types.ValidatorType_VALIDATOR_TYPE_UNSPECIFIED,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k.SetValidatorType(ctx, tc.valAddr, tc.valType)

			got, found := k.GetValidatorType(ctx, tc.valAddr)
			require.True(t, found)
			require.Equal(t, tc.valType, got)
		})
	}
}

func TestGetValidatorTypeNotFound(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	vt, found := k.GetValidatorType(ctx, "cosmosvaloper1doesnotexist")
	require.False(t, found)
	require.Equal(t, types.ValidatorType_VALIDATOR_TYPE_UNSPECIFIED, vt)
}

func TestSetValidatorTypeOverwrite(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	valAddr := "cosmosvaloper1overwrite"

	k.SetValidatorType(ctx, valAddr, types.ValidatorType_VALIDATOR_TYPE_FULL)
	got, found := k.GetValidatorType(ctx, valAddr)
	require.True(t, found)
	require.Equal(t, types.ValidatorType_VALIDATOR_TYPE_FULL, got)

	// Overwrite with LIQUIDITY
	k.SetValidatorType(ctx, valAddr, types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY)
	got, found = k.GetValidatorType(ctx, valAddr)
	require.True(t, found)
	require.Equal(t, types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY, got)
}

// ---------------------------------------------------------------------------
// 4. Composite Scores
// ---------------------------------------------------------------------------

func TestCompositeScoreSetGet(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	score := types.CompositeScore{
		ValidatorAddress: "cosmosvaloper1score",
		ChainStake:       math.NewInt(500000),
		VaultValue:       math.NewInt(100000),
	}

	err := k.SetCompositeScore(ctx, score)
	require.NoError(t, err)

	got, found := k.GetCompositeScore(ctx, "cosmosvaloper1score")
	require.True(t, found)
	require.Equal(t, score.ValidatorAddress, got.ValidatorAddress)
	require.True(t, score.ChainStake.Equal(got.ChainStake))
	require.True(t, score.VaultValue.Equal(got.VaultValue))
}

func TestGetCompositeScoreNotFound(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	_, found := k.GetCompositeScore(ctx, "cosmosvaloper1missing")
	require.False(t, found)
}

func TestGetAllCompositeScores(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	scores := []types.CompositeScore{
		{ValidatorAddress: "cosmosvaloper1a", ChainStake: math.NewInt(100), VaultValue: math.NewInt(10)},
		{ValidatorAddress: "cosmosvaloper1b", ChainStake: math.NewInt(200), VaultValue: math.NewInt(20)},
		{ValidatorAddress: "cosmosvaloper1c", ChainStake: math.NewInt(300), VaultValue: math.NewInt(30)},
	}

	for _, s := range scores {
		err := k.SetCompositeScore(ctx, s)
		require.NoError(t, err)
	}

	all := k.GetAllCompositeScores(ctx)
	require.Len(t, all, 3)

	gotAddrs := make(map[string]bool)
	for _, s := range all {
		gotAddrs[s.ValidatorAddress] = true
	}
	for _, s := range scores {
		require.True(t, gotAddrs[s.ValidatorAddress])
	}
}

func TestCalculateMedianValueOdd(t *testing.T) {
	// Odd number of values: median is the middle element
	posts := []types.ValuePost{
		{Value: math.NewInt(10)},
		{Value: math.NewInt(30)},
		{Value: math.NewInt(20)},
		{Value: math.NewInt(50)},
		{Value: math.NewInt(40)},
	}
	// Sorted: 10, 20, 30, 40, 50 -> median = 30
	result := keeper.CalculateMedianValue(posts)
	require.True(t, result.Equal(math.NewInt(30)), "expected 30, got %s", result)
}

func TestCalculateMedianValueEven(t *testing.T) {
	// Even number of values: median is average of two middle values
	posts := []types.ValuePost{
		{Value: math.NewInt(10)},
		{Value: math.NewInt(40)},
		{Value: math.NewInt(20)},
		{Value: math.NewInt(30)},
	}
	// Sorted: 10, 20, 30, 40 -> median = (20+30)/2 = 25
	result := keeper.CalculateMedianValue(posts)
	require.True(t, result.Equal(math.NewInt(25)), "expected 25, got %s", result)
}

func TestCalculateMedianValueEmpty(t *testing.T) {
	result := keeper.CalculateMedianValue([]types.ValuePost{})
	require.True(t, result.IsZero(), "expected zero for empty slice, got %s", result)
}

func TestCalculateMedianValueSingle(t *testing.T) {
	posts := []types.ValuePost{
		{Value: math.NewInt(42)},
	}
	result := keeper.CalculateMedianValue(posts)
	require.True(t, result.Equal(math.NewInt(42)), "expected 42, got %s", result)
}

func TestCalculateMedianValueTwoElements(t *testing.T) {
	posts := []types.ValuePost{
		{Value: math.NewInt(10)},
		{Value: math.NewInt(20)},
	}
	// (10+20)/2 = 15
	result := keeper.CalculateMedianValue(posts)
	require.True(t, result.Equal(math.NewInt(15)), "expected 15, got %s", result)
}

func TestCalculateMedianValueAllSame(t *testing.T) {
	posts := []types.ValuePost{
		{Value: math.NewInt(7)},
		{Value: math.NewInt(7)},
		{Value: math.NewInt(7)},
	}
	result := keeper.CalculateMedianValue(posts)
	require.True(t, result.Equal(math.NewInt(7)), "expected 7, got %s", result)
}

func TestCalculateMedianValueEvenTruncation(t *testing.T) {
	// Even count where average truncates (integer division)
	posts := []types.ValuePost{
		{Value: math.NewInt(1)},
		{Value: math.NewInt(2)},
	}
	// (1+2)/2 = 1 (integer division truncates)
	result := keeper.CalculateMedianValue(posts)
	require.True(t, result.Equal(math.NewInt(1)), "expected 1 (truncated), got %s", result)
}

func TestGetRankedValidators(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	// Setup scores with varying ChainStake and VaultValue
	scores := []types.CompositeScore{
		{ValidatorAddress: "val_low", ChainStake: math.NewInt(100), VaultValue: math.NewInt(50)},
		{ValidatorAddress: "val_high", ChainStake: math.NewInt(300), VaultValue: math.NewInt(10)},
		{ValidatorAddress: "val_mid", ChainStake: math.NewInt(200), VaultValue: math.NewInt(100)},
	}
	for _, s := range scores {
		err := k.SetCompositeScore(ctx, s)
		require.NoError(t, err)
	}

	ranked := k.GetRankedValidators(ctx)
	require.Len(t, ranked, 3)

	// Should be sorted by ChainStake desc: 300, 200, 100
	require.Equal(t, "val_high", ranked[0].ValidatorAddress)
	require.Equal(t, "val_mid", ranked[1].ValidatorAddress)
	require.Equal(t, "val_low", ranked[2].ValidatorAddress)
}

func TestGetRankedValidatorsTiebreaker(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	// Two validators with same ChainStake, different VaultValue
	scores := []types.CompositeScore{
		{ValidatorAddress: "val_a", ChainStake: math.NewInt(500), VaultValue: math.NewInt(10)},
		{ValidatorAddress: "val_b", ChainStake: math.NewInt(500), VaultValue: math.NewInt(90)},
		{ValidatorAddress: "val_c", ChainStake: math.NewInt(500), VaultValue: math.NewInt(50)},
	}
	for _, s := range scores {
		err := k.SetCompositeScore(ctx, s)
		require.NoError(t, err)
	}

	ranked := k.GetRankedValidators(ctx)
	require.Len(t, ranked, 3)

	// All same ChainStake, so sorted by VaultValue desc: 90, 50, 10
	require.Equal(t, "val_b", ranked[0].ValidatorAddress)
	require.Equal(t, "val_c", ranked[1].ValidatorAddress)
	require.Equal(t, "val_a", ranked[2].ValidatorAddress)
}

func TestGetRankedValidatorsEmpty(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	ranked := k.GetRankedValidators(ctx)
	require.Empty(t, ranked)
}

// ---------------------------------------------------------------------------
// 5. Value Posts
// ---------------------------------------------------------------------------

func TestValuePostsAddAndGet(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	valAddr := "cosmosvaloper1vp"

	err := k.AddValuePost(ctx, valAddr, math.NewInt(100), 10)
	require.NoError(t, err)
	err = k.AddValuePost(ctx, valAddr, math.NewInt(200), 20)
	require.NoError(t, err)
	err = k.AddValuePost(ctx, valAddr, math.NewInt(300), 30)
	require.NoError(t, err)

	posts := k.GetValuePosts(ctx, valAddr)
	require.Len(t, posts, 3)

	// Posts should be iterated in key order (block height ascending since
	// the keys encode block height as big-endian)
	require.True(t, posts[0].Value.Equal(math.NewInt(100)))
	require.Equal(t, int64(10), posts[0].BlockHeight)
	require.True(t, posts[1].Value.Equal(math.NewInt(200)))
	require.Equal(t, int64(20), posts[1].BlockHeight)
	require.True(t, posts[2].Value.Equal(math.NewInt(300)))
	require.Equal(t, int64(30), posts[2].BlockHeight)
}

func TestGetValuePostsEmpty(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	posts := k.GetValuePosts(ctx, "cosmosvaloper1noposts")
	require.Empty(t, posts)
}

func TestClearValuePostsForSpecificValidator(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	val1 := "cosmosvaloper1val1"
	val2 := "cosmosvaloper1val2"

	// Add posts for both validators
	require.NoError(t, k.AddValuePost(ctx, val1, math.NewInt(100), 10))
	require.NoError(t, k.AddValuePost(ctx, val1, math.NewInt(200), 20))
	require.NoError(t, k.AddValuePost(ctx, val2, math.NewInt(300), 10))
	require.NoError(t, k.AddValuePost(ctx, val2, math.NewInt(400), 20))

	// Clear only val1
	k.ClearValuePosts(ctx, val1)

	posts1 := k.GetValuePosts(ctx, val1)
	require.Empty(t, posts1, "val1 posts should be cleared")

	posts2 := k.GetValuePosts(ctx, val2)
	require.Len(t, posts2, 2, "val2 posts should remain intact")
	require.True(t, posts2[0].Value.Equal(math.NewInt(300)))
	require.True(t, posts2[1].Value.Equal(math.NewInt(400)))
}

func TestClearAllValuePosts(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	val1 := "cosmosvaloper1val1"
	val2 := "cosmosvaloper1val2"

	require.NoError(t, k.AddValuePost(ctx, val1, math.NewInt(100), 10))
	require.NoError(t, k.AddValuePost(ctx, val2, math.NewInt(200), 20))

	k.ClearAllValuePosts(ctx)

	require.Empty(t, k.GetValuePosts(ctx, val1))
	require.Empty(t, k.GetValuePosts(ctx, val2))
}

func TestLastSimpleCheckHeight(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	// Default should be 0
	h := k.GetLastSimpleCheckHeight(ctx)
	require.Equal(t, int64(0), h)

	// Set and verify
	k.SetLastSimpleCheckHeight(ctx, 12345)
	h = k.GetLastSimpleCheckHeight(ctx)
	require.Equal(t, int64(12345), h)

	// Overwrite
	k.SetLastSimpleCheckHeight(ctx, 99999)
	h = k.GetLastSimpleCheckHeight(ctx)
	require.Equal(t, int64(99999), h)
}

func TestLastComplexCheckHeight(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	// Default should be 0
	h := k.GetLastComplexCheckHeight(ctx)
	require.Equal(t, int64(0), h)

	// Set and verify
	k.SetLastComplexCheckHeight(ctx, 54321)
	h = k.GetLastComplexCheckHeight(ctx)
	require.Equal(t, int64(54321), h)

	// Overwrite
	k.SetLastComplexCheckHeight(ctx, 88888)
	h = k.GetLastComplexCheckHeight(ctx)
	require.Equal(t, int64(88888), h)
}

// ---------------------------------------------------------------------------
// 6. Scheduled Value Posts
// ---------------------------------------------------------------------------

func TestScheduleValuePosts(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	// We need a header hash for the RNG seed in ScheduleValuePosts
	hashBytes := make([]byte, 32)
	for i := range hashBytes {
		hashBytes[i] = byte(i + 1)
	}
	ctx = ctx.WithHeaderHash(hashBytes)

	startBlock := int64(100)
	endBlock := int64(200)

	// Schedule value posts
	k.ScheduleValuePosts(ctx, startBlock, endBlock)

	// At least one block in the range should be scheduled
	// (DefaultValuePostsPerComplexInterval = 6, interval = 100 blocks,
	// so we should have up to 6 scheduled blocks in [100, 200))
	scheduledCount := 0
	for h := startBlock; h < endBlock; h++ {
		if k.IsValuePostBlock(ctx, h) {
			scheduledCount++
		}
	}
	require.Greater(t, scheduledCount, 0, "should have at least one scheduled value post block")
	require.LessOrEqual(t, scheduledCount, int(types.DefaultValuePostsPerComplexInterval),
		"should not exceed configured number of posts")

	// Blocks outside the range should NOT be scheduled
	require.False(t, k.IsValuePostBlock(ctx, startBlock-1))
	require.False(t, k.IsValuePostBlock(ctx, endBlock+100))
}

func TestIsValuePostBlockFalseByDefault(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	// No blocks should be scheduled initially
	require.False(t, k.IsValuePostBlock(ctx, 1))
	require.False(t, k.IsValuePostBlock(ctx, 100))
	require.False(t, k.IsValuePostBlock(ctx, 999999))
}

func TestClearScheduledValuePosts(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	hashBytes := make([]byte, 32)
	for i := range hashBytes {
		hashBytes[i] = byte(i + 42)
	}
	ctx = ctx.WithHeaderHash(hashBytes)

	k.ScheduleValuePosts(ctx, 100, 200)

	// Verify at least some were scheduled
	scheduledBefore := 0
	for h := int64(100); h < 200; h++ {
		if k.IsValuePostBlock(ctx, h) {
			scheduledBefore++
		}
	}
	require.Greater(t, scheduledBefore, 0)

	// Clear all
	k.ClearScheduledValuePosts(ctx)

	// Now none should be scheduled
	for h := int64(100); h < 200; h++ {
		require.False(t, k.IsValuePostBlock(ctx, h), "block %d should not be scheduled after clear", h)
	}
}

func TestScheduleValuePostsDeterministic(t *testing.T) {
	// Two calls with the same header hash and interval should produce the
	// same set of scheduled blocks.
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	hashBytes := make([]byte, 32)
	for i := range hashBytes {
		hashBytes[i] = byte(i + 7)
	}
	ctx = ctx.WithHeaderHash(hashBytes)

	k.ScheduleValuePosts(ctx, 1000, 2000)
	scheduled1 := make(map[int64]bool)
	for h := int64(1000); h < 2000; h++ {
		if k.IsValuePostBlock(ctx, h) {
			scheduled1[h] = true
		}
	}

	// Clear and reschedule with same seed
	k.ClearScheduledValuePosts(ctx)
	k.ScheduleValuePosts(ctx, 1000, 2000)
	scheduled2 := make(map[int64]bool)
	for h := int64(1000); h < 2000; h++ {
		if k.IsValuePostBlock(ctx, h) {
			scheduled2[h] = true
		}
	}

	require.Equal(t, scheduled1, scheduled2, "same seed should produce same schedule")
}

// ---------------------------------------------------------------------------
// 7. BeginBlock
// ---------------------------------------------------------------------------

func TestBeginBlockSimpleCheck(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	params := k.GetParams(ctx)

	// Initially, lastSimpleCheckHeight = 0
	require.Equal(t, int64(0), k.GetLastSimpleCheckHeight(ctx))

	// Set block height to exactly the simple check interval.
	// Since lastSimple=0 and blockHeight=SimpleCheckInterval,
	// the condition blockHeight - lastSimple >= SimpleCheckInterval is met.
	blockHeight := int64(params.SimpleCheckInterval)
	ctx = ctx.WithBlockHeight(blockHeight)

	hashBytes := make([]byte, 32)
	for i := range hashBytes {
		hashBytes[i] = byte(i)
	}
	ctx = ctx.WithHeaderHash(hashBytes)

	err := k.BeginBlock(ctx)
	require.NoError(t, err)

	// lastSimpleCheckHeight should now be updated to the current block height
	require.Equal(t, blockHeight, k.GetLastSimpleCheckHeight(ctx))
}

func TestBeginBlockSimpleCheckNotTriggeredEarly(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	params := k.GetParams(ctx)

	// Set block height to just before the interval triggers
	blockHeight := int64(params.SimpleCheckInterval) - 1
	ctx = ctx.WithBlockHeight(blockHeight)
	ctx = ctx.WithHeaderHash(make([]byte, 32))

	err := k.BeginBlock(ctx)
	require.NoError(t, err)

	// Should NOT have been updated since interval not reached
	require.Equal(t, int64(0), k.GetLastSimpleCheckHeight(ctx))
}

func TestBeginBlockComplexCheck(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	params := k.GetParams(ctx)

	// Set block height to the complex check interval
	blockHeight := int64(params.ComplexCheckInterval)
	ctx = ctx.WithBlockHeight(blockHeight)

	hashBytes := make([]byte, 32)
	for i := range hashBytes {
		hashBytes[i] = byte(i + 99)
	}
	ctx = ctx.WithHeaderHash(hashBytes)

	require.Equal(t, int64(0), k.GetLastComplexCheckHeight(ctx))

	err := k.BeginBlock(ctx)
	require.NoError(t, err)

	// lastComplexCheckHeight should be updated
	require.Equal(t, blockHeight, k.GetLastComplexCheckHeight(ctx))
}

func TestBeginBlockComplexCheckClearsValuePosts(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	params := k.GetParams(ctx)
	valAddr := "cosmosvaloper1test"

	// Add some value posts
	require.NoError(t, k.AddValuePost(ctx, valAddr, math.NewInt(100), 1))
	require.NoError(t, k.AddValuePost(ctx, valAddr, math.NewInt(200), 2))
	require.Len(t, k.GetValuePosts(ctx, valAddr), 2)

	// Trigger complex check
	blockHeight := int64(params.ComplexCheckInterval)
	ctx = ctx.WithBlockHeight(blockHeight)

	hashBytes := make([]byte, 32)
	for i := range hashBytes {
		hashBytes[i] = byte(i + 50)
	}
	ctx = ctx.WithHeaderHash(hashBytes)

	err := k.BeginBlock(ctx)
	require.NoError(t, err)

	// Value posts should have been cleared by the complex check
	posts := k.GetValuePosts(ctx, valAddr)
	require.Empty(t, posts, "value posts should be cleared after complex check")
}

func TestBeginBlockSimpleAndComplexTogether(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	// Set params so both triggers fire at the same block
	params := types.NewParams(
		math.NewInt(1_000_000),
		uint64(100), // simple check every 100
		uint64(100), // complex check every 100
		uint64(3),
		math.LegacyNewDec(50),
	)
	err := k.SetParams(ctx, params)
	require.NoError(t, err)

	blockHeight := int64(100)
	ctx = ctx.WithBlockHeight(blockHeight)

	hashBytes := make([]byte, 32)
	for i := range hashBytes {
		hashBytes[i] = byte(i + 1)
	}
	ctx = ctx.WithHeaderHash(hashBytes)

	err = k.BeginBlock(ctx)
	require.NoError(t, err)

	// Both should have been updated
	require.Equal(t, blockHeight, k.GetLastSimpleCheckHeight(ctx))
	require.Equal(t, blockHeight, k.GetLastComplexCheckHeight(ctx))
}

func TestBeginBlockSequentialSimpleChecks(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	// Use a small simple check interval for easier testing
	params := types.NewParams(
		math.NewInt(1_000_000),
		uint64(10), // simple check every 10
		uint64(1_000_000), // complex check far out (won't trigger)
		uint64(3),
		math.LegacyNewDec(50),
	)
	err := k.SetParams(ctx, params)
	require.NoError(t, err)

	ctx = ctx.WithHeaderHash(make([]byte, 32))

	// First simple check at block 10
	ctx = ctx.WithBlockHeight(10)
	err = k.BeginBlock(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(10), k.GetLastSimpleCheckHeight(ctx))

	// Block 15 should NOT trigger (only 5 blocks since last)
	ctx = ctx.WithBlockHeight(15)
	err = k.BeginBlock(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(10), k.GetLastSimpleCheckHeight(ctx)) // unchanged

	// Block 20 should trigger (10 blocks since last = interval)
	ctx = ctx.WithBlockHeight(20)
	err = k.BeginBlock(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(20), k.GetLastSimpleCheckHeight(ctx))

	// Block 35 should trigger (15 blocks since last >= 10)
	ctx = ctx.WithBlockHeight(35)
	err = k.BeginBlock(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(35), k.GetLastSimpleCheckHeight(ctx))
}

// ---------------------------------------------------------------------------
// Additional edge case tests
// ---------------------------------------------------------------------------

func TestVaultWithPositions(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	vault := types.Vault{
		ValidatorAddress:       "cosmosvaloper1pos",
		TotalDeposited:         sdk.NewCoin("ubluechip", math.NewInt(5000)),
		DelegatorRewardPercent: math.LegacyNewDec(25),
		Positions: []types.PoolPosition{
			{
				PoolContractAddress: "cosmos1pool1",
				PositionId:          "pos-1",
				DepositAmount0:      math.NewInt(2500),
				DepositAmount1:      math.NewInt(1000),
			},
			{
				PoolContractAddress: "cosmos1pool2",
				PositionId:          "pos-2",
				DepositAmount0:      math.NewInt(2500),
				DepositAmount1:      math.NewInt(2000),
			},
		},
		ValidatorType: types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	}

	err := k.SetVault(ctx, vault)
	require.NoError(t, err)

	got, found := k.GetVault(ctx, "cosmosvaloper1pos")
	require.True(t, found)
	require.Len(t, got.Positions, 2)
	require.Equal(t, "pos-1", got.Positions[0].PositionId)
	require.Equal(t, "pos-2", got.Positions[1].PositionId)
	require.True(t, got.Positions[0].DepositAmount0.Equal(math.NewInt(2500)))
	require.True(t, got.Positions[1].DepositAmount1.Equal(math.NewInt(2000)))
}

func TestValuePostOverwriteAtSameBlockHeight(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	valAddr := "cosmosvaloper1ow"

	// Add two posts at the same block height (same key) -- second should overwrite first
	require.NoError(t, k.AddValuePost(ctx, valAddr, math.NewInt(100), 10))
	require.NoError(t, k.AddValuePost(ctx, valAddr, math.NewInt(999), 10))

	posts := k.GetValuePosts(ctx, valAddr)
	require.Len(t, posts, 1) // key is the same, so overwritten
	require.True(t, posts[0].Value.Equal(math.NewInt(999)))
}

func TestMultipleValidatorsValuePostsIndependent(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	val1 := "cosmosvaloper1ind1"
	val2 := "cosmosvaloper1ind2"
	val3 := "cosmosvaloper1ind3"

	require.NoError(t, k.AddValuePost(ctx, val1, math.NewInt(10), 1))
	require.NoError(t, k.AddValuePost(ctx, val2, math.NewInt(20), 1))
	require.NoError(t, k.AddValuePost(ctx, val2, math.NewInt(30), 2))
	require.NoError(t, k.AddValuePost(ctx, val3, math.NewInt(40), 1))
	require.NoError(t, k.AddValuePost(ctx, val3, math.NewInt(50), 2))
	require.NoError(t, k.AddValuePost(ctx, val3, math.NewInt(60), 3))

	require.Len(t, k.GetValuePosts(ctx, val1), 1)
	require.Len(t, k.GetValuePosts(ctx, val2), 2)
	require.Len(t, k.GetValuePosts(ctx, val3), 3)
}

func TestDeleteVaultDoesNotAffectOtherVaults(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	val1 := "cosmosvaloper1del1"
	val2 := "cosmosvaloper1del2"

	for _, addr := range []string{val1, val2} {
		err := k.SetVault(ctx, types.Vault{
			ValidatorAddress:       addr,
			TotalDeposited:         sdk.NewCoin("ubluechip", math.NewInt(1000)),
			DelegatorRewardPercent: math.LegacyNewDec(50),
			Positions:              []types.PoolPosition{},
			ValidatorType:          types.ValidatorType_VALIDATOR_TYPE_FULL,
		})
		require.NoError(t, err)
	}

	k.DeleteVault(ctx, val1)

	_, found := k.GetVault(ctx, val1)
	require.False(t, found)

	_, found = k.GetVault(ctx, val2)
	require.True(t, found, "deleting val1 should not affect val2")
}

func TestCompositeScoreOverwrite(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	valAddr := "cosmosvaloper1ow"

	score1 := types.CompositeScore{
		ValidatorAddress: valAddr,
		ChainStake:       math.NewInt(100),
		VaultValue:       math.NewInt(50),
	}
	require.NoError(t, k.SetCompositeScore(ctx, score1))

	score2 := types.CompositeScore{
		ValidatorAddress: valAddr,
		ChainStake:       math.NewInt(999),
		VaultValue:       math.NewInt(888),
	}
	require.NoError(t, k.SetCompositeScore(ctx, score2))

	got, found := k.GetCompositeScore(ctx, valAddr)
	require.True(t, found)
	require.True(t, got.ChainStake.Equal(math.NewInt(999)))
	require.True(t, got.VaultValue.Equal(math.NewInt(888)))
}

func TestGetRankedValidatorsMixedTiebreaker(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	// Mix of different stake levels with ties
	scores := []types.CompositeScore{
		{ValidatorAddress: "val1", ChainStake: math.NewInt(100), VaultValue: math.NewInt(50)},
		{ValidatorAddress: "val2", ChainStake: math.NewInt(200), VaultValue: math.NewInt(10)},
		{ValidatorAddress: "val3", ChainStake: math.NewInt(200), VaultValue: math.NewInt(30)},
		{ValidatorAddress: "val4", ChainStake: math.NewInt(300), VaultValue: math.NewInt(5)},
	}
	for _, s := range scores {
		require.NoError(t, k.SetCompositeScore(ctx, s))
	}

	ranked := k.GetRankedValidators(ctx)
	require.Len(t, ranked, 4)

	// ChainStake desc: 300 > 200 (tied) > 100
	// For the tied 200s, VaultValue desc: val3 (30) > val2 (10)
	require.Equal(t, "val4", ranked[0].ValidatorAddress)
	require.Equal(t, "val3", ranked[1].ValidatorAddress)
	require.Equal(t, "val2", ranked[2].ValidatorAddress)
	require.Equal(t, "val1", ranked[3].ValidatorAddress)
}

func TestScheduleValuePostsZeroInterval(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	ctx = ctx.WithHeaderHash(make([]byte, 32))

	// startBlock == endBlock should not schedule anything (intervalSize <= 0)
	k.ScheduleValuePosts(ctx, 100, 100)

	for h := int64(90); h < 110; h++ {
		require.False(t, k.IsValuePostBlock(ctx, h))
	}
}

func TestScheduleValuePostsNegativeInterval(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	ctx = ctx.WithHeaderHash(make([]byte, 32))

	// endBlock < startBlock should not schedule anything
	k.ScheduleValuePosts(ctx, 200, 100)

	for h := int64(90); h < 210; h++ {
		require.False(t, k.IsValuePostBlock(ctx, h))
	}
}
