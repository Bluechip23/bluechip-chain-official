package keeper_test

import (
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
// Standard Liquidity Pool Tests
// ===========================================================================
//
// These tests cover the core liquidity pool operations:
//   1. Deposit liquidity to pool (wasm execution flow)
//   2. Query position value calculations
//   3. Query total vault value across positions
//   4. Full lifecycle: deposit → value post → composite score → ranking
//   5. Edge cases in pool value calculations

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupKeeperWithWasm returns a keeper with mock wasm, bank, staking, and account keepers.
func setupKeeperWithWasm(t testing.TB) (keeper.Keeper, sdk.Context, *keepertest.MockBankKeeper, *keepertest.MockStakingKeeper, *keepertest.MockAccountKeeper, *keepertest.MockWasmKeeper) {
	return keepertest.LiquidityvaultKeeperWithWasm(t)
}

// setupMsgServerWithWasm returns a keeper with wasm enabled and a MsgServer.
func setupMsgServerWithWasm(t testing.TB) (keeper.Keeper, types.MsgServer, sdk.Context, *keepertest.MockBankKeeper, *keepertest.MockStakingKeeper, *keepertest.MockWasmKeeper) {
	k, ctx, bk, sk, _, wk := setupKeeperWithWasm(t)
	return k, keeper.NewMsgServerImpl(k), ctx, bk, sk, wk
}

// mockPositionsByOwnerResponse creates a JSON response for a positions_by_owner query.
func mockPositionsByOwnerResponse(positions ...types.WasmPositionResponse) []byte {
	resp := types.WasmPositionsResponse{Positions: positions}
	bz, _ := json.Marshal(resp)
	return bz
}

// accAddrStr creates a deterministic AccAddress string from a label (for deposit tests).
// DepositToVault parses req.ValidatorAddress via sdk.AccAddressFromBech32, so deposit
// tests must use AccAddress (cosmos prefix), not ValAddress (cosmosvaloper prefix).
func accAddrStr(label string) string {
	// Pad or truncate to 20 bytes for a consistent address length
	b := make([]byte, 20)
	copy(b, label)
	return sdk.AccAddress(b).String()
}

// ---------------------------------------------------------------------------
// 1. DepositLiquidityToPool
// ---------------------------------------------------------------------------

func TestDepositLiquidityToPool_Success(t *testing.T) {
	k, ctx, _, _, _, wk := setupKeeperWithWasm(t)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	cw20Addr := sdk.AccAddress([]byte("cw20_contract_____"))

	// Mock: the execute call succeeds, then the query returns the new position
	wk.QueryResults[poolAddr.String()] = mockPositionsByOwnerResponse(
		types.WasmPositionResponse{PositionId: "pos-42", Liquidity: "1000"},
	)

	positionId, err := k.DepositLiquidityToPool(
		ctx,
		poolAddr.String(),
		cw20Addr.String(),
		sdk.NewCoin("ubluechip", math.NewInt(5000)),
		math.NewInt(2500),
	)
	require.NoError(t, err)
	require.Equal(t, "pos-42", positionId)

	// Verify wasm execute was called once
	require.Equal(t, 1, wk.ExecuteCalls)

	// Verify wasm query was called to get the position
	require.Equal(t, 1, wk.QueryCalls)
}

func TestDepositLiquidityToPool_WasmKeeperNotSet(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	// Don't set wasm keeper — it should return ErrWasmKeeperNotSet
	_, err := k.DepositLiquidityToPool(
		ctx,
		"cosmos1pool",
		"cosmos1cw20",
		sdk.NewCoin("ubluechip", math.NewInt(1000)),
		math.NewInt(500),
	)
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrWasmKeeperNotSet)
}

func TestDepositLiquidityToPool_ExecuteFails(t *testing.T) {
	k, ctx, _, _, _, wk := setupKeeperWithWasm(t)

	cw20Addr := sdk.AccAddress([]byte("cw20_contract_____"))

	// Set up execute to fail
	wk.ExecuteErrors = []error{fmt.Errorf("out of gas")}

	_, err := k.DepositLiquidityToPool(
		ctx,
		sdk.AccAddress([]byte("pool_contract_____")).String(),
		cw20Addr.String(),
		sdk.NewCoin("ubluechip", math.NewInt(1000)),
		math.NewInt(500),
	)
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrWasmExecutionFailed)
}

