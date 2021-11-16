import { graph } from "./modules/graph.js";
import "./modules/ui.js";

const worker = new Worker("ui/websocket-worker.js");

function handleSimMessage(msg) {
    // console.log(msg.data);
    switch(msg.data.MsgID) {
    case 1: // Initial State
        for (let [key, value] of Object.entries(msg.data.Nodes)) {
            graph.addNode(key, value.PublicKey);
            graph.updateRootAnnouncement(key, value.RootState.Root, value.RootState.AnnSequence, value.RootState.AnnTime, value.RootState.Coords);

            if (value.Peers) {
                for (let i = 0; i < value.Peers.length; i++) {
                    graph.addPeer(key, value.Peers[i].ID, value.Peers[i].Port);
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
            graph.addNode(event.Node, event.PublicKey);
            break;
        case 2: // Node Removed
            graph.removeNode(event.Node);
            break;
        case 3: // Peer Added
            graph.addPeer(event.Node, event.Peer, event.Port);
            break;
        case 4: // Peer Removed
            graph.removePeer(event.Node, event.Peer);
            break;
        case 5: // Tree Parent Updated
            graph.setTreeParent(event.Node, event.Peer, event.Prev);
            break;
        case 6: // Snake Ascending Updated
            graph.setSnekAsc(event.Node, event.Peer, event.Prev, event.PathID);
            break;
        case 7: // Snake Descending Updated
            graph.setSnekDesc(event.Node, event.Peer, event.Prev, event.PathID);
            break;
        case 8: // Tree Root Announcement Updated
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
