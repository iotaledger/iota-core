import {
    AliasOutput,
    ExtendedLockedOutput,
    Output,
    SigLockedColoredOutput,
    SigLockedSingleOutput
} from "../misc/Payload";
import { SigLockedSingleOutputComponent} from "../components/SigLockedSingleOutputComponent";
import * as React from "react";
import {SigLockedColoredOutputComponent} from "../components/SigLockedColoredOutputComponent";
import {AliasOutputComponent} from "../components/AliasOutputComponent.tsx";
import {ExtendedLockedOutputComponent} from "../components/ExtendedLockedOutput";
import {ExplorerOutput} from "../stores/ExplorerStore";
import {Base58EncodedColorIOTA, resolveColor} from "./color";
import {ConfirmationState} from "./confirmation_state";

export function outputToComponent(output: Output) {
    let id = output.outputID
    switch (output.type) {
        case "SigLockedSingleOutputType":
            return <SigLockedSingleOutputComponent output={output.output as SigLockedSingleOutput} id={id}/>;
        case "SigLockedColoredOutputType":
            return <SigLockedColoredOutputComponent output={output.output as SigLockedColoredOutput} id={id}/>;
        case "AliasOutputType":
            return <AliasOutputComponent output={output.output as AliasOutput} id={id}/>;
        case "ExtendedLockedOutputType":
            return <ExtendedLockedOutputComponent output={output.output as ExtendedLockedOutput} id={id}/>;
        default:
            return;
    }
}

export function totalBalanceFromExplorerOutputs(outputs: Array<ExplorerOutput>, addy: string): Map<string, number> {
    let totalBalance: Map<string,number> = new Map();
    if (outputs.length === 0) {return totalBalance;}
    for (let i = 0; i < outputs.length; i++) {
        let o = outputs[i];
        if (o.metadata.confirmationState < ConfirmationState.Accepted) {
            // ignore all unconfirmed balances
            continue
        }
        switch (o.output.type) {
            case "SigLockedSingleOutputType":
                let single = o.output.output as SigLockedSingleOutput;
                let resolvedColor = resolveColor(Base58EncodedColorIOTA);
                let prevBalance = totalBalance.get(resolvedColor);
                if (prevBalance === undefined) {prevBalance = 0;}
                totalBalance.set(resolvedColor, single.balance + prevBalance);
                break;
            case "ExtendedLockedOutputType":
                let extended = o.output.output as ExtendedLockedOutput;
                if (extended.fallbackAddress === undefined) {
                    // no fallback addy, address controls the output
                    extractBalanceInfo(o, totalBalance);
                    break;
                } else {
                    let now = new Date().getTime()/1000;
                    // there is a fallback address, it it us?
                    if (extended.fallbackAddress === addy) {
                        // our address is the fallback
                        // check if fallback deadline expired
                        if (now > extended.fallbackDeadline) {
                            extractBalanceInfo(o, totalBalance);
                            break;
                        }
                        // we have the fallback addy and fallback hasn't expired yet, balance not available
                        break;
                    }
                    // means addy can only be the original address
                    // we own the balance if we are before the deadline
                    if (now < extended.fallbackDeadline) {
                        extractBalanceInfo(o, totalBalance);
                        break;
                    }
                }
                break;
            default:
                if (o.output.output.balances === null) {return;}
                extractBalanceInfo(o, totalBalance);
        }
    }
    return totalBalance
}

let extractBalanceInfo = (o: ExplorerOutput, result: Map<string, number>) => {
    let colorKeys = Object.keys(o.output.output.balances);
    for (let i = 0; i< colorKeys.length; i++) {
        let color = colorKeys[i];
        let balance = o.output.output.balances[color];
        let resolvedColor = resolveColor(color);
        let prevBalance = result.get(resolvedColor);
        if (prevBalance === undefined) {
            prevBalance = 0;
        }
        result.set(resolvedColor, balance + prevBalance);
    }
}