func TestDepositLiquidityToPool_InvalidCw20Address(t *testing.T) {
	k, ctx, _, _, _, _ := setupKeeperWithWasm(t)

	_, err := k.DepositLiquidityToPool(
		ctx,
		sdk.AccAddress([]byte("pool_contract_____")).String(),
		"not_a_valid_address!!!",
		sdk.NewCoin("ubluechip", math.NewInt(1000)),
		math.NewInt(500),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid CW20 contract address")
}

func TestDepositLiquidityToPool_NoPositionsReturned(t *testing.T) {
	k, ctx, _, _, _, wk := setupKeeperWithWasm(t)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	cw20Addr := sdk.AccAddress([]byte("cw20_contract_____"))

	// Mock: execute succeeds but query returns empty positions
	wk.QueryResults[poolAddr.String()] = mockPositionsByOwnerResponse()

	_, err := k.DepositLiquidityToPool(
		ctx,
		poolAddr.String(),
		cw20Addr.String(),
		sdk.NewCoin("ubluechip", math.NewInt(1000)),
		math.NewInt(500),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no positions found")
}

func TestDepositLiquidityToPool_MultiplePositions_ReturnsLatest(t *testing.T) {
	k, ctx, _, _, _, wk := setupKeeperWithWasm(t)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	cw20Addr := sdk.AccAddress([]byte("cw20_contract_____"))

	// Mock: query returns multiple positions, should pick the last one
	wk.QueryResults[poolAddr.String()] = mockPositionsByOwnerResponse(
		types.WasmPositionResponse{PositionId: "pos-1", Liquidity: "100"},
		types.WasmPositionResponse{PositionId: "pos-2", Liquidity: "200"},
		types.WasmPositionResponse{PositionId: "pos-3", Liquidity: "300"},
	)

	positionId, err := k.DepositLiquidityToPool(
		ctx,
		poolAddr.String(),
		cw20Addr.String(),
		sdk.NewCoin("ubluechip", math.NewInt(1000)),
		math.NewInt(500),
	)
	require.NoError(t, err)
	require.Equal(t, "pos-3", positionId, "should return the latest (last) position")
}

// ---------------------------------------------------------------------------
// 2. QueryPositionValue
// ---------------------------------------------------------------------------

func TestQueryPositionValue_WasmKeeperNotSet(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	value, err := k.QueryPositionValue(ctx, "cosmos1pool", "pos-1")
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrWasmKeeperNotSet)
	require.True(t, value.IsZero())
}

func TestQueryPositionValue_InvalidPoolAddress(t *testing.T) {
	k, ctx, _, _, _, _ := setupKeeperWithWasm(t)

	value, err := k.QueryPositionValue(ctx, "not_valid!!!", "pos-1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid pool address")
	require.True(t, value.IsZero())
}

func TestQueryPositionValue_QueryFails(t *testing.T) {
	k, ctx, _, _, _, wk := setupKeeperWithWasm(t)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	wk.QueryErrors[poolAddr.String()] = fmt.Errorf("contract not found")

	value, err := k.QueryPositionValue(ctx, poolAddr.String(), "pos-1")
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrWasmQueryFailed)
	require.True(t, value.IsZero())
}

// ---------------------------------------------------------------------------
// 3. QueryTotalVaultValue
// ---------------------------------------------------------------------------

func TestQueryTotalVaultValue_EmptyPositions(t *testing.T) {
	k, ctx, _, _, _, _ := setupKeeperWithWasm(t)

	vault := types.Vault{
		ValidatorAddress:       "cosmosvaloper1test",
		TotalDeposited:         sdk.NewCoin("ubluechip", math.NewInt(10000)),
		DelegatorRewardPercent: math.LegacyNewDec(50),
		Positions:              []types.PoolPosition{},
		ValidatorType:          types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	}

	totalValue, err := k.QueryTotalVaultValue(ctx, vault)
	require.NoError(t, err)
	require.True(t, totalValue.IsZero(), "vault with no positions should have zero value")
}

func TestQueryTotalVaultValue_FallbackOnQueryError(t *testing.T) {
	k, ctx, _, _, _, wk := setupKeeperWithWasm(t)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))

	// Make the query fail - the keeper should fall back to deposit amounts
	wk.QueryErrors[poolAddr.String()] = fmt.Errorf("contract error")

	vault := types.Vault{
		ValidatorAddress:       "cosmosvaloper1test",
		TotalDeposited:         sdk.NewCoin("ubluechip", math.NewInt(10000)),
		DelegatorRewardPercent: math.LegacyNewDec(50),
		Positions: []types.PoolPosition{
			{
				PoolContractAddress: poolAddr.String(),
				PositionId:          "pos-1",
				DepositAmount0:      math.NewInt(3000),
				DepositAmount1:      math.NewInt(1500),
			},
			{
				PoolContractAddress: poolAddr.String(),
				PositionId:          "pos-2",
				DepositAmount0:      math.NewInt(7000),
				DepositAmount1:      math.NewInt(3500),
			},
		},
		ValidatorType: types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	}

	totalValue, err := k.QueryTotalVaultValue(ctx, vault)
	require.NoError(t, err)
	// Fallback: 3000 + 7000 = 10000
	require.True(t, totalValue.Equal(math.NewInt(10000)),
		"fallback should sum deposit amounts; got %s", totalValue)
}

func TestQueryTotalVaultValue_WasmKeeperNotSet(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	vault := types.Vault{
		ValidatorAddress:       "cosmosvaloper1test",
		TotalDeposited:         sdk.NewCoin("ubluechip", math.NewInt(10000)),
		DelegatorRewardPercent: math.LegacyNewDec(50),
		Positions: []types.PoolPosition{
			{
				PoolContractAddress: "cosmos1pool",
				PositionId:          "pos-1",
				DepositAmount0:      math.NewInt(5000),
				DepositAmount1:      math.NewInt(2500),
			},
		},
		ValidatorType: types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	}

	// Without wasm keeper, it should fallback to deposit amounts
	totalValue, err := k.QueryTotalVaultValue(ctx, vault)
	require.NoError(t, err)
	require.True(t, totalValue.Equal(math.NewInt(5000)),
		"should fallback to deposit amount; got %s", totalValue)
}

// ---------------------------------------------------------------------------
// 4. Full Deposit Flow via MsgServer
// ---------------------------------------------------------------------------

