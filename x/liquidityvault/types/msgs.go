package types

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ sdk.Msg = &MsgDeposit{}
	_ sdk.Msg = &MsgInitiateWithdrawal{}
	_ sdk.Msg = &MsgSetRewardShare{}
	_ sdk.Msg = &MsgAllocateToPool{}
	_ sdk.Msg = &MsgDeallocateFromPool{}
	_ sdk.Msg = &MsgRegisterPool{}
	_ sdk.Msg = &MsgSetPoolEnabled{}
	_ sdk.Msg = &MsgUpdateParams{}
)

// ValidateBasic does a sanity check on the provided data.
func (m *MsgDeposit) ValidateBasic() error {
	if _, err := sdk.ValAddressFromBech32(m.ValidatorAddress); err != nil {
		return errorsmod.Wrap(err, "invalid validator address")
	}
	if err := m.Amount.Validate(); err != nil {
		return errorsmod.Wrap(ErrInvalidDeposit, err.Error())
	}
	if m.Amount.IsZero() {
		return errorsmod.Wrap(ErrInvalidDeposit, "deposit amount cannot be zero")
	}
	return nil
}

// ValidateBasic does a sanity check on the provided data.
func (m *MsgInitiateWithdrawal) ValidateBasic() error {
	if _, err := sdk.ValAddressFromBech32(m.ValidatorAddress); err != nil {
		return errorsmod.Wrap(err, "invalid validator address")
	}
	if err := m.Amount.Validate(); err != nil {
		return errorsmod.Wrap(ErrInvalidWithdrawal, err.Error())
	}
	if m.Amount.IsZero() {
		return errorsmod.Wrap(ErrInvalidWithdrawal, "withdrawal amount cannot be zero")
	}
	return nil
}

// ValidateBasic does a sanity check on the provided data.
func (m *MsgSetRewardShare) ValidateBasic() error {
	if _, err := sdk.ValAddressFromBech32(m.ValidatorAddress); err != nil {
		return errorsmod.Wrap(err, "invalid validator address")
	}
	return ValidateRewardShare(m.DelegatorRewardShare)
}

// ValidateBasic does a sanity check on the provided data.
func (m *MsgAllocateToPool) ValidateBasic() error {
	if _, err := sdk.ValAddressFromBech32(m.ValidatorAddress); err != nil {
		return errorsmod.Wrap(err, "invalid validator address")
	}
	if m.PoolId == 0 {
		return errorsmod.Wrap(ErrInvalidAllocation, "pool id cannot be zero")
	}
	if err := m.Amount.Validate(); err != nil {
		return errorsmod.Wrap(ErrInvalidAllocation, err.Error())
	}
	if m.Amount.IsZero() {
		return errorsmod.Wrap(ErrInvalidAllocation, "allocation amount cannot be zero")
	}
	return nil
}

// ValidateBasic does a sanity check on the provided data.
func (m *MsgDeallocateFromPool) ValidateBasic() error {
	if _, err := sdk.ValAddressFromBech32(m.ValidatorAddress); err != nil {
		return errorsmod.Wrap(err, "invalid validator address")
	}
	if m.PoolId == 0 {
		return errorsmod.Wrap(ErrInvalidDeallocation, "pool id cannot be zero")
	}
	if m.Shares.IsNil() || !m.Shares.IsPositive() {
		return errorsmod.Wrap(ErrInvalidDeallocation, "deallocation shares must be positive")
	}
	return nil
}

// ValidateBasic does a sanity check on the provided data.
func (m *MsgRegisterPool) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return errorsmod.Wrap(err, "invalid authority address")
	}
	if _, err := sdk.AccAddressFromBech32(m.ContractAddress); err != nil {
		return errorsmod.Wrap(err, "invalid pool contract address")
	}
	return nil
}

// ValidateBasic does a sanity check on the provided data.
func (m *MsgSetPoolEnabled) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return errorsmod.Wrap(err, "invalid authority address")
	}
	if m.PoolId == 0 {
		return errorsmod.Wrap(ErrInvalidAllocation, "pool id cannot be zero")
	}
	return nil
}

// ValidateBasic does a sanity check on the provided data.
func (m *MsgUpdateParams) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return errorsmod.Wrap(err, "invalid authority address")
	}
	return m.Params.Validate()
}
