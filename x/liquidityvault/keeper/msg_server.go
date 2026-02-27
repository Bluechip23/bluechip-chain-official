package keeper

import (
	"context"
	"fmt"

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

// resolveValAddress attempts to resolve a validator operator address from the given string.
// It accepts both account address format (cosmos1...) and valoper format (cosmosvaloper1...).
func resolveValAddress(addr string) (sdk.ValAddress, error) {
	// Try valoper format first
	valAddr, err := sdk.ValAddressFromBech32(addr)
	if err == nil {
		return valAddr, nil
	}
	// Try account address format and convert to valoper
	accAddr, err2 := sdk.AccAddressFromBech32(addr)
	if err2 == nil {
		return sdk.ValAddress(accAddr), nil
	}
	return nil, fmt.Errorf("invalid address %s: not a valid account or validator address", addr)
}

// resolveAccAddress attempts to resolve an account address from the given string.
// It accepts both account address format (cosmos1...) and valoper format (cosmosvaloper1...).
func resolveAccAddress(addr string) (sdk.AccAddress, error) {
	// Try account address format first
	accAddr, err := sdk.AccAddressFromBech32(addr)
	if err == nil {
		return accAddr, nil
	}
	// Try valoper format and convert to account address
	valAddr, err2 := sdk.ValAddressFromBech32(addr)
	if err2 == nil {
		return sdk.AccAddress(valAddr), nil
	}
	return nil, fmt.Errorf("invalid address %s: not a valid account or validator address", addr)
}

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

	// Validate the address is a proper bech32 address (accepts both acc and valoper formats)
	valAddr, err := resolveValAddress(req.ValidatorAddress)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "%s", err)
	}

	// For both FULL and LIQUIDITY validators, verify they are registered in the staking module.
	// This prevents griefing attacks where someone registers vaults for addresses they don't control,
	// and ensures all vault participants are legitimate validators.
	if _, err := k.stakingKeeper.GetValidator(goCtx, valAddr); err != nil {
		return nil, errorsmod.Wrapf(types.ErrNotValidator, "address %s is not a registered staking validator", req.ValidatorAddress)
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

	// Validate address format (defense-in-depth; cosmos.msg.v1.signer already enforces signer)
	senderAddr, err := resolveAccAddress(req.ValidatorAddress)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "%s", err)
	}

	// Enforce StakeCap: check if this deposit would exceed the validator's stake cap
	params := k.GetParams(goCtx)
	if !params.StakeCap.IsZero() {
		newTotal := vault.TotalDeposited.Amount.Add(req.Amount0.Amount)
		if newTotal.GT(params.StakeCap) {
			return nil, errorsmod.Wrapf(types.ErrStakeCapExceeded,
				"deposit of %s would bring total to %s, exceeding stake cap of %s",
				req.Amount0.Amount, newTotal, params.StakeCap)
		}
	}

	sdkCtx := sdk.UnwrapSDKContext(goCtx)

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
	// Validate address format (defense-in-depth; cosmos.msg.v1.signer already enforces signer)
	if _, err := resolveAccAddress(req.ValidatorAddress); err != nil {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "%s", err)
	}

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

func (k msgServer) WithdrawFromVault(goCtx context.Context, req *types.MsgWithdrawFromVault) (*types.MsgWithdrawFromVaultResponse, error) {
	// Validate address format (defense-in-depth; cosmos.msg.v1.signer already enforces signer)
	validatorAddr, err := resolveAccAddress(req.ValidatorAddress)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "%s", err)
	}

	vault, found := k.GetVault(goCtx, req.ValidatorAddress)
	if !found {
		return nil, types.ErrValidatorNotRegistered
	}

	// Find the position in the vault
	posIdx := -1
	for i, pos := range vault.Positions {
		if pos.PoolContractAddress == req.PoolContractAddress && pos.PositionId == req.PositionId {
			posIdx = i
			break
		}
	}
	if posIdx == -1 {
		return nil, errorsmod.Wrapf(types.ErrVaultNotFound, "position %s not found in vault for pool %s", req.PositionId, req.PoolContractAddress)
	}

	position := vault.Positions[posIdx]
	sdkCtx := sdk.UnwrapSDKContext(goCtx)

	// Withdraw from the pool contract via WasmKeeper
	withdrawnAmount, err := k.WithdrawLiquidityFromPool(
		goCtx,
		req.PoolContractAddress,
		req.PositionId,
	)
	if err != nil {
		return nil, err
	}

	// Send recovered ubluechip from module account back to validator
	if withdrawnAmount.IsPositive() {
		returnCoins := sdk.NewCoins(sdk.NewCoin("ubluechip", withdrawnAmount))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(sdkCtx, types.ModuleName, validatorAddr, returnCoins); err != nil {
			return nil, fmt.Errorf("failed to return funds to validator: %w", err)
		}
	}

	// Remove the position from the vault
	vault.Positions = append(vault.Positions[:posIdx], vault.Positions[posIdx+1:]...)

	// Reduce total deposited (capped at zero to prevent underflow)
	depositedCoin := sdk.NewCoin("ubluechip", position.DepositAmount0)
	if vault.TotalDeposited.IsGTE(depositedCoin) {
		vault.TotalDeposited = vault.TotalDeposited.Sub(depositedCoin)
	} else {
		vault.TotalDeposited = sdk.NewCoin("ubluechip", math.ZeroInt())
	}

	if err := k.SetVault(goCtx, vault); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			"withdraw_from_vault",
			sdk.NewAttribute("validator_address", req.ValidatorAddress),
			sdk.NewAttribute("pool_contract", req.PoolContractAddress),
			sdk.NewAttribute("position_id", req.PositionId),
			sdk.NewAttribute("withdrawn_amount", withdrawnAmount.String()),
		),
	)

	return &types.MsgWithdrawFromVaultResponse{
		WithdrawnAmount: sdk.NewCoin("ubluechip", withdrawnAmount),
	}, nil
}
