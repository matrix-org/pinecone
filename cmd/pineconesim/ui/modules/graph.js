// You can supply an element as your title.
var titleElement = document.createElement("div");
titleElement.style.height = "10em";
titleElement.style.width = "10em";
titleElement.style.color = getComputedStyle(document.documentElement)
    .getPropertyValue('--color-dull-grey');
titleElement.style.backgroundColor = getComputedStyle(document.documentElement)
    .getPropertyValue('--color-dark-grey');
titleElement.style.padding = "5px";
titleElement.style.margin = "-4px";
titleElement.id = "nodeTooltip";

let selectedNodes = null;

class Graph {
    nodeIDs = [];

    peerNodes = new vis.DataSet([]);
    peerEdges = new vis.DataSet([]);
    peerData = {
        nodes: this.peerNodes,
        edges: this.peerEdges
    };

    snakeNodes = new vis.DataSet([]);
    snakeEdges = new vis.DataSet([]);
    snakeData = {
        nodes: this.snakeNodes,
        edges: this.snakeEdges
    };

    treeNodes = new vis.DataSet([]);
    treeEdges = new vis.DataSet([]);
    treeData = {
        nodes: this.treeNodes,
        edges: this.treeEdges
    };

    geoNodes = new vis.DataSet([]);
    geoEdges = new vis.DataSet([]);
    geoData = {
        nodes: this.geoNodes,
        edges: this.geoEdges
    };

    options = {
        interaction: {
            dragNodes: true,
            dragView: true,
            zoomView: true,
            hover: true,
            tooltipDelay: 100,
        },
        physics: {
            enabled: true,
            maxVelocity: 40,
            minVelocity: 2,
            timestep: 0.6,
            adaptiveTimestep: true,
            solver: "forceAtlas2Based",
            forceAtlas2Based: {
                theta: 0.5,
                centralGravity: 0.001,
                gravitationalConstant: -70,
                springLength: 100,
                springConstant: 0.6,
                damping: 0.8,
                avoidOverlap: 0,
            },
            stabilization: {
                enabled: true,
                onlyDynamicEdges: true,
            },
        },
        layout: {
            clusterThreshold: 50,
            improvedLayout: false,
        },
        nodes: {
            title: titleElement,
            borderWidth: 10,
            borderWidthSelected: 10,
            color: {
                background: getComputedStyle(document.documentElement)
                    .getPropertyValue('--color-router-blue'),
                border: getComputedStyle(document.documentElement)
                    .getPropertyValue('--color-router-blue'),
                highlight: {
                    background: getComputedStyle(document.documentElement)
                        .getPropertyValue('--color-ems-purple'),
                    border: getComputedStyle(document.documentElement)
                        .getPropertyValue('--color-ems-purple'),
                },
                hover: {
                    background: getComputedStyle(document.documentElement)
                        .getPropertyValue('--color-router-blue'),
                    border: getComputedStyle(document.documentElement)
                        .getPropertyValue('--color-router-blue'),
                },
            },
            font: {
                color: getComputedStyle(document.documentElement)
                    .getPropertyValue('--color-dull-grey'),
            },
            shadow: {
                enabled: true,
                size: 15,
                color: getComputedStyle(document.documentElement)
                    .getPropertyValue('--color-dark-grey'),
                x: 3,
                y: 3,
            },
        },
        edges: {
            color: {
                color: getComputedStyle(document.documentElement)
                    .getPropertyValue('--color-blue-pill'),
                highlight: getComputedStyle(document.documentElement)
                    .getPropertyValue('--color-dark-red'),
                hover: getComputedStyle(document.documentElement)
                    .getPropertyValue('--color-blue-pill'),
            },
            // arrows: {
            //     to: {
            //         enabled: true,
            //     },
            // },
            width: 2,
            selectionWidth: 4,
        },
    };

    network = null;
    canvas = null;
    currentData = null;

    constructor(canvas) {
        this.canvas = canvas;
        this.currentData = this.peerData;
    }

    startGraph() {
        this.network = new vis.Network(this.canvas, this.currentData, this.options);
        this.setupHandlers();

        // HACK : network won't restabilize unless I give a node a kick...
        this.kickNode(this.nodeIDs[0]);
    }

