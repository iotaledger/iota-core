import * as React from 'react';
import Container from "react-bootstrap/Container";
import NodeStore from "../stores/NodeStore";
import { inject, observer } from "mobx-react";
import ExplorerStore from "../stores/ExplorerStore";
import ListGroup from "react-bootstrap/ListGroup";
import {resolveBase58SpendID} from "../utils/spend";
import {resolveConfirmationState} from "../utils/confirmation_state";


interface Props {
    nodeStore?: NodeStore;
    explorerStore?: ExplorerStore;
    match?: {
        params: {
            id: string,
        }
    }
}

@inject("nodeStore")
@inject("explorerStore")
@observer
export class ExplorerSpendQueryResult extends React.Component<Props, any> {
    componentDidMount() {
        this.props.explorerStore.getSpend(this.props.match.params.id);
        this.props.explorerStore.getSpendChildren(this.props.match.params.id);
        this.props.explorerStore.getSpendSpends(this.props.match.params.id);
        this.props.explorerStore.getSpendVoters(this.props.match.params.id);
    }

    componentWillUnmount() {
        this.props.explorerStore.reset();
    }
    render() {
        let {id} = this.props.match.params;
        let { query_err, spend, spendChildren, spendSpends, spendVoters } = this.props.explorerStore;

        if (query_err) {
            return (
                <Container>
                    <h4>Spend not found - 404</h4>
                    <span>{id}</span>
                </Container>
            );
        }
        return (
            <Container>
                <h4>Spend</h4>
                {spend && <ListGroup>
                    <ListGroup.Item>ID: {resolveBase58SpendID(spend.id)}</ListGroup.Item>
                    <ListGroup.Item>Parents:
                        <ListGroup>
                        {spend.parents.map((p,i) => <ListGroup.Item key={i}><a href={`/explorer/spend/${p}`}>{resolveBase58SpendID(p)}</a></ListGroup.Item>)}
                        </ListGroup>
                    </ListGroup.Item>
                    {<ListGroup.Item>Spends:
                        {spend.spendIDs && <ListGroup>
                            {spend.spendIDs.map((c,i) => <ListGroup.Item key={i}><a href={`/explorer/output/${c}`}>{c}</a></ListGroup.Item>)}
                        </ListGroup>}
                    </ListGroup.Item>}
                    <ListGroup.Item>ConfirmationState: {resolveConfirmationState(spend.confirmationState)}</ListGroup.Item>
                    <ListGroup.Item> Children:
                        {spendChildren && <ListGroup>
                            {spendChildren.childSpends.map((c,i) => <ListGroup.Item key={i}><a href={`/explorer/spend/${c.spendID}`}>{resolveBase58SpendID(c.spendID)}</a></ListGroup.Item>)}
                        </ListGroup> }
                    </ListGroup.Item>
                    {<ListGroup.Item> Spends:
                            {spendSpends && <ListGroup>
                                {spendSpends.spends.map((c,i) => <div key={i}>
                                    OutputID: <a href={`/explorer/output/${c.outputID.base58}`}>{c.outputID.base58}</a>
                                    <ListGroup className={"mb-2"}>
                                        {c.spendIDs.map((b,j) => <ListGroup.Item key={j}>
                                            <a href={`/explorer/spend/${b}`}>{resolveBase58SpendID(b)}</a>
                                        </ListGroup.Item>)}
                                    </ListGroup>
                                </div>)}
                            </ListGroup> }
                        </ListGroup.Item>}
                    <ListGroup.Item> Voters:
                        {spendVoters && <ListGroup>
                            {spendVoters.voters.map((s,i) => <ListGroup.Item key={s+i}>{s}</ListGroup.Item>)}
                        </ListGroup> }
                    </ListGroup.Item>
                </ListGroup>}
            </Container>
        )
    }
}
