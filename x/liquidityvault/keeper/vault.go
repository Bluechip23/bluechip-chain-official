package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// GetVault returns a validator's vault, if it exists.
func (k Keeper) GetVault(ctx context.Context, valAddr sdk.ValAddress) (types.Vault, bool) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := store.Get(types.VaultKey(valAddr))
	if bz == nil {
		return types.Vault{}, false
	}

	var vault types.Vault
	k.cdc.MustUnmarshal(bz, &vault)
	return vault, true
}

// SetVault stores a validator's vault.
func (k Keeper) SetVault(ctx context.Context, vault types.Vault) error {
	valAddr, err := sdk.ValAddressFromBech32(vault.ValidatorAddress)
	if err != nil {
		return errorsmod.Wrap(err, "invalid vault validator address")
	}

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz, err := k.cdc.Marshal(&vault)
	if err != nil {
		return err
	}
	store.Set(types.VaultKey(valAddr), bz)

	return nil
}

// IterateVaults iterates over all vaults, stopping when cb returns true.
func (k Keeper) IterateVaults(ctx context.Context, cb func(vault types.Vault) (stop bool)) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iterator := storetypes.KVStorePrefixIterator(store, types.VaultKeyPrefix)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		var vault types.Vault
		k.cdc.MustUnmarshal(iterator.Value(), &vault)
		if cb(vault) {
			break
		}
	}
}

// GetAllVaults returns every vault in the store.
func (k Keeper) GetAllVaults(ctx context.Context) []types.Vault {
	var vaults []types.Vault
	k.IterateVaults(ctx, func(vault types.Vault) bool {
		vaults = append(vaults, vault)
		return false
	})
	return vaults
}
