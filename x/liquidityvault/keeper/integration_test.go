package keeper_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	keepertest "bluechipChain/testutil/keeper"
	"bluechipChain/x/liquidityvault/keeper"
	"bluechipChain/x/liquidityvault/types"
)

// ===========================================================================
// End-to-End Integration Tests
// ===========================================================================
//
// These tests exercise the full liquidity vault lifecycle through the
// MsgServer, simulating what a user would do on a live chain:
//
//   1. Register a validator
//   2. Deposit to a liquidity pool (via mocked wasm)
//   3. Query position values from the pool
//   4. Execute value posts (periodic on-chain snapshots)
//   5. Compute composite scores and rankings
//
// The WasmKeeper is mocked to simulate real pool contract responses.

// ---------------------------------------------------------------------------
// Helper: build mock wasm responses for position + pool state queries
// ---------------------------------------------------------------------------

// mockSmartQueryRouter sets up the MockWasmKeeper to return different
// responses based on the query message content (position vs pool_state).
// This is needed because QueryPositionValue makes two sequential queries
// to the same contract address.
type smartQueryRouter struct {
	positionResponse *types.WasmPositionResponse
	poolState        *types.WasmPoolStateResponse
	positionsForOwner *types.WasmPositionsResponse
	callCount        int
}

func (r *smartQueryRouter) buildQueryResults() []byte {
	// QueryPositionValue calls QuerySmart twice:
	//   1st call: position query
	//   2nd call: pool_state query
	// Since MockWasmKeeper uses a simple map keyed by contract address,
	// we need a different approach. We'll set up the responses sequentially.
	r.callCount++
	if r.callCount%2 == 1 {
		// First call: position query
		bz, _ := json.Marshal(r.positionResponse)
		return bz
	}
	// Second call: pool state query
	bz, _ := json.Marshal(r.poolState)
	return bz
}

// SequentialMockWasmKeeper extends the basic mock to support sequential
// query responses (needed for QueryPositionValue which makes 2 queries).
type SequentialMockWasmKeeper struct {
	ExecuteResults [][]byte
	ExecuteErrors  []error
	QueryResponses [][]byte // responses returned in order
	QueryErrors    []error
	executeCalls   int
	queryCalls     int
}

func (m *SequentialMockWasmKeeper) Execute(_ sdk.Context, _ sdk.AccAddress, _ sdk.AccAddress, _ []byte, _ sdk.Coins) ([]byte, error) {
	idx := m.executeCalls
	m.executeCalls++
	if idx < len(m.ExecuteErrors) && m.ExecuteErrors[idx] != nil {
		return nil, m.ExecuteErrors[idx]
	}
	if idx < len(m.ExecuteResults) {
		return m.ExecuteResults[idx], nil
	}
	return []byte(`{}`), nil
}

func (m *SequentialMockWasmKeeper) QuerySmart(_ context.Context, _ sdk.AccAddress, _ []byte) ([]byte, error) {
	idx := m.queryCalls
	m.queryCalls++
	if idx < len(m.QueryErrors) && m.QueryErrors[idx] != nil {
		return nil, m.QueryErrors[idx]
	}
	if idx < len(m.QueryResponses) {
		return m.QueryResponses[idx], nil
	}
	return []byte(`{}`), nil
}

// setupKeeperWithSequentialWasm creates a keeper with the sequential mock.
func setupKeeperWithSequentialWasm(t testing.TB) (keeper.Keeper, sdk.Context, *keepertest.MockBankKeeper, *keepertest.MockStakingKeeper, *SequentialMockWasmKeeper) {
	k, ctx, bk, sk, _ := keepertest.LiquidityvaultKeeper(t)
	wk := &SequentialMockWasmKeeper{}
	k.SetWasmKeeper(wk)
	return k, ctx, bk, sk, wk
}

// ---------------------------------------------------------------------------
// Test: QueryPositionValue with successful mock responses
// ---------------------------------------------------------------------------

