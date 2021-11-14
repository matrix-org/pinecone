import { Graph } from "./modules/graph.js";
import "./modules/ui.js";

const worker = new Worker("ui/websocket-worker.js");
export var graph = new Graph(document.getElementById("canvas"));

function handleSimMessage(msg) {
    // console.log(msg.data);
    switch(msg.data.MsgID) {
    case 1: // Initial State
        for (let [key, value] of Object.entries(msg.data.Nodes)) {
            graph.addNode(key);
            graph.updateRootAnnouncement(key, value.RootState.Root, value.RootState.AnnSequence, value.RootState.AnnTime, value.RootState.Coords);

            for (let i = 0; i < value.Peers.length; i++) {
                graph.addEdge("peer", key, value.Peers[i]);
            }

            for (let i = 0; i < value.SnakeNeighbours.length; i++) {
                graph.addEdge("snake", key, value.SnakeNeighbours[i]);
            }

            graph.addEdge("tree", key, value.TreeParent);
        }

        if (msg.data.End === true) {
            graph.startGraph();
        }
        break;
    case 2: // State Update
        let event = msg.data.Event.Event;
        switch(msg.data.Event.UpdateID) {
        case 1: // Node Added
            // console.log("Node added " + event.Node);
            graph.addNode(event.Node);
            break;
        case 2: // Node Removed
            // console.log("Node removed " + event.Node);
            graph.removeNode(event.Node);
            break;
        case 3: // Peer Added
            // console.log("Peer added: Node: " + event.Node + " Peer: " + event.Peer);
            graph.addEdge("peer", event.Node, event.Peer);
            break;
        case 4: // Peer Removed
            // console.log("Peer removed: Node: " + event.Node + " Peer: " + event.Peer);
            graph.removeEdge("peer", event.Node, event.Peer);
            break;
        case 5: // Tree Parent Updated
            // console.log("Tree Parent Updated: Node: " + event.Node + " Peer: " + event.Peer);
            graph.removeEdge("tree", event.Node, event.Prev);
            if (event.Peer != "") {
                graph.addEdge("tree", event.Node, event.Peer);
            }
            break;
        case 6: // Snake Ascending Updated
            // console.log("Snake Asc Updated: Node: " + event.Node + " Peer: " + event.Peer);
            graph.removeEdge("snake", event.Node, event.Prev);
            if (event.Peer != "") {
                graph.addEdge("snake", event.Node, event.Peer);
            }
            break;
        case 7: // Snake Descending Updated
            // console.log("Snake Desc Updated: Node: " + event.Node + " Peer: " + event.Peer);
            graph.removeEdge("snake", event.Node, event.Prev);
            if (event.Peer != "") {
                graph.addEdge("snake", event.Node, event.Peer);
            }
            break;
        case 8: // Tree Root Announcement Updated
            // console.log("Tree Root Announcement Updated: Node: " + event.Node + " Root: " + event.Root + " Sequence: " + event.Sequence + " Time: " + event.Time + " Coords: " + event.Coords);
            graph.updateRootAnnouncement(event.Node, event.Root, event.Sequence, event.Time, event.Coords);
        }
        break;
    default:
        console.log("Unhandled message ID");
        break;
    }
};

worker.onmessage = handleSimMessage;

// Start the websocket worker with the current url
worker.postMessage({url: window.origin.replace("http", "ws") + '/ws'});