func TestMsgDepositToVault_FullFlow(t *testing.T) {
	k, msgServer, ctx, _, _, wk := setupMsgServerWithWasm(t)

	// Use AccAddress because DepositToVault calls AccAddressFromBech32
	valAddr := accAddrStr("val_deposit_flow")

	// Register as LIQUIDITY type (doesn't need staking module validation)
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	// Verify vault starts empty
	vault, found := k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.True(t, vault.TotalDeposited.Amount.IsZero())
	require.Empty(t, vault.Positions)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	cw20Addr := sdk.AccAddress([]byte("cw20_contract_____"))

	// Mock wasm: execute succeeds, query returns the new position
	wk.QueryResults[poolAddr.String()] = mockPositionsByOwnerResponse(
		types.WasmPositionResponse{PositionId: "pos-100", Liquidity: "5000"},
	)

	// Deposit
	depositAmount0 := sdk.NewCoin("ubluechip", math.NewInt(5000))
	depositAmount1 := math.NewInt(2500)

	resp, err := msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
		ValidatorAddress:    valAddr,
		PoolContractAddress: poolAddr.String(),
		Cw20ContractAddress: cw20Addr.String(),
		Amount0:             depositAmount0,
		Amount1:             depositAmount1,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "pos-100", resp.PositionId)

	// Verify vault was updated
	vault, found = k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.True(t, vault.TotalDeposited.Amount.Equal(math.NewInt(5000)),
		"total deposited should be 5000; got %s", vault.TotalDeposited.Amount)
	require.Len(t, vault.Positions, 1)
	require.Equal(t, "pos-100", vault.Positions[0].PositionId)
	require.Equal(t, poolAddr.String(), vault.Positions[0].PoolContractAddress)
	require.True(t, vault.Positions[0].DepositAmount0.Equal(math.NewInt(5000)))
	require.True(t, vault.Positions[0].DepositAmount1.Equal(math.NewInt(2500)))
}

func TestMsgDepositToVault_MultipleDeposits(t *testing.T) {
	k, msgServer, ctx, _, _, wk := setupMsgServerWithWasm(t)

	valAddr := accAddrStr("val_multi_dep")

	// Register
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	cw20Addr := sdk.AccAddress([]byte("cw20_contract_____"))

	// First deposit
	wk.QueryResults[poolAddr.String()] = mockPositionsByOwnerResponse(
		types.WasmPositionResponse{PositionId: "pos-1", Liquidity: "1000"},
	)

	resp1, err := msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
		ValidatorAddress:    valAddr,
		PoolContractAddress: poolAddr.String(),
		Cw20ContractAddress: cw20Addr.String(),
		Amount0:             sdk.NewCoin("ubluechip", math.NewInt(1000)),
		Amount1:             math.NewInt(500),
	})
	require.NoError(t, err)
	require.Equal(t, "pos-1", resp1.PositionId)

	// Second deposit (to same pool, but yields new position)
	wk.QueryResults[poolAddr.String()] = mockPositionsByOwnerResponse(
		types.WasmPositionResponse{PositionId: "pos-1", Liquidity: "1000"},
		types.WasmPositionResponse{PositionId: "pos-2", Liquidity: "2000"},
	)

	resp2, err := msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
		ValidatorAddress:    valAddr,
		PoolContractAddress: poolAddr.String(),
		Cw20ContractAddress: cw20Addr.String(),
		Amount0:             sdk.NewCoin("ubluechip", math.NewInt(2000)),
		Amount1:             math.NewInt(1000),
	})
	require.NoError(t, err)
	require.Equal(t, "pos-2", resp2.PositionId)

	// Verify vault state
	vault, found := k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.True(t, vault.TotalDeposited.Amount.Equal(math.NewInt(3000)),
		"expected 3000; got %s", vault.TotalDeposited.Amount)
	require.Len(t, vault.Positions, 2)
	require.Equal(t, "pos-1", vault.Positions[0].PositionId)
	require.Equal(t, "pos-2", vault.Positions[1].PositionId)
}

func TestMsgDepositToVault_BankSendRecorded(t *testing.T) {
	_, msgServer, ctx, bk, _, wk := setupMsgServerWithWasm(t)

	valAddr := accAddrStr("val_bank_send")

	// Register
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	cw20Addr := sdk.AccAddress([]byte("cw20_contract_____"))

	wk.QueryResults[poolAddr.String()] = mockPositionsByOwnerResponse(
		types.WasmPositionResponse{PositionId: "pos-1", Liquidity: "1000"},
	)

	initialSentCount := len(bk.SentCoins)

	_, err = msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
		ValidatorAddress:    valAddr,
		PoolContractAddress: poolAddr.String(),
		Cw20ContractAddress: cw20Addr.String(),
		Amount0:             sdk.NewCoin("ubluechip", math.NewInt(3000)),
		Amount1:             math.NewInt(1500),
	})
	require.NoError(t, err)

	// Verify bank transfer was recorded
	require.Greater(t, len(bk.SentCoins), initialSentCount, "bank send should be recorded")
	lastSend := bk.SentCoins[len(bk.SentCoins)-1]
	require.Equal(t, types.ModuleName, lastSend.To)
	require.True(t, lastSend.Amount.Equal(sdk.NewCoins(sdk.NewCoin("ubluechip", math.NewInt(3000)))))
}

func TestMsgDepositToVault_BankSendFails(t *testing.T) {
	_, msgServer, ctx, bk, _, _ := setupMsgServerWithWasm(t)

	valAddr := accAddrStr("val_bank_fail")

	// Register
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	// Make bank send fail
	bk.FailOnSend = true

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	cw20Addr := sdk.AccAddress([]byte("cw20_contract_____"))

	resp, err := msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
		ValidatorAddress:    valAddr,
		PoolContractAddress: poolAddr.String(),
		Cw20ContractAddress: cw20Addr.String(),
		Amount0:             sdk.NewCoin("ubluechip", math.NewInt(1000)),
		Amount1:             math.NewInt(500),
	})
	require.Error(t, err)
	require.Nil(t, resp)
}

func TestMsgDepositToVault_WasmExecuteFails(t *testing.T) {
	k, msgServer, ctx, _, _, wk := setupMsgServerWithWasm(t)

	valAddr := accAddrStr("val_wasm_fail")

	// Register
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	// Make wasm execute fail
	wk.ExecuteErrors = []error{fmt.Errorf("wasm execution error")}

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	cw20Addr := sdk.AccAddress([]byte("cw20_contract_____"))

	resp, err := msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
		ValidatorAddress:    valAddr,
		PoolContractAddress: poolAddr.String(),
		Cw20ContractAddress: cw20Addr.String(),
		Amount0:             sdk.NewCoin("ubluechip", math.NewInt(1000)),
		Amount1:             math.NewInt(500),
	})
	require.Error(t, err)
	require.Nil(t, resp)
	require.ErrorIs(t, err, types.ErrWasmExecutionFailed)

	// Vault should NOT have been updated (deposit failed)
	vault, found := k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.True(t, vault.TotalDeposited.Amount.IsZero(), "vault should be unchanged on failure")
	require.Empty(t, vault.Positions, "no positions should be added on failure")
}