func TestQueryPositionValue_SuccessfulCalculation(t *testing.T) {
	tests := []struct {
		name           string
		liquidity      string
		unclaimedFees0 string
		reserve0       string
		totalLiquidity string
		expectedValue  math.Int
	}{
		{
			name:           "10% of pool with no fees",
			liquidity:      "100",
			unclaimedFees0: "0",
			reserve0:       "10000",
			totalLiquidity: "1000",
			expectedValue:  math.NewInt(1000), // 100 * 10000 / 1000 + 0
		},
		{
			name:           "10% of pool with 50 unclaimed fees",
			liquidity:      "100",
			unclaimedFees0: "50",
			reserve0:       "10000",
			totalLiquidity: "1000",
			expectedValue:  math.NewInt(1050), // 100 * 10000 / 1000 + 50
		},
		{
			name:           "50% ownership",
			liquidity:      "500",
			unclaimedFees0: "0",
			reserve0:       "20000",
			totalLiquidity: "1000",
			expectedValue:  math.NewInt(10000), // 500 * 20000 / 1000
		},
		{
			name:           "100% ownership with fees",
			liquidity:      "1000",
			unclaimedFees0: "250",
			reserve0:       "50000",
			totalLiquidity: "1000",
			expectedValue:  math.NewInt(50250), // 1000 * 50000 / 1000 + 250
		},
		{
			name:           "large values",
			liquidity:      "1000000",
			unclaimedFees0: "5000",
			reserve0:       "100000000",
			totalLiquidity: "10000000",
			expectedValue:  math.NewInt(10005000), // 1M * 100M / 10M + 5000
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k, ctx, _, _, wk := setupKeeperWithSequentialWasm(t)

			poolAddr := sdk.AccAddress([]byte("pool_contract_____"))

			// Set up sequential responses: position query first, then pool state
			positionResp, _ := json.Marshal(types.WasmPositionResponse{
				PositionId:     "pos-1",
				Liquidity:      tc.liquidity,
				Owner:          "cosmos1owner",
				UnclaimedFees0: tc.unclaimedFees0,
				UnclaimedFees1: "0",
			})
			poolStateResp, _ := json.Marshal(types.WasmPoolStateResponse{
				Reserve0:       tc.reserve0,
				Reserve1:       "0",
				TotalLiquidity: tc.totalLiquidity,
			})

			wk.QueryResponses = [][]byte{positionResp, poolStateResp}

			value, err := k.QueryPositionValue(ctx, poolAddr.String(), "pos-1")
			require.NoError(t, err)
			require.True(t, value.Equal(tc.expectedValue),
				"expected %s, got %s", tc.expectedValue, value)
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Full E2E - Register → Deposit → Query Value → Value Post → Score
// ---------------------------------------------------------------------------

func TestE2E_RegisterDepositQueryScoreRanking(t *testing.T) {
	k, ctx, _, sk, wk := setupKeeperWithSequentialWasm(t)
	msgServer := keeper.NewMsgServerImpl(k)

	// --- Setup: two validators with different staking power ---
	valAddr1 := sdk.ValAddress([]byte("val_e2e_alpha_____"))
	valAddr2 := sdk.ValAddress([]byte("val_e2e_beta______"))
	sk.AddValidator(valAddr1, math.NewInt(200_000)) // high stake
	sk.AddValidator(valAddr2, math.NewInt(100_000)) // low stake

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))

	// --- Step 1: Register both validators ---
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr1.String(),
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_FULL,
	})
	require.NoError(t, err)

	_, err = msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr2.String(),
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_FULL,
	})
	require.NoError(t, err)

	// Verify both vaults exist with defaults
	vault1, found := k.GetVault(ctx, valAddr1.String())
	require.True(t, found)
	require.True(t, vault1.TotalDeposited.Amount.IsZero())
	require.True(t, vault1.DelegatorRewardPercent.Equal(math.LegacyNewDec(50)))

	vault2, found := k.GetVault(ctx, valAddr2.String())
	require.True(t, found)
	require.True(t, vault2.TotalDeposited.Amount.IsZero())

	// --- Step 2: Deposit to vaults ---
	// For deposit, the msg_server uses AccAddressFromBech32 on the validator address,
	// so we set up vaults using AccAddress-compatible addresses for deposit tests.
	// But for staking/composite scores we need ValAddress. We'll deposit directly
	// to the vaults by manipulating state, which is what happens on-chain.

	// Simulate deposits by setting vault state directly (since DepositToVault
	// uses AccAddressFromBech32 on the validator address which differs from ValAddress)
	vault1.TotalDeposited = sdk.NewCoin("ubluechip", math.NewInt(5000))
	vault1.Positions = []types.PoolPosition{
		{
			PoolContractAddress: poolAddr.String(),
			PositionId:          "pos-alpha-1",
			DepositAmount0:      math.NewInt(5000),
			DepositAmount1:      math.NewInt(2500),
		},
	}
	require.NoError(t, k.SetVault(ctx, vault1))

	vault2.TotalDeposited = sdk.NewCoin("ubluechip", math.NewInt(15000))
	vault2.Positions = []types.PoolPosition{
		{
			PoolContractAddress: poolAddr.String(),
			PositionId:          "pos-beta-1",
			DepositAmount0:      math.NewInt(8000),
			DepositAmount1:      math.NewInt(4000),
		},
		{
			PoolContractAddress: poolAddr.String(),
			PositionId:          "pos-beta-2",
			DepositAmount0:      math.NewInt(7000),
			DepositAmount1:      math.NewInt(3500),
		},
	}
	require.NoError(t, k.SetVault(ctx, vault2))

	// --- Step 3: Query position values via mocked wasm ---
	// Validator 1's position: 100 liquidity, pool has 10000 reserve, 1000 total liquidity
	// Value = (100 * 10000 / 1000) + 200 = 1200
	posResp1, _ := json.Marshal(types.WasmPositionResponse{
		PositionId: "pos-alpha-1", Liquidity: "100",
		UnclaimedFees0: "200", UnclaimedFees1: "0",
	})
	poolStateResp, _ := json.Marshal(types.WasmPoolStateResponse{
		Reserve0: "10000", Reserve1: "5000", TotalLiquidity: "1000",
	})

	wk.QueryResponses = [][]byte{posResp1, poolStateResp}
	value1, err := k.QueryPositionValue(ctx, poolAddr.String(), "pos-alpha-1")
	require.NoError(t, err)
	require.True(t, value1.Equal(math.NewInt(1200)),
		"expected position value 1200, got %s", value1)

	// --- Step 4: Execute value posts ---
	ctx = ctx.WithBlockHeight(100)
	require.NoError(t, k.AddValuePost(ctx, valAddr1.String(), math.NewInt(5000), 100))
	require.NoError(t, k.AddValuePost(ctx, valAddr2.String(), math.NewInt(15000), 100))

	// --- Step 5: Calculate composite scores ---
	err = k.ExecuteSimpleCheck(ctx)
	require.NoError(t, err)

	score1, found := k.GetCompositeScore(ctx, valAddr1.String())
	require.True(t, found)
	require.True(t, score1.ChainStake.Equal(math.NewInt(200_000)),
		"expected ChainStake 200000, got %s", score1.ChainStake)
	require.True(t, score1.VaultValue.Equal(math.NewInt(5000)),
		"expected VaultValue 5000, got %s", score1.VaultValue)

	score2, found := k.GetCompositeScore(ctx, valAddr2.String())
	require.True(t, found)
	require.True(t, score2.ChainStake.Equal(math.NewInt(100_000)))
	require.True(t, score2.VaultValue.Equal(math.NewInt(15000)))

	// --- Step 6: Check rankings ---
	rankings := k.GetRankedValidators(ctx)
	require.Len(t, rankings, 2)
	// val1 has 200k chain stake vs val2's 100k → val1 ranks first
	require.Equal(t, valAddr1.String(), rankings[0].ValidatorAddress)
	require.Equal(t, valAddr2.String(), rankings[1].ValidatorAddress)

	// --- Step 7: Update delegator reward percent ---
	_, err = msgServer.SetDelegatorRewardPercent(ctx, &types.MsgSetDelegatorRewardPercent{
		ValidatorAddress: valAddr1.String(),
		Percent:          math.LegacyNewDec(75),
	})
	require.NoError(t, err)

	vault1Updated, found := k.GetVault(ctx, valAddr1.String())
	require.True(t, found)
	require.True(t, vault1Updated.DelegatorRewardPercent.Equal(math.LegacyNewDec(75)))
	// Positions should still be intact
	require.Len(t, vault1Updated.Positions, 1)
	require.Equal(t, "pos-alpha-1", vault1Updated.Positions[0].PositionId)
}

