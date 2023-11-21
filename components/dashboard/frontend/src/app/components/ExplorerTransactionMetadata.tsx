import * as React from 'react';
import Container from "react-bootstrap/Container";
import NodeStore from "../stores/NodeStore";
import { inject, observer } from "mobx-react";
import ExplorerStore from "../stores/ExplorerStore";
import ListGroup from "react-bootstrap/ListGroup";
// import {resolveBase58SpendID} from "../utils/spend";

interface Props {
    nodeStore?: NodeStore;
    explorerStore?: ExplorerStore;
    txId: string;
}

@inject("nodeStore")
@inject("explorerStore")
@observer
export class ExplorerTransactionMetadata extends React.Component<Props, any> {
    componentDidMount() {
        this.props.explorerStore.getTransactionMetadata(this.props.txId);
    }

    componentWillUnmount() {
        this.props.explorerStore.reset();
    }
    render() {
        let { txId } = this.props;
        let { query_err, txMetadata } = this.props.explorerStore;

        if (query_err) {
            return (
                <Container>
                <h4>Metadata</h4>
                    <p> Metadata for transaction ID {txId} not found.</p>
                </Container>
            );
        }
        return (
            <div style={{marginTop: "20px", marginBottom: "20px"}}>
                <h4>Metadata</h4>
                {txMetadata && <ListGroup>
                    <ListGroup.Item>
                        SpendIDs: 
                        <ListGroup>
                            { 
                                txMetadata.spendIDs && txMetadata.spendIDs.map((value, index) => {
                                    return (
                                        <ListGroup.Item key={"SpendID" + index + 1} className="text-break">
                                            <a href={`/explorer/transaction/${value}`}>{value}</a>
                                        </ListGroup.Item>
                                    )
                                })
                            }
                        </ListGroup>
                    </ListGroup.Item>
                    <ListGroup.Item>Booked: {txMetadata.booked.toString()}</ListGroup.Item>
                    {/* <ListGroup.Item>Booked time: {new Date(txMetadata.bookedTime * 1000).toLocaleString()}</ListGroup.Item> */}
                    <ListGroup.Item>Confirmation State: {txMetadata.confirmationState}</ListGroup.Item>
                    {/* <ListGroup.Item>Confirmation State Time: {new Date(txMetadata.confirmationStateTime * 1000).toLocaleString()}</ListGroup.Item> */}
                </ListGroup>}
            </div>
        )
    }
}