// ---------------------------------------------------------------------------
// 5. GetLatestPositionForOwner
// ---------------------------------------------------------------------------

func TestGetLatestPositionForOwner_WasmKeeperNotSet(t *testing.T) {
	k, ctx, _, _, _ := keepertest.LiquidityvaultKeeper(t)

	_, err := k.GetLatestPositionForOwner(ctx, "cosmos1pool", "cosmos1owner")
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrWasmKeeperNotSet)
}

func TestGetLatestPositionForOwner_InvalidPoolAddress(t *testing.T) {
	k, ctx, _, _, _, _ := setupKeeperWithWasm(t)

	_, err := k.GetLatestPositionForOwner(ctx, "invalid!!!", "cosmos1owner")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid pool address")
}

func TestGetLatestPositionForOwner_QueryFails(t *testing.T) {
	k, ctx, _, _, _, wk := setupKeeperWithWasm(t)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	wk.QueryErrors[poolAddr.String()] = fmt.Errorf("contract not found")

	_, err := k.GetLatestPositionForOwner(ctx, poolAddr.String(), "cosmos1owner")
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrWasmQueryFailed)
}

func TestGetLatestPositionForOwner_EmptyPositions(t *testing.T) {
	k, ctx, _, _, _, wk := setupKeeperWithWasm(t)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	wk.QueryResults[poolAddr.String()] = mockPositionsByOwnerResponse()

	_, err := k.GetLatestPositionForOwner(ctx, poolAddr.String(), "cosmos1owner")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no positions found")
}

func TestGetLatestPositionForOwner_SinglePosition(t *testing.T) {
	k, ctx, _, _, _, wk := setupKeeperWithWasm(t)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	wk.QueryResults[poolAddr.String()] = mockPositionsByOwnerResponse(
		types.WasmPositionResponse{PositionId: "only-pos"},
	)

	posId, err := k.GetLatestPositionForOwner(ctx, poolAddr.String(), "cosmos1owner")
	require.NoError(t, err)
	require.Equal(t, "only-pos", posId)
}

// ---------------------------------------------------------------------------
// 6. ExecuteValuePost (liquidity pool value recording)
// ---------------------------------------------------------------------------

func TestExecuteValuePost_RecordsVaultValues(t *testing.T) {
	k, ctx, _, _, _, wk := setupKeeperWithWasm(t)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))

	// Create two vaults with positions
	vault1 := types.Vault{
		ValidatorAddress:       "cosmosvaloper1val1",
		TotalDeposited:         sdk.NewCoin("ubluechip", math.NewInt(5000)),
		DelegatorRewardPercent: math.LegacyNewDec(50),
		Positions: []types.PoolPosition{
			{
				PoolContractAddress: poolAddr.String(),
				PositionId:          "pos-1",
				DepositAmount0:      math.NewInt(5000),
				DepositAmount1:      math.NewInt(2500),
			},
		},
		ValidatorType: types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	}
	require.NoError(t, k.SetVault(ctx, vault1))

	vault2 := types.Vault{
		ValidatorAddress:       "cosmosvaloper1val2",
		TotalDeposited:         sdk.NewCoin("ubluechip", math.NewInt(3000)),
		DelegatorRewardPercent: math.LegacyNewDec(50),
		Positions: []types.PoolPosition{
			{
				PoolContractAddress: poolAddr.String(),
				PositionId:          "pos-2",
				DepositAmount0:      math.NewInt(3000),
				DepositAmount1:      math.NewInt(1500),
			},
		},
		ValidatorType: types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	}
	require.NoError(t, k.SetVault(ctx, vault2))

	// Set up context with block height
	ctx = ctx.WithBlockHeight(100)

	// When wasm queries fail, the keeper falls back to deposit amounts
	wk.QueryErrors[poolAddr.String()] = fmt.Errorf("query error")

	err := k.ExecuteValuePost(ctx)
	require.NoError(t, err)

	// Check value posts were recorded
	posts1 := k.GetValuePosts(ctx, "cosmosvaloper1val1")
	require.Len(t, posts1, 1)
	require.True(t, posts1[0].Value.Equal(math.NewInt(5000)),
		"fallback value should equal total deposited; got %s", posts1[0].Value)
	require.Equal(t, int64(100), posts1[0].BlockHeight)

	posts2 := k.GetValuePosts(ctx, "cosmosvaloper1val2")
	require.Len(t, posts2, 1)
	require.True(t, posts2[0].Value.Equal(math.NewInt(3000)),
		"fallback value should equal total deposited; got %s", posts2[0].Value)
}

func TestExecuteValuePost_EmptyVault(t *testing.T) {
	k, ctx, _, _, _, _ := setupKeeperWithWasm(t)

	// Create a vault with no positions
	vault := types.Vault{
		ValidatorAddress:       "cosmosvaloper1empty",
		TotalDeposited:         sdk.NewCoin("ubluechip", math.ZeroInt()),
		DelegatorRewardPercent: math.LegacyNewDec(50),
		Positions:              []types.PoolPosition{},
		ValidatorType:          types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	}
	require.NoError(t, k.SetVault(ctx, vault))

	ctx = ctx.WithBlockHeight(50)

	err := k.ExecuteValuePost(ctx)
	require.NoError(t, err)

	// Value post should record zero value
	posts := k.GetValuePosts(ctx, "cosmosvaloper1empty")
	require.Len(t, posts, 1)
	require.True(t, posts[0].Value.IsZero())
}