// ---------------------------------------------------------------------------
// Test: Deposit via MsgServer with position value verification
// ---------------------------------------------------------------------------

func TestE2E_DepositAndQueryTotalVaultValue(t *testing.T) {
	k, ctx, _, sk, wk := setupKeeperWithSequentialWasm(t)
	msgServer := keeper.NewMsgServerImpl(k)

	valAddr := accAddrStr("val_e2e_value")

	// Add validator to staking module (padded to 20 bytes to match accAddrStr)
	valBytes := make([]byte, 20)
	copy(valBytes, "val_e2e_value")
	sk.AddValidator(sdk.ValAddress(valBytes), math.NewInt(1000000))

	// Register
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	cw20Addr := sdk.AccAddress([]byte("cw20_contract_____"))

	// First deposit: 5000 ubluechip + 2500 CW20
	wk.QueryResponses = [][]byte{
		mustJSON(types.WasmPositionsResponse{
			Positions: []types.WasmPositionResponse{
				{PositionId: "pos-1", Liquidity: "5000"},
			},
		}),
	}

	resp, err := msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
		ValidatorAddress:    valAddr,
		PoolContractAddress: poolAddr.String(),
		Cw20ContractAddress: cw20Addr.String(),
		Amount0:             sdk.NewCoin("ubluechip", math.NewInt(5000)),
		Amount1:             math.NewInt(2500),
	})
	require.NoError(t, err)
	require.Equal(t, "pos-1", resp.PositionId)

	// Second deposit: 3000 ubluechip + 1500 CW20
	wk.QueryResponses = append(wk.QueryResponses,
		mustJSON(types.WasmPositionsResponse{
			Positions: []types.WasmPositionResponse{
				{PositionId: "pos-1", Liquidity: "5000"},
				{PositionId: "pos-2", Liquidity: "3000"},
			},
		}),
	)

	resp, err = msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
		ValidatorAddress:    valAddr,
		PoolContractAddress: poolAddr.String(),
		Cw20ContractAddress: cw20Addr.String(),
		Amount0:             sdk.NewCoin("ubluechip", math.NewInt(3000)),
		Amount1:             math.NewInt(1500),
	})
	require.NoError(t, err)
	require.Equal(t, "pos-2", resp.PositionId)

	// Verify vault state
	vault, found := k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.True(t, vault.TotalDeposited.Amount.Equal(math.NewInt(8000)),
		"expected total deposited 8000, got %s", vault.TotalDeposited.Amount)
	require.Len(t, vault.Positions, 2)

	// Query total vault value with simulated pool state
	// Position 1: 500 liquidity, pool 100k reserve, 10k total → value = 5000 + 100 fees = 5100
	// Position 2: 300 liquidity, pool 100k reserve, 10k total → value = 3000 + 50 fees = 3050
	wk.QueryResponses = append(wk.QueryResponses,
		// Position 1 query
		mustJSON(types.WasmPositionResponse{
			PositionId: "pos-1", Liquidity: "500",
			UnclaimedFees0: "100", UnclaimedFees1: "0",
		}),
		// Pool state for position 1
		mustJSON(types.WasmPoolStateResponse{
			Reserve0: "100000", Reserve1: "50000", TotalLiquidity: "10000",
		}),
		// Position 2 query
		mustJSON(types.WasmPositionResponse{
			PositionId: "pos-2", Liquidity: "300",
			UnclaimedFees0: "50", UnclaimedFees1: "0",
		}),
		// Pool state for position 2
		mustJSON(types.WasmPoolStateResponse{
			Reserve0: "100000", Reserve1: "50000", TotalLiquidity: "10000",
		}),
	)

	totalValue, err := k.QueryTotalVaultValue(ctx, vault)
	require.NoError(t, err)
	// pos-1: (500 * 100000 / 10000) + 100 = 5100
	// pos-2: (300 * 100000 / 10000) + 50 = 3050
	// Total: 8150
	require.True(t, totalValue.Equal(math.NewInt(8150)),
		"expected total vault value 8150, got %s", totalValue)
}

