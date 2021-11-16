import { openRightPanel, closeRightPanel } from "./ui.js";

// You can supply an element as your title.
var titleElement = document.createElement("div");
titleElement.style.height = "10em";
// titleElement.style.minWidth = "10em";
titleElement.style.width = "max-content";
titleElement.style.color = getComputedStyle(document.documentElement)
    .getPropertyValue('--color-dull-grey');
titleElement.style.backgroundColor = getComputedStyle(document.documentElement)
    .getPropertyValue('--color-dark-grey');
titleElement.style.padding = "5px";
titleElement.style.margin = "-4px";
titleElement.id = "nodeTooltip";

let selectedNodes = null;
let panelNodes = null;
let hoverNode = null;

let Nodes = new Map();

function handleNodeHoverUpdate(nodeID) {
    let node = Nodes.get(nodeID);
    let hoverPanel = document.getElementById('nodePopupText');
    if (hoverPanel) {
        let time = node.announcement.time.toString();
        hoverPanel.innerHTML = "<u><b>Node " + nodeID + "</b></u>" +
            "<br>Coords: [" + node.coords + "]" +
            "<br><br><u>Announcement</u>" +
            "<br>Root: Node " + node.announcement.root +
            "<br>Sequence: " + node.announcement.sequence +
            "<br>Time: " + time.slice(0, time.length - 3) + " ms";
    }
}

function handleNodePanelUpdate(nodeID) {
    let node = Nodes.get(nodeID);
    let nodePanel = document.getElementById('currentNodeState');
    if (nodePanel) {
        nodePanel.innerHTML =
            "<h3>Node Details</h3>" +
            "<hr><table>" +
            "<tr><td>Name:</td><td>" + nodeID + "</td></tr>" +
            "<tr><td>Coordinates:</td><td>[" + node.coords + "]</td></tr>" +
            "<tr><td>Public Key:</td><td><code>TODO</code></td></tr>" +
            "<tr><td>Descending Key:</td><td><code>TODO</code></td></tr>" +
            "<tr><td>Descending Path:</td><td><code>TODO</code></td></tr>" +
            "<tr><td>Ascending Key:</td><td><code>TODO</code></td></tr>" +
            "<tr><td>Ascending Path:</td><td><code>TODO</code></td></tr>" +
            "</table>" +
            "<hr><h4><u>Peers</u></h4>" +
            "<table>" +
            "<tr><th>Name</th><th>Public Key</th><th>Port</th><th>Root</th></tr>" +
            // {{range .NodeInfo.Peers}}
            "<tr><td><code>TODO</code></td><td><code>TODO</code></td><td><code>TODO</code></td><td><code>TODO</code></td></tr>" +
            "</table>" +
            "<hr><h4><u>SNEK Routes</u></h4>" +
            "<table>" +
            "<tr><th>Public Key</th><th>Path ID</th><th>Src</th><th>Dst</th><th>Seq</th></tr>" +
            // {{range .NodeInfo.Entries}}
            "<tr><td><code>TODO</code></td><td><code>TODO</code></td><td><code>TODO</code></td><td><code>TODO</code></td><td><code>TODO</code></td></tr>" +
            "</table>";
    }
}

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
            let text = document.createElement('div');
            text.id = "nodePopupText";
            titleElement.appendChild(text);

            hoverNode = params;
            handleNodeHoverUpdate(params);
        });

        this.network.on("hidePopup", function () {
            let prevText = document.getElementById('nodePopupText');
            if (prevText) {
                titleElement.removeChild(prevText);
            }

            hoverNode = null;
        });

        this.network.on("stabilized", function (params) {
            console.log("Network stabilized");
        });

        this.network.on("selectNode", function (params) {
            selectedNodes = params.nodes;
            panelNodes = selectedNodes;
            for (const node of params.nodes) {
                handleNodePanelUpdate(node);
            }
            openRightPanel();
        });

        this.network.on("deselectNode", function (params) {
            selectedNodes = params.nodes;
        });
    }

    addNode(id, coords) {
        this.peerData.nodes.add({ id: id, label: id });
        this.snakeData.nodes.add({ id: id, label: id });
        this.treeData.nodes.add({ id: id, label: id });
        this.geoData.nodes.add({ id: id, label: id });
        this.nodeIDs.push(id);

        Nodes.set(id, {
            announcement: {
                root: "",
                sequence: 0,
                time: 0,
            },
            coords: [],
        });
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

        Nodes.delete(id);
    }

    updateRootAnnouncement(id, root, sequence, time, coords) {
        if (Nodes.has(id)) {
            let node = Nodes.get(id);
            node.announcement.root = root;
            node.announcement.sequence = sequence;
            node.announcement.time = time;
            node.coords = coords;

            if (panelNodes && panelNodes.indexOf(id) > -1) {
                handleNodePanelUpdate(id);
            }
            if (hoverNode && hoverNode === id) {
                handleNodeHoverUpdate(id);
            }
        }
    }

    getCoordinates(id) {
        if (Node.has(id)) {
            return Nodes.get(id);
        }

        return [];
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
