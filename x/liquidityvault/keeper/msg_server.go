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
