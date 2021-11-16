import { graph } from "./modules/graph.js";
import "./modules/ui.js";

const worker = new Worker("ui/websocket-worker.js");

function handleSimMessage(msg) {
    // console.log(msg.data);
    switch(msg.data.MsgID) {
    case 1: // Initial State
        for (let [key, value] of Object.entries(msg.data.Nodes)) {
            graph.addNode(key);
            graph.updateRootAnnouncement(key, value.RootState.Root, value.RootState.AnnSequence, value.RootState.AnnTime, value.RootState.Coords);

            if (value.Peers) {
                for (let i = 0; i < value.Peers.length; i++) {
                    graph.addPeer(key, value.Peers[i]);
                }

            }

            if (value.SnakeAsc && value.SnakeAscPath) {
                graph.setSnekAsc(key, value.SnakeAsc, "", value.SnakeAscPath);
            }
            if (value.SnakeDesc && value.SnakeDescPath) {
                graph.setSnekDesc(key, value.SnakeDesc, "", value.SnakeDescPath);
            }

            if (value.TreeParent) {
                graph.setTreeParent(key, value.TreeParent, "");
            }
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
            graph.addPeer(event.Node, event.Peer);
            break;
        case 4: // Peer Removed
            // console.log("Peer removed: Node: " + event.Node + " Peer: " + event.Peer);
            graph.removePeer(event.Node, event.Peer);
            break;
        case 5: // Tree Parent Updated
            // console.log("Tree Parent Updated: Node: " + event.Node + " Peer: " + event.Peer);
            graph.setTreeParent(event.Node, event.Peer, event.Prev);
            break;
        case 6: // Snake Ascending Updated
            // console.log("Snake Asc Updated: Node: " + event.Node + " Peer: " + event.Peer);
            graph.setSnekAsc(event.Node, event.Peer, event.Prev, event.PathID);
            break;
        case 7: // Snake Descending Updated
            // console.log("Snake Desc Updated: Node: " + event.Node + " Peer: " + event.Peer);
            graph.setSnekDesc(event.Node, event.Peer, event.Prev, event.PathID);
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
