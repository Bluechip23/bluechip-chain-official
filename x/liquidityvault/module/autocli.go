package liquidityvault

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	modulev1 "bluechipChain/api/bluechipchain/liquidityvault"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: modulev1.Query_ServiceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Shows the parameters of the liquidityvault module",
				},
				{
					RpcMethod: "Vault",
					Use:       "vault [validator-address]",
					Short:     "Query a validator's liquidity vault",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "validator_address"},
					},
				},
				{
					RpcMethod: "AllVaults",
					Use:       "vaults",
					Short:     "Query all liquidity vaults",
				},
				{
					RpcMethod: "CompositeScore",
					Use:       "composite-score [validator-address]",
					Short:     "Query a validator's composite score",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "validator_address"},
					},
				},
				{
					RpcMethod: "ValidatorRankings",
					Use:       "rankings",
					Short:     "Query validator rankings by composite score",
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              modulev1.Msg_ServiceDesc.ServiceName,
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "UpdateParams",
					Skip:      true, // skipped because authority gated
				},
				{
					RpcMethod: "RegisterValidator",
					Use:       "register-validator [validator-address] [validator-type]",
					Short:     "Register a validator in the liquidity vault system",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "validator_address"},
						{ProtoField: "validator_type"},
					},
				},
				{
					RpcMethod: "DepositToVault",
					Use:       "deposit-to-vault [validator-address] [pool-contract] [amount0] [cw20-contract] [amount1]",
					Short:     "Deposit tokens into a validator's liquidity vault",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "validator_address"},
						{ProtoField: "pool_contract_address"},
						{ProtoField: "amount0"},
						{ProtoField: "cw20_contract_address"},
						{ProtoField: "amount1"},
					},
				},
				{
					RpcMethod: "SetDelegatorRewardPercent",
					Use:       "set-delegator-reward-percent [validator-address] [percent]",
					Short:     "Set the delegator reward pass-through percentage",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "validator_address"},
						{ProtoField: "percent"},
					},
				},
			},
		},
	}
}
