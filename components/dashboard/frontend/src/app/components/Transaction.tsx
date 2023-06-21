import * as React from "react";
import Row from "react-bootstrap/Row";
import Col from "react-bootstrap/Col";
import ListGroup from "react-bootstrap/ListGroup";
import Badge from "react-bootstrap/Badge";
import {outputToComponent} from "../utils/output";
import {IconContext} from "react-icons";
import {FaChevronCircleRight} from "react-icons/fa";
import {UnlockBlock} from "./UnlockBlock";
import {TransactionPayload} from "../misc/Payload";

const style = {
    maxHeight: "1000px",
    overflow: "auto",
    width: "47%",
    fontSize: "85%",
}

interface Props {
    txID?: string;
    tx?: TransactionPayload;
}

export class Transaction extends React.Component<Props, any> {
    render() {
        let txID = this.props.txID;
        let tx = this.props.tx;
        return (
            tx && txID &&
            <div>
                <h4>Transaction</h4>
                <p> {txID} </p>
                <Row className={"mb-3"}>
                    <Col>
                        <div style={{
                            marginTop: "10px",
                            marginBottom: "20px",
                            paddingBottom: "10px",
                            borderBottom: "2px solid #eee"}}><h5>Transaction Essence</h5></div>
                        <ListGroup>
                            <ListGroup.Item>ID: <a href={`/explorer/transaction/${txID}`}> {txID}</a></ListGroup.Item>
                            <ListGroup.Item>Network ID: {tx.networkId}</ListGroup.Item>
                            <ListGroup.Item>Creation Time: {tx.creationTime}</ListGroup.Item>
                            <ListGroup.Item>
                                <div className="d-flex justify-content-between align-items-center">
                                    <div className="align-self-start input-output-list" style={style}>
                                        <span>Inputs</span>
                                        <hr/>
                                        {tx.inputs.map((input, i) => {
                                            return (
                                                <div className={"mb-2"} key={i}>
                                                    <span className="mb-2">Index: <Badge variant={"primary"}>{i}</Badge></span>
                                                    <div className={"mb-2"} key={"input"+i}>
                                                        <ListGroup>
                                                            <ListGroup.Item>Output ID: {input.referencedOutputID.hex}</ListGroup.Item>
                                                        </ListGroup>
                                                    </div>
                                                </div>
                                            )
                                        })}
                                    </div>
                                    <IconContext.Provider value={{ color: "#00a0ff", size: "2em"}}>
                                        <div>
                                            <FaChevronCircleRight />
                                        </div>
                                    </IconContext.Provider>
                                    <div style={style}>
                                        <span>Outputs</span>
                                        <hr/>
                                        {tx.outputs.map((output, i) => {
                                            return (
                                                <div className={"mb-2"} key={i}>
                                                    <span className="mb-2">Index: <Badge variant={"primary"}>{i}</Badge></span>
                                                    {outputToComponent(output)}
                                                </div>
                                            )
                                        })}
                                    </div>
                                </div>
                            </ListGroup.Item>
                            { tx.payload && <ListGroup.Item>Data payload: {tx.payload}</ListGroup.Item>}
                        </ListGroup>
                    </Col>
                </Row>
                <Row className={"mb-3"}>
                    <Col>
                        <div style={{
                            marginBottom: "20px",
                            paddingBottom: "10px",
                            borderBottom: "2px solid #eee"}}><h5>Unlock Blocks</h5></div>
                        <React.Fragment>
                            {
                                tx.unlocks.map((block,index) => (
                                    <UnlockBlock
                                        block={block}
                                        key={index}
                                    />
                                ))}
                        </React.Fragment>
                    </Col>
                </Row>
            </div>
        );
    }
}