// ---------------------------------------------------------------------------
// Test: Multiple validators compete after depositing to same pool
// ---------------------------------------------------------------------------

func TestE2E_MultipleValidatorsSamePool(t *testing.T) {
	k, ctx, _, sk, _ := keepertest.LiquidityvaultKeeper(t)
	msgServer := keeper.NewMsgServerImpl(k)

	poolAddr := sdk.AccAddress([]byte("shared_pool_______"))

	// Create 3 validators with different staking power
	validators := []struct {
		addr   sdk.ValAddress
		stake  int64
		deposit int64
	}{
		{sdk.ValAddress([]byte("val_multi_comp_1__")), 300_000, 2000},
		{sdk.ValAddress([]byte("val_multi_comp_2__")), 200_000, 8000},
		{sdk.ValAddress([]byte("val_multi_comp_3__")), 100_000, 5000},
	}

	for _, v := range validators {
		sk.AddValidator(v.addr, math.NewInt(v.stake))

		// Register
		_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
			ValidatorAddress: v.addr.String(),
			ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_FULL,
		})
		require.NoError(t, err)

		// Simulate deposit by setting vault state
		vault, found := k.GetVault(ctx, v.addr.String())
		require.True(t, found)
		vault.TotalDeposited = sdk.NewCoin("ubluechip", math.NewInt(v.deposit))
		vault.Positions = []types.PoolPosition{
			{
				PoolContractAddress: poolAddr.String(),
				PositionId:          fmt.Sprintf("pos-%s", v.addr.String()[:8]),
				DepositAmount0:      math.NewInt(v.deposit),
				DepositAmount1:      math.NewInt(v.deposit / 2),
			},
		}
		require.NoError(t, k.SetVault(ctx, vault))

		// Add value posts
		require.NoError(t, k.AddValuePost(ctx, v.addr.String(), math.NewInt(v.deposit), 50))
	}

	// Calculate composite scores
	ctx = ctx.WithBlockHeight(100)
	err := k.ExecuteSimpleCheck(ctx)
	require.NoError(t, err)

	// Get rankings
	rankings := k.GetRankedValidators(ctx)
	require.Len(t, rankings, 3)

	// Rankings should be by chain stake (primary):
	// val1: 300k stake → rank 1
	// val2: 200k stake → rank 2
	// val3: 100k stake → rank 3
	require.Equal(t, validators[0].addr.String(), rankings[0].ValidatorAddress,
		"300k stake should rank first")
	require.Equal(t, validators[1].addr.String(), rankings[1].ValidatorAddress,
		"200k stake should rank second")
	require.Equal(t, validators[2].addr.String(), rankings[2].ValidatorAddress,
		"100k stake should rank third")

	// Verify vault values are correctly recorded in scores
	for i, v := range validators {
		score, found := k.GetCompositeScore(ctx, v.addr.String())
		require.True(t, found)
		require.True(t, score.ChainStake.Equal(math.NewInt(v.stake)),
			"validator %d: expected ChainStake %d, got %s", i, v.stake, score.ChainStake)
		require.True(t, score.VaultValue.Equal(math.NewInt(v.deposit)),
			"validator %d: expected VaultValue %d, got %s", i, v.deposit, score.VaultValue)
	}
}