// ---------------------------------------------------------------------------
// 7. Full Lifecycle: Deposit → Value Post → Composite Score → Ranking
// ---------------------------------------------------------------------------

func TestFullLiquidityPoolLifecycle(t *testing.T) {
	k, ctx, _, sk, _ := keepertest.LiquidityvaultKeeper(t)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))

	// Create two validators with ValAddress (needed by CalculateCompositeScore)
	valAddr1 := sdk.ValAddress([]byte("val_lifecycle_1___"))
	valAddr2 := sdk.ValAddress([]byte("val_lifecycle_2___"))

	sk.AddValidator(valAddr1, math.NewInt(100_000))
	sk.AddValidator(valAddr2, math.NewInt(50_000))

	// Create vaults
	require.NoError(t, k.CreateVault(ctx, valAddr1.String(), types.ValidatorType_VALIDATOR_TYPE_FULL))
	require.NoError(t, k.CreateVault(ctx, valAddr2.String(), types.ValidatorType_VALIDATOR_TYPE_FULL))

	// Manually set positions on the vaults (simulating deposits)
	vault1, _ := k.GetVault(ctx, valAddr1.String())
	vault1.TotalDeposited = sdk.NewCoin("ubluechip", math.NewInt(5000))
	vault1.Positions = []types.PoolPosition{
		{
			PoolContractAddress: poolAddr.String(),
			PositionId:          "pos-v1",
			DepositAmount0:      math.NewInt(5000),
			DepositAmount1:      math.NewInt(2500),
		},
	}
	require.NoError(t, k.SetVault(ctx, vault1))

	vault2, _ := k.GetVault(ctx, valAddr2.String())
	vault2.TotalDeposited = sdk.NewCoin("ubluechip", math.NewInt(10000))
	vault2.Positions = []types.PoolPosition{
		{
			PoolContractAddress: poolAddr.String(),
			PositionId:          "pos-v2",
			DepositAmount0:      math.NewInt(10000),
			DepositAmount1:      math.NewInt(5000),
		},
	}
	require.NoError(t, k.SetVault(ctx, vault2))

	// Add value posts for the vaults
	ctx = ctx.WithBlockHeight(100)
	require.NoError(t, k.AddValuePost(ctx, valAddr1.String(), math.NewInt(5000), 100))
	require.NoError(t, k.AddValuePost(ctx, valAddr2.String(), math.NewInt(10000), 100))

	// Execute simple check to compute composite scores
	err := k.ExecuteSimpleCheck(ctx)
	require.NoError(t, err)

	// Validator 1: ChainStake=100000, VaultValue=5000
	score1, found := k.GetCompositeScore(ctx, valAddr1.String())
	require.True(t, found)
	require.True(t, score1.ChainStake.Equal(math.NewInt(100_000)),
		"expected ChainStake 100000; got %s", score1.ChainStake)
	require.True(t, score1.VaultValue.Equal(math.NewInt(5000)),
		"expected VaultValue 5000; got %s", score1.VaultValue)

	// Validator 2: ChainStake=50000, VaultValue=10000
	score2, found := k.GetCompositeScore(ctx, valAddr2.String())
	require.True(t, found)
	require.True(t, score2.ChainStake.Equal(math.NewInt(50_000)),
		"expected ChainStake 50000; got %s", score2.ChainStake)
	require.True(t, score2.VaultValue.Equal(math.NewInt(10000)),
		"expected VaultValue 10000; got %s", score2.VaultValue)

	// Get rankings: validator 1 should rank higher (chain stake 100000 > 50000)
	rankings := k.GetRankedValidators(ctx)
	require.Len(t, rankings, 2)
	require.Equal(t, valAddr1.String(), rankings[0].ValidatorAddress,
		"validator with higher chain stake should rank first")
	require.Equal(t, valAddr2.String(), rankings[1].ValidatorAddress)
}

func TestLifecycle_MultipleValuePosts_MedianUsed(t *testing.T) {
	k, ctx, _, sk, _ := keepertest.LiquidityvaultKeeper(t)

	// Use a proper ValAddress for CalculateCompositeScore
	valAddr := sdk.ValAddress([]byte("val_median________"))
	sk.AddValidator(valAddr, math.NewInt(1000))

	// Create vault using the ValAddress string
	err := k.CreateVault(ctx, valAddr.String(), types.ValidatorType_VALIDATOR_TYPE_FULL)
	require.NoError(t, err)

	// Add multiple value posts with different values
	require.NoError(t, k.AddValuePost(ctx, valAddr.String(), math.NewInt(100), 10))
	require.NoError(t, k.AddValuePost(ctx, valAddr.String(), math.NewInt(300), 20))
	require.NoError(t, k.AddValuePost(ctx, valAddr.String(), math.NewInt(200), 30))
	require.NoError(t, k.AddValuePost(ctx, valAddr.String(), math.NewInt(500), 40))
	require.NoError(t, k.AddValuePost(ctx, valAddr.String(), math.NewInt(400), 50))

	// Calculate composite score — should use median of value posts
	score, err := k.CalculateCompositeScore(ctx, valAddr.String())
	require.NoError(t, err)

	// Sorted values: 100, 200, 300, 400, 500 → median = 300
	require.True(t, score.VaultValue.Equal(math.NewInt(300)),
		"expected median 300; got %s", score.VaultValue)
	// ChainStake should come from the staking module
	require.True(t, score.ChainStake.Equal(math.NewInt(1000)),
		"expected ChainStake 1000; got %s", score.ChainStake)
}

