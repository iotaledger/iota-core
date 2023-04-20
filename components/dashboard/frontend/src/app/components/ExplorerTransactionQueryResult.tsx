import * as React from 'react';
import Container from "react-bootstrap/Container";
import { ExplorerTransaction } from "./ExplorerTransaction"
import { ExplorerTransactionMetadata } from "./ExplorerTransactionMetadata"
import { ExplorerTransactionAttachments } from "./ExplorerTransactionAttachments"


interface Props {
    match?: {
        params: {
            id: string,
        }
    }
}

export class ExplorerTransactionQueryResult extends React.Component<Props, any> {
    render() {
        let { id } = this.props.match.params;
        return (
            <Container>
                <ExplorerTransaction txId={id}/>
                <ExplorerTransactionMetadata txId={id}/>
                <ExplorerTransactionAttachments txId={id}/>
            </Container>
        )
    }
}