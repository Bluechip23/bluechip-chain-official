package types

import (
	"fmt"
)

// DefaultGenesis returns the default genesis state
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params: DefaultParams(),
	}
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	seen := make(map[string]bool, len(gs.Vaults))
	for _, vault := range gs.Vaults {
		if err := vault.Validate(); err != nil {
			return err
		}
		if seen[vault.ValidatorAddress] {
			return fmt.Errorf("duplicate vault for validator %s", vault.ValidatorAddress)
		}
		seen[vault.ValidatorAddress] = true
	}

	for _, withdrawal := range gs.PendingWithdrawals {
		if err := withdrawal.Validate(); err != nil {
			return err
		}
	}

	return nil
}
