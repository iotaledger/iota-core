export enum PayloadType {
    TaggedData = 5,
    Transaction = 6,
    Faucet = 3,
}

// BasicPayload
export class BasicPayload {
    content_title: string;
    content: string;
}

export class TaggedDataPayload {
    tag: string;
    data: string;
}

export class FaucetPayload {
    address: string;
    accessManaPledgeID: string;
    consensusManaPledgeID: string;
    nonce: number;
}

export class TransactionPayload {
    networkId: number;
    creationTime: number;
    inputs: Array<Input>;
    inputsCommitment: string;
    outputs: Array<Output>;
    payload: any;
    unlocks: Array<UnlockBlock>;
}

export class Input {
    type: string;
    referencedOutputID: OutputID;
    output: Output;
}

export class Output {
    outputID: OutputID;
    type: number;
    output: 
        BasicOutput|
        AliasOutput|
        FoundryOutput|
        NFTOutput;
}

export class OutputID {
    hex: string;
    transactionID: string;
    outputIndex: number;
}

export class UnlockBlock {
    type: string;
    referencedIndex: number;
    signatureType: number;
    // TODO: make it any and add struct for other types of signature.
    signature: SignatureEd25519;
}

export class SignatureEd25519 {
    publicKey: string;
    signature: string;
}

export class BasicOutput {
    amount: number
    nativeTokens?: Array<NativeToken>;
    unlockConditions?: Array<any>;
    features?: Array<Feature>;
}

export class AliasOutput {
    amount: number;
    nativeTokens?: Array<NativeToken>;
    aliasId: string;
    stateIndex: number;
    stateMetadata?: string;
    foundryCounter: number;
    unlockConditions?: Array<any>;
    features?: Array<Feature>;
    immutableFeatures?: Array<Feature>;
}

export class FoundryOutput {
    amount: number;
    nativeTokens?: Array<NativeToken>;
    serialNumber: number;
    tokenScheme: number;    
    unlockConditions?: Array<any>;
    features?: Array<Feature>;
    immutableFeatures?: Array<Feature>;
}

export class NFTOutput {
    amount: number;
    nativeTokens?: Array<NativeToken>;
    nftId: string;  
    unlockConditions?: Array<any>;
    features?: Array<Feature>;
    immutableFeatures?: Array<Feature>;
}

export class Balance {
    value: number;
    color: string;
}

export class Conflict {
    tx_id: string;
    opinion: Opinion;
}

export class Timestamp {
    blk_id: string;
    opinion: Opinion;
}

// @ts-ignore
export class Opinion {
    value: string;
    round: number;
}

export class NativeToken {
    id: string;
    amount: number;
}

// address, stateController, Governor share the same struct
export class AddressUnlockCondition {
    type: number;
    address: Address;
}

export class Address {
    type: number;
    pubKeyHash: string;
}
export class StorageDepositReturnUnlockCondition {
    type: number;
    returnAddress: string;
    amount: number;
}

export class TimelockUnlockCondition {
    type: number;
    unixTime?: number;
}

export class ExpirationUnlockCondition {
    type: number;
    returnAddress: string;
    unixTime: number;
}

export class Feature {
    type: number;
    feature:
        IssuerFeature |
        MetadataFeature |
        TagFeature;
}

// Issuer and sender share a struct
export class IssuerFeature {
    address: string
}

export class MetadataFeature {
    data: string;
}

export class TagFeature {
    tag: string;
}

export function getPayloadType(p: number){
    switch (p) {
        case PayloadType.TaggedData:
            return "TaggedData"
        case PayloadType.Transaction:
            return "Transaction"
        case PayloadType.Faucet:
            return "Faucet"
        default:
            return "Unknown"
    }
}
