package keeper

import (
	"context"
	"encoding/json"
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// DepositLiquidityToPool deposits tokens into a pool contract via WasmKeeper.
// It sends ubluechip as native funds and CW20 tokens via CW20 Send hook.
// Returns the position ID from the pool contract.
func (k Keeper) DepositLiquidityToPool(
	ctx context.Context,
	poolContractAddr string,
	cw20ContractAddr string,
	amount0 sdk.Coin, // ubluechip
	amount1 math.Int, // CW20 creator token amount
) (string, error) {
	if k.wasmKeeper == nil {
		return "", types.ErrWasmKeeperNotSet
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	moduleAddr := k.GetModuleAddress()

	cw20Addr, err := sdk.AccAddressFromBech32(cw20ContractAddr)
	if err != nil {
		return "", fmt.Errorf("invalid CW20 contract address: %w", err)
	}

	// Step 1: Send CW20 tokens to the pool via CW20 Send hook.
	// The CW20 Send hook includes the DepositLiquidity inner message.
	innerMsg := types.WasmCw20DepositLiquidityHook{
		DepositLiquidity: &types.Cw20DepositLiquidityInner{
			Amount0: amount0.Amount.String(),
		},
	}
	innerMsgBz, err := json.Marshal(innerMsg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal inner msg: %w", err)
	}

	// Build CW20 Send message with base64-encoded inner hook
	cw20SendMsg := types.WasmCw20SendMsg{
		Send: &types.Cw20SendInner{
			Contract: poolContractAddr,
			Amount:   amount1.String(),
			Msg:      encodeBase64(innerMsgBz),
		},
	}
	cw20SendBz, err := json.Marshal(cw20SendMsg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal CW20 send msg: %w", err)
	}

	// Execute CW20 Send (sends creator tokens to pool with hook)
	_, err = k.wasmKeeper.Execute(sdkCtx, cw20Addr, moduleAddr, cw20SendBz, sdk.NewCoins(amount0))
	if err != nil {
		return "", fmt.Errorf("%w: %v", types.ErrWasmExecutionFailed, err)
	}

	// Query the module's positions to find the newly created one
	positionId, err := k.GetLatestPositionForOwner(ctx, poolContractAddr, moduleAddr.String())
	if err != nil {
		return "", fmt.Errorf("failed to get new position: %w", err)
	}

	return positionId, nil
}

// QueryPositionValue queries the value of an LP position from the pool contract.
func (k Keeper) QueryPositionValue(ctx context.Context, poolContractAddr string, positionId string) (math.Int, error) {
	if k.wasmKeeper == nil {
		return math.ZeroInt(), types.ErrWasmKeeperNotSet
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	poolAddr, err := sdk.AccAddressFromBech32(poolContractAddr)
	if err != nil {
		return math.ZeroInt(), fmt.Errorf("invalid pool address: %w", err)
	}

	// Query position details
	posQuery := types.WasmQueryPositionMsg{
		Position: &types.QueryPositionInner{
			PositionId: positionId,
		},
	}
	posQueryBz, err := json.Marshal(posQuery)
	if err != nil {
		return math.ZeroInt(), err
	}

	posResBz, err := k.wasmKeeper.QuerySmart(sdkCtx, poolAddr, posQueryBz)
	if err != nil {
		return math.ZeroInt(), fmt.Errorf("%w: %v", types.ErrWasmQueryFailed, err)
	}

	var posRes types.WasmPositionResponse
	if err := json.Unmarshal(posResBz, &posRes); err != nil {
		return math.ZeroInt(), fmt.Errorf("failed to unmarshal position response: %w", err)
	}

	// Query pool state to compute value
	stateQuery := types.WasmQueryPoolStateMsg{
		PoolState: &struct{}{},
	}
	stateQueryBz, err := json.Marshal(stateQuery)
	if err != nil {
		return math.ZeroInt(), err
	}

	stateResBz, err := k.wasmKeeper.QuerySmart(sdkCtx, poolAddr, stateQueryBz)
	if err != nil {
		return math.ZeroInt(), fmt.Errorf("%w: %v", types.ErrWasmQueryFailed, err)
	}

	var stateRes types.WasmPoolStateResponse
	if err := json.Unmarshal(stateResBz, &stateRes); err != nil {
		return math.ZeroInt(), fmt.Errorf("failed to unmarshal pool state response: %w", err)
	}

	// Compute position value in ubluechip terms:
	// value = (position_liquidity / total_liquidity) * reserve0 + unclaimed_fees_0
	liquidity, ok := math.NewIntFromString(posRes.Liquidity)
	if !ok {
		return math.ZeroInt(), fmt.Errorf("invalid liquidity value: %s", posRes.Liquidity)
	}

	totalLiquidity, ok := math.NewIntFromString(stateRes.TotalLiquidity)
	if !ok || totalLiquidity.IsZero() {
		return math.ZeroInt(), nil
	}

	reserve0, ok := math.NewIntFromString(stateRes.Reserve0)
	if !ok {
		return math.ZeroInt(), fmt.Errorf("invalid reserve0 value: %s", stateRes.Reserve0)
	}

	unclaimedFees0, ok := math.NewIntFromString(posRes.UnclaimedFees0)
	if !ok {
		unclaimedFees0 = math.ZeroInt()
	}

	// position_value = (liquidity * reserve0 / total_liquidity) + unclaimed_fees_0
	posValue := liquidity.Mul(reserve0).Quo(totalLiquidity).Add(unclaimedFees0)
	return posValue, nil
}

// GetLatestPositionForOwner queries the pool contract for positions owned by the given address
// and returns the latest one (highest position ID).
func (k Keeper) GetLatestPositionForOwner(ctx context.Context, poolContractAddr string, owner string) (string, error) {
	if k.wasmKeeper == nil {
		return "", types.ErrWasmKeeperNotSet
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	poolAddr, err := sdk.AccAddressFromBech32(poolContractAddr)
	if err != nil {
		return "", fmt.Errorf("invalid pool address: %w", err)
	}

	query := types.WasmQueryPositionsByOwnerMsg{
		PositionsByOwner: &types.QueryPositionsByOwnerInner{
			Owner: owner,
		},
	}
	queryBz, err := json.Marshal(query)
	if err != nil {
		return "", err
	}

	resBz, err := k.wasmKeeper.QuerySmart(sdkCtx, poolAddr, queryBz)
	if err != nil {
		return "", fmt.Errorf("%w: %v", types.ErrWasmQueryFailed, err)
	}

	var res types.WasmPositionsResponse
	if err := json.Unmarshal(resBz, &res); err != nil {
		return "", fmt.Errorf("failed to unmarshal positions response: %w", err)
	}

	if len(res.Positions) == 0 {
		return "", fmt.Errorf("no positions found for owner %s", owner)
	}

	// Return the last position (highest ID)
	return res.Positions[len(res.Positions)-1].PositionId, nil
}

// QueryTotalVaultValue calculates the total value of all positions in a vault.
func (k Keeper) QueryTotalVaultValue(ctx context.Context, vault types.Vault) (math.Int, error) {
	totalValue := math.ZeroInt()

	for _, pos := range vault.Positions {
		value, err := k.QueryPositionValue(ctx, pos.PoolContractAddress, pos.PositionId)
		if err != nil {
			k.Logger().Error("failed to query position value",
				"pool", pos.PoolContractAddress,
				"position", pos.PositionId,
				"error", err,
			)
			// Use the deposit amount as fallback
			totalValue = totalValue.Add(pos.DepositAmount0)
			continue
		}
		totalValue = totalValue.Add(value)
	}

	return totalValue, nil
}

// encodeBase64 encodes bytes to base64 string
func encodeBase64(data []byte) string {
	const encodeStd = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	if len(data) == 0 {
		return ""
	}

	result := make([]byte, 0, ((len(data)+2)/3)*4)
	for i := 0; i < len(data); i += 3 {
		var b0, b1, b2 byte
		b0 = data[i]
		if i+1 < len(data) {
			b1 = data[i+1]
		}
		if i+2 < len(data) {
			b2 = data[i+2]
		}

		result = append(result, encodeStd[(b0>>2)&0x3F])
		result = append(result, encodeStd[((b0<<4)|(b1>>4))&0x3F])
		if i+1 < len(data) {
			result = append(result, encodeStd[((b1<<2)|(b2>>6))&0x3F])
		} else {
			result = append(result, '=')
		}
		if i+2 < len(data) {
			result = append(result, encodeStd[b2&0x3F])
		} else {
			result = append(result, '=')
		}
	}
	return string(result)
}
