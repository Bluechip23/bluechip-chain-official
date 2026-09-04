package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
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

// Deposit moves bond-denom tokens from the validator's own account into its
// Liquidity Vault.
func (k msgServer) Deposit(goCtx context.Context, msg *types.MsgDeposit) (*types.MsgDepositResponse, error) {
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	valAddr, err := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if err != nil {
		return nil, err
	}

	if err := k.Keeper.Deposit(goCtx, valAddr, msg.Amount); err != nil {
		return nil, err
	}

	return &types.MsgDepositResponse{}, nil
}

// InitiateWithdrawal starts a grace-period withdrawal from the validator's
// vault.
func (k msgServer) InitiateWithdrawal(goCtx context.Context, msg *types.MsgInitiateWithdrawal) (*types.MsgInitiateWithdrawalResponse, error) {
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	valAddr, err := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if err != nil {
		return nil, err
	}

	withdrawal, err := k.Keeper.InitiateWithdrawal(goCtx, valAddr, msg.Amount)
	if err != nil {
		return nil, err
	}

	return &types.MsgInitiateWithdrawalResponse{CompleteTime: withdrawal.CompleteTime}, nil
}

// SetRewardShare sets the fraction of vault rewards passed to delegators.
func (k msgServer) SetRewardShare(goCtx context.Context, msg *types.MsgSetRewardShare) (*types.MsgSetRewardShareResponse, error) {
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	valAddr, err := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if err != nil {
		return nil, err
	}

	if err := k.Keeper.SetRewardShare(goCtx, valAddr, msg.DelegatorRewardShare); err != nil {
		return nil, err
	}

	return &types.MsgSetRewardShareResponse{}, nil
}

// AllocateToPool moves active vault balance into a registered pool.
func (k msgServer) AllocateToPool(goCtx context.Context, msg *types.MsgAllocateToPool) (*types.MsgAllocateToPoolResponse, error) {
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	valAddr, err := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if err != nil {
		return nil, err
	}

	shares, err := k.Keeper.AllocateToPool(goCtx, valAddr, msg.PoolId, msg.Amount)
	if err != nil {
		return nil, err
	}

	return &types.MsgAllocateToPoolResponse{Shares: shares}, nil
}

// DeallocateFromPool queues removal of liquidity from a pool.
func (k msgServer) DeallocateFromPool(goCtx context.Context, msg *types.MsgDeallocateFromPool) (*types.MsgDeallocateFromPoolResponse, error) {
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	valAddr, err := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if err != nil {
		return nil, err
	}

	deallocation, err := k.Keeper.DeallocateFromPool(goCtx, valAddr, msg.PoolId, msg.Shares)
	if err != nil {
		return nil, err
	}

	return &types.MsgDeallocateFromPoolResponse{CompleteTime: deallocation.CompleteTime}, nil
}

// CollectPoolRewards pulls and distributes a pool's accrued fees.
func (k msgServer) CollectPoolRewards(goCtx context.Context, msg *types.MsgCollectPoolRewards) (*types.MsgCollectPoolRewardsResponse, error) {
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	valAddr, err := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if err != nil {
		return nil, err
	}

	collected, err := k.Keeper.CollectPoolRewards(goCtx, valAddr, msg.PoolId)
	if err != nil {
		return nil, err
	}

	return &types.MsgCollectPoolRewardsResponse{Collected: collected}, nil
}

// ClaimVaultRewards pays out a delegator's accrued vault rewards.
func (k msgServer) ClaimVaultRewards(goCtx context.Context, msg *types.MsgClaimVaultRewards) (*types.MsgClaimVaultRewardsResponse, error) {
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	delAddr, err := sdk.AccAddressFromBech32(msg.DelegatorAddress)
	if err != nil {
		return nil, err
	}
	valAddr, err := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if err != nil {
		return nil, err
	}

	amount, err := k.Keeper.ClaimVaultRewards(goCtx, delAddr, valAddr)
	if err != nil {
		return nil, err
	}

	return &types.MsgClaimVaultRewardsResponse{Amount: amount}, nil
}

// RegisterPool registers a pool contract. Only the governance authority may
// execute it.
func (k msgServer) RegisterPool(goCtx context.Context, msg *types.MsgRegisterPool) (*types.MsgRegisterPoolResponse, error) {
	if k.GetAuthority() != msg.Authority {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", k.GetAuthority(), msg.Authority)
	}
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	poolID, err := k.Keeper.RegisterPool(goCtx, msg.ContractAddress, msg.Description)
	if err != nil {
		return nil, err
	}

	return &types.MsgRegisterPoolResponse{PoolId: poolID}, nil
}

// SetPoolEnabled toggles a pool's allocation switch. Only the governance
// authority may execute it.
func (k msgServer) SetPoolEnabled(goCtx context.Context, msg *types.MsgSetPoolEnabled) (*types.MsgSetPoolEnabledResponse, error) {
	if k.GetAuthority() != msg.Authority {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", k.GetAuthority(), msg.Authority)
	}
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	if err := k.Keeper.SetPoolEnabled(goCtx, msg.PoolId, msg.Enabled); err != nil {
		return nil, err
	}

	return &types.MsgSetPoolEnabledResponse{}, nil
}

// UpdateParams updates the module parameters. Only the governance authority
// may execute it.
func (k msgServer) UpdateParams(goCtx context.Context, req *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if k.GetAuthority() != req.Authority {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", k.GetAuthority(), req.Authority)
	}

	if err := req.Params.Validate(); err != nil {
		return nil, err
	}

	if err := k.SetParams(goCtx, req.Params); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, nil
}
