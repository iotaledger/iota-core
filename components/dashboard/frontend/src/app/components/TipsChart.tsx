import * as React from 'react';
import Card from "react-bootstrap/Card";
import NodeStore from "../stores/NodeStore";
import {inject, observer} from "mobx-react";
import {Line} from "react-chartjs-2";
import {defaultChartOptions} from "../misc/Chart";

interface Props {
    nodeStore?: NodeStore;
}

const lineChartOptions = Object.assign({
    scales: {
        xAxes: [{
            ticks: {
                autoSkip: true,
                maxTicksLimit: 8,
                fontSize: 8,
                minRotation: 0,
                maxRotation: 0,
            },
            gridLines: {
                display: false
            }
        }],
        yAxes: [{
            gridLines: {
                display: false
            },
            ticks: {
                callback: function (value, index, values) {
                    return Math.abs(value);
                },
                fontSize: 10,
                maxTicksLimit: 4,
                beginAtZero: true,
            },
        }],
    },
    tooltips: {
        callbacks: {
            label: function (tooltipItem, data) {
                let label = data.datasets[tooltipItem.datasetIndex].label;
                return `${label} ${Math.abs(tooltipItem.value)}`;
            }
        }
    }
}, defaultChartOptions);

@inject("nodeStore")
@observer
export default class TipsChart extends React.Component<Props, any> {
    render() {
        return (
            <Card>
                <Card.Body>
                    <Card.Title>Current Tips</Card.Title>
                    <small>
                        Tips: {this.props.nodeStore.last_tips_metric.totaltips}.
                    </small>

                    <Line height={50} data={this.props.nodeStore.tipsSeries} options={lineChartOptions}/>
                </Card.Body>
            </Card>
        );
    }
}
