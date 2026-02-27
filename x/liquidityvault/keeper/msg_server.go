package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns an implementation of the MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

func (k msgServer) UpdateParams(goCtx context.Context, req *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if k.GetAuthority() != req.Authority {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", k.GetAuthority(), req.Authority)
	}

	if err := req.Params.Validate(); err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := k.SetParams(ctx, req.Params); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, nil
}

func (k msgServer) RegisterValidator(goCtx context.Context, req *types.MsgRegisterValidator) (*types.MsgRegisterValidatorResponse, error) {
	if req.ValidatorType == types.ValidatorType_VALIDATOR_TYPE_UNSPECIFIED {
		return nil, types.ErrInvalidValidatorType
	}

	// For Full validators, verify they are registered in the staking module
	if req.ValidatorType == types.ValidatorType_VALIDATOR_TYPE_FULL {
		valAddr, err := sdk.ValAddressFromBech32(req.ValidatorAddress)
		if err != nil {
			return nil, err
		}
		if _, err := k.stakingKeeper.GetValidator(goCtx, valAddr); err != nil {
			return nil, errorsmod.Wrapf(types.ErrNotValidator, "address %s is not a staking validator", req.ValidatorAddress)
		}
	}

	if err := k.CreateVault(goCtx, req.ValidatorAddress, req.ValidatorType); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(goCtx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			"register_validator",
			sdk.NewAttribute("validator_address", req.ValidatorAddress),
			sdk.NewAttribute("validator_type", req.ValidatorType.String()),
		),
	)

	return &types.MsgRegisterValidatorResponse{}, nil
}

func (k msgServer) DepositToVault(goCtx context.Context, req *types.MsgDepositToVault) (*types.MsgDepositToVaultResponse, error) {
	vault, found := k.GetVault(goCtx, req.ValidatorAddress)
	if !found {
		return nil, types.ErrValidatorNotRegistered
	}

	if req.Amount0.IsZero() || req.Amount1.IsZero() {
		return nil, types.ErrInvalidDepositAmount
	}

	sdkCtx := sdk.UnwrapSDKContext(goCtx)
	senderAddr, err := sdk.AccAddressFromBech32(req.ValidatorAddress)
	if err != nil {
		return nil, err
	}

	// Transfer ubluechip from sender to module account
	if err := k.bankKeeper.SendCoinsFromAccountToModule(sdkCtx, senderAddr, types.ModuleName, sdk.NewCoins(req.Amount0)); err != nil {
		return nil, err
	}

	// Deposit into the pool contract via WasmKeeper
	positionId, err := k.DepositLiquidityToPool(
		goCtx,
		req.PoolContractAddress,
		req.Cw20ContractAddress,
		req.Amount0,
		req.Amount1,
	)
	if err != nil {
		return nil, err
	}

	// Update vault state
	vault.TotalDeposited = vault.TotalDeposited.Add(req.Amount0)
	vault.Positions = append(vault.Positions, types.PoolPosition{
		PoolContractAddress: req.PoolContractAddress,
		PositionId:          positionId,
		DepositAmount0:      req.Amount0.Amount,
		DepositAmount1:      req.Amount1,
	})

	if err := k.SetVault(goCtx, vault); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			"deposit_to_vault",
			sdk.NewAttribute("validator_address", req.ValidatorAddress),
			sdk.NewAttribute("pool_contract", req.PoolContractAddress),
			sdk.NewAttribute("amount0", req.Amount0.String()),
			sdk.NewAttribute("amount1", req.Amount1.String()),
			sdk.NewAttribute("position_id", positionId),
		),
	)

	return &types.MsgDepositToVaultResponse{PositionId: positionId}, nil
}

func (k msgServer) SetDelegatorRewardPercent(goCtx context.Context, req *types.MsgSetDelegatorRewardPercent) (*types.MsgSetDelegatorRewardPercentResponse, error) {
	vault, found := k.GetVault(goCtx, req.ValidatorAddress)
	if !found {
		return nil, types.ErrValidatorNotRegistered
	}

	if req.Percent.IsNegative() || req.Percent.GT(math.LegacyNewDec(100)) {
		return nil, types.ErrInvalidDelegatorPercent
	}

	vault.DelegatorRewardPercent = req.Percent

	if err := k.SetVault(goCtx, vault); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(goCtx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			"set_delegator_reward_percent",
			sdk.NewAttribute("validator_address", req.ValidatorAddress),
			sdk.NewAttribute("percent", req.Percent.String()),
		),
	)

	return &types.MsgSetDelegatorRewardPercentResponse{}, nil
}
