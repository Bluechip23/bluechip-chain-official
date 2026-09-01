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
					Short:     "Shows the parameters of the module",
				},
				{
					RpcMethod:      "Vault",
					Use:            "vault [validator-address]",
					Short:          "Shows a validator's liquidity vault and pending withdrawals",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "validator_address"}},
				},
				{
					RpcMethod: "Vaults",
					Use:       "vaults",
					Short:     "Lists all liquidity vaults",
				},
				{
					RpcMethod:      "CompositeScore",
					Use:            "composite-score [validator-address]",
					Short:          "Shows a validator's composite score (staked tokens + vault balance)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "validator_address"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              modulev1.Msg_ServiceDesc.ServiceName,
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod:      "Deposit",
					Use:            "deposit [amount]",
					Short:          "Deposit bond-denom tokens from the validator account into its liquidity vault",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "amount"}},
				},
				{
					RpcMethod:      "InitiateWithdrawal",
					Use:            "initiate-withdrawal [amount]",
					Short:          "Start a grace-period withdrawal from the validator's liquidity vault",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "amount"}},
				},
				{
					RpcMethod:      "SetRewardShare",
					Use:            "set-reward-share [delegator-reward-share]",
					Short:          "Set the fraction of vault rewards passed through to delegators",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "delegator_reward_share"}},
				},
				{
					RpcMethod: "UpdateParams",
					Skip:      true, // skipped because authority gated
				},
			},
		},
	}
}
