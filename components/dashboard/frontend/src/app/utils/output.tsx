import {
    BasicOutput as BasicJSON,
    AliasOutput as AliasJSON,
    FoundryOutput as FoundryJSON,
    NFTOutput as NFTJSON,
    Output,
} from "../misc/Payload";
import * as React from "react";
import { BasicOutput } from "app/components/BasicOutput";
import { AliasOutput } from "app/components/AliasOutput";
import { FoundryOutput } from "app/components/FoundryOutput";
import { NFTOutput } from "app/components/NFTOutput";

export enum OutputType {
    Treasury = 2,
    Basic,
    Alias,
    Foundry,
    NFT,
}

export function outputToComponent(output: Output) {
    let id = output.outputID
    switch (output.type) {
        case OutputType.Basic:
            return <BasicOutput output={output.output as BasicJSON} id={id}/>;
        case OutputType.Alias:
            return <AliasOutput output={output.output as AliasJSON} id={id}/>;
        case OutputType.Foundry:
            return <FoundryOutput output={output.output as FoundryJSON} id={id}/>;
            case OutputType.NFT:
                return <NFTOutput output={output.output as NFTJSON} id={id}/>;
        default:
            return;
    }
}


export function outputTypeToName(type: number) {
    switch (type) {
        case OutputType.Basic:
            return "Basic Output";
        case OutputType.Alias:
            return "Alias Output";
        case OutputType.Foundry:
            return "Foundry Output";
            case OutputType.NFT:
                return "NFT Output";
        default:
            return;
    }
}
