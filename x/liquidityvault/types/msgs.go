package types

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ sdk.Msg = &MsgDeposit{}
	_ sdk.Msg = &MsgInitiateWithdrawal{}
	_ sdk.Msg = &MsgSetRewardShare{}
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
func (m *MsgUpdateParams) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return errorsmod.Wrap(err, "invalid authority address")
	}
	return m.Params.Validate()
}