    setupHandlers() {
        this.network.on("showPopup", function (params) {
            // params is actually the node ID
            let text = document.createElement('div');
            text.id = "nodePopupText";
            text.innerHTML = "Node " + params;
            titleElement.appendChild(text);
        });

        this.network.on("hidePopup", function () {
            let prevText = document.getElementById('nodePopupText');
            if (prevText) {
                titleElement.removeChild(prevText);
            }
        });

        this.network.on("stabilized", function (params) {
            console.log("Network stabilized");
        });

        this.network.on("selectNode", function (params) {
            selectedNodes = params.nodes;
        });

        this.network.on("deselectNode", function (params) {
            selectedNodes = params.nodes;
        });
    }

    addNode(id) {
        this.peerData.nodes.add({ id: id, label: id });
        this.snakeData.nodes.add({ id: id, label: id });
        this.treeData.nodes.add({ id: id, label: id });
        this.geoData.nodes.add({ id: id, label: id });
        this.nodeIDs.push(id);
    }

    removeNode(id) {
        this.peerData.nodes.remove(id);
        this.snakeData.nodes.remove(id);
        this.treeData.nodes.remove(id);
        this.geoData.nodes.remove(id);

        const index = this.nodeIDs.indexOf(id);
        if (index > -1) {
            this.nodeIDs.splice(index, 1);
        }
    }

    addEdge(dataset, from, to) {
        switch(dataset) {
        case "peer":
            let matchingEdges = this.peerData.edges.get({
                filter: function (item) {
                    return ((item.from === from && item.to === to) || (item.to === from && item.from === to));
                }
            });

            // Don't create duplicate edges in the peer topology
            if (matchingEdges.length === 0) {
                this.peerData.edges.add({from, to});
            }
            break;
        case "snake":
            this.snakeData.edges.add({from, to});
            break;
        case "tree":
            this.treeData.edges.add({from, to});
            break;
        case "geographic":
            this.geoData.edges.add({from, to});
            break;
        }
    }

    removeEdge(dataset, from, to) {
        let matchingEdges = [];
        switch(dataset) {
        case "peer":
            matchingEdges = this.peerData.edges.get({
                filter: function (item) {
                    return ((item.from === from && item.to === to) || (item.to === from && item.from === to));
                }
            });

            if (matchingEdges.length >= 1) {
                for (let i = 0; i < matchingEdges.length; i++) {
                    this.peerData.edges.remove(matchingEdges[i]);
                }
            }
            break;
        case "snake":
            matchingEdges = this.snakeData.edges.get({
                filter: function (item) {
                    return (item.from === from && item.to === to);
                }
            });

            if (matchingEdges.length >= 1) {
                this.snakeData.edges.remove(matchingEdges[0]);
            }
            break;
        case "tree":
            matchingEdges = this.treeData.edges.get({
                filter: function (item) {
                    return (item.from === from && item.to === to);
                }
            });

            if (matchingEdges.length >= 1) {
                this.treeData.edges.remove(matchingEdges[0]);
            }
            break;
        case "geographic":
            matchingEdges = this.geoData.edges.get({
                filter: function (item) {
                    return (item.from === from && item.to === to);
                }
            });

            if (matchingEdges.length >= 1) {
                this.geoData.edges.remove(matchingEdges[0]);
            }
            break;
        }
    }

    stabilize() {
        if (this.network) {
            this.network.stabilize();
        }
    }

    startSimulation() {
        if (this.network) {
            this.network.startSimulation();
        }
    }

    stopSimulation() {
        if (this.network) {
            this.network.stopSimulation();
        }
    }

    saveNodePositions() {
        if (this.network) {
            this.network.storePositions();
        }
    }

    getNodePositions() {
        return this.network.getPositions();
    }

    changeDataSet(dataset) {
        this.saveNodePositions();

        switch(dataset) {
        case "peer":
            this.currentData = this.peerData;
            if (this.network) {
                this.network.setData(this.currentData);
            }
            break;
        case "snake":
            this.currentData = this.snakeData;
            if (this.network) {
                this.network.setData(this.currentData);
            }
            break;
        case "tree":
            this.currentData = this.treeData;
            if (this.network) {
                this.network.setData(this.treeData);
            }
            break;
        case "geographic":
            this.currentData = this.geoData;
            if (this.network) {
                this.network.setData(this.geoData);
            }
            break;
        }

        if (selectedNodes) {
            if (this.network) {
                this.network.selectNodes(selectedNodes);
            }
        }

        // HACK : network won't restabilize unless I give a node a kick...
        this.kickNode(this.nodeIDs[0]);
    }

    kickNode(node) {
        if (node && this.network) {
            let position = this.network.getPosition(node);
            this.network.moveNode(node, position.x + 0.1, position.y);
        }
    }
}

export { Graph };