func TestLifecycle_NoValuePosts_FallbackToDeposited(t *testing.T) {
	k, ctx, _, sk, _ := keepertest.LiquidityvaultKeeper(t)

	valAddr := sdk.ValAddress([]byte("val_fallback______"))
	sk.AddValidator(valAddr, math.NewInt(500))

	// Create vault with some deposited amount but no value posts
	vault := types.Vault{
		ValidatorAddress:       valAddr.String(),
		TotalDeposited:         sdk.NewCoin("ubluechip", math.NewInt(7777)),
		DelegatorRewardPercent: math.LegacyNewDec(50),
		Positions:              []types.PoolPosition{},
		ValidatorType:          types.ValidatorType_VALIDATOR_TYPE_FULL,
	}
	require.NoError(t, k.SetVault(ctx, vault))

	// Calculate composite score — should fallback to TotalDeposited
	score, err := k.CalculateCompositeScore(ctx, valAddr.String())
	require.NoError(t, err)
	require.True(t, score.VaultValue.Equal(math.NewInt(7777)),
		"expected fallback to 7777; got %s", score.VaultValue)
}

// ---------------------------------------------------------------------------
// 8. Ranking with Vault Values as Tiebreaker
// ---------------------------------------------------------------------------

func TestRanking_VaultValueTiebreaker(t *testing.T) {
	k, ctx, _, sk, _ := keepertest.LiquidityvaultKeeper(t)

	// Create two validators with equal chain stake but different vault values
	valAddr1 := sdk.ValAddress([]byte("val_rank_tie_1____"))
	valAddr2 := sdk.ValAddress([]byte("val_rank_tie_2____"))

	sk.AddValidator(valAddr1, math.NewInt(50000))
	sk.AddValidator(valAddr2, math.NewInt(50000))

	// Create vaults
	require.NoError(t, k.CreateVault(ctx, valAddr1.String(), types.ValidatorType_VALIDATOR_TYPE_FULL))
	require.NoError(t, k.CreateVault(ctx, valAddr2.String(), types.ValidatorType_VALIDATOR_TYPE_FULL))

	// Add value posts: val1 has lower vault value
	require.NoError(t, k.AddValuePost(ctx, valAddr1.String(), math.NewInt(1000), 10))
	require.NoError(t, k.AddValuePost(ctx, valAddr2.String(), math.NewInt(9000), 10))

	// Calculate and store composite scores
	for _, addr := range []string{valAddr1.String(), valAddr2.String()} {
		score, err := k.CalculateCompositeScore(ctx, addr)
		require.NoError(t, err)
		require.NoError(t, k.SetCompositeScore(ctx, score))
	}

	// Get rankings
	rankings := k.GetRankedValidators(ctx)
	require.Len(t, rankings, 2)

	// Both have ChainStake=50000, so VaultValue is the tiebreaker
	// val2 (9000) should rank above val1 (1000)
	require.Equal(t, valAddr2.String(), rankings[0].ValidatorAddress,
		"validator with higher vault value should rank first when chain stake is tied")
	require.Equal(t, valAddr1.String(), rankings[1].ValidatorAddress)
}

func TestRanking_ChainStakeDominates(t *testing.T) {
	k, ctx, _, sk, _ := keepertest.LiquidityvaultKeeper(t)

	// val1 has high chain stake but low vault value
	valAddr1 := sdk.ValAddress([]byte("val_stake_hi______"))
	sk.AddValidator(valAddr1, math.NewInt(1_000_000))

	// val2 has low chain stake but high vault value
	valAddr2 := sdk.ValAddress([]byte("val_stake_lo______"))
	sk.AddValidator(valAddr2, math.NewInt(100))

	require.NoError(t, k.CreateVault(ctx, valAddr1.String(), types.ValidatorType_VALIDATOR_TYPE_FULL))
	require.NoError(t, k.CreateVault(ctx, valAddr2.String(), types.ValidatorType_VALIDATOR_TYPE_FULL))

	// val1: low vault value, val2: high vault value
	require.NoError(t, k.AddValuePost(ctx, valAddr1.String(), math.NewInt(10), 10))
	require.NoError(t, k.AddValuePost(ctx, valAddr2.String(), math.NewInt(999_999), 10))

	for _, addr := range []string{valAddr1.String(), valAddr2.String()} {
		score, err := k.CalculateCompositeScore(ctx, addr)
		require.NoError(t, err)
		require.NoError(t, k.SetCompositeScore(ctx, score))
	}

	rankings := k.GetRankedValidators(ctx)
	require.Len(t, rankings, 2)

	// Chain stake (primary) dominates: val1 (1M) > val2 (100)
	require.Equal(t, valAddr1.String(), rankings[0].ValidatorAddress,
		"validator with higher chain stake should rank first regardless of vault value")
}

// ---------------------------------------------------------------------------
// 9. Deposit to Multiple Pools
// ---------------------------------------------------------------------------

func TestMsgDepositToVault_MultiplePools(t *testing.T) {
	k, msgServer, ctx, _, _, wk := setupMsgServerWithWasm(t)

	valAddr := accAddrStr("val_multi_pool")

	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	pool1 := sdk.AccAddress([]byte("pool_1____________"))
	pool2 := sdk.AccAddress([]byte("pool_2____________"))
	cw20 := sdk.AccAddress([]byte("cw20_contract_____"))

	// Deposit to pool 1
	wk.QueryResults[pool1.String()] = mockPositionsByOwnerResponse(
		types.WasmPositionResponse{PositionId: "p1-pos-1", Liquidity: "1000"},
	)

	_, err = msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
		ValidatorAddress:    valAddr,
		PoolContractAddress: pool1.String(),
		Cw20ContractAddress: cw20.String(),
		Amount0:             sdk.NewCoin("ubluechip", math.NewInt(1000)),
		Amount1:             math.NewInt(500),
	})
	require.NoError(t, err)

	// Deposit to pool 2
	wk.QueryResults[pool2.String()] = mockPositionsByOwnerResponse(
		types.WasmPositionResponse{PositionId: "p2-pos-1", Liquidity: "2000"},
	)

	_, err = msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
		ValidatorAddress:    valAddr,
		PoolContractAddress: pool2.String(),
		Cw20ContractAddress: cw20.String(),
		Amount0:             sdk.NewCoin("ubluechip", math.NewInt(2000)),
		Amount1:             math.NewInt(1000),
	})
	require.NoError(t, err)

	// Verify vault has positions from both pools
	vault, found := k.GetVault(ctx, valAddr)
	require.True(t, found)
	require.Len(t, vault.Positions, 2)
	require.Equal(t, pool1.String(), vault.Positions[0].PoolContractAddress)
	require.Equal(t, "p1-pos-1", vault.Positions[0].PositionId)
	require.Equal(t, pool2.String(), vault.Positions[1].PoolContractAddress)
	require.Equal(t, "p2-pos-1", vault.Positions[1].PositionId)

	// Total deposited should sum both
	require.True(t, vault.TotalDeposited.Amount.Equal(math.NewInt(3000)),
		"expected 3000; got %s", vault.TotalDeposited.Amount)
}

