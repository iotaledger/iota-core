import { UnlockConditionAddress, UnlockConditionExpiration, UnlockConditionStorageDepositReturn, UnlockConditionTimelock } from "app/components/Condition";
import { AddressUnlockCondition, ExpirationUnlockCondition, StorageDepositReturnUnlockCondition, TimelockUnlockCondition } from "app/misc/Payload";
import * as React from 'react';

export enum ConditionType {
    Address = 0,
    StorageDepositReturn,
    Timelock,
    Expiration,
    StateControllerAddress,
    GovernorAddress,
    ImmutableAlias,
}

export function resolveConditionType(condType: number) {
    switch (condType) {
        case ConditionType.Address:
            return "Address Condition";
        case ConditionType.StorageDepositReturn:
            return "StorageDepositReturn Condition";
        case ConditionType.Timelock:
            return "Timelock Condition";
        case ConditionType.Expiration:
            return "Expiration Condition";
        case ConditionType.StateControllerAddress:
            return "StateControllerAddress Condition";
        case ConditionType.GovernorAddress:
            return "GovernorAddress Condition";
        case ConditionType.ImmutableAlias:
            return "ImmutableAlias Condition";
        default:
            return "Unknown Condition Type";
    }
}

export function conditionToComponent(cond: 
    AddressUnlockCondition |
    StorageDepositReturnUnlockCondition |
    TimelockUnlockCondition |
    ExpirationUnlockCondition) {
    switch (cond.type) {
        case ConditionType.Address:
        case ConditionType.StateControllerAddress:
        case ConditionType.GovernorAddress:
            return <UnlockConditionAddress unlock={cond as AddressUnlockCondition} />;
        case ConditionType.StorageDepositReturn:
            return <UnlockConditionStorageDepositReturn unlock={cond as StorageDepositReturnUnlockCondition} />;
        case ConditionType.Expiration:
            return <UnlockConditionExpiration unlock={cond as ExpirationUnlockCondition} />;
        case ConditionType.Timelock:
            return <UnlockConditionTimelock unlock={cond as TimelockUnlockCondition} />;
    }
}