// ---------------------------------------------------------------------------
// Test: BeginBlock triggers the full cycle
// ---------------------------------------------------------------------------

func TestE2E_BeginBlockDrivesFullCycle(t *testing.T) {
	k, ctx, _, sk, _ := keepertest.LiquidityvaultKeeper(t)

	// Configure short intervals for testing
	params := types.NewParams(
		math.NewInt(1_000_000),
		uint64(5),   // simple check every 5 blocks
		uint64(10),  // complex check every 10 blocks
		uint64(2),   // 2 value posts per interval
		math.LegacyNewDec(50),
	)
	require.NoError(t, k.SetParams(ctx, params))

	// Set up a validator with vault
	valAddr := sdk.ValAddress([]byte("val_beginblk_e2e__"))
	sk.AddValidator(valAddr, math.NewInt(50_000))
	require.NoError(t, k.CreateVault(ctx, valAddr.String(), types.ValidatorType_VALIDATOR_TYPE_FULL))

	vault, _ := k.GetVault(ctx, valAddr.String())
	vault.TotalDeposited = sdk.NewCoin("ubluechip", math.NewInt(3000))
	vault.Positions = []types.PoolPosition{
		{
			PoolContractAddress: "cosmos1pool",
			PositionId:          "pos-1",
			DepositAmount0:      math.NewInt(3000),
			DepositAmount1:      math.NewInt(1500),
		},
	}
	require.NoError(t, k.SetVault(ctx, vault))

	// Add some value posts
	require.NoError(t, k.AddValuePost(ctx, valAddr.String(), math.NewInt(3000), 1))

	// Simulate block 5: should trigger simple check
	hashBytes := make([]byte, 32)
	for i := range hashBytes {
		hashBytes[i] = byte(i)
	}
	ctx = ctx.WithBlockHeight(5).WithHeaderHash(hashBytes)
	err := k.BeginBlock(ctx)
	require.NoError(t, err)

	// Simple check should have computed composite score
	score, found := k.GetCompositeScore(ctx, valAddr.String())
	require.True(t, found)
	require.True(t, score.ChainStake.Equal(math.NewInt(50_000)))
	require.True(t, score.VaultValue.Equal(math.NewInt(3000)))

	// Simulate block 10: should trigger complex check
	ctx = ctx.WithBlockHeight(10).WithHeaderHash(hashBytes)
	err = k.BeginBlock(ctx)
	require.NoError(t, err)

	// Complex check should have cleared value posts
	posts := k.GetValuePosts(ctx, valAddr.String())
	require.Empty(t, posts, "complex check should clear value posts")

	// Score should still be computed
	score, found = k.GetCompositeScore(ctx, valAddr.String())
	require.True(t, found)
	require.True(t, score.ChainStake.Equal(math.NewInt(50_000)))
}

// ---------------------------------------------------------------------------
// Test: QueryPositionValue returns zero for zero total liquidity
// ---------------------------------------------------------------------------

func TestQueryPositionValue_ZeroTotalLiquidity(t *testing.T) {
	k, ctx, _, _, wk := setupKeeperWithSequentialWasm(t)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))

	positionResp, _ := json.Marshal(types.WasmPositionResponse{
		PositionId: "pos-1", Liquidity: "100",
		UnclaimedFees0: "0", UnclaimedFees1: "0",
	})
	poolStateResp, _ := json.Marshal(types.WasmPoolStateResponse{
		Reserve0: "10000", Reserve1: "5000", TotalLiquidity: "0",
	})

	wk.QueryResponses = [][]byte{positionResp, poolStateResp}

	value, err := k.QueryPositionValue(ctx, poolAddr.String(), "pos-1")
	require.NoError(t, err)
	require.True(t, value.IsZero(), "zero total liquidity should return zero value")
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

func mustJSON(v interface{}) []byte {
	bz, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return bz
}
