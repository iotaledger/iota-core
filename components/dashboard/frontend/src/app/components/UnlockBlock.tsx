import {UnlockBlock as unlockBlockJSON} from "../misc/Payload";
import * as React from "react";
import Row from "react-bootstrap/Row";
import Col from "react-bootstrap/Col";
import Badge from "react-bootstrap/Badge";
import ListGroup from "react-bootstrap/ListGroup";
import {SignatureType, resolveSignatureType} from "../utils/unlock_block";
import { Ed25519Signature } from "./Ed25519Signature";

interface UnlockProps {
    block: unlockBlockJSON;
    key: number;
}

export class UnlockBlock extends React.Component<UnlockProps, any> {
    renderSignatureComponent() {
        switch (this.props.block.signatureType) {
            case SignatureType.Ed25519:
                return <Ed25519Signature signature={this.props.block.signature} />
        }
    }

    render() {
        let block = this.props.block;
        return (
            <Row className={"mb-3"}>
                <Col>
                    Index: <Badge variant={"primary"}>{this.props.key}</Badge>
                    <ListGroup>
                        <ListGroup.Item>Type: {block.type}</ListGroup.Item>
                        {
                            block.signatureType && <ListGroup.Item>Signature Type: {resolveSignatureType(block.signatureType)}</ListGroup.Item>
                        } 
                        {
                            this.renderSignatureComponent()
                        }                        
                    </ListGroup>
                </Col>
            </Row>
        );
    }
}