// ---------------------------------------------------------------------------
// 10. BeginBlock Integration with Liquidity Values
// ---------------------------------------------------------------------------

func TestBeginBlock_ComplexCheckWithLiquidityVaults(t *testing.T) {
	k, ctx, _, sk, _ := keepertest.LiquidityvaultKeeper(t)

	// Set up params with small intervals for testing
	params := types.NewParams(
		math.NewInt(1_000_000),
		uint64(10),
		uint64(20),
		uint64(3),
		math.LegacyNewDec(50),
	)
	require.NoError(t, k.SetParams(ctx, params))

	// Create validators and vaults
	valAddr1 := sdk.ValAddress([]byte("val_begin_blk_1___"))
	valAddr2 := sdk.ValAddress([]byte("val_begin_blk_2___"))

	sk.AddValidator(valAddr1, math.NewInt(200_000))
	sk.AddValidator(valAddr2, math.NewInt(100_000))

	require.NoError(t, k.CreateVault(ctx, valAddr1.String(), types.ValidatorType_VALIDATOR_TYPE_FULL))
	require.NoError(t, k.CreateVault(ctx, valAddr2.String(), types.ValidatorType_VALIDATOR_TYPE_FULL))

	// Add value posts
	require.NoError(t, k.AddValuePost(ctx, valAddr1.String(), math.NewInt(8000), 5))
	require.NoError(t, k.AddValuePost(ctx, valAddr2.String(), math.NewInt(12000), 5))

	// Trigger complex check
	hashBytes := make([]byte, 32)
	for i := range hashBytes {
		hashBytes[i] = byte(i + 1)
	}
	ctx = ctx.WithBlockHeight(20).WithHeaderHash(hashBytes)

	err := k.BeginBlock(ctx)
	require.NoError(t, err)

	// After complex check:
	// - Composite scores should be calculated
	// - Value posts should be cleared
	// - New value posts should be scheduled

	// Verify value posts were cleared
	require.Empty(t, k.GetValuePosts(ctx, valAddr1.String()))
	require.Empty(t, k.GetValuePosts(ctx, valAddr2.String()))

	// Verify composite scores exist
	score1, found := k.GetCompositeScore(ctx, valAddr1.String())
	require.True(t, found)
	require.True(t, score1.ChainStake.Equal(math.NewInt(200_000)))

	score2, found := k.GetCompositeScore(ctx, valAddr2.String())
	require.True(t, found)
	require.True(t, score2.ChainStake.Equal(math.NewInt(100_000)))

	// Verify rankings: val1 (200k) > val2 (100k)
	rankings := k.GetRankedValidators(ctx)
	require.Len(t, rankings, 2)
	require.Equal(t, valAddr1.String(), rankings[0].ValidatorAddress)
	require.Equal(t, valAddr2.String(), rankings[1].ValidatorAddress)
}

// ---------------------------------------------------------------------------
// 11. Position Value Calculation Math
// ---------------------------------------------------------------------------

func TestPositionValueCalculation_Manual(t *testing.T) {
	// Test the math: value = (liquidity * reserve0 / total_liquidity) + unclaimed_fees_0
	// This tests the formula used in QueryPositionValue

	tests := []struct {
		name           string
		liquidity      math.Int
		reserve0       math.Int
		totalLiquidity math.Int
		unclaimedFees0 math.Int
		expected       math.Int
	}{
		{
			name:           "basic proportional value",
			liquidity:      math.NewInt(100),
			reserve0:       math.NewInt(10000),
			totalLiquidity: math.NewInt(1000),
			unclaimedFees0: math.ZeroInt(),
			expected:       math.NewInt(1000), // 100 * 10000 / 1000 = 1000
		},
		{
			name:           "with unclaimed fees",
			liquidity:      math.NewInt(100),
			reserve0:       math.NewInt(10000),
			totalLiquidity: math.NewInt(1000),
			unclaimedFees0: math.NewInt(50),
			expected:       math.NewInt(1050), // 1000 + 50
		},
		{
			name:           "50% of pool",
			liquidity:      math.NewInt(500),
			reserve0:       math.NewInt(20000),
			totalLiquidity: math.NewInt(1000),
			unclaimedFees0: math.ZeroInt(),
			expected:       math.NewInt(10000), // 500 * 20000 / 1000 = 10000
		},
		{
			name:           "small position in large pool",
			liquidity:      math.NewInt(1),
			reserve0:       math.NewInt(1000000),
			totalLiquidity: math.NewInt(1000),
			unclaimedFees0: math.ZeroInt(),
			expected:       math.NewInt(1000), // 1 * 1000000 / 1000
		},
		{
			name:           "integer truncation",
			liquidity:      math.NewInt(1),
			reserve0:       math.NewInt(10),
			totalLiquidity: math.NewInt(3),
			unclaimedFees0: math.ZeroInt(),
			expected:       math.NewInt(3), // 1 * 10 / 3 = 3 (truncated from 3.33)
		},
		{
			name:           "full ownership of pool",
			liquidity:      math.NewInt(1000),
			reserve0:       math.NewInt(50000),
			totalLiquidity: math.NewInt(1000),
			unclaimedFees0: math.NewInt(100),
			expected:       math.NewInt(50100), // 1000 * 50000 / 1000 + 100
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Replicate the calculation from wasm.go:QueryPositionValue
			posValue := tc.liquidity.Mul(tc.reserve0).Quo(tc.totalLiquidity).Add(tc.unclaimedFees0)
			require.True(t, posValue.Equal(tc.expected),
				"expected %s, got %s", tc.expected, posValue)
		})
	}
}

// ---------------------------------------------------------------------------
// 12. Vault Queries After Pool Operations
// ---------------------------------------------------------------------------

func TestQueryVault_AfterDeposit(t *testing.T) {
	k, msgServer, ctx, _, _, wk := setupMsgServerWithWasm(t)

	valAddr := accAddrStr("val_query_dep")
	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	cw20Addr := sdk.AccAddress([]byte("cw20_contract_____"))

	// Register
	_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
		ValidatorAddress: valAddr,
		ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
	})
	require.NoError(t, err)

	// Deposit
	wk.QueryResults[poolAddr.String()] = mockPositionsByOwnerResponse(
		types.WasmPositionResponse{PositionId: "pos-99", Liquidity: "5000"},
	)

	_, err = msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
		ValidatorAddress:    valAddr,
		PoolContractAddress: poolAddr.String(),
		Cw20ContractAddress: cw20Addr.String(),
		Amount0:             sdk.NewCoin("ubluechip", math.NewInt(5000)),
		Amount1:             math.NewInt(2500),
	})
	require.NoError(t, err)

	// Query vault via gRPC query handler
	resp, err := k.Vault(ctx, &types.QueryVaultRequest{
		ValidatorAddress: valAddr,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Vault)
	require.Equal(t, valAddr, resp.Vault.ValidatorAddress)
	require.True(t, resp.Vault.TotalDeposited.Amount.Equal(math.NewInt(5000)))
	require.Len(t, resp.Vault.Positions, 1)
	require.Equal(t, "pos-99", resp.Vault.Positions[0].PositionId)
}

func TestQueryAllVaults_AfterMultipleDeposits(t *testing.T) {
	k, msgServer, ctx, _, _, wk := setupMsgServerWithWasm(t)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	cw20Addr := sdk.AccAddress([]byte("cw20_contract_____"))

	valAddrs := []string{
		accAddrStr("val_all_1"),
		accAddrStr("val_all_2"),
		accAddrStr("val_all_3"),
	}

	for i, valAddr := range valAddrs {
		// Register
		_, err := msgServer.RegisterValidator(ctx, &types.MsgRegisterValidator{
			ValidatorAddress: valAddr,
			ValidatorType:    types.ValidatorType_VALIDATOR_TYPE_LIQUIDITY,
		})
		require.NoError(t, err)

		// Deposit
		posId := fmt.Sprintf("pos-%d", i+1)
		wk.QueryResults[poolAddr.String()] = mockPositionsByOwnerResponse(
			types.WasmPositionResponse{PositionId: posId, Liquidity: "1000"},
		)

		_, err = msgServer.DepositToVault(ctx, &types.MsgDepositToVault{
			ValidatorAddress:    valAddr,
			PoolContractAddress: poolAddr.String(),
			Cw20ContractAddress: cw20Addr.String(),
			Amount0:             sdk.NewCoin("ubluechip", math.NewInt(int64((i+1)*1000))),
			Amount1:             math.NewInt(int64((i + 1) * 500)),
		})
		require.NoError(t, err)
	}

	// Query all vaults
	resp, err := k.AllVaults(ctx, &types.QueryAllVaultsRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Vaults, 3)
	require.Equal(t, uint64(3), resp.Pagination.Total)

	// Verify each vault has one position
	for _, vault := range resp.Vaults {
		require.Len(t, vault.Positions, 1, "each vault should have 1 position")
	}
}

// ---------------------------------------------------------------------------
// 13. encodeBase64 correctness
// ---------------------------------------------------------------------------

func TestEncodeBase64_ViaDepositCall(t *testing.T) {
	// Ensure that the deposit flow doesn't panic from base64 encoding.
	// The encodeBase64 function is internal, but we test it through the
	// deposit flow which must correctly base64-encode the CW20 hook message.
	k, ctx, _, _, _, wk := setupKeeperWithWasm(t)

	poolAddr := sdk.AccAddress([]byte("pool_contract_____"))
	cw20Addr := sdk.AccAddress([]byte("cw20_contract_____"))

	wk.QueryResults[poolAddr.String()] = mockPositionsByOwnerResponse(
		types.WasmPositionResponse{PositionId: "pos-b64", Liquidity: "1000"},
	)

	// Various amounts to exercise different base64 padding scenarios
	amounts := []math.Int{
		math.NewInt(1),
		math.NewInt(100),
		math.NewInt(12345),
		math.NewInt(999999999),
	}

	for _, amt := range amounts {
		wk.ExecuteCalls = 0
		wk.QueryCalls = 0

		posId, err := k.DepositLiquidityToPool(
			ctx,
			poolAddr.String(),
			cw20Addr.String(),
			sdk.NewCoin("ubluechip", amt),
			amt,
		)
		require.NoError(t, err, "amount: %s", amt)
		require.Equal(t, "pos-b64", posId)
		require.Equal(t, 1, wk.ExecuteCalls, "should call execute once per deposit")
	}
}
