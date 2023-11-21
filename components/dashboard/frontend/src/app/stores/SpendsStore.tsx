import {action, computed, observable} from 'mobx';
import {registerHandler, WSMsgType} from "../misc/WS";
import * as React from "react";
import {RouterStore,} from "mobx-react-router";
import {Link} from "react-router-dom";
import NodeStore from './NodeStore';
import {Table} from "react-bootstrap";
import {ConfirmationState, resolveConfirmationState} from "../utils/confirmation_state";

export class SpendSet {
    spendSetID: string;
    arrivalTime: number;
    resolved: boolean;
    timeToResolve: number;
    shown: boolean;
}

export class Spend {
    spendID: string;
    spendSetIDs: Array<string>;
    confirmationState: number;
    issuingTime: number;
    issuerNodeID: string;
}

// const liveFeedSize = 10;

export class SpendsStore {
    // live feed
    @observable spendSets: Map<String, SpendSet>;
    @observable spends: Map<String, Spend>;
    
    routerStore: RouterStore;
    nodeStore: NodeStore;

    constructor(routerStore: RouterStore, nodeStore: NodeStore) {
        this.routerStore = routerStore;
        this.nodeStore = nodeStore;
        this.spendSets = new Map;
        this.spends = new Map;
        registerHandler(WSMsgType.SpendSet, this.updateSpendSets);
        registerHandler(WSMsgType.Spend, this.updateSpends);
    }

    @action
    updateSpendSets = (blk: SpendSet) => {
        this.spendSets.set(blk.spendSetID, blk);
    };

    @action
    updateSpends = (blk: Spend) => {
        this.spends.set(blk.spendID, blk);
    };
   
    @computed
    get spendsLiveFeed() {
        // sort branches by time and ID to prevent "jumping"
        let spendsArr = Array.from(this.spendSets.values());
        spendsArr.sort((x: SpendSet, y: SpendSet): number => {
                return y.arrivalTime - x.arrivalTime || x.spendSetID.localeCompare(y.spendSetID);
            }
        )

        let feed = [];
        for (let spend of spendsArr) {
            feed.push(
                <tr key={spend.spendSetID} onClick={() => spend.shown = !spend.shown} style={{cursor:"pointer"}}>
                    <td>
                        <Link to={`/explorer/output/${spend.spendSetID}`}>
                            {spend.spendSetID}
                        </Link>
                    </td>
                    <td>
                        {new Date(spend.arrivalTime * 1000).toLocaleString()}
                    </td>
                    <td>
                        {spend.resolved ? 'Yes' : 'No'}
                    </td>
                    <td>
                        {spend.timeToResolve/1000000}
                    </td>
                </tr>
            );

            // only render and show branches if it has been clicked
            if (!spend.shown) {
                continue
            }

            // sort branches by time and ID to prevent "jumping"
            let branchesArr = Array.from(this.spends.values());
            branchesArr.sort((x: Spend, y: Spend): number => {
                   return x.issuingTime - y.issuingTime || x.spendID.localeCompare(y.spendID)
                }
            )

            let branches = [];
            for (let branch of branchesArr) {
                for(let spendID of branch.spendSetIDs){
                    if (spendID === spend.spendSetID) {
                        branches.push(
                                    <tr key={branch.spendID} className={branch.confirmationState > ConfirmationState.Accepted ? "table-success" : ""}>
                                        <td>
                                            <Link to={`/explorer/branch/${branch.spendID}`}>
                                                {branch.spendID}
                                            </Link>
                                        </td>
                                        <td>{resolveConfirmationState(branch.confirmationState)}</td>
                                        <td> {new Date(branch.issuingTime * 1000).toLocaleString()}</td>
                                        <td>{branch.issuerNodeID}</td>
                                    </tr>
                        );
                    }
                }
            }
            feed.push(
                <tr key={spend.spendSetID+"_branches"}>
                    <td colSpan={4}>
                        <Table size="sm">
                            <thead>
                            <tr>
                                <th>BranchID</th>
                                <th>ConfirmationState</th>
                                <th>IssuingTime</th>
                                <th>Issuer NodeID</th>
                            </tr>
                            </thead>
                            <tbody>
                            {branches}
                            </tbody>
                        </Table>
                    </td>
                </tr>
            );
        }

        return feed;
    }

}

export default SpendsStore;
