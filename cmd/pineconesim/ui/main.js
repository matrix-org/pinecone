import { Graph } from "./modules/graph.js";
import "./modules/ui.js";

const worker = new Worker("ui/websocket-worker.js");
export var graph = new Graph(document.getElementById("canvas"));

function handleSimMessage(msg) {
    console.log(msg.data);
    switch(msg.data.MsgID.ID) {
    case 1: // Initial State
        for (let i = 0; i < msg.data.Nodes.length; i++) {
            graph.addNode(msg.data.Nodes[i]);
        }

        for (let [key, value] of Object.entries(msg.data.PhysEdges)) {
            for (let i = 0; i < msg.data.PhysEdges[key].length; i++) {
                graph.addEdge("physical", key, msg.data.PhysEdges[key][i]);
            }
        }

        for (let [key, value] of Object.entries(msg.data.SnakeEdges)) {
            for (let i = 0; i < msg.data.SnakeEdges[key].length; i++) {
                graph.addEdge("snake", key, msg.data.SnakeEdges[key][i]);
            }
        }

        for (let [key, value] of Object.entries(msg.data.TreeEdges)) {
            for (let i = 0; i < msg.data.TreeEdges[key].length; i++) {
                graph.addEdge("tree", key, msg.data.TreeEdges[key][i]);
            }
        }

        if (msg.data.End === true) {
            graph.startGraph();
        }
        break;
    case 2: // Node Added
        console.log("Node added " + msg.data.Event.Node);
        graph.addNode(msg.data.Event.Node);
        break;
    case 3: // Node Removed
        console.log("Node removed " + msg.data.Event.Node);
        graph.removeNode(msg.data.Event.Node);
        break;
    case 4: // Peer Added
        console.log("Peer added: Node: " + msg.data.Event.Node + " Peer: " + msg.data.Event.Peer);
        graph.addEdge("physical", msg.data.Event.Node, msg.data.Event.Peer);
        break;
    case 5: // Peer Removed
        console.log("Peer removed: Node: " + msg.data.Event.Node + " Peer: " + msg.data.Event.Peer);
        graph.removeEdge("physical", msg.data.Event.Node, msg.data.Event.Peer);
        break;
    default:
        console.log("Unhandled message ID");
        break;
    }
};

worker.onmessage = handleSimMessage;

// Start the websocket worker with the current url
worker.postMessage({url: window.origin.replace("http", "ws") + '/ws'});
