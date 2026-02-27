package keeper

import (
	"context"

	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"bluechipChain/x/liquidityvault/types"
)

// GetVault retrieves a vault for a validator
func (k Keeper) GetVault(ctx context.Context, valAddr string) (types.Vault, bool) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := store.Get(types.VaultKey(valAddr))
	if bz == nil {
		return types.Vault{}, false
	}

	var vault types.Vault
	k.cdc.MustUnmarshal(bz, &vault)
	return vault, true
}

// SetVault persists a vault
func (k Keeper) SetVault(ctx context.Context, vault types.Vault) error {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz, err := k.cdc.Marshal(&vault)
	if err != nil {
		return err
	}
	store.Set(types.VaultKey(vault.ValidatorAddress), bz)
	return nil
}

// DeleteVault removes a vault
func (k Keeper) DeleteVault(ctx context.Context, valAddr string) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	store.Delete(types.VaultKey(valAddr))
}

// IterateVaults iterates over all vaults
func (k Keeper) IterateVaults(ctx context.Context, cb func(vault types.Vault) bool) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	iter := storetypes.KVStorePrefixIterator(store, types.VaultKeyPrefix)
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		var vault types.Vault
		k.cdc.MustUnmarshal(iter.Value(), &vault)
		if cb(vault) {
			break
		}
	}
}

// GetAllVaults returns all vaults
func (k Keeper) GetAllVaults(ctx context.Context) []types.Vault {
	var vaults []types.Vault
	k.IterateVaults(ctx, func(vault types.Vault) bool {
		vaults = append(vaults, vault)
		return false
	})
	return vaults
}

// CreateVault creates a new vault for a validator
func (k Keeper) CreateVault(ctx context.Context, valAddr string, valType types.ValidatorType) error {
	if _, exists := k.GetVault(ctx, valAddr); exists {
		return types.ErrVaultAlreadyExists
	}

	params := k.GetParams(ctx)
	vault := types.Vault{
		ValidatorAddress:       valAddr,
		TotalDeposited:         sdk.NewCoin("ubluechip", math.ZeroInt()),
		DelegatorRewardPercent: params.DefaultDelegatorRewardPercent,
		Positions:              []types.PoolPosition{},
		ValidatorType:          valType,
	}

	if err := k.SetVault(ctx, vault); err != nil {
		return err
	}

	k.SetValidatorType(ctx, valAddr, valType)
	return nil
}

// SetValidatorType sets the validator type
func (k Keeper) SetValidatorType(ctx context.Context, valAddr string, valType types.ValidatorType) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	store.Set(types.ValidatorTypeKey(valAddr), []byte{byte(valType)})
}

// GetValidatorType gets the validator type
func (k Keeper) GetValidatorType(ctx context.Context, valAddr string) (types.ValidatorType, bool) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz := store.Get(types.ValidatorTypeKey(valAddr))
	if bz == nil {
		return types.ValidatorType_VALIDATOR_TYPE_UNSPECIFIED, false
	}
	return types.ValidatorType(bz[0]), true
}
