import { graph } from "./graph.js";
import { APICommandMessageID, APICommandID, SendToServer } from "./server-api.js";

let leftShown = true;
let rightShown = false;

export function openRightPanel() {
    if (!rightShown) {
        toggleRightPanel();
    }
}

export function closeRightPanel() {
    if (rightShown) {
        toggleRightPanel();
    }
}

function toggleLeftPanel() {
    let panel = document.getElementById("left");

    if (panel) {
        if (!leftShown) {
            panel.style.width = "max-content";
            leftShown= true;
        } else {
            panel.style.width = "0";
            leftShown= false;
        }
    }
}
document.getElementById("leftToggle").onclick = toggleLeftPanel;

function toggleRightPanel() {
    let panel = document.getElementById("right");

    if (panel) {
        if (!rightShown) {
            panel.style.width = "max-content";
            let toggleSize = parseInt(getComputedStyle(document.documentElement)
                                      .getPropertyValue('--toggle-box-size'), 10);
            let toggleSizePadding = parseInt(getComputedStyle(document.documentElement)
                                             .getPropertyValue('--toggle-box-margin'), 10);

            panel.style.minWidth = (toggleSize + toggleSizePadding * 2).toString() + "px";
            rightShown= true;
        } else {
            panel.style.width = "0";
            panel.style.minWidth = "0vw";
            rightShown= false;
        }
    }
}
document.getElementById("rightToggle").onclick = toggleRightPanel;

function focusSelectedNode() {
    let button = document.getElementById("focusNode");
    if (button) {
        graph.focusSelectedNode();
    }
}
document.getElementById("focusNode").onclick = focusSelectedNode;

function selectNetworkType(networkType) {
    let selectionTabs = document.getElementsByClassName("netselect");
    for (let i = 0; i < selectionTabs.length; i++) {
        selectionTabs[i].className = selectionTabs[i].className.replace(" active", "");
    }

    this.className += " active";

    switch(this.id) {
    case "peerTopo":
        graph.changeDataSet("peer");
        break;
    case "snakeTopo":
        graph.changeDataSet("snake");
        break;
    case "treeTopo":
        graph.changeDataSet("tree");
        break;
    case "geographicTopo":
        graph.changeDataSet("geographic");
        break;
    }
}

function setupNetworkSelection() {
    let selectionTabs = document.getElementsByClassName("netselect");
    for (let i = 0; i < selectionTabs.length; i++) {
        selectionTabs[i].onclick = selectNetworkType;
    }
}
setupNetworkSelection();

function selectTool(toolType) {
    switch(this.id) {
    case "scenario-new":
        // TODO
        break;
    case "scenario-load":
        // TODO
        break;
    case "capture-start-stop":
        if (this.className.includes("active")) {
            this.className = this.className.replace(" active", "");
            let tooltip = this.getElementsByClassName("tooltiptext")[0];
            tooltip.textContent = "Start Capture";
            // TODO : stop capture
        } else {
            this.className += " active";
            let tooltip = this.getElementsByClassName("tooltiptext")[0];
            tooltip.textContent = "Stop Capture";
            // TODO : start capture
        }
        break;
    case "replay-upload":
        // TODO
        break;
    case "replay-play-pause":
        if (this.className.includes("active")) {
            ResetReplayUI(this);
            // TODO : pause replay
            console.log("pause replay");
        } else {
            this.className += " active";
            let tooltip = this.getElementsByClassName("tooltiptext")[0];
            tooltip.textContent = "Pause Replay";
            let icon = this.getElementsByClassName("fa")[0];
            icon.className = icon.className.replace(" fa-repeat", " fa-pause");
            // TODO : resume replay
            console.log("resume replay");
        }
        break;
    case "add-nodes":
        // TODO
        break;
    case "add-peerings":
        // TODO
        break;
    case "remove":
        // TODO : do peer connections as well (but not tree or snake)
        let commands = [];
        let nodes = graph.GetSelectedNodes();
        if (nodes) {
            for (let i = 0; i < nodes.length; i++) {
                commands.push({"MsgID": APICommandID.RemoveNode, "Event": {"Name": nodes[i]}});
            }
        }

        if (commands.length > 0) {
            SendToServer({"MsgID": APICommandMessageID.PlaySequence, "Events": commands});
        }
        break;
    }
}

function setupToolSelection() {
    let tools = document.getElementsByClassName("toolselect");
    for (let i = 0; i < tools.length; i++) {
        tools[i].onclick = selectTool;
    }

    let subtools = document.getElementsByClassName("subtoolselect");
    for (let i = 0; i < subtools.length; i++) {
        console.log("sadf");
        subtools[i].onclick = selectTool;
    }
}
setupToolSelection();

export function ResetReplayUI(element) {
    element.className = element.className.replace(" active", "");
    let tooltip = element.getElementsByClassName("tooltiptext")[0];
    tooltip.textContent = "Resume Replay";
    let icon = element.getElementsByClassName("fa")[0];
    icon.className = icon.className.replace(" fa-pause", " fa-repeat